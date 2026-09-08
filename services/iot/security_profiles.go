package iot

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// AttachSecurityProfile attaches a security profile to a target. Real AWS
// IoT returns ResourceNotFoundException for an unknown security profile name.
func (b *InMemoryBackend) AttachSecurityProfile(input *AttachSecurityProfileInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.securityProfiles.Has(input.SecurityProfileName) {
		return fmt.Errorf("security profile %q not found: %w", input.SecurityProfileName, ErrResourceNotFound)
	}

	b.securityProfileTargets[input.SecurityProfileName] = appendUnique(
		b.securityProfileTargets[input.SecurityProfileName],
		input.SecurityProfileTargetArn,
	)

	return nil
}

// DetachSecurityProfile removes a target ARN from a security profile. Real
// AWS IoT returns ResourceNotFoundException for an unknown security profile
// name -- the same gap AttachSecurityProfile had before it was fixed
// (gopherstack-ep0r); DetachSecurityProfile silently no-op'd for any name
// previously.
func (b *InMemoryBackend) DetachSecurityProfile(profileName, targetARN string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.securityProfiles.Has(profileName) {
		return fmt.Errorf("security profile %q not found: %w", profileName, ErrResourceNotFound)
	}

	targets := b.securityProfileTargets[profileName]
	filtered := make([]string, 0, len(targets))
	for _, t := range targets {
		if t != targetARN {
			filtered = append(filtered, t)
		}
	}
	b.securityProfileTargets[profileName] = filtered

	return nil
}

// ListTargetsForSecurityProfile returns the target ARNs attached to a security profile.
func (b *InMemoryBackend) ListTargetsForSecurityProfile(profileName string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	targets := b.securityProfileTargets[profileName]
	out := make([]string, len(targets))
	copy(out, targets)

	return out
}

// ListSecurityProfilesForTarget returns profile names attached to a target ARN.
func (b *InMemoryBackend) ListSecurityProfilesForTarget(targetARN string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []string
	for profileName, targets := range b.securityProfileTargets {
		if slices.Contains(targets, targetARN) {
			out = append(out, profileName)
		}
	}
	sort.Strings(out)

	return out
}

// SecurityProfile represents an IoT security profile.
//
// Tags is internal-storage-only (json:"-"): real DescribeSecurityProfileOutput
// and UpdateSecurityProfileOutput have no "tags" field at all
// (awsRestjson1_deserializeOpDocumentDescribeSecurityProfileOutput/
// UpdateSecurityProfileOutput, v1.77.4) — tags attached at creation are only
// retrievable via the separate ListTagsForResource op, so surfacing it here
// would be the same "invented field" bug class fixed for Job/JobTemplate's
// leaked "tags" elsewhere in this service.
type SecurityProfile struct {
	Tags                        map[string]string                     `json:"-"`
	AlertTargets                map[string]SecurityProfileAlertTarget `json:"alertTargets,omitempty"`
	MetricsExportConfig         *SecurityProfileMetricsExportConfig   `json:"metricsExportConfig,omitempty"`
	SecurityProfileName         string                                `json:"securityProfileName"`
	SecurityProfileARN          string                                `json:"securityProfileArn"`
	SecurityProfileDescription  string                                `json:"securityProfileDescription,omitempty"`
	Behaviors                   []SecurityProfileBehavior             `json:"behaviors,omitempty"`
	AdditionalMetricsToRetain   []string                              `json:"additionalMetricsToRetain,omitempty"`
	AdditionalMetricsToRetainV2 []SecurityProfileMetricToRetain       `json:"additionalMetricsToRetainV2,omitempty"`
	Version                     int64                                 `json:"version"`
	CreationDate                float64                               `json:"creationDate,omitempty"`
	LastModifiedDate            float64                               `json:"lastModifiedDate,omitempty"`
}

// SecurityProfileMetricDimension narrows a behavior's (or additional
// retained metric's) evaluation to a named dimension
// (types.MetricDimension).
type SecurityProfileMetricDimension struct {
	DimensionName string `json:"dimensionName,omitempty"`
	Operator      string `json:"operator,omitempty"`
}

// SecurityProfileMetricValue is the value side of a STATIC behavior
// criteria comparison (types.MetricValue).
type SecurityProfileMetricValue struct {
	Count   *int64    `json:"count,omitempty"`
	Number  *float64  `json:"number,omitempty"`
	Cidrs   []string  `json:"cidrs,omitempty"`
	Numbers []float64 `json:"numbers,omitempty"`
	Ports   []int32   `json:"ports,omitempty"`
	Strings []string  `json:"strings,omitempty"`
}

// SecurityProfileStatisticalThreshold selects the percentile statistic a
// STATISTICAL behavior criteria is evaluated against
// (types.StatisticalThreshold).
type SecurityProfileStatisticalThreshold struct {
	Statistic string `json:"statistic,omitempty"`
}

// SecurityProfileMLDetectionConfig configures a MACHINE_LEARNING behavior
// criteria's confidence level (types.MachineLearningDetectionConfig).
type SecurityProfileMLDetectionConfig struct {
	ConfidenceLevel string `json:"confidenceLevel,omitempty"`
}

// SecurityProfileAlertTarget is an alert destination for behavior
// violations (types.AlertTarget). Real AlertTargets is a
// map[string]AlertTarget keyed by AlertTargetType (e.g. "SNS"); the map
// itself is modeled as SecurityProfile.AlertTargets/
// CreateSecurityProfileInput.AlertTargets.
type SecurityProfileAlertTarget struct {
	AlertTargetArn string `json:"alertTargetArn"`
	RoleArn        string `json:"roleArn"`
}

// SecurityProfileMetricToRetain is an additional metric retained beyond
// those already used by the profile's own behaviors
// (types.MetricToRetain).
type SecurityProfileMetricToRetain struct {
	MetricDimension *SecurityProfileMetricDimension `json:"metricDimension,omitempty"`
	ExportMetric    *bool                           `json:"exportMetric,omitempty"`
	Metric          string                          `json:"metric"`
}

// SecurityProfileMetricsExportConfig configures MQTT export of behavior
// evaluation metrics (types.MetricsExportConfig).
type SecurityProfileMetricsExportConfig struct {
	MqttTopic string `json:"mqttTopic"`
	RoleArn   string `json:"roleArn"`
}

func cloneSecurityProfile(sp *SecurityProfile) *SecurityProfile {
	cp := *sp
	if sp.Tags != nil {
		cp.Tags = maps.Clone(sp.Tags)
	}
	if sp.AlertTargets != nil {
		cp.AlertTargets = maps.Clone(sp.AlertTargets)
	}
	cp.Behaviors = cloneSecurityProfileBehaviors(sp.Behaviors)
	cp.AdditionalMetricsToRetain = slices.Clone(sp.AdditionalMetricsToRetain)
	cp.AdditionalMetricsToRetainV2 = cloneSecurityProfileMetricsToRetain(sp.AdditionalMetricsToRetainV2)
	if sp.MetricsExportConfig != nil {
		mec := *sp.MetricsExportConfig
		cp.MetricsExportConfig = &mec
	}

	return &cp
}

// cloneSecurityProfileMetricDimension deep-copies an optional
// SecurityProfileMetricDimension pointer.
func cloneSecurityProfileMetricDimension(md *SecurityProfileMetricDimension) *SecurityProfileMetricDimension {
	if md == nil {
		return nil
	}
	cp := *md

	return &cp
}

// cloneSecurityProfileBehaviors deep-copies a Behavior list, including each
// behavior's Criteria (and the Criteria's own Value/StatisticalThreshold/
// MlDetectionConfig pointers) so that mutating a cloned SecurityProfile
// never aliases the stored one's nested pointers/slices.
func cloneSecurityProfileBehaviors(behaviors []SecurityProfileBehavior) []SecurityProfileBehavior {
	if behaviors == nil {
		return nil
	}
	out := make([]SecurityProfileBehavior, len(behaviors))
	for i, beh := range behaviors {
		out[i] = beh
		out[i].MetricDimension = cloneSecurityProfileMetricDimension(beh.MetricDimension)
		out[i].Criteria = cloneSecurityProfileBehaviorCriteria(beh.Criteria)
		if beh.ExportMetric != nil {
			v := *beh.ExportMetric
			out[i].ExportMetric = &v
		}
		if beh.SuppressAlerts != nil {
			v := *beh.SuppressAlerts
			out[i].SuppressAlerts = &v
		}
	}

	return out
}

func cloneSecurityProfileBehaviorCriteria(c *SecurityProfileBehaviorCriteria) *SecurityProfileBehaviorCriteria {
	if c == nil {
		return nil
	}
	cp := *c
	if c.Value != nil {
		v := *c.Value
		v.Cidrs = slices.Clone(c.Value.Cidrs)
		v.Numbers = slices.Clone(c.Value.Numbers)
		v.Ports = slices.Clone(c.Value.Ports)
		v.Strings = slices.Clone(c.Value.Strings)
		cp.Value = &v
	}
	if c.StatisticalThreshold != nil {
		st := *c.StatisticalThreshold
		cp.StatisticalThreshold = &st
	}
	if c.MlDetectionConfig != nil {
		ml := *c.MlDetectionConfig
		cp.MlDetectionConfig = &ml
	}

	return &cp
}

// cloneSecurityProfileMetricsToRetain deep-copies an
// AdditionalMetricsToRetainV2 list.
func cloneSecurityProfileMetricsToRetain(items []SecurityProfileMetricToRetain) []SecurityProfileMetricToRetain {
	if items == nil {
		return nil
	}
	out := make([]SecurityProfileMetricToRetain, len(items))
	for i, m := range items {
		out[i] = m
		out[i].MetricDimension = cloneSecurityProfileMetricDimension(m.MetricDimension)
		if m.ExportMetric != nil {
			v := *m.ExportMetric
			out[i].ExportMetric = &v
		}
	}

	return out
}

func (b *InMemoryBackend) securityProfileARN(name string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("securityprofile/%s", name))
}

// SecurityProfileARN returns the ARN a security profile of the given name
// would have. Used by ListSecurityProfilesForTarget's handler to populate
// the real SecurityProfileIdentifier.arn field (previously entirely
// missing from that op's response -- see handler_security_profiles.go).
// Pure string formatting; no lock needed.
func (b *InMemoryBackend) SecurityProfileARN(name string) string {
	return b.securityProfileARN(name)
}

// CreateSecurityProfileInput holds input for CreateSecurityProfile.
//
// Field-diffed against types.CreateSecurityProfileInput (v1.77.4): Behaviors/
// AlertTargets/AdditionalMetricsToRetain/AdditionalMetricsToRetainV2/
// MetricsExportConfig are now modeled and wired through to the stored
// SecurityProfile (previously silently dropped -- see SecurityProfile's own
// doc comment).
type CreateSecurityProfileInput struct {
	// []types.Tag on the wire, not a map (serializers.go:4507, aws-sdk-go-v2/service/iot@v1.77.4).
	Tags                        []tags.KV                             `json:"tags,omitempty"`
	AlertTargets                map[string]SecurityProfileAlertTarget `json:"alertTargets,omitempty"`
	MetricsExportConfig         *SecurityProfileMetricsExportConfig   `json:"metricsExportConfig,omitempty"`
	SecurityProfileName         string                                `json:"securityProfileName"`
	SecurityProfileDescription  string                                `json:"securityProfileDescription,omitempty"`
	Behaviors                   []SecurityProfileBehavior             `json:"behaviors,omitempty"`
	AdditionalMetricsToRetain   []string                              `json:"additionalMetricsToRetain,omitempty"`
	AdditionalMetricsToRetainV2 []SecurityProfileMetricToRetain       `json:"additionalMetricsToRetainV2,omitempty"`
}

func (b *InMemoryBackend) CreateSecurityProfile(
	input *CreateSecurityProfileInput,
) (*SecurityProfile, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.securityProfiles.Has(input.SecurityProfileName) {
		return nil, fmt.Errorf(
			"security profile %q already exists: %w",
			input.SecurityProfileName,
			ErrAlreadyExists,
		)
	}
	now := float64(time.Now().Unix())
	sp := &SecurityProfile{
		SecurityProfileName:         input.SecurityProfileName,
		SecurityProfileARN:          b.securityProfileARN(input.SecurityProfileName),
		SecurityProfileDescription:  input.SecurityProfileDescription,
		Tags:                        tags.MapFromKV(input.Tags),
		Behaviors:                   input.Behaviors,
		AlertTargets:                input.AlertTargets,
		AdditionalMetricsToRetain:   input.AdditionalMetricsToRetain,
		AdditionalMetricsToRetainV2: input.AdditionalMetricsToRetainV2,
		MetricsExportConfig:         input.MetricsExportConfig,
		Version:                     1,
		CreationDate:                now,
		LastModifiedDate:            now,
	}
	b.securityProfiles.Put(sp)
	b.putResourceTagsLocked(sp.SecurityProfileARN, sp.Tags)

	return cloneSecurityProfile(sp), nil
}

func (b *InMemoryBackend) DescribeSecurityProfile(name string) (*SecurityProfile, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sp, ok := b.securityProfiles.Get(name)
	if !ok {
		return nil, fmt.Errorf("security profile %q not found: %w", name, ErrResourceNotFound)
	}

	return cloneSecurityProfile(sp), nil
}

func (b *InMemoryBackend) ListSecurityProfiles() []*SecurityProfile {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*SecurityProfile, 0, b.securityProfiles.Len())
	for _, v := range b.securityProfiles.Snapshot() {
		out = append(out, cloneSecurityProfile(v))
	}

	return out
}

// UpdateSecurityProfileInput holds input for UpdateSecurityProfile
// (types.UpdateSecurityProfileInput, v1.77.4). Each DeleteX flag clears the
// corresponding field; supplying both a DeleteX flag and a non-nil value
// for that same field in one call is invalid per real AWS, enforced in
// applySecurityProfileUpdate.
type UpdateSecurityProfileInput struct {
	AlertTargets                    map[string]SecurityProfileAlertTarget
	MetricsExportConfig             *SecurityProfileMetricsExportConfig
	ExpectedVersion                 *int64
	SecurityProfileDescription      *string
	SecurityProfileName             string
	Behaviors                       []SecurityProfileBehavior
	AdditionalMetricsToRetain       []string
	AdditionalMetricsToRetainV2     []SecurityProfileMetricToRetain
	DeleteAdditionalMetricsToRetain bool
	DeleteAlertTargets              bool
	DeleteBehaviors                 bool
	DeleteMetricsExportConfig       bool
}

// UpdateSecurityProfile updates a security profile's mutable fields.
// ExpectedVersion, when set, must match the profile's current version or
// ErrVersionConflict (-> VersionConflictException) is returned, matching
// real AWS's optimistic-locking semantics.
func (b *InMemoryBackend) UpdateSecurityProfile(
	input *UpdateSecurityProfileInput,
) (*SecurityProfile, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sp, ok := b.securityProfiles.Get(input.SecurityProfileName)
	if !ok {
		return nil, fmt.Errorf("security profile %q not found: %w", input.SecurityProfileName, ErrResourceNotFound)
	}

	if input.ExpectedVersion != nil && *input.ExpectedVersion != sp.Version {
		return nil, fmt.Errorf(
			"expected version %d does not match current version %d: %w",
			*input.ExpectedVersion, sp.Version, ErrVersionConflict,
		)
	}

	if err := applySecurityProfileUpdate(sp, input); err != nil {
		return nil, err
	}

	sp.Version++
	sp.LastModifiedDate = float64(time.Now().Unix())

	return cloneSecurityProfile(sp), nil
}

// applySecurityProfileUpdate mutates sp in place per input, enforcing the
// DeleteX-vs-field mutual exclusion documented on UpdateSecurityProfileInput.
// Split out of UpdateSecurityProfile to keep both functions' cyclomatic
// complexity low.
func applySecurityProfileUpdate(sp *SecurityProfile, input *UpdateSecurityProfileInput) error {
	if err := applyBehaviorsUpdate(sp, input); err != nil {
		return err
	}
	if err := applyAlertTargetsUpdate(sp, input); err != nil {
		return err
	}
	if err := applyMetricsToRetainUpdate(sp, input); err != nil {
		return err
	}
	if err := applyMetricsExportConfigUpdate(sp, input); err != nil {
		return err
	}
	if input.SecurityProfileDescription != nil {
		sp.SecurityProfileDescription = *input.SecurityProfileDescription
	}

	return nil
}

func applyBehaviorsUpdate(sp *SecurityProfile, input *UpdateSecurityProfileInput) error {
	if input.DeleteBehaviors {
		if input.Behaviors != nil {
			return fmt.Errorf("cannot set behaviors and deleteBehaviors together: %w", ErrValidation)
		}
		sp.Behaviors = nil

		return nil
	}
	if input.Behaviors != nil {
		sp.Behaviors = input.Behaviors
	}

	return nil
}

func applyAlertTargetsUpdate(sp *SecurityProfile, input *UpdateSecurityProfileInput) error {
	if input.DeleteAlertTargets {
		if input.AlertTargets != nil {
			return fmt.Errorf("cannot set alertTargets and deleteAlertTargets together: %w", ErrValidation)
		}
		sp.AlertTargets = nil

		return nil
	}
	if input.AlertTargets != nil {
		sp.AlertTargets = input.AlertTargets
	}

	return nil
}

func applyMetricsToRetainUpdate(sp *SecurityProfile, input *UpdateSecurityProfileInput) error {
	if input.DeleteAdditionalMetricsToRetain {
		if input.AdditionalMetricsToRetain != nil || input.AdditionalMetricsToRetainV2 != nil {
			return fmt.Errorf(
				"cannot set additionalMetricsToRetain(V2) and deleteAdditionalMetricsToRetain together: %w",
				ErrValidation,
			)
		}
		sp.AdditionalMetricsToRetain = nil
		sp.AdditionalMetricsToRetainV2 = nil

		return nil
	}
	if input.AdditionalMetricsToRetain != nil {
		sp.AdditionalMetricsToRetain = input.AdditionalMetricsToRetain
	}
	if input.AdditionalMetricsToRetainV2 != nil {
		sp.AdditionalMetricsToRetainV2 = input.AdditionalMetricsToRetainV2
	}

	return nil
}

func applyMetricsExportConfigUpdate(sp *SecurityProfile, input *UpdateSecurityProfileInput) error {
	if input.DeleteMetricsExportConfig {
		if input.MetricsExportConfig != nil {
			return fmt.Errorf(
				"cannot set metricsExportConfig and deleteMetricsExportConfig together: %w", ErrValidation,
			)
		}
		sp.MetricsExportConfig = nil

		return nil
	}
	if input.MetricsExportConfig != nil {
		sp.MetricsExportConfig = input.MetricsExportConfig
	}

	return nil
}

// DeleteSecurityProfile removes a security profile. Also cascade-deletes its
// target attachments (b.securityProfileTargets) -- previously left behind
// as a ghost row keyed by the now-deleted profile's name (AttachSecurityProfile
// would then succeed again for that name via re-Create and immediately see
// stale prior attachments; ListTargetsForSecurityProfile/
// ListSecurityProfilesForTarget could leak attachment data for a profile
// that DescribeSecurityProfile reports as ResourceNotFoundException).
func (b *InMemoryBackend) DeleteSecurityProfile(name string, expectedVersion int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	sp, ok := b.securityProfiles.Get(name)
	if !ok {
		return fmt.Errorf("security profile %q not found: %w", name, ErrResourceNotFound)
	}

	if expectedVersion != 0 && expectedVersion != sp.Version {
		return fmt.Errorf("%w: expected version %d, current version %d",
			ErrVersionConflict, expectedVersion, sp.Version)
	}
	b.securityProfiles.Delete(name)
	delete(b.securityProfileTargets, name)
	delete(b.resourceTags, sp.SecurityProfileARN)

	return nil
}

// SecurityProfileBehavior describes one behavior, used both for
// ValidateSecurityProfileBehaviors' standalone validation and, now, as the
// actual persisted shape of SecurityProfile.Behaviors (types.Behavior).
// ExportMetric/MetricDimension/SuppressAlerts were added when Behaviors was
// wired up to real Create/UpdateSecurityProfile storage -- previously this
// type only existed for the validation-only endpoint and was never field-
// diffed against the full real shape.
type SecurityProfileBehavior struct {
	Criteria        *SecurityProfileBehaviorCriteria `json:"criteria,omitempty"`
	MetricDimension *SecurityProfileMetricDimension  `json:"metricDimension,omitempty"`
	ExportMetric    *bool                            `json:"exportMetric,omitempty"`
	SuppressAlerts  *bool                            `json:"suppressAlerts,omitempty"`
	Name            string                           `json:"name"`
	Metric          string                           `json:"metric,omitempty"`
}

// SecurityProfileBehaviorCriteria is the criteria portion of a behavior
// (types.BehaviorCriteria). Value/StatisticalThreshold/MlDetectionConfig
// were added alongside SecurityProfileBehavior's own field-diff, above --
// which of the three is set determines the behavior's
// types.BehaviorCriteriaType (STATIC/STATISTICAL/MACHINE_LEARNING
// respectively), see behaviorCriteriaTypeOf.
type SecurityProfileBehaviorCriteria struct {
	Value                        *SecurityProfileMetricValue          `json:"value,omitempty"`
	StatisticalThreshold         *SecurityProfileStatisticalThreshold `json:"statisticalThreshold,omitempty"`
	MlDetectionConfig            *SecurityProfileMLDetectionConfig    `json:"mlDetectionConfig,omitempty"`
	ComparisonOperator           string                               `json:"comparisonOperator,omitempty"`
	DurationSeconds              int32                                `json:"durationSeconds,omitempty"`
	ConsecutiveDatapointsToAlarm int32                                `json:"consecutiveDatapointsToAlarm,omitempty"`
	ConsecutiveDatapointsToClear int32                                `json:"consecutiveDatapointsToClear,omitempty"`
}

// behaviorCriteriaTypeOf derives types.BehaviorCriteriaType
// ("STATIC"/"STATISTICAL"/"MACHINE_LEARNING") for a behavior criteria. Real
// AWS's three shapes are mutually exclusive: static value comparison, OR a
// percentile statisticalThreshold, OR mlDetectionConfig. Nil criteria has no
// type (""); a non-nil criteria with neither sub-message set defaults to
// STATIC, the one that needs none to be well-formed.
func behaviorCriteriaTypeOf(c *SecurityProfileBehaviorCriteria) string {
	switch {
	case c == nil:
		return ""
	case c.MlDetectionConfig != nil:
		return "MACHINE_LEARNING"
	case c.StatisticalThreshold != nil:
		return "STATISTICAL"
	default:
		return "STATIC"
	}
}

// securityProfileBehaviorCriteriaTypeLocked resolves the criteria type
// (see behaviorCriteriaTypeOf) of the named behavior on the named security
// profile, as stored via Create/UpdateSecurityProfile. Returns "" if the
// profile or the named behavior on it cannot be found -- this is what ties
// ListActiveViolations/ListViolationEvents' behaviorCriteriaType filter to
// real persisted behavior data instead of guessing. Must be called with
// b.mu already held by the caller (read or write), matching this package's
// other *Locked helpers (e.g. jobs.go's fanOutJobExecutionsLocked).
func (b *InMemoryBackend) securityProfileBehaviorCriteriaTypeLocked(
	securityProfileName string,
	beh *ViolationBehavior,
) string {
	if beh == nil || beh.Name == "" {
		return ""
	}
	sp, ok := b.securityProfiles.Get(securityProfileName)
	if !ok {
		return ""
	}
	for _, sb := range sp.Behaviors {
		if sb.Name == beh.Name {
			return behaviorCriteriaTypeOf(sb.Criteria)
		}
	}

	return ""
}

// isValidComparisonOperator reports whether op is one of the AWS IoT Device
// Defender comparison operators (types.ComparisonOperator).
func isValidComparisonOperator(op string) bool {
	switch op {
	case "less-than", "less-than-equals", "greater-than", "greater-than-equals",
		"in-cidr-set", "not-in-cidr-set", "in-port-set", "not-in-port-set",
		"in-set", "not-in-set":
		return true
	default:
		return false
	}
}

// ValidateSecurityProfileBehaviors validates the supplied behaviors against
// AWS's structural rules (name required, criteria required, comparison
// operator must be a known value, non-negative durations/thresholds).
func (b *InMemoryBackend) ValidateSecurityProfileBehaviors(
	behaviors []SecurityProfileBehavior,
) (bool, []string) {
	errs := make([]string, 0, len(behaviors))

	for _, beh := range behaviors {
		errs = append(errs, validateOneBehavior(beh)...)
	}

	return len(errs) == 0, errs
}

func validateOneBehavior(beh SecurityProfileBehavior) []string {
	var errs []string

	if beh.Name == "" {
		errs = append(errs, "behavior name is required")
	}

	if beh.Criteria == nil {
		errs = append(errs, fmt.Sprintf("behavior %q: criteria is required", beh.Name))

		return errs
	}

	if beh.Criteria.ComparisonOperator != "" && !isValidComparisonOperator(beh.Criteria.ComparisonOperator) {
		errs = append(errs, fmt.Sprintf(
			"behavior %q: invalid comparisonOperator %q", beh.Name, beh.Criteria.ComparisonOperator,
		))
	}

	if beh.Criteria.DurationSeconds < 0 {
		errs = append(errs, fmt.Sprintf("behavior %q: durationSeconds must not be negative", beh.Name))
	}

	if beh.Criteria.ConsecutiveDatapointsToAlarm < 0 {
		errs = append(errs, fmt.Sprintf("behavior %q: consecutiveDatapointsToAlarm must not be negative", beh.Name))
	}

	if beh.Criteria.ConsecutiveDatapointsToClear < 0 {
		errs = append(errs, fmt.Sprintf("behavior %q: consecutiveDatapointsToClear must not be negative", beh.Name))
	}

	return errs
}
