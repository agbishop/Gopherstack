package resiliencehub

import (
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// resolveAssessmentLocked resolves assessmentArn to its stored AppAssessment.
// Callers must hold b.mu.
func (b *InMemoryBackend) resolveAssessmentLocked(assessmentArn string) (*AppAssessment, bool) {
	id, ok := resourceIDFromARN(assessmentArn, ":app-assessment/")
	if !ok {
		return nil, false
	}

	return b.assessments.Get(id)
}

// complianceStatusForPolicy returns the ONE clear, documented coarse
// compliance rule this backend applies everywhere a real assessment would
// otherwise need to run its proprietary scoring model (see PARITY.md's
// "assessment state machine" section, which explicitly sanctions "a binary/
// coarse compliance signal ... IF the implementer picks one clear, stated
// rule"): MissingPolicy when the app has no bound ResiliencyPolicy (a real,
// derivable fact), PolicyMet otherwise (a documented STAND-IN, not the
// output of real compliance evaluation -- this backend never evaluates
// whether the underlying resources would actually meet the policy's
// RTO/RPO targets).
func complianceStatusForPolicy(policy *ResiliencyPolicy) string {
	if policy == nil {
		return ComplianceStatusMissingPolicy
	}

	return ComplianceStatusPolicyMet
}

// buildComplianceMap builds the per-DisruptionType Compliance map for an
// assessment snapshot of policy, per complianceStatusForPolicy's documented
// placeholder rule. AchievableRpoInSecs/RtoInSecs echo the bound policy's
// real configured targets (not fabricated); CurrentRpoInSecs/RtoInSecs are
// documented as assumed equal to the achievable target, since no real
// assessment runs to measure an actual current value.
func buildComplianceMap(policy *ResiliencyPolicy) map[string]DisruptionCompliance {
	status := complianceStatusForPolicy(policy)
	out := make(map[string]DisruptionCompliance, len(disruptionTypes))

	for _, dt := range disruptionTypes {
		var fp FailurePolicy
		if policy != nil {
			fp = policy.Policy[dt]
		}

		out[dt] = DisruptionCompliance{
			ComplianceStatus:    status,
			AchievableRpoInSecs: fp.RpoInSecs,
			AchievableRtoInSecs: fp.RtoInSecs,
			CurrentRpoInSecs:    fp.RpoInSecs,
			CurrentRtoInSecs:    fp.RtoInSecs,
		}
	}

	return out
}

// StartAppAssessment starts a new (Pending -> InProgress -> Success) async
// assessment of (appArn, appVersion). See PARITY.md's honest-gap section:
// Summary/ResiliencyScore never carry fabricated content (Summary is always
// nil; ResiliencyScore.Score is always scorePlaceholder).
func (b *InMemoryBackend) StartAppAssessment(req *startAppAssessmentRequest) (*AppAssessment, error) {
	if req.AppArn == "" {
		return nil, validationError("appArn is required")
	}

	if req.AssessmentName == "" {
		return nil, validationError("assessmentName is required")
	}

	b.mu.Lock("StartAppAssessment")
	defer b.mu.Unlock()

	a, ok := b.resolveAppLocked(req.AppArn)
	if !ok {
		return nil, notFoundError(resourceApp, req.AppArn)
	}

	appVersion := req.AppVersion
	if appVersion == "" {
		appVersion = draftVersion
	}

	if _, verOK := resolveAppVersionLocked(a, appVersion); !verOK {
		return nil, notFoundError(resourceAppVersion, appVersion)
	}

	var policySnapshot *ResiliencyPolicy
	if a.PolicyArn != "" {
		if p, polOK := b.resolvePolicyLocked(a.PolicyArn); polOK {
			policySnapshot = p.clone()
		}
	}

	id := newAssessmentID()
	t := tags.New("resiliencehub.assessment." + id + ".tags")
	t.Merge(req.Tags)

	asmt := &AppAssessment{
		ARN:            b.AssessmentARN(id),
		AppArn:         a.ARN,
		AppID:          a.ID,
		AppVersion:     appVersion,
		AssessmentName: req.AssessmentName,
		Status:         AssessmentStatusPending,
		Invoker:        InvokerUser,
		StartTime:      time.Now().UTC(),
		Policy:         policySnapshot,
		DriftStatus:    DriftStatusNotChecked,
		Tags:           t,
		ClientToken:    req.ClientToken,
	}

	b.assessments.Put(asmt)
	b.scheduleAssessment(asmt.ARN)

	return asmt.clone(), nil
}

// scheduleAssessment transitions an assessment Pending -> InProgress ->
// Success, applying the documented coarse compliance rule (see
// complianceStatusForPolicy) once it completes.
func (b *InMemoryBackend) scheduleAssessment(assessmentArn string) {
	id, _ := resourceIDFromARN(assessmentArn, ":app-assessment/")

	b.work.After("AssessmentInProgress", asyncTransitionDelay, func() {
		b.mu.Lock("AssessmentInProgress-async")

		if asmt, ok := b.assessments.Get(id); ok && asmt.Status == AssessmentStatusPending {
			asmt.Status = AssessmentStatusInProgress
		}

		b.mu.Unlock()

		b.work.After("AssessmentComplete", asyncTransitionDelay, func() {
			b.mu.Lock("AssessmentComplete-async")
			defer b.mu.Unlock()

			asmt, ok := b.assessments.Get(id)
			if !ok || asmt.Status != AssessmentStatusInProgress {
				return
			}

			asmt.Compliance = buildComplianceMap(asmt.Policy)
			asmt.ComplianceStatus = complianceStatusForPolicy(asmt.Policy)
			asmt.ResiliencyScore = &ResiliencyScore{Score: scorePlaceholder}
			asmt.ResourceErrorsDetails = &ResourceErrorsDetails{}
			asmt.EndTime = time.Now().UTC()
			asmt.Status = AssessmentStatusSuccess

			if app, appOK := b.apps.Get(asmt.AppID); appOK {
				app.ComplianceStatus = asmt.ComplianceStatus
				app.LastAppComplianceEvaluationTime = asmt.EndTime
				app.LastResiliencyScoreEvaluationTime = asmt.EndTime
			}
		})
	})
}

// DescribeAppAssessment returns a copy of the assessment identified by
// assessmentArn.
func (b *InMemoryBackend) DescribeAppAssessment(assessmentArn string) (*AppAssessment, error) {
	b.mu.RLock("DescribeAppAssessment")
	defer b.mu.RUnlock()

	a, ok := b.resolveAssessmentLocked(assessmentArn)
	if !ok {
		return nil, notFoundError(resourceAssessment, assessmentArn)
	}

	return a.clone(), nil
}

// DeleteAppAssessment deletes the assessment identified by assessmentArn.
// Rejected (ConflictException) while it is still Pending/InProgress.
func (b *InMemoryBackend) DeleteAppAssessment(assessmentArn string) (string, error) {
	b.mu.Lock("DeleteAppAssessment")
	defer b.mu.Unlock()

	a, ok := b.resolveAssessmentLocked(assessmentArn)
	if !ok {
		return "", notFoundError(resourceAssessment, assessmentArn)
	}

	if a.Status == AssessmentStatusPending || a.Status == AssessmentStatusInProgress {
		return "", conflictErrorWithResource(
			resourceAssessment,
			assessmentKeyFn(a),
			"assessment is still running: "+a.ARN,
		)
	}

	status := a.Status

	if a.Tags != nil {
		a.Tags.Close()
	}

	b.assessments.Delete(assessmentKeyFn(a))

	return status, nil
}

// listAssessmentsFilter holds ListAppAssessments' optional filters.
type listAssessmentsFilter struct {
	appArn           string
	assessmentName   string
	complianceStatus string
	invoker          string
	statuses         []string
	reverseOrder     bool
}

func matchesAssessmentFilter(a *AppAssessment, f listAssessmentsFilter) bool {
	if f.appArn != "" && a.AppArn != f.appArn {
		return false
	}

	if f.assessmentName != "" && a.AssessmentName != f.assessmentName {
		return false
	}

	if f.complianceStatus != "" && a.ComplianceStatus != f.complianceStatus {
		return false
	}

	if f.invoker != "" && a.Invoker != f.invoker {
		return false
	}

	if len(f.statuses) > 0 && !containsStr(f.statuses, a.Status) {
		return false
	}

	return true
}

// ListAppAssessments returns a page of assessment summaries matching f.
func (b *InMemoryBackend) ListAppAssessments(
	f listAssessmentsFilter,
	token string,
	limit int,
) page.Page[*AppAssessment] {
	b.mu.RLock("ListAppAssessments")
	defer b.mu.RUnlock()

	all := b.assessments.Snapshot()
	filtered := make([]*AppAssessment, 0, len(all))

	for _, a := range all {
		if matchesAssessmentFilter(a, f) {
			filtered = append(filtered, a)
		}
	}

	// ListAppAssessmentsInput.ReverseOrder: "The default is to sort by
	// ascending startTime" (api_op_ListAppAssessments.go,
	// resiliencehub@v1.38.3) -- sort by StartTime explicitly rather than
	// relying on b.assessments.Snapshot()'s key order.
	sort.Slice(filtered, func(i, j int) bool {
		if f.reverseOrder {
			return filtered[i].StartTime.After(filtered[j].StartTime)
		}

		return filtered[i].StartTime.Before(filtered[j].StartTime)
	})

	return page.New(filtered, token, limit, defaultPageLimit)
}

// ListAppAssessmentComplianceDrifts always returns an empty list: drift
// detection requires comparing a resource's compliance evaluation across two
// points in time using the same proprietary scoring model ResiliencyScore
// depends on -- see PARITY.md. An honestly-empty list is correct here, not a
// stub, since no drift is ever fabricated. Does NOT gate on assessmentArn
// existing (gopherstack-ulsj): confirmed via deserializers.go that this op's
// error set omits ResourceNotFoundException, unlike DescribeAppAssessment's;
// an unknown ARN gets the same empty list, not a 404.
func (b *InMemoryBackend) ListAppAssessmentComplianceDrifts(_ string) error {
	return nil
}

// ListAppAssessmentResourceDrifts always returns an empty list -- same
// honest-gap rationale and gopherstack-ulsj not-found fix as
// ListAppAssessmentComplianceDrifts above.
func (b *InMemoryBackend) ListAppAssessmentResourceDrifts(_ string) error {
	return nil
}

// ListAppComponentCompliances returns one compliance entry per AppComponent
// on the assessed AppVersion, using the same documented coarse rule as
// buildComplianceMap.
func (b *InMemoryBackend) ListAppComponentCompliances(assessmentArn string) (*AppAssessment, []*AppComponent, error) {
	b.mu.RLock("ListAppComponentCompliances")
	defer b.mu.RUnlock()

	asmt, ok := b.resolveAssessmentLocked(assessmentArn)
	if !ok {
		return nil, nil, notFoundError(resourceAssessment, assessmentArn)
	}

	app, ok := b.apps.Get(asmt.AppID)
	if !ok {
		return asmt.clone(), nil, nil
	}

	v, ok := resolveAppVersionLocked(app, asmt.AppVersion)
	if !ok {
		return asmt.clone(), nil, nil
	}

	out := make([]*AppComponent, len(v.AppComponents))
	for i, c := range v.AppComponents {
		out[i] = c.clone()
	}

	return asmt.clone(), out, nil
}
