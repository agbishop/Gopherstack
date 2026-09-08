package iot

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// AddAuditTaskInternal seeds an audit task status with a caller-chosen ID
// directly into the backend for testing (mirrors AddThingInternal).
// StartOnDemandAuditTask always generates a random ID, so tests that need a
// deterministic task ID use this instead.
func (b *InMemoryBackend) AddAuditTaskInternal(taskID, status string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.auditTasks[taskID] = status
}

// AddAuditMitigationTaskInternal seeds an audit mitigation actions task
// status with a caller-chosen ID directly into the backend for testing
// (mirrors AddThingInternal). StartAuditMitigationActionsTask always
// generates a random ID, so tests that need a deterministic task ID use this
// instead.
func (b *InMemoryBackend) AddAuditMitigationTaskInternal(taskID, status string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.auditMitigationTasks[taskID] = status
}

// CancelAuditMitigationActionsTask cancels an audit mitigation actions task.
// Real AWS IoT returns ResourceNotFoundException for an unknown task ID and
// InvalidRequestException if the task is known but not in progress; when the
// task is known and in progress, its rich AuditMitigationTask record (as
// returned by DescribeAuditMitigationActionsTask) is transitioned to
// CANCELED with an end time, keeping the two representations consistent.
func (b *InMemoryBackend) CancelAuditMitigationActionsTask(input *CancelAuditMitigationActionsTaskInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	status, ok := b.auditMitigationTasks[input.TaskID]
	if !ok {
		return fmt.Errorf("%w: audit mitigation actions task %q", ErrResourceNotFound, input.TaskID)
	}

	if status != string(JobStatusInProgress) {
		return fmt.Errorf("%w: audit mitigation actions task %q is not in progress", ErrValidation, input.TaskID)
	}

	b.auditMitigationTasks[input.TaskID] = string(JobStatusCanceled)
	if t, found := b.auditMitigationTaskObjects.Get(input.TaskID); found {
		t.TaskStatus = string(JobStatusCanceled)
		t.EndTime = float64(time.Now().Unix())
	}

	return nil
}

// CancelAuditTask cancels an in-progress audit task. Real AWS IoT returns
// ResourceNotFoundException for an unknown task ID and InvalidRequestException
// if the task is known but not in progress.
func (b *InMemoryBackend) CancelAuditTask(input *CancelAuditTaskInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	status, ok := b.auditTasks[input.AuditTaskID]
	if !ok {
		return fmt.Errorf("%w: audit task %q", ErrResourceNotFound, input.AuditTaskID)
	}

	if status != string(JobStatusInProgress) {
		return fmt.Errorf("%w: audit task %q is not in progress", ErrValidation, input.AuditTaskID)
	}

	b.auditTasks[input.AuditTaskID] = "CANCELED"

	return nil
}

// AuditCheckConfig holds config for a single audit check.
//
// Configuration mirrors types.AuditCheckConfiguration's "configuration" map
// member (v1.77.4) -- previously entirely unmodeled, so a real client's
// UpdateAccountAuditConfiguration call setting per-check configuration
// values had them silently dropped, and DescribeAccountAuditConfiguration
// could never surface them.
type AuditCheckConfig struct {
	Configuration map[string]string `json:"configuration,omitempty"`
	Enabled       bool              `json:"enabled"`
}

// AccountAuditConfiguration holds the account-level audit configuration.
type AccountAuditConfiguration struct {
	AuditCheckConfigurations map[string]*AuditCheckConfig `json:"auditCheckConfigurations,omitempty"`
	AuditNotificationTarget  any                          `json:"auditNotificationTargetConfigurations,omitempty"`
	RoleARN                  string                       `json:"roleArn,omitempty"`
}

// UpdateAccountAuditConfiguration merges roleARN and checks into the
// account's audit configuration. checks is a map[checkName]*AuditCheckConfig
// (types.UpdateAccountAuditConfigurationInput.AuditCheckConfigurations); a
// real client only ever names the checks it's changing, so merging by key
// keeps checks omitted from this call at whatever state a previous call left
// them in, rather than wholesale-replacing the map and disabling every check
// not named this time (gopherstack-c8ge).
func (b *InMemoryBackend) UpdateAccountAuditConfiguration(roleARN string, checks map[string]*AuditCheckConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.auditConfiguration == nil {
		b.auditConfiguration = &AccountAuditConfiguration{}
	}
	if roleARN != "" {
		b.auditConfiguration.RoleARN = roleARN
	}
	if len(checks) > 0 {
		if b.auditConfiguration.AuditCheckConfigurations == nil {
			b.auditConfiguration.AuditCheckConfigurations = make(map[string]*AuditCheckConfig, len(checks))
		}
		maps.Copy(b.auditConfiguration.AuditCheckConfigurations, checks)
	}

	return nil
}

func (b *InMemoryBackend) DescribeAccountAuditConfiguration() *AccountAuditConfiguration {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.auditConfiguration == nil {
		return &AccountAuditConfiguration{
			AuditCheckConfigurations: map[string]*AuditCheckConfig{},
		}
	}
	cp := *b.auditConfiguration

	return &cp
}

// DeleteAccountAuditConfiguration clears the account-level audit configuration,
// restoring it to its unconfigured state. It is idempotent: deleting an
// unconfigured account still succeeds, matching AWS IoT behavior.
func (b *InMemoryBackend) DeleteAccountAuditConfiguration() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.auditConfiguration = nil

	return nil
}

// AuditTask represents an IoT audit task.
type AuditTask struct {
	TaskID             string  `json:"taskId"`
	TaskStatus         string  `json:"taskStatus"`
	TaskType           string  `json:"taskType"`
	ScheduledAuditName string  `json:"scheduledAuditName,omitempty"`
	TaskStartTime      float64 `json:"taskStartTime,omitempty"`
}

func (b *InMemoryBackend) StartOnDemandAuditTask(_ []string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := uuid.NewString()[:12]
	b.auditTasks[id] = string(JobStatusInProgress)
	b.auditTaskObjects.Put(&AuditTask{
		TaskID:        id,
		TaskStatus:    string(JobStatusInProgress),
		TaskType:      "ON_DEMAND_AUDIT_TASK",
		TaskStartTime: float64(time.Now().Unix()),
	})

	return id, nil
}

func (b *InMemoryBackend) DescribeAuditTask(taskID string) (*AuditTask, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	task, ok := b.auditTaskObjects.Get(taskID)
	if !ok {
		return nil, fmt.Errorf("audit task %q not found: %w", taskID, ErrResourceNotFound)
	}
	cp := *task

	return &cp, nil
}

func (b *InMemoryBackend) ListAuditTasks(taskType string) []*AuditTask {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []*AuditTask
	items := b.auditTaskObjects.Snapshot()
	for _, v := range items {
		task := v
		if taskType == "" || task.TaskType == taskType {
			cp := *task
			out = append(out, &cp)
		}
	}

	return out
}

// AuditSuppression represents an AWS IoT audit suppression.
type AuditSuppression struct {
	ResourceIdentifier   map[string]any `json:"resourceIdentifier"`
	CheckName            string         `json:"checkName"`
	Description          string         `json:"description,omitempty"`
	ExpirationDate       float64        `json:"expirationDate,omitempty"`
	SuppressIndefinitely bool           `json:"suppressIndefinitely"`
}

func auditSuppressionKey(checkName string, resourceID map[string]any) string {
	// Use a stable key from checkName plus the first value found in the map.
	var sb strings.Builder
	sb.WriteString(checkName)
	for k, v := range resourceID {
		sb.WriteString("/" + k + "=" + fmt.Sprint(v))

		break
	}

	return sb.String()
}

func cloneAuditSuppression(s *AuditSuppression) *AuditSuppression {
	cp := *s
	cp.ResourceIdentifier = make(map[string]any, len(s.ResourceIdentifier))
	maps.Copy(cp.ResourceIdentifier, s.ResourceIdentifier)

	return &cp
}

func (b *InMemoryBackend) CreateAuditSuppression(
	checkName string,
	resourceID map[string]any,
	description string,
	suppressIndefinitely bool,
	expirationDate float64,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := auditSuppressionKey(checkName, resourceID)
	if b.auditSuppressions.Has(key) {
		return fmt.Errorf("audit suppression %q already exists: %w", key, ErrAlreadyExists)
	}
	rid := make(map[string]any, len(resourceID))
	maps.Copy(rid, resourceID)
	b.auditSuppressions.Put(&AuditSuppression{
		CheckName:            checkName,
		ResourceIdentifier:   rid,
		Description:          description,
		SuppressIndefinitely: suppressIndefinitely,
		ExpirationDate:       expirationDate,
	})

	return nil
}

func (b *InMemoryBackend) DescribeAuditSuppression(
	checkName string,
	resourceID map[string]any,
) (*AuditSuppression, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	key := auditSuppressionKey(checkName, resourceID)
	s, ok := b.auditSuppressions.Get(key)
	if !ok {
		return nil, fmt.Errorf("audit suppression %q not found: %w", key, ErrResourceNotFound)
	}

	return cloneAuditSuppression(s), nil
}

func (b *InMemoryBackend) UpdateAuditSuppression(
	checkName string,
	resourceID map[string]any,
	description string,
	suppressIndefinitely bool,
	expirationDate float64,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := auditSuppressionKey(checkName, resourceID)
	s, ok := b.auditSuppressions.Get(key)
	if !ok {
		return fmt.Errorf("audit suppression %q not found: %w", key, ErrResourceNotFound)
	}
	if description != "" {
		s.Description = description
	}
	s.SuppressIndefinitely = suppressIndefinitely
	if expirationDate != 0 {
		s.ExpirationDate = expirationDate
	}

	return nil
}

func (b *InMemoryBackend) DeleteAuditSuppression(checkName string, resourceID map[string]any) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := auditSuppressionKey(checkName, resourceID)
	if !b.auditSuppressions.Has(key) {
		return fmt.Errorf("audit suppression %q not found: %w", key, ErrResourceNotFound)
	}
	b.auditSuppressions.Delete(key)

	return nil
}

func (b *InMemoryBackend) ListAuditSuppressions() []*AuditSuppression {
	b.mu.RLock()
	defer b.mu.RUnlock()

	items := b.auditSuppressions.Snapshot()
	out := make([]*AuditSuppression, 0, len(items))
	for _, v := range items {
		out = append(out, cloneAuditSuppression(v))
	}

	return out
}

// IssuerCertificateIdentifier identifies a certificate issuer
// (aws-sdk-go-v2/service/iot/types.IssuerCertificateIdentifier), one of
// ResourceIdentifier's nested discriminator fields.
type IssuerCertificateIdentifier struct {
	IssuerCertificateSubject      string `json:"issuerCertificateSubject,omitempty"`
	IssuerID                      string `json:"issuerId,omitempty"`
	IssuerCertificateSerialNumber string `json:"issuerCertificateSerialNumber,omitempty"`
}

// PolicyVersionIdentifier identifies a specific version of an IoT policy
// (types.PolicyVersionIdentifier), one of ResourceIdentifier's nested
// discriminator fields.
type PolicyVersionIdentifier struct {
	PolicyName      string `json:"policyName,omitempty"`
	PolicyVersionID string `json:"policyVersionId,omitempty"`
}

// ResourceIdentifier identifies the noncompliant resource behind an audit
// finding (types.ResourceIdentifier, v1.77.4's ten discriminator fields).
// Real AWS populates only the field(s) relevant to the check that produced
// the finding, e.g. DEVICE_CERTIFICATE_EXPIRING_CHECK sets
// deviceCertificateId. A fully-typed struct (vs. a freeform map) is what
// lets ListAuditFindings' resourceIdentifier filter match honestly per
// matchResourceIdentifier below.
type ResourceIdentifier struct {
	IssuerCertificateIdentifier *IssuerCertificateIdentifier `json:"issuerCertificateIdentifier,omitempty"`
	PolicyVersionIdentifier     *PolicyVersionIdentifier     `json:"policyVersionIdentifier,omitempty"`
	Account                     string                       `json:"account,omitempty"`
	CaCertificateID             string                       `json:"caCertificateId,omitempty"`
	ClientID                    string                       `json:"clientId,omitempty"`
	CognitoIdentityPoolID       string                       `json:"cognitoIdentityPoolId,omitempty"`
	DeviceCertificateArn        string                       `json:"deviceCertificateArn,omitempty"`
	DeviceCertificateID         string                       `json:"deviceCertificateId,omitempty"`
	IamRoleArn                  string                       `json:"iamRoleArn,omitempty"`
	RoleAliasArn                string                       `json:"roleAliasArn,omitempty"`
}

func cloneResourceIdentifier(r *ResourceIdentifier) *ResourceIdentifier {
	if r == nil {
		return nil
	}
	cp := *r
	if r.IssuerCertificateIdentifier != nil {
		ic := *r.IssuerCertificateIdentifier
		cp.IssuerCertificateIdentifier = &ic
	}
	if r.PolicyVersionIdentifier != nil {
		pv := *r.PolicyVersionIdentifier
		cp.PolicyVersionIdentifier = &pv
	}

	return &cp
}

// NonCompliantResource describes the resource an audit check found to be
// noncompliant (types.NonCompliantResource).
type NonCompliantResource struct {
	ResourceIdentifier *ResourceIdentifier `json:"resourceIdentifier,omitempty"`
	AdditionalInfo     map[string]string   `json:"additionalInfo,omitempty"`
	ResourceType       string              `json:"resourceType,omitempty"`
}

func cloneNonCompliantResource(r *NonCompliantResource) *NonCompliantResource {
	if r == nil {
		return nil
	}
	cp := *r
	cp.ResourceIdentifier = cloneResourceIdentifier(r.ResourceIdentifier)
	if r.AdditionalInfo != nil {
		cp.AdditionalInfo = maps.Clone(r.AdditionalInfo)
	}

	return &cp
}

// AuditFinding represents an AWS IoT audit finding.
type AuditFinding struct {
	NonCompliantResource       *NonCompliantResource `json:"nonCompliantResource,omitempty"`
	FindingID                  string                `json:"findingId"`
	TaskID                     string                `json:"taskId,omitempty"`
	CheckName                  string                `json:"checkName"`
	Severity                   string                `json:"severity"`
	ReasonForNonCompliance     string                `json:"reasonForNonCompliance,omitempty"`
	ReasonForNonComplianceCode string                `json:"reasonForNonComplianceCode,omitempty"`
	RelatedResources           []map[string]any      `json:"relatedResources,omitempty"`
	FindingTime                float64               `json:"findingTime,omitempty"`
	TaskStartTime              float64               `json:"taskStartTime,omitempty"`
	IsSuppressed               bool                  `json:"isSuppressed,omitempty"`
}

func cloneAuditFinding(f *AuditFinding) *AuditFinding {
	cp := *f
	cp.NonCompliantResource = cloneNonCompliantResource(f.NonCompliantResource)
	cp.RelatedResources = make([]map[string]any, len(f.RelatedResources))
	copy(cp.RelatedResources, f.RelatedResources)

	return &cp
}

// SeedAuditFinding injects an audit finding into the backend so
// DescribeAuditFinding, ListAuditFindings, and ListRelatedResourcesForAuditFinding
// return realistic data instead of an empty set. Returns the stored finding,
// generating a FindingID when none is supplied.
//
// TaskStartTime, when unset, is derived from the referenced AuditTask so it
// stays consistent with the task that produced the finding.
func (b *InMemoryBackend) SeedAuditFinding(f *AuditFinding) *AuditFinding {
	b.mu.Lock()
	defer b.mu.Unlock()

	stored := cloneAuditFinding(f)
	if stored.FindingID == "" {
		stored.FindingID = uuid.NewString()
	}

	if stored.FindingTime == 0 {
		stored.FindingTime = float64(time.Now().Unix())
	}

	if stored.TaskStartTime == 0 && stored.TaskID != "" {
		if task, ok := b.auditTaskObjects.Get(stored.TaskID); ok {
			stored.TaskStartTime = task.TaskStartTime
		}
	}

	b.auditFindings.Put(stored)

	return cloneAuditFinding(stored)
}

func (b *InMemoryBackend) DescribeAuditFinding(findingID string) (*AuditFinding, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	f, ok := b.auditFindings.Get(findingID)
	if !ok {
		return nil, fmt.Errorf("audit finding %q not found: %w", findingID, ErrResourceNotFound)
	}

	return cloneAuditFinding(f), nil
}

// ListAuditFindingsFilter carries ListAuditFindings' optional filter fields
// (aws-sdk-go-v2/service/iot.ListAuditFindingsInput). checkName, taskId,
// listSuppressedFindings, the [startTime,endTime] time range, and
// resourceIdentifier are all implemented -- see matchResourceIdentifier's
// doc comment for how resourceIdentifier filtering is made honest despite
// AuditFinding.NonCompliantResource being otherwise synthetic/seeded data.
type ListAuditFindingsFilter struct {
	ListSuppressedFindings *bool
	ResourceIdentifier     *ResourceIdentifier
	CheckName              string
	TaskID                 string
	StartTime              float64
	EndTime                float64
}

// matchResourceIdentifier reports whether actual satisfies filter: every
// field SET on filter must be present and equal on actual (AWS's per-field
// discriminator matching). Nil filter always matches; non-nil filter against
// a finding with no resourceIdentifier never matches.
func matchResourceIdentifier(filter, actual *ResourceIdentifier) bool {
	if filter == nil {
		return true
	}
	if actual == nil {
		return false
	}

	switch {
	case filter.Account != "" && filter.Account != actual.Account,
		filter.CaCertificateID != "" && filter.CaCertificateID != actual.CaCertificateID,
		filter.ClientID != "" && filter.ClientID != actual.ClientID,
		filter.CognitoIdentityPoolID != "" && filter.CognitoIdentityPoolID != actual.CognitoIdentityPoolID,
		filter.DeviceCertificateArn != "" && filter.DeviceCertificateArn != actual.DeviceCertificateArn,
		filter.DeviceCertificateID != "" && filter.DeviceCertificateID != actual.DeviceCertificateID,
		filter.IamRoleArn != "" && filter.IamRoleArn != actual.IamRoleArn,
		filter.RoleAliasArn != "" && filter.RoleAliasArn != actual.RoleAliasArn:
		return false
	}

	if !matchPolicyVersionIdentifier(filter.PolicyVersionIdentifier, actual.PolicyVersionIdentifier) {
		return false
	}

	return matchIssuerCertificateIdentifier(filter.IssuerCertificateIdentifier, actual.IssuerCertificateIdentifier)
}

func matchPolicyVersionIdentifier(filter, actual *PolicyVersionIdentifier) bool {
	if filter == nil {
		return true
	}
	if actual == nil {
		return false
	}

	return (filter.PolicyName == "" || filter.PolicyName == actual.PolicyName) &&
		(filter.PolicyVersionID == "" || filter.PolicyVersionID == actual.PolicyVersionID)
}

func matchIssuerCertificateIdentifier(filter, actual *IssuerCertificateIdentifier) bool {
	if filter == nil {
		return true
	}
	if actual == nil {
		return false
	}

	serialMatches := filter.IssuerCertificateSerialNumber == "" ||
		filter.IssuerCertificateSerialNumber == actual.IssuerCertificateSerialNumber

	return (filter.IssuerID == "" || filter.IssuerID == actual.IssuerID) &&
		(filter.IssuerCertificateSubject == "" || filter.IssuerCertificateSubject == actual.IssuerCertificateSubject) &&
		serialMatches
}

func (f *AuditFinding) matchesFilter(filter ListAuditFindingsFilter) bool {
	if filter.CheckName != "" && f.CheckName != filter.CheckName {
		return false
	}

	if filter.TaskID != "" && f.TaskID != filter.TaskID {
		return false
	}

	if filter.ListSuppressedFindings != nil && f.IsSuppressed != *filter.ListSuppressedFindings {
		return false
	}

	if filter.StartTime != 0 && f.FindingTime < filter.StartTime {
		return false
	}

	if filter.EndTime != 0 && f.FindingTime > filter.EndTime {
		return false
	}

	var actual *ResourceIdentifier
	if f.NonCompliantResource != nil {
		actual = f.NonCompliantResource.ResourceIdentifier
	}

	return matchResourceIdentifier(filter.ResourceIdentifier, actual)
}

// ListAuditFindings returns findings matching filter. See
// [ListAuditFindingsFilter]'s doc comment for what is and is not modeled.
func (b *InMemoryBackend) ListAuditFindings(filter ListAuditFindingsFilter) []*AuditFinding {
	b.mu.RLock()
	defer b.mu.RUnlock()

	items := b.auditFindings.Snapshot()
	out := make([]*AuditFinding, 0, len(items))

	for _, v := range items {
		if !v.matchesFilter(filter) {
			continue
		}

		out = append(out, cloneAuditFinding(v))
	}

	return out
}

// ListRelatedResourcesForAuditFinding returns the resources related to a stored
// audit finding (e.g. certificates, policies) identified when the audit ran.
func (b *InMemoryBackend) ListRelatedResourcesForAuditFinding(findingID string) ([]map[string]any, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	f, ok := b.auditFindings.Get(findingID)
	if !ok {
		return nil, fmt.Errorf("audit finding %q not found: %w", findingID, ErrResourceNotFound)
	}

	out := make([]map[string]any, len(f.RelatedResources))
	copy(out, f.RelatedResources)

	return out, nil
}

// EventConfigurations holds the IoT event configurations.
type EventConfigurations struct {
	EventConfigurations map[string]*EventConfigEntry `json:"eventConfigurations"`
	CreationDate        float64                      `json:"creationDate,omitempty"`
	LastModifiedDate    float64                      `json:"lastModifiedDate,omitempty"`
}

// EventConfigEntry holds the enabled flag for an event type.
type EventConfigEntry struct {
	Enabled bool `json:"Enabled"`
}

func (b *InMemoryBackend) DescribeEventConfigurations() *EventConfigurations {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.eventConfigurations == nil {
		return &EventConfigurations{EventConfigurations: map[string]*EventConfigEntry{}}
	}
	cp := EventConfigurations{
		EventConfigurations: make(map[string]*EventConfigEntry, len(b.eventConfigurations.EventConfigurations)),
		CreationDate:        b.eventConfigurations.CreationDate,
		LastModifiedDate:    b.eventConfigurations.LastModifiedDate,
	}
	for k, v := range b.eventConfigurations.EventConfigurations {
		e := *v
		cp.EventConfigurations[k] = &e
	}

	return &cp
}

func (b *InMemoryBackend) UpdateEventConfigurations(cfgs map[string]*EventConfigEntry) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := float64(time.Now().Unix())

	if b.eventConfigurations == nil {
		b.eventConfigurations = &EventConfigurations{
			EventConfigurations: make(map[string]*EventConfigEntry),
			CreationDate:        now,
		}
	}
	for k, v := range cfgs {
		e := *v
		b.eventConfigurations.EventConfigurations[k] = &e
	}
	b.eventConfigurations.LastModifiedDate = now

	return nil
}

// ScheduledAudit represents an IoT scheduled audit.
//
// Tags is internal-only storage for ListTagsForResource -- real
// DescribeScheduledAuditOutput has no "tags" member (confirmed against
// v1.77.4's awsRestjson1_deserializeOpDocumentDescribeScheduledAuditOutput),
// same leaked-field class already fixed for Job/JobTemplate/SecurityProfile.
type ScheduledAudit struct {
	Tags               map[string]string `json:"-"`
	ScheduledAuditName string            `json:"scheduledAuditName"`
	ScheduledAuditARN  string            `json:"scheduledAuditArn"`
	Frequency          string            `json:"frequency"`
	DayOfMonth         string            `json:"dayOfMonth,omitempty"`
	DayOfWeek          string            `json:"dayOfWeek,omitempty"`
	TargetCheckNames   []string          `json:"targetCheckNames,omitempty"`
}

func cloneScheduledAudit(sa *ScheduledAudit) *ScheduledAudit {
	cp := *sa
	cp.TargetCheckNames = append([]string(nil), sa.TargetCheckNames...)

	return &cp
}

func (b *InMemoryBackend) scheduledAuditARN(name string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("scheduledaudit/%s", name))
}

// CreateScheduledAuditInput holds input for CreateScheduledAudit.
type CreateScheduledAuditInput struct {
	// []types.Tag on the wire, not a map (serializers.go:4389, aws-sdk-go-v2/service/iot@v1.77.4).
	Tags               []tags.KV `json:"tags,omitempty"`
	ScheduledAuditName string    `json:"scheduledAuditName"`
	Frequency          string    `json:"frequency"`
	DayOfMonth         string    `json:"dayOfMonth,omitempty"`
	DayOfWeek          string    `json:"dayOfWeek,omitempty"`
	TargetCheckNames   []string  `json:"targetCheckNames,omitempty"`
}

func (b *InMemoryBackend) CreateScheduledAudit(
	input *CreateScheduledAuditInput,
) (*ScheduledAudit, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.scheduledAudits.Has(input.ScheduledAuditName) {
		return nil, fmt.Errorf(
			"scheduled audit %q already exists: %w",
			input.ScheduledAuditName,
			ErrAlreadyExists,
		)
	}
	sa := &ScheduledAudit{
		ScheduledAuditName: input.ScheduledAuditName,
		ScheduledAuditARN:  b.scheduledAuditARN(input.ScheduledAuditName),
		Frequency:          input.Frequency,
		DayOfMonth:         input.DayOfMonth,
		DayOfWeek:          input.DayOfWeek,
		TargetCheckNames:   append([]string(nil), input.TargetCheckNames...),
		Tags:               tags.MapFromKV(input.Tags),
	}
	b.scheduledAudits.Put(sa)
	b.putResourceTagsLocked(sa.ScheduledAuditARN, sa.Tags)

	return cloneScheduledAudit(sa), nil
}

func (b *InMemoryBackend) DescribeScheduledAudit(name string) (*ScheduledAudit, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sa, ok := b.scheduledAudits.Get(name)
	if !ok {
		return nil, fmt.Errorf("scheduled audit %q not found: %w", name, ErrResourceNotFound)
	}

	return cloneScheduledAudit(sa), nil
}

func (b *InMemoryBackend) ListScheduledAudits() []*ScheduledAudit {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*ScheduledAudit, 0, b.scheduledAudits.Len())
	for _, v := range b.scheduledAudits.Snapshot() {
		out = append(out, cloneScheduledAudit(v))
	}

	return out
}

func (b *InMemoryBackend) UpdateScheduledAudit(
	name, frequency, dayOfMonth, dayOfWeek string,
	checks []string,
) (*ScheduledAudit, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sa, ok := b.scheduledAudits.Get(name)
	if !ok {
		return nil, fmt.Errorf("scheduled audit %q not found: %w", name, ErrResourceNotFound)
	}
	if frequency != "" {
		sa.Frequency = frequency
	}
	if dayOfMonth != "" {
		sa.DayOfMonth = dayOfMonth
	}
	if dayOfWeek != "" {
		sa.DayOfWeek = dayOfWeek
	}
	if len(checks) > 0 {
		sa.TargetCheckNames = append([]string(nil), checks...)
	}

	return cloneScheduledAudit(sa), nil
}

func (b *InMemoryBackend) DeleteScheduledAudit(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.scheduledAudits.Has(name) {
		return fmt.Errorf("scheduled audit %q not found: %w", name, ErrResourceNotFound)
	}
	b.scheduledAudits.Delete(name)
	delete(b.resourceTags, b.scheduledAuditARN(name))

	return nil
}

// MitigationAction represents an IoT mitigation action.
type MitigationAction struct {
	Tags             map[string]string `json:"tags,omitempty"`
	ActionParams     map[string]any    `json:"actionParams,omitempty"`
	ActionName       string            `json:"actionName"`
	ActionARN        string            `json:"actionArn"`
	ActionID         string            `json:"actionId"`
	RoleARN          string            `json:"roleArn,omitempty"`
	CreationDate     float64           `json:"creationDate,omitempty"`
	LastModifiedDate float64           `json:"lastModifiedDate,omitempty"`
}

func cloneMitigationAction(ma *MitigationAction) *MitigationAction {
	cp := *ma

	return &cp
}

func (b *InMemoryBackend) mitigationActionARN(name string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("mitigationaction/%s", name))
}

// CreateMitigationActionInput holds input for CreateMitigationAction.
type CreateMitigationActionInput struct {
	ActionParams map[string]any `json:"actionParams,omitempty"`
	ActionName   string         `json:"actionName"`
	RoleARN      string         `json:"roleArn,omitempty"`
	Tags         []tags.KV      `json:"tags,omitempty"`
}

func (b *InMemoryBackend) CreateMitigationAction(
	input *CreateMitigationActionInput,
) (*MitigationAction, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.mitigationActions.Has(input.ActionName) {
		return nil, fmt.Errorf(
			"mitigation action %q already exists: %w",
			input.ActionName,
			ErrAlreadyExists,
		)
	}
	now := float64(time.Now().Unix())
	ma := &MitigationAction{
		ActionName:       input.ActionName,
		ActionARN:        b.mitigationActionARN(input.ActionName),
		ActionID:         uuid.NewString(),
		RoleARN:          input.RoleARN,
		ActionParams:     input.ActionParams,
		Tags:             tags.MapFromKV(input.Tags),
		CreationDate:     now,
		LastModifiedDate: now,
	}
	b.mitigationActions.Put(ma)
	b.putResourceTagsLocked(ma.ActionARN, ma.Tags)

	return cloneMitigationAction(ma), nil
}

func (b *InMemoryBackend) DescribeMitigationAction(name string) (*MitigationAction, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	ma, ok := b.mitigationActions.Get(name)
	if !ok {
		return nil, fmt.Errorf("mitigation action %q not found: %w", name, ErrResourceNotFound)
	}

	return cloneMitigationAction(ma), nil
}

func (b *InMemoryBackend) ListMitigationActions() []*MitigationAction {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*MitigationAction, 0, b.mitigationActions.Len())
	for _, v := range b.mitigationActions.Snapshot() {
		out = append(out, cloneMitigationAction(v))
	}

	return out
}

func (b *InMemoryBackend) UpdateMitigationAction(
	name, roleARN string,
	params map[string]any,
) (*MitigationAction, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ma, ok := b.mitigationActions.Get(name)
	if !ok {
		return nil, fmt.Errorf("mitigation action %q not found: %w", name, ErrResourceNotFound)
	}
	if roleARN != "" {
		ma.RoleARN = roleARN
	}
	if params != nil {
		ma.ActionParams = params
	}
	ma.LastModifiedDate = float64(time.Now().Unix())

	return cloneMitigationAction(ma), nil
}

func (b *InMemoryBackend) DeleteMitigationAction(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.mitigationActions.Has(name) {
		return fmt.Errorf("mitigation action %q not found: %w", name, ErrResourceNotFound)
	}
	b.mitigationActions.Delete(name)
	delete(b.resourceTags, b.mitigationActionARN(name))

	return nil
}
