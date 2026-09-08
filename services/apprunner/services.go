package apprunner

import (
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// CreateService creates a new App Runner service.
func (b *InMemoryBackend) CreateService(params CreateServiceParams) (*Service, error) {
	b.mu.Lock("CreateService")
	defer b.mu.Unlock()

	if existing := b.byName.Get(params.Name); len(existing) > 0 {
		return nil, fmt.Errorf("service %s already exists: %w", params.Name, ErrAlreadyExists)
	}

	if err := b.validateCreateService(params); err != nil {
		return nil, err
	}

	asgCfg, err := b.resolveOrDefaultASG(params.AutoScalingConfigurationArn)
	if err != nil {
		return nil, err
	}

	id := newID()
	svcArn := b.serviceARN(id)
	now := time.Now().UTC()

	instance := params.Instance
	if instance.CPU == "" {
		instance.CPU = defaultCPU
	}

	if instance.Memory == "" {
		instance.Memory = defaultMemory
	}

	svcTags := make(map[string]string)
	maps.Copy(svcTags, params.Tags)

	svc := &storedService{
		ServiceArn:                  svcArn,
		ServiceID:                   id,
		ServiceName:                 params.Name,
		ServiceURL:                  buildServiceURL(id, b.region),
		Status:                      statusRunning,
		Instance:                    instance,
		Source:                      normalizeSource(params.Source),
		AutoScalingConfigurationArn: asgCfg.AutoScalingConfigurationArn,
		Network:                     normalizeNetwork(params.Network),
		HealthCheck:                 normalizeHealthCheck(params.HealthCheck),
		EncryptionKmsKey:            params.EncryptionKmsKey,
		Observability:               normalizeObservability(params.Observability),
		CreatedAt:                   now,
		UpdatedAt:                   now,
		Tags:                        svcTags,
	}
	b.addOperation(svc, opTypeCreate)
	b.services.Put(svc)
	asgCfg.HasAssociatedService = true

	if len(svcTags) > 0 {
		b.tags[svcArn] = make(map[string]string)
		maps.Copy(b.tags[svcArn], svcTags)
	}

	cp := svc.toService()

	return &cp, nil
}

// validateCreateService checks the parts of CreateServiceParams that need
// cross-resource validation against other backend state (VPC connectors,
// observability configurations, connections) or business rules (exactly one
// of ImageRepository/CodeRepository). Called with the lock already held.
func (b *InMemoryBackend) validateCreateService(params CreateServiceParams) error {
	if err := validateSourceConfig(params.Source, true); err != nil {
		return err
	}

	if err := b.validateSourceAuth(params.Source); err != nil {
		return err
	}

	if err := b.validateNetworkConfig(params.Network); err != nil {
		return err
	}

	return b.validateObservability(params.Observability)
}

// validateSourceConfig checks that SourceConfig specifies exactly one of
// ImageRepository/CodeRepository (required is true for CreateService, which
// mandates one; false for UpdateService, which allows omitting Source
// entirely -- callers only invoke this when a Source patch was supplied).
func validateSourceConfig(s SourceConfig, required bool) error {
	hasImage := s.ImageRepository != nil
	hasCode := s.CodeRepository != nil

	if !hasImage && !hasCode {
		if required {
			return fmt.Errorf(
				"%w: SourceConfiguration must specify ImageRepository or CodeRepository", ErrInvalidParameter,
			)
		}

		return nil
	}

	if hasImage && hasCode {
		return fmt.Errorf(
			"%w: SourceConfiguration must specify only one of ImageRepository or CodeRepository", ErrInvalidParameter,
		)
	}

	if hasImage && s.ImageRepository.ImageIdentifier == "" {
		return fmt.Errorf("%w: ImageRepository.ImageIdentifier is required", ErrInvalidParameter)
	}

	if hasCode && s.CodeRepository.RepositoryURL == "" {
		return fmt.Errorf("%w: CodeRepository.RepositoryUrl is required", ErrInvalidParameter)
	}

	// CodeRepository.SourceCodeVersion is required (types.go, apprunner@v1.42.4:
	// both Type and Value on SourceCodeVersion are themselves "This member is
	// required."). Unvalidated, an omitted SourceCodeVersion silently dropped
	// the required output field entirely on Describe/CreateService instead of
	// rejecting the request the way real AWS does.
	if hasCode && s.CodeRepository.SourceCodeVersionType == "" {
		return fmt.Errorf("%w: CodeRepository.SourceCodeVersion is required", ErrInvalidParameter)
	}

	return nil
}

// DescribeService returns full service details.
func (b *InMemoryBackend) DescribeService(serviceArn string) (*Service, error) {
	b.mu.RLock("DescribeService")
	defer b.mu.RUnlock()

	svc, ok := b.services.Get(serviceArn)
	if !ok {
		return nil, fmt.Errorf("service %s not found: %w", serviceArn, ErrNotFound)
	}

	cp := svc.toService()

	return &cp, nil
}

// UpdateService updates a service's configuration.
func (b *InMemoryBackend) UpdateService(params UpdateServiceParams) (*Service, error) {
	b.mu.Lock("UpdateService")
	defer b.mu.Unlock()

	svc, ok := b.services.Get(params.ServiceArn)
	if !ok {
		return nil, fmt.Errorf("service %s not found: %w", params.ServiceArn, ErrNotFound)
	}

	if svc.Status != statusRunning {
		return nil, fmt.Errorf(
			"service %s cannot be updated in status %s: %w",
			params.ServiceArn, svc.Status, ErrInvalidState,
		)
	}

	if err := b.applyServiceUpdate(svc, params); err != nil {
		return nil, err
	}

	svc.UpdatedAt = time.Now().UTC()
	b.addOperation(svc, opTypeUpdate)

	cp := svc.toService()

	return &cp, nil
}

// applyServiceUpdate mutates svc in place from params, validating
// cross-resource references before any mutation is applied (so a rejected
// update never leaves svc partially changed). Called with the lock held.
func (b *InMemoryBackend) applyServiceUpdate(svc *storedService, params UpdateServiceParams) error {
	if params.Source != nil {
		if err := validateSourceConfig(*params.Source, false); err != nil {
			return err
		}

		if err := b.validateSourceAuth(*params.Source); err != nil {
			return err
		}
	}

	var newASG *storedAutoScalingConfiguration

	if params.AutoScalingConfigurationArn != "" {
		cfg, ok := b.resolveASG(params.AutoScalingConfigurationArn)
		if !ok {
			return fmt.Errorf(
				"auto scaling configuration %s not found: %w", params.AutoScalingConfigurationArn, ErrInvalidParameter,
			)
		}

		newASG = cfg
	}

	if err := b.validateNetworkConfig(params.Network); err != nil {
		return err
	}

	if err := b.validateObservability(params.Observability); err != nil {
		return err
	}

	if params.Instance != nil {
		applyInstanceUpdate(&svc.Instance, params.Instance)
	}

	if params.Source != nil {
		if err := applySourceUpdate(&svc.Source, *params.Source); err != nil {
			return err
		}
	}

	if newASG != nil {
		oldArn := svc.AutoScalingConfigurationArn
		svc.AutoScalingConfigurationArn = newASG.AutoScalingConfigurationArn
		newASG.HasAssociatedService = true
		b.recomputeASGAssociation(oldArn)
	}

	if params.Network != nil {
		svc.Network = mergeNetwork(svc.Network, params.Network)
	}

	if params.HealthCheck != nil {
		svc.HealthCheck = mergeHealthCheck(svc.HealthCheck, params.HealthCheck)
	}

	if params.Observability != nil {
		svc.Observability = *params.Observability
	}

	return nil
}

// applyInstanceUpdate copies non-empty fields from patch onto existing,
// matching UpdateService's "only replace what's provided" semantics.
func applyInstanceUpdate(existing *InstanceConfig, patch *InstanceConfig) {
	if patch.CPU != "" {
		existing.CPU = patch.CPU
	}

	if patch.Memory != "" {
		existing.Memory = patch.Memory
	}

	if patch.InstanceRoleArn != "" {
		existing.InstanceRoleArn = patch.InstanceRoleArn
	}
}

// applySourceUpdate copies patch onto existing. Real App Runner forbids
// switching a service between code and image sources ("you must provide the
// same structure member... that you originally included when you created the
// service"), so a patch supplying the other kind is rejected.
func applySourceUpdate(existing *SourceConfig, patch SourceConfig) error {
	switch {
	case patch.ImageRepository != nil:
		if existing.ImageRepository == nil {
			return fmt.Errorf(
				"%w: cannot change a code-repository service to an image repository", ErrInvalidParameter,
			)
		}

		img := *patch.ImageRepository
		existing.ImageRepository = &img
	case patch.CodeRepository != nil:
		if existing.CodeRepository == nil {
			return fmt.Errorf(
				"%w: cannot change an image-repository service to a code repository", ErrInvalidParameter,
			)
		}

		cr := *patch.CodeRepository
		existing.CodeRepository = &cr
	}

	if patch.AccessRoleArn != "" {
		existing.AccessRoleArn = patch.AccessRoleArn
	}

	if patch.ConnectionArn != "" {
		existing.ConnectionArn = patch.ConnectionArn
	}

	if patch.AutoDeploymentsEnabled != nil {
		existing.AutoDeploymentsEnabled = patch.AutoDeploymentsEnabled
	}

	return nil
}

// mergeNetwork applies only the fields patch specifies onto existing.
func mergeNetwork(existing NetworkConfig, patch *NetworkConfig) NetworkConfig {
	out := existing
	if patch.EgressType != "" {
		out.EgressType = patch.EgressType
	}

	if patch.EgressVpcConnectorArn != "" {
		out.EgressVpcConnectorArn = patch.EgressVpcConnectorArn
	}

	if patch.IPAddressType != "" {
		out.IPAddressType = patch.IPAddressType
	}

	if patch.IsPubliclyAccessible != nil {
		out.IsPubliclyAccessible = patch.IsPubliclyAccessible
	}

	return out
}

// mergeHealthCheck applies only the fields patch specifies onto existing.
func mergeHealthCheck(existing HealthCheckConfig, patch *HealthCheckConfig) HealthCheckConfig {
	out := existing
	if patch.Protocol != "" {
		out.Protocol = patch.Protocol
	}

	if patch.Path != "" {
		out.Path = patch.Path
	}

	if patch.Interval != 0 {
		out.Interval = patch.Interval
	}

	if patch.Timeout != 0 {
		out.Timeout = patch.Timeout
	}

	if patch.HealthyThreshold != 0 {
		out.HealthyThreshold = patch.HealthyThreshold
	}

	if patch.UnhealthyThreshold != 0 {
		out.UnhealthyThreshold = patch.UnhealthyThreshold
	}

	return out
}

// normalizeSource fills SourceConfig.AutoDeploymentsEnabled with App
// Runner's documented default when the caller didn't specify it: false for
// an ECR Public image source, true otherwise.
func normalizeSource(s SourceConfig) SourceConfig {
	out := s
	if out.AutoDeploymentsEnabled != nil {
		return out
	}

	autoDeploy := out.ImageRepository == nil || out.ImageRepository.ImageRepositoryType != imageRepositoryTypeECRPublic

	out.AutoDeploymentsEnabled = &autoDeploy

	return out
}

// normalizeNetwork fills every unset NetworkConfig field with App Runner's
// documented default (DEFAULT egress, publicly accessible, IPv4).
func normalizeNetwork(n *NetworkConfig) NetworkConfig {
	out := NetworkConfig{
		EgressType:    egressTypeDefault,
		IPAddressType: ipAddressTypeIPv4,
	}
	isPublic := true

	if n != nil {
		if n.EgressType != "" {
			out.EgressType = n.EgressType
		}

		out.EgressVpcConnectorArn = n.EgressVpcConnectorArn

		if n.IPAddressType != "" {
			out.IPAddressType = n.IPAddressType
		}

		if n.IsPubliclyAccessible != nil {
			isPublic = *n.IsPubliclyAccessible
		}
	}

	out.IsPubliclyAccessible = &isPublic

	return out
}

// normalizeHealthCheck fills every unset HealthCheckConfig field with App
// Runner's documented defaults.
func normalizeHealthCheck(h *HealthCheckConfig) HealthCheckConfig {
	out := HealthCheckConfig{
		Protocol:           healthCheckProtocolTCP,
		Path:               defaultHealthCheckPath,
		Interval:           defaultHealthCheckInterval,
		Timeout:            defaultHealthCheckTimeout,
		HealthyThreshold:   defaultHealthyThreshold,
		UnhealthyThreshold: defaultUnhealthyThreshold,
	}

	if h == nil {
		return out
	}

	if h.Protocol != "" {
		out.Protocol = h.Protocol
	}

	if h.Path != "" {
		out.Path = h.Path
	}

	if h.Interval != 0 {
		out.Interval = h.Interval
	}

	if h.Timeout != 0 {
		out.Timeout = h.Timeout
	}

	if h.HealthyThreshold != 0 {
		out.HealthyThreshold = h.HealthyThreshold
	}

	if h.UnhealthyThreshold != 0 {
		out.UnhealthyThreshold = h.UnhealthyThreshold
	}

	return out
}

// normalizeObservability returns the zero value (disabled) when o is nil.
func normalizeObservability(o *ServiceObservability) ServiceObservability {
	if o == nil {
		return ServiceObservability{}
	}

	return *o
}

// DeleteService marks a service as deleted and removes it from active lookup.
func (b *InMemoryBackend) DeleteService(serviceArn string) (*Service, error) {
	b.mu.Lock("DeleteService")
	defer b.mu.Unlock()

	svc, ok := b.services.Get(serviceArn)
	if !ok {
		return nil, fmt.Errorf("service %s not found: %w", serviceArn, ErrNotFound)
	}

	// DeleteService doc (api_op_DeleteService.go): "Make sure that you don't
	// have any active VPCIngressConnections associated with the service you
	// want to delete." DeleteService's error set models InvalidStateException
	// (deserializers.go), the same mechanism UpdateService/PauseService/
	// ResumeService use to enforce their own state preconditions.
	if b.hasActiveVpcIngressConnections(serviceArn) {
		return nil, fmt.Errorf(
			"service %s has active VPC ingress connections: %w", serviceArn, ErrInvalidState,
		)
	}

	now := time.Now().UTC()
	svc.Status = statusDeleted
	svc.UpdatedAt = now
	svc.DeletedAt = now
	b.addOperation(svc, opTypeDelete)

	cp := svc.toService()

	b.services.Delete(serviceArn)
	delete(b.tags, serviceArn)
	// Cascade-clean the service's custom domain associations so no ghost
	// row lingers in b.customDomains keyed by an ARN no service can ever
	// reference again (DescribeCustomDomains itself 404s on a deleted
	// ServiceArn, so an orphaned entry here would be unreachable dead state).
	delete(b.customDomains, serviceArn)
	b.recomputeASGAssociation(svc.AutoScalingConfigurationArn)

	return &cp, nil
}

// ListServices returns services sorted by ARN with pagination.
func (b *InMemoryBackend) ListServices(maxResults int32, nextToken string) ([]*ServiceSummary, string, error) {
	b.mu.RLock("ListServices")
	defer b.mu.RUnlock()

	items := b.services.Snapshot()

	all := make([]*ServiceSummary, 0, len(items))
	for _, svc := range items {
		s := svc.toSummary()
		all = append(all, &s)
	}

	limit := int(maxResults)
	pg := page.New(all, nextToken, limit, defaultMaxResults)

	return pg.Data, pg.Next, nil
}

// PauseService pauses a running service.
func (b *InMemoryBackend) PauseService(serviceArn string) (*Service, error) {
	b.mu.Lock("PauseService")
	defer b.mu.Unlock()

	svc, ok := b.services.Get(serviceArn)
	if !ok {
		return nil, fmt.Errorf("service %s not found: %w", serviceArn, ErrNotFound)
	}

	if svc.Status != statusRunning {
		return nil, fmt.Errorf(
			"service %s cannot be paused in status %s: %w",
			serviceArn, svc.Status, ErrInvalidState,
		)
	}

	svc.Status = statusPaused
	svc.UpdatedAt = time.Now().UTC()
	b.addOperation(svc, opTypePause)

	cp := svc.toService()

	return &cp, nil
}

// ResumeService resumes a paused service.
func (b *InMemoryBackend) ResumeService(serviceArn string) (*Service, error) {
	b.mu.Lock("ResumeService")
	defer b.mu.Unlock()

	svc, ok := b.services.Get(serviceArn)
	if !ok {
		return nil, fmt.Errorf("service %s not found: %w", serviceArn, ErrNotFound)
	}

	if svc.Status != statusPaused {
		return nil, fmt.Errorf(
			"service %s cannot be resumed in status %s: %w",
			serviceArn, svc.Status, ErrInvalidState,
		)
	}

	svc.Status = statusRunning
	svc.UpdatedAt = time.Now().UTC()
	b.addOperation(svc, opTypeResume)

	cp := svc.toService()

	return &cp, nil
}

// StartDeployment triggers a deployment for a service.
func (b *InMemoryBackend) StartDeployment(serviceArn string) (string, error) {
	b.mu.Lock("StartDeployment")
	defer b.mu.Unlock()

	svc, ok := b.services.Get(serviceArn)
	if !ok {
		return "", fmt.Errorf("service %s not found: %w", serviceArn, ErrNotFound)
	}

	if svc.Status != statusRunning {
		// Unlike UpdateService/PauseService/ResumeService, StartDeployment's
		// documented error set has no InvalidStateException (only
		// InternalServiceErrorException, InvalidRequestException, and
		// ResourceNotFoundException -- confirmed against
		// deserializeOpErrorStartDeployment in the vendored SDK), so a
		// non-running service is reported via ErrInvalidParameter here
		// rather than the usual ErrInvalidState.
		return "", fmt.Errorf(
			"service %s cannot start deployment in status %s: %w",
			serviceArn, svc.Status, ErrInvalidParameter,
		)
	}

	b.addOperation(svc, opTypeDeploy)
	opID := svc.Operations[len(svc.Operations)-1].ID

	return opID, nil
}
