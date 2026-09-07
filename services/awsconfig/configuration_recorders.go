package awsconfig

import (
	"fmt"
	"slices"
	"strings"
)

// PutConfigurationRecorder creates or updates the customer managed configuration
// recorder. When updating an existing recorder, the Status is preserved; RoleARN
// and RecordingGroup are updated. A new recorder starts in PENDING state. An
// empty/blank name errors InvalidConfigurationRecorderNameException and an
// empty roleARN errors InvalidRoleException, matching real AWS Config's
// declared error model (verified against aws-sdk-go-v2/service/configservice's
// PutConfigurationRecorder deserializer).
//
// "You can create only one customer managed configuration recorder for each
// account for each Amazon Web Services Region" (api_op_PutConfigurationRecorder.go
// doc comment). Creating a second one under a different name errors
// MaxNumberOfConfigurationRecordersExceededException; service-linked recorders
// (ServicePrincipal/ConnectorArn set) don't count against this limit.
func (b *InMemoryBackend) PutConfigurationRecorder(name, roleARN string, recordingGroup *RecordingGroup) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: ConfigurationRecorder name is required", ErrInvalidConfigurationRecorderName)
	}

	if roleARN == "" {
		return fmt.Errorf("%w: ConfigurationRecorder roleARN is required", ErrInvalidRole)
	}

	b.mu.Lock("PutConfigurationRecorder")
	defer b.mu.Unlock()

	if existing, ok := b.recorders.Get(name); ok {
		existing.RoleARN = roleARN
		existing.RecordingGroup = recordingGroup

		return nil
	}

	if b.hasCustomerManagedRecorderLocked() {
		return fmt.Errorf("%w: account already has a customer managed configuration recorder", ErrAlreadyExists)
	}

	b.recorders.Put(&ConfigurationRecorder{
		Name:           name,
		RoleARN:        roleARN,
		Status:         recorderStatusPending,
		RecordingGroup: recordingGroup,
	})

	return nil
}

// hasCustomerManagedRecorderLocked reports whether any customer-managed
// configuration recorder already exists. Third-party service-linked recorders
// (ServicePrincipal/ConnectorArn set directly on the record) and AWS-native
// service-linked recorders (tracked only via the serviceLinkedRecorders link
// table, since PutServiceLinkedConfigurationRecorder leaves ConfigurationRecorder's
// own ServicePrincipal field unset) are both AWS-managed and don't count
// against the one-per-account limit. Caller must already hold b.mu.
func (b *InMemoryBackend) hasCustomerManagedRecorderLocked() bool {
	linked := make(map[string]struct{}, b.serviceLinkedRecorders.Len())
	for _, link := range b.serviceLinkedRecorders.All() {
		linked[link.RecorderName] = struct{}{}
	}

	for _, r := range b.recorders.All() {
		if r.ServicePrincipal != "" || r.ConnectorArn != "" {
			continue
		}

		if _, ok := linked[r.Name]; ok {
			continue
		}

		return true
	}

	return false
}

// recorderArn builds the ARN for a configuration recorder owned by this backend,
// matching the "arn" field the real service serializes on ConfigurationRecorder
// (aws-sdk-go-v2/service/configservice/types.ConfigurationRecorder.Arn).
func (b *InMemoryBackend) recorderArn(name string) string {
	return fmt.Sprintf("arn:aws:config:%s:%s:config-recorder/%s", b.region, b.accountID, name)
}

// DescribeConfigurationRecorders returns configuration recorders filtered by the
// provided name list.  An empty/nil names list returns all recorders sorted by name.
func (b *InMemoryBackend) DescribeConfigurationRecorders(names []string) []ConfigurationRecorder {
	b.mu.RLock("DescribeConfigurationRecorders")
	defer b.mu.RUnlock()

	out := make([]ConfigurationRecorder, 0, b.recorders.Len())

	if len(names) == 0 {
		for _, r := range b.recorders.All() {
			cp := *r
			cp.Arn = b.recorderArn(r.Name)
			out = append(out, cp)
		}
	} else {
		for _, n := range names {
			if r, ok := b.recorders.Get(n); ok {
				cp := *r
				cp.Arn = b.recorderArn(r.Name)
				out = append(out, cp)
			}
		}
	}

	slices.SortFunc(out, func(a, b ConfigurationRecorder) int {
		if a.Name < b.Name {
			return -1
		}

		if a.Name > b.Name {
			return 1
		}

		return 0
	})

	return out
}

// StartConfigurationRecorder starts a configuration recorder.
func (b *InMemoryBackend) StartConfigurationRecorder(name string) error {
	if name == "" {
		// Declared set is NoAvailableDeliveryChannelException/
		// NoSuchConfigurationRecorderException/UnmodifiableEntityException -- no
		// validation-shaped code fits an empty name (configservice@v1.68.4 deserializers.go).
		return fmt.Errorf("%w: ConfigurationRecorderName is required", ErrValidation)
	}

	b.mu.Lock("StartConfigurationRecorder")
	defer b.mu.Unlock()

	r, ok := b.recorders.Get(name)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	if b.channels.Len() == 0 {
		return fmt.Errorf("%w: no delivery channel configured", ErrNoDeliveryChannel)
	}

	r.Status = recorderStatusActive

	return nil
}

// StopConfigurationRecorder stops an active configuration recorder.
func (b *InMemoryBackend) StopConfigurationRecorder(name string) error {
	if name == "" {
		// Declared set is NoSuchConfigurationRecorderException/UnmodifiableEntityException --
		// no validation-shaped code fits an empty name (configservice@v1.68.4 deserializers.go).
		return fmt.Errorf("%w: ConfigurationRecorderName is required", ErrValidation)
	}

	b.mu.Lock("StopConfigurationRecorder")
	defer b.mu.Unlock()

	r, ok := b.recorders.Get(name)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	r.Status = recorderStatusPending

	return nil
}

// DeleteConfigurationRecorder removes a configuration recorder by name.
func (b *InMemoryBackend) DeleteConfigurationRecorder(name string) error {
	if name == "" {
		// Same declared set as StopConfigurationRecorder -- no validation-shaped code
		// fits an empty name.
		return fmt.Errorf("%w: ConfigurationRecorderName is required", ErrValidation)
	}

	b.mu.Lock("DeleteConfigurationRecorder")
	defer b.mu.Unlock()

	if !b.recorders.Has(name) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	b.recorders.Delete(name)
	b.deleteServiceLinkedLinkForRecorderLocked(name)

	return nil
}

// deleteServiceLinkedLinkForRecorderLocked removes the ServiceLinkedRecorderLink
// (if any) pointing at recorderName, so a service-linked recorder deleted
// through the generic DeleteConfigurationRecorder path (rather than
// DeleteServiceLinkedConfigurationRecorder) doesn't leave a ghost link row.
// Caller must already hold the write lock.
func (b *InMemoryBackend) deleteServiceLinkedLinkForRecorderLocked(recorderName string) {
	for _, link := range b.serviceLinkedRecorders.All() {
		if link.RecorderName == recorderName {
			b.serviceLinkedRecorders.Delete(link.ServicePrincipal)

			return
		}
	}
}

// recorderStatus builds a ConfigurationRecorderStatus from a recorder.
func recorderStatus(r *ConfigurationRecorder) ConfigurationRecorderStatus {
	recording := r.Status == recorderStatusActive
	lastStatus := recorderStatusPending
	if recording {
		lastStatus = recorderStatusSuccess
	}

	return ConfigurationRecorderStatus{
		Name:       r.Name,
		Recording:  recording,
		LastStatus: lastStatus,
	}
}

// DescribeConfigurationRecorderStatus returns recording status for recorders filtered
// by the provided name list.  An empty/nil list returns status for all recorders,
// sorted by name.
func (b *InMemoryBackend) DescribeConfigurationRecorderStatus(names []string) []ConfigurationRecorderStatus {
	b.mu.RLock("DescribeConfigurationRecorderStatus")
	defer b.mu.RUnlock()

	out := make([]ConfigurationRecorderStatus, 0, b.recorders.Len())

	if len(names) == 0 {
		for _, r := range b.recorders.All() {
			out = append(out, recorderStatus(r))
		}
	} else {
		for _, n := range names {
			if r, ok := b.recorders.Get(n); ok {
				out = append(out, recorderStatus(r))
			}
		}
	}

	slices.SortFunc(out, func(a, b ConfigurationRecorderStatus) int {
		if a.Name < b.Name {
			return -1
		}

		if a.Name > b.Name {
			return 1
		}

		return 0
	})

	return out
}

// recorderNameFromArn extracts a recorder's name from either a bare name or a
// full "arn:aws:config:<region>:<account>:config-recorder/<name>" ARN, so
// AssociateResourceTypes/DisassociateResourceTypes accept both forms (real
// SDK callers always send the full ARN; unit tests exercise the bare name).
func recorderNameFromArn(recorderARN string) string {
	if idx := strings.LastIndex(recorderARN, "/"); idx >= 0 {
		return recorderARN[idx+1:]
	}

	return recorderARN
}

// mergeResourceTypes returns existing with every type in added that is not
// already present appended, preserving existing order and de-duplicating.
func mergeResourceTypes(existing, added []string) []string {
	seen := make(map[string]struct{}, len(existing))
	for _, t := range existing {
		seen[t] = struct{}{}
	}

	out := existing

	for _, t := range added {
		if _, ok := seen[t]; ok {
			continue
		}

		seen[t] = struct{}{}
		out = append(out, t)
	}

	return out
}

// removeResourceTypes returns existing with every type in removed dropped.
func removeResourceTypes(existing, removed []string) []string {
	if len(existing) == 0 || len(removed) == 0 {
		return existing
	}

	drop := make(map[string]struct{}, len(removed))
	for _, t := range removed {
		drop[t] = struct{}{}
	}

	out := existing[:0]

	for _, t := range existing {
		if _, ok := drop[t]; !ok {
			out = append(out, t)
		}
	}

	return out
}

// AssociateResourceTypes adds resourceTypes to a configuration recorder's
// RecordingGroup, matching AssociateResourceTypesInput/Output
// (aws-sdk-go-v2/service/configservice). recorderARN may be the recorder's
// bare name or its full ARN. Errors with ErrNotFound (wire type
// NoSuchConfigurationRecorderException) when no matching recorder exists,
// matching the real API's declared error model instead of fabricating a
// synthetic recorder for unknown input.
func (b *InMemoryBackend) AssociateResourceTypes(
	recorderARN string,
	resourceTypes []string,
) (*ConfigurationRecorder, error) {
	if recorderARN == "" {
		return nil, fmt.Errorf("%w: ConfigurationRecorderArn is required", ErrValidation)
	}

	b.mu.Lock("AssociateResourceTypes")
	defer b.mu.Unlock()

	name := recorderNameFromArn(recorderARN)

	r, ok := b.recorders.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, recorderARN)
	}

	if r.RecordingGroup == nil {
		r.RecordingGroup = &RecordingGroup{}
	}

	r.RecordingGroup.ResourceTypes = mergeResourceTypes(r.RecordingGroup.ResourceTypes, resourceTypes)

	cp := *r
	cp.Arn = b.recorderArn(r.Name)
	rgCopy := *r.RecordingGroup
	cp.RecordingGroup = &rgCopy

	return &cp, nil
}

// DisassociateResourceTypes removes resourceTypes from a configuration
// recorder's RecordingGroup, the inverse of AssociateResourceTypes.
// recorderARN may be the recorder's bare name or its full ARN. Errors with
// ErrNotFound (wire type NoSuchConfigurationRecorderException) when no
// matching recorder exists, matching the real API's declared error model
// (verified against aws-sdk-go-v2/service/configservice's
// DisassociateResourceTypes deserializer).
func (b *InMemoryBackend) DisassociateResourceTypes(recorderARN string, resourceTypes []string) error {
	if recorderARN == "" {
		return fmt.Errorf("%w: ConfigurationRecorderArn is required", ErrValidation)
	}

	b.mu.Lock("DisassociateResourceTypes")
	defer b.mu.Unlock()

	name := recorderNameFromArn(recorderARN)

	r, ok := b.recorders.Get(name)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, recorderARN)
	}

	if r.RecordingGroup != nil {
		r.RecordingGroup.ResourceTypes = removeResourceTypes(r.RecordingGroup.ResourceTypes, resourceTypes)
	}

	return nil
}

// serviceLinkedRecorderPrefix is the fixed name prefix real AWS Config assigns
// to every service-linked configuration recorder (verified against
// aws-sdk-go-v2/service/configservice's ConfigurationRecorder.Name doc comment).
const serviceLinkedRecorderPrefix = "AWSConfigurationRecorderFor"

// serviceLinkedRecorderName deterministically derives a service-linked
// recorder's name from its owning service principal (e.g.
// "guardduty.amazonaws.com" -> "AWSConfigurationRecorderForGuardduty"),
// matching the real "AWSConfigurationRecorderFor<Service>" naming convention.
// The exact per-service capitalization AWS uses is not publicly enumerable, so
// this is a best-effort, deterministic title-case of the principal's leading
// label rather than a hardcoded lookup table.
func serviceLinkedRecorderName(servicePrincipal string) string {
	label := servicePrincipal
	if idx := strings.Index(label, "."); idx >= 0 {
		label = label[:idx]
	}

	return serviceLinkedRecorderPrefix + titleCaseWords(label, "-")
}

// titleCaseWords splits s on sep and upper-cases the first rune of each
// non-empty segment, joining the segments back together with no separator
// (e.g. "cost-optimization" -> "CostOptimization"). Used instead of the
// deprecated strings.Title for deterministic service-name casing.
func titleCaseWords(s, sep string) string {
	var b strings.Builder

	for word := range strings.SplitSeq(s, sep) {
		if word == "" {
			continue
		}

		b.WriteString(strings.ToUpper(word[:1]))
		b.WriteString(word[1:])
	}

	return b.String()
}

// PutServiceLinkedConfigurationRecorder creates (or idempotently returns) the
// service-linked configuration recorder for servicePrincipal. Service-linked
// recorders are AWS-managed: they need no caller-supplied IAM role and start
// ACTIVE immediately (matching real AWS Config, which auto-starts them),
// unlike customer-managed recorders created via PutConfigurationRecorder. The
// servicePrincipal -> recorder-name link is tracked separately (see
// ServiceLinkedRecorderLink's doc comment) so it survives persistence without
// leaking onto ConfigurationRecorder's wire-verbatim shape.
func (b *InMemoryBackend) PutServiceLinkedConfigurationRecorder(
	servicePrincipal string, tags []Tag,
) (string, string, error) {
	if servicePrincipal == "" {
		return "", "", fmt.Errorf("%w: ServicePrincipal is required", ErrValidation)
	}

	b.mu.Lock("PutServiceLinkedConfigurationRecorder")
	defer b.mu.Unlock()

	if link, ok := b.serviceLinkedRecorders.Get(servicePrincipal); ok {
		arn := b.recorderArn(link.RecorderName)
		b.setResourceTagsLocked(arn, tags)

		return link.RecorderName, arn, nil
	}

	recName := serviceLinkedRecorderName(servicePrincipal)
	b.recorders.Put(&ConfigurationRecorder{Name: recName, Status: recorderStatusActive})
	b.serviceLinkedRecorders.Put(&ServiceLinkedRecorderLink{
		ServicePrincipal: servicePrincipal,
		RecorderName:     recName,
	})

	arn := b.recorderArn(recName)
	b.setResourceTagsLocked(arn, tags)

	return recName, arn, nil
}

// DeleteServiceLinkedConfigurationRecorder deletes the service-linked
// configuration recorder owned by servicePrincipal. Errors with ErrNotFound
// (wire type NoSuchConfigurationRecorderException) when no matching
// service-linked recorder exists, matching the real API's declared error
// model (verified against aws-sdk-go-v2/service/configservice's
// DeleteServiceLinkedConfigurationRecorder deserializer).
func (b *InMemoryBackend) DeleteServiceLinkedConfigurationRecorder(
	servicePrincipal string,
) (string, string, error) {
	if servicePrincipal == "" {
		return "", "", fmt.Errorf("%w: ServicePrincipal is required", ErrValidation)
	}

	b.mu.Lock("DeleteServiceLinkedConfigurationRecorder")
	defer b.mu.Unlock()

	link, ok := b.serviceLinkedRecorders.Get(servicePrincipal)
	if !ok {
		return "", "", fmt.Errorf("%w: no service-linked recorder for %s", ErrNotFound, servicePrincipal)
	}

	recName := link.RecorderName
	b.recorders.Delete(recName)
	b.serviceLinkedRecorders.Delete(servicePrincipal)

	return recName, b.recorderArn(recName), nil
}

// ListConfigurationRecorders returns summaries of all configuration recorders.
func (b *InMemoryBackend) ListConfigurationRecorders() []ConfigurationRecorderSummary {
	b.mu.RLock("ListConfigurationRecorders")
	defer b.mu.RUnlock()

	all := b.recorders.All()
	out := make([]ConfigurationRecorderSummary, 0, len(all))

	for _, r := range all {
		arn := fmt.Sprintf(
			"arn:aws:config:%s:%s:config-recorder/%s",
			b.region, b.accountID, r.Name,
		)
		out = append(out, ConfigurationRecorderSummary{
			Arn:            arn,
			Name:           r.Name,
			RecordingScope: "INTERNAL",
		})
	}

	return out
}

// thirdPartyRecorderFor returns the third-party service-linked recorder
// (identified by a non-empty ConnectorArn) owned by servicePrincipal via the
// recordersByServicePrincipal index, or nil if none exists. Caller must
// already hold b.mu.
func (b *InMemoryBackend) thirdPartyRecorderFor(servicePrincipal string) *ConfigurationRecorder {
	for _, r := range b.recordersByServicePrincipal.Get(servicePrincipal) {
		if r.ConnectorArn != "" {
			return r
		}
	}

	return nil
}

// PutThirdPartyServiceLinkedConfigurationRecorder creates or updates the
// service-linked configuration recorder that links a third-party cloud
// service provider (via connectorArn) to servicePrincipal. Verified against
// aws-sdk-go-v2/service/configservice's
// PutThirdPartyServiceLinkedConfigurationRecorder doc comment and
// deserializer:
//
//   - ServicePrincipal, ConnectorArn, and ScopeConfiguration (with ScopeType
//     set) are all required -- ValidationException otherwise.
//   - connectorArn must reference a connector already known to this backend
//     ("The specified connector must exist"). The op's declared error model
//     has no ResourceNotFoundException (only ConflictException/
//     InsufficientPermissionsException/ValidationException), so an unknown
//     connector errors ValidationException, not ErrResourceNotFound.
//   - If a service-linked recorder already exists for servicePrincipal with
//     the SAME connectorArn, the call is idempotent: only ScopeConfiguration
//     is updated ("calling this operation again updates the
//     ScopeConfiguration").
//   - If a service-linked recorder already exists for servicePrincipal with
//     a DIFFERENT connectorArn, this errors ConflictException ("the
//     specified service principal does not support multiple configuration
//     recorders and one already exists") -- one recorder per service
//     principal, the real API's documented constraint for this op (unlike
//     the still-unenforced single-customer-managed-recorder limit elsewhere
//     in this file).
//
// The created recorder reuses serviceLinkedRecorderName's
// "AWSConfigurationRecorderFor<Service>" convention, since real AWS Config
// documents that name prefix for service-linked recorders generally (not
// just the AWS-native ones PutServiceLinkedConfigurationRecorder creates).
func (b *InMemoryBackend) PutThirdPartyServiceLinkedConfigurationRecorder(
	servicePrincipal, connectorArn string, scope *ScopeConfiguration, tags []Tag,
) (string, string, error) {
	if servicePrincipal == "" {
		return "", "", fmt.Errorf("%w: ServicePrincipal is required", ErrValidation)
	}

	if connectorArn == "" {
		return "", "", fmt.Errorf("%w: ConnectorArn is required", ErrValidation)
	}

	if scope == nil || scope.ScopeType == "" {
		return "", "", fmt.Errorf("%w: ScopeConfiguration.ScopeType is required", ErrValidation)
	}

	b.mu.Lock("PutThirdPartyServiceLinkedConfigurationRecorder")
	defer b.mu.Unlock()

	if !b.connectors.Has(connectorArn) {
		return "", "", fmt.Errorf("%w: connector %s does not exist", ErrValidation, connectorArn)
	}

	scopeCopy := *scope

	if existing := b.thirdPartyRecorderFor(servicePrincipal); existing != nil {
		if existing.ConnectorArn != connectorArn {
			return "", "", fmt.Errorf(
				"%w: service principal %s does not support multiple configuration recorders and one already exists",
				ErrConflict, servicePrincipal,
			)
		}

		existing.ScopeConfiguration = &scopeCopy

		return existing.Name, b.recorderArn(existing.Name), nil
	}

	name := serviceLinkedRecorderName(servicePrincipal)
	arn := b.recorderArn(name)

	b.recorders.Put(&ConfigurationRecorder{
		Name:               name,
		Status:             recorderStatusActive,
		ConnectorArn:       connectorArn,
		ServicePrincipal:   servicePrincipal,
		ScopeConfiguration: &scopeCopy,
	})
	b.setResourceTagsLocked(arn, tags)

	return name, arn, nil
}
