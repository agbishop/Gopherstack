package ecs

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// ----- Service handlers -----

// placementStrategyInput/placementStrategyView and toPlacementStrategies are
// service-exclusive (only CreateService/UpdateService consume them; unlike
// placementConstraint*, RegisterTaskDefinition never does), so unlike
// placementConstraintInput they live here rather than in
// handler_task_definitions.go.
type placementStrategyInput struct {
	Type  string `json:"type,omitempty"`
	Field string `json:"field,omitempty"`
}

type deploymentCircuitBreakerInput struct {
	Enable   bool `json:"enable"`
	Rollback bool `json:"rollback"`
}

type deploymentConfigurationInput struct {
	DeploymentCircuitBreaker *deploymentCircuitBreakerInput `json:"deploymentCircuitBreaker,omitempty"`
	MinimumHealthyPercent    *int                           `json:"minimumHealthyPercent,omitempty"`
	MaximumPercent           *int                           `json:"maximumPercent,omitempty"`
}

type deploymentControllerInput struct {
	Type string `json:"type"`
}

type awsvpcConfigurationInput struct {
	AssignPublicIP string   `json:"assignPublicIp,omitempty"`
	Subnets        []string `json:"subnets"`
	SecurityGroups []string `json:"securityGroups,omitempty"`
}

type networkConfigurationInput struct {
	AwsvpcConfiguration *awsvpcConfigurationInput `json:"awsvpcConfiguration,omitempty"`
}

type serviceConnectClientAliasInput struct {
	DNSName string `json:"dnsName,omitempty"`
	Port    int    `json:"port"`
}

type serviceConnectServiceInput struct {
	PortName      string                           `json:"portName"`
	DiscoveryName string                           `json:"discoveryName,omitempty"`
	ClientAliases []serviceConnectClientAliasInput `json:"clientAliases,omitempty"`
}

type serviceConnectConfigurationInput struct {
	Namespace string                       `json:"namespace,omitempty"`
	Services  []serviceConnectServiceInput `json:"services,omitempty"`
	Enabled   bool                         `json:"enabled"`
}

type loadBalancerInput struct {
	TargetGroupArn   string `json:"targetGroupArn,omitempty"`
	LoadBalancerName string `json:"loadBalancerName,omitempty"`
	ContainerName    string `json:"containerName,omitempty"`
	ContainerPort    int    `json:"containerPort,omitempty"`
}

type serviceRegistryInput struct {
	RegistryArn   string `json:"registryArn,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
	Port          int    `json:"port,omitempty"`
	ContainerPort int    `json:"containerPort,omitempty"`
}

type metricConfigurationInput struct {
	MetricNames       []string `json:"metricNames"`
	ResolutionSeconds int      `json:"resolutionSeconds"`
}

type monitoringConfigurationInput struct {
	MetricConfigurations []metricConfigurationInput `json:"metricConfigurations,omitempty"`
}

func toMonitoringConfiguration(in *monitoringConfigurationInput) *MonitoringConfiguration {
	if in == nil {
		return nil
	}

	out := &MonitoringConfiguration{
		MetricConfigurations: make([]MetricConfiguration, 0, len(in.MetricConfigurations)),
	}

	for _, mc := range in.MetricConfigurations {
		out.MetricConfigurations = append(out.MetricConfigurations, MetricConfiguration(mc))
	}

	return out
}

type createServiceInput struct {
	DeploymentConfiguration       *deploymentConfigurationInput     `json:"deploymentConfiguration,omitempty"`
	DeploymentController          *deploymentControllerInput        `json:"deploymentController,omitempty"`
	NetworkConfiguration          *networkConfigurationInput        `json:"networkConfiguration,omitempty"`
	ServiceConnectConfiguration   *serviceConnectConfigurationInput `json:"serviceConnectConfiguration,omitempty"`
	HealthCheckGracePeriodSeconds *int                              `json:"healthCheckGracePeriodSeconds,omitempty"`
	Monitoring                    *monitoringConfigurationInput     `json:"monitoring,omitempty"`
	ServiceName                   string                            `json:"serviceName"`
	Cluster                       string                            `json:"cluster,omitempty"`
	TaskDefinition                string                            `json:"taskDefinition"`
	LaunchType                    string                            `json:"launchType,omitempty"`
	SchedulingStrategy            string                            `json:"schedulingStrategy,omitempty"`
	PropagateTags                 string                            `json:"propagateTags,omitempty"`
	AvailabilityZoneRebalancing   string                            `json:"availabilityZoneRebalancing,omitempty"`
	Tags                          []Tag                             `json:"tags,omitempty"`
	LoadBalancers                 []loadBalancerInput               `json:"loadBalancers,omitempty"`
	ServiceRegistries             []serviceRegistryInput            `json:"serviceRegistries,omitempty"`
	CapacityProviderStrategy      []cpStrategyItemInput             `json:"capacityProviderStrategy,omitempty"`
	PlacementConstraints          []placementConstraintInput        `json:"placementConstraints,omitempty"`
	PlacementStrategy             []placementStrategyInput          `json:"placementStrategy,omitempty"`
	DesiredCount                  int                               `json:"desiredCount"`
	EnableExecuteCommand          bool                              `json:"enableExecuteCommand,omitempty"`
}

type createServiceOutput struct {
	Service serviceView `json:"service"`
}

func (h *Handler) handleCreateService(
	_ context.Context,
	in *createServiceInput,
) (*createServiceOutput, error) {
	svc, err := h.Backend.CreateService(CreateServiceInput{
		ServiceName:                   in.ServiceName,
		Cluster:                       in.Cluster,
		TaskDefinition:                in.TaskDefinition,
		LaunchType:                    in.LaunchType,
		SchedulingStrategy:            in.SchedulingStrategy,
		PropagateTags:                 in.PropagateTags,
		AvailabilityZoneRebalancing:   in.AvailabilityZoneRebalancing,
		Tags:                          in.Tags,
		LoadBalancers:                 toLoadBalancers(in.LoadBalancers),
		ServiceRegistries:             toServiceRegistries(in.ServiceRegistries),
		DeploymentConfiguration:       toDeploymentConfiguration(in.DeploymentConfiguration),
		DeploymentController:          toDeploymentController(in.DeploymentController),
		NetworkConfiguration:          toNetworkConfiguration(in.NetworkConfiguration),
		CapacityProviderStrategy:      toCPStrategyItems(in.CapacityProviderStrategy),
		PlacementConstraints:          toPlacementConstraints(in.PlacementConstraints),
		PlacementStrategy:             toPlacementStrategies(in.PlacementStrategy),
		ServiceConnectConfiguration:   toServiceConnectConfiguration(in.ServiceConnectConfiguration),
		HealthCheckGracePeriodSeconds: in.HealthCheckGracePeriodSeconds,
		Monitoring:                    toMonitoringConfiguration(in.Monitoring),
		DesiredCount:                  in.DesiredCount,
		EnableExecuteCommand:          in.EnableExecuteCommand,
	})
	if err != nil {
		return nil, err
	}

	view := toServiceView(*svc)
	// CreateService has no `include` gating (unlike DescribeServices): the
	// backend already returned the resourceTags-synced tags (see CreateService
	// in services.go), so echo them straight back.
	view.Tags = svc.Tags

	return &createServiceOutput{Service: view}, nil
}

// describeServiceIncludeTags is the AWS-defined `include` value that requests
// resource tags be returned alongside each Service (mirrors
// describeClusterIncludeTags for DescribeClusters and
// describeExpressGatewayIncludeTags for DescribeExpressGatewayService).
const describeServiceIncludeTags = "TAGS"

type describeServicesInput struct {
	Cluster  string   `json:"cluster,omitempty"`
	Services []string `json:"services"`
	Include  []string `json:"include,omitempty"`
}

type describeServicesOutput struct {
	Services []serviceView `json:"services"`
	Failures []failureView `json:"failures"`
}

func (h *Handler) handleDescribeServices(
	_ context.Context,
	in *describeServicesInput,
) (*describeServicesOutput, error) {
	svcs, failures, err := h.Backend.DescribeServices(in.Cluster, in.Services)
	if err != nil {
		return nil, err
	}

	wantTags := wantsIncludeTag(in.Include, describeServiceIncludeTags)

	views, err := attachTagsIfWanted(
		h, svcs,
		func(s Service) string { return s.ServiceArn },
		toServiceView,
		func(v *serviceView, tags []Tag) { v.Tags = tags },
		wantTags,
	)
	if err != nil {
		return nil, err
	}

	failViews := make([]failureView, 0, len(failures))
	for _, f := range failures {
		failViews = append(failViews, failureView(f))
	}

	return &describeServicesOutput{Services: views, Failures: failViews}, nil
}

type updateServiceInput struct {
	EnableExecuteCommand          *bool                             `json:"enableExecuteCommand,omitempty"`
	DesiredCount                  *int                              `json:"desiredCount,omitempty"`
	HealthCheckGracePeriodSeconds *int                              `json:"healthCheckGracePeriodSeconds,omitempty"`
	DeploymentConfiguration       *deploymentConfigurationInput     `json:"deploymentConfiguration,omitempty"`
	NetworkConfiguration          *networkConfigurationInput        `json:"networkConfiguration,omitempty"`
	ServiceConnectConfiguration   *serviceConnectConfigurationInput `json:"serviceConnectConfiguration,omitempty"`
	Monitoring                    *monitoringConfigurationInput     `json:"monitoring,omitempty"`
	Cluster                       string                            `json:"cluster,omitempty"`
	Service                       string                            `json:"service"`
	TaskDefinition                string                            `json:"taskDefinition,omitempty"`
	PropagateTags                 string                            `json:"propagateTags,omitempty"`
	AvailabilityZoneRebalancing   string                            `json:"availabilityZoneRebalancing,omitempty"`
	LoadBalancers                 []loadBalancerInput               `json:"loadBalancers,omitempty"`
	CapacityProviderStrategy      []cpStrategyItemInput             `json:"capacityProviderStrategy,omitempty"`
	PlacementConstraints          []placementConstraintInput        `json:"placementConstraints,omitempty"`
	PlacementStrategy             []placementStrategyInput          `json:"placementStrategy,omitempty"`
	ForceNewDeployment            bool                              `json:"forceNewDeployment,omitempty"`
}

type updateServiceOutput struct {
	Service serviceView `json:"service"`
}

func (h *Handler) handleUpdateService(
	_ context.Context,
	in *updateServiceInput,
) (*updateServiceOutput, error) {
	svc, err := h.Backend.UpdateService(UpdateServiceInput{
		Cluster:                       in.Cluster,
		Service:                       in.Service,
		PropagateTags:                 in.PropagateTags,
		AvailabilityZoneRebalancing:   in.AvailabilityZoneRebalancing,
		LoadBalancers:                 toLoadBalancers(in.LoadBalancers),
		NetworkConfiguration:          toNetworkConfiguration(in.NetworkConfiguration),
		TaskDefinition:                in.TaskDefinition,
		DesiredCount:                  in.DesiredCount,
		HealthCheckGracePeriodSeconds: in.HealthCheckGracePeriodSeconds,
		DeploymentConfiguration:       toDeploymentConfiguration(in.DeploymentConfiguration),
		CapacityProviderStrategy:      toCPStrategyItems(in.CapacityProviderStrategy),
		PlacementConstraints:          toPlacementConstraints(in.PlacementConstraints),
		PlacementStrategy:             toPlacementStrategies(in.PlacementStrategy),
		ServiceConnectConfiguration:   toServiceConnectConfiguration(in.ServiceConnectConfiguration),
		Monitoring:                    toMonitoringConfiguration(in.Monitoring),
		EnableExecuteCommand:          in.EnableExecuteCommand,
		ForceNewDeployment:            in.ForceNewDeployment,
	})
	if err != nil {
		return nil, err
	}

	view := toServiceView(*svc)
	// UpdateService has no `include` gating either; the backend already
	// returned the resourceTags-synced tags -- see UpdateService in services.go.
	view.Tags = svc.Tags

	return &updateServiceOutput{Service: view}, nil
}

type deleteServiceInput struct {
	Cluster string `json:"cluster,omitempty"`
	Service string `json:"service"`
	Force   bool   `json:"force,omitempty"`
}

type deleteServiceOutput struct {
	Service serviceView `json:"service"`
}

func (h *Handler) handleDeleteService(
	_ context.Context,
	in *deleteServiceInput,
) (*deleteServiceOutput, error) {
	svc, err := h.Backend.DeleteService(in.Cluster, in.Service, in.Force)
	if err != nil {
		return nil, err
	}

	view := toServiceView(*svc)
	// DeleteService has no `include` gating either; the backend already
	// captured the resourceTags-synced tags before deletion -- see DeleteService
	// in services.go.
	view.Tags = svc.Tags

	return &deleteServiceOutput{Service: view}, nil
}

type listServicesInput struct {
	Cluster            string `json:"cluster,omitempty"`
	LaunchType         string `json:"launchType,omitempty"`
	SchedulingStrategy string `json:"schedulingStrategy,omitempty"`
	NextToken          string `json:"nextToken,omitempty"`
	MaxResults         int    `json:"maxResults,omitempty"`
}

type listServicesOutput struct {
	NextToken   string   `json:"nextToken,omitempty"`
	ServiceArns []string `json:"serviceArns"`
}

func (h *Handler) handleListServices(
	_ context.Context,
	in *listServicesInput,
) (*listServicesOutput, error) {
	arns, err := h.Backend.ListServices(in.Cluster, in.LaunchType, in.SchedulingStrategy)
	if err != nil {
		return nil, err
	}

	if arns == nil {
		arns = []string{}
	}

	arns, nextToken := applyNextTokenSlice(arns, in.NextToken, in.MaxResults)

	return &listServicesOutput{ServiceArns: arns, NextToken: nextToken}, nil
}

// ----- ListServicesByNamespace -----

type listServicesByNamespaceInput struct {
	Namespace  string `json:"namespace,omitempty"`
	Cluster    string `json:"cluster,omitempty"`
	NextToken  string `json:"nextToken,omitempty"`
	MaxResults int    `json:"maxResults,omitempty"`
}

type listServicesByNamespaceOutput struct {
	NextToken   string   `json:"nextToken,omitempty"`
	ServiceArns []string `json:"serviceArns"`
}

func (h *Handler) handleListServicesByNamespace(
	_ context.Context,
	in *listServicesByNamespaceInput,
) (*listServicesByNamespaceOutput, error) {
	arns, err := h.Backend.ListServicesByNamespace(in.Cluster, in.Namespace)
	if err != nil {
		return nil, err
	}

	p := page.New(arns, in.NextToken, in.MaxResults, defaultECSMaxResults)

	return &listServicesByNamespaceOutput{ServiceArns: p.Data, NextToken: p.Next}, nil
}

// ----- View types (JSON serialization) -----

type placementStrategyView struct {
	Type  string `json:"type,omitempty"`
	Field string `json:"field,omitempty"`
}

type deploymentCircuitBreakerView struct {
	Enable   bool `json:"enable"`
	Rollback bool `json:"rollback"`
}

type deploymentConfigurationView struct {
	DeploymentCircuitBreaker *deploymentCircuitBreakerView `json:"deploymentCircuitBreaker,omitempty"`
	MinimumHealthyPercent    *int                          `json:"minimumHealthyPercent,omitempty"`
	MaximumPercent           *int                          `json:"maximumPercent,omitempty"`
}

type deploymentControllerView struct {
	Type string `json:"type"`
}

type awsvpcConfigurationView struct {
	AssignPublicIP string   `json:"assignPublicIp,omitempty"`
	Subnets        []string `json:"subnets"`
	SecurityGroups []string `json:"securityGroups,omitempty"`
}

type networkConfigurationView struct {
	AwsvpcConfiguration *awsvpcConfigurationView `json:"awsvpcConfiguration,omitempty"`
}

type loadBalancerView struct {
	TargetGroupArn   string `json:"targetGroupArn,omitempty"`
	LoadBalancerName string `json:"loadBalancerName,omitempty"`
	ContainerName    string `json:"containerName,omitempty"`
	ContainerPort    int    `json:"containerPort,omitempty"`
}

type serviceRegistryView struct {
	RegistryArn   string `json:"registryArn,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
	Port          int    `json:"port,omitempty"`
	ContainerPort int    `json:"containerPort,omitempty"`
}

func toDeploymentConfigurationView(dc *DeploymentConfiguration) *deploymentConfigurationView {
	if dc == nil {
		return nil
	}

	v := &deploymentConfigurationView{
		MinimumHealthyPercent: dc.MinimumHealthyPercent,
		MaximumPercent:        dc.MaximumPercent,
	}

	if dc.DeploymentCircuitBreaker != nil {
		v.DeploymentCircuitBreaker = &deploymentCircuitBreakerView{
			Enable:   dc.DeploymentCircuitBreaker.Enable,
			Rollback: dc.DeploymentCircuitBreaker.Rollback,
		}
	}

	return v
}

type serviceConnectClientAliasView struct {
	DNSName string `json:"dnsName,omitempty"`
	Port    int    `json:"port"`
}

type serviceConnectServiceView struct {
	PortName      string                          `json:"portName"`
	DiscoveryName string                          `json:"discoveryName,omitempty"`
	ClientAliases []serviceConnectClientAliasView `json:"clientAliases,omitempty"`
}

type serviceConnectConfigurationView struct {
	Namespace string                      `json:"namespace,omitempty"`
	Services  []serviceConnectServiceView `json:"services,omitempty"`
	Enabled   bool                        `json:"enabled"`
}

type serviceView struct {
	ServiceConnectConfiguration   *serviceConnectConfigurationView `json:"serviceConnectConfiguration,omitempty"`
	DeploymentConfiguration       *deploymentConfigurationView     `json:"deploymentConfiguration,omitempty"`
	DeploymentController          *deploymentControllerView        `json:"deploymentController,omitempty"`
	NetworkConfiguration          *networkConfigurationView        `json:"networkConfiguration,omitempty"`
	HealthCheckGracePeriodSeconds *int                             `json:"healthCheckGracePeriodSeconds,omitempty"`
	ClusterArn                    string                           `json:"clusterArn"`
	TaskDefinition                string                           `json:"taskDefinition"`
	Status                        string                           `json:"status"`
	LaunchType                    string                           `json:"launchType,omitempty"`
	SchedulingStrategy            string                           `json:"schedulingStrategy,omitempty"`
	PropagateTags                 string                           `json:"propagateTags,omitempty"`
	AvailabilityZoneRebalancing   string                           `json:"availabilityZoneRebalancing,omitempty"`
	ServiceArn                    string                           `json:"serviceArn"`
	ServiceName                   string                           `json:"serviceName"`
	LoadBalancers                 []loadBalancerView               `json:"loadBalancers"`
	ServiceRegistries             []serviceRegistryView            `json:"serviceRegistries"`
	CapacityProviderStrategy      []cpStrategyItemInput            `json:"capacityProviderStrategy,omitempty"`
	PlacementConstraints          []placementConstraintView        `json:"placementConstraints,omitempty"`
	PlacementStrategy             []placementStrategyView          `json:"placementStrategy,omitempty"`
	Deployments                   []deploymentView                 `json:"deployments,omitempty"`
	Tags                          []Tag                            `json:"tags,omitempty"`
	CreatedAt                     float64                          `json:"createdAt"`
	DesiredCount                  int                              `json:"desiredCount"`
	PendingCount                  int                              `json:"pendingCount"`
	RunningCount                  int                              `json:"runningCount"`
	EnableExecuteCommand          bool                             `json:"enableExecuteCommand,omitempty"`
}

func toServiceView(s Service) serviceView {
	v := serviceView{
		ServiceArn:                    s.ServiceArn,
		ServiceName:                   s.ServiceName,
		ClusterArn:                    s.ClusterArn,
		TaskDefinition:                s.TaskDefinition,
		Status:                        s.Status,
		LaunchType:                    s.LaunchType,
		SchedulingStrategy:            s.SchedulingStrategy,
		PropagateTags:                 s.PropagateTags,
		AvailabilityZoneRebalancing:   s.AvailabilityZoneRebalancing,
		HealthCheckGracePeriodSeconds: s.HealthCheckGracePeriodSeconds,
		CreatedAt:                     float64(s.CreatedAt.Unix()),
		DeploymentConfiguration:       toDeploymentConfigurationView(s.DeploymentConfiguration),
		ServiceConnectConfiguration: toServiceConnectConfigurationView(
			s.ServiceConnectConfiguration,
		),
		NetworkConfiguration: toNetworkConfigurationView(s.NetworkConfiguration),
		DesiredCount:         s.DesiredCount,
		PendingCount:         s.PendingCount,
		RunningCount:         s.RunningCount,
		EnableExecuteCommand: s.EnableExecuteCommand,
	}

	if s.DeploymentController != nil {
		v.DeploymentController = &deploymentControllerView{Type: s.DeploymentController.Type}
	}

	v.LoadBalancers = []loadBalancerView{}
	for _, lb := range s.LoadBalancers {
		v.LoadBalancers = append(v.LoadBalancers, loadBalancerView(lb))
	}

	v.ServiceRegistries = []serviceRegistryView{}
	for _, sr := range s.ServiceRegistries {
		v.ServiceRegistries = append(v.ServiceRegistries, serviceRegistryView(sr))
	}

	for _, item := range s.CapacityProviderStrategy {
		v.CapacityProviderStrategy = append(v.CapacityProviderStrategy, cpStrategyItemInput(item))
	}

	for _, c := range s.PlacementConstraints {
		v.PlacementConstraints = append(v.PlacementConstraints, placementConstraintView(c))
	}

	for _, ps := range s.PlacementStrategy {
		v.PlacementStrategy = append(v.PlacementStrategy, placementStrategyView(ps))
	}

	for _, d := range s.Deployments {
		v.Deployments = append(v.Deployments, toDeploymentView(d))
	}

	return v
}

// deploymentView is the handler view of a service deployment record.
type deploymentView struct {
	CreatedAt          *float64 `json:"createdAt,omitempty"`
	UpdatedAt          *float64 `json:"updatedAt,omitempty"`
	ID                 string   `json:"id"`
	Status             string   `json:"status"`
	TaskDefinition     string   `json:"taskDefinition"`
	LaunchType         string   `json:"launchType,omitempty"`
	PlatformVersion    string   `json:"platformVersion,omitempty"`
	RolloutState       string   `json:"rolloutState,omitempty"`
	RolloutStateReason string   `json:"rolloutStateReason,omitempty"`
	DesiredCount       int      `json:"desiredCount"`
	PendingCount       int      `json:"pendingCount"`
	RunningCount       int      `json:"runningCount"`
	FailedTasks        int      `json:"failedTasks"`
}

// toDeploymentView maps a backend Deployment onto the wire view. ServiceRevisionArn
// is tracked internally (for DescribeServiceRevisions) but is not part of the real
// AWS Deployment wire shape embedded in DescribeServices/DescribeService responses,
// so it is intentionally omitted here.
func toDeploymentView(d Deployment) deploymentView {
	return deploymentView{
		CreatedAt:          d.CreatedAt,
		UpdatedAt:          d.UpdatedAt,
		ID:                 d.ID,
		Status:             d.Status,
		TaskDefinition:     d.TaskDefinition,
		LaunchType:         d.LaunchType,
		PlatformVersion:    d.PlatformVersion,
		RolloutState:       d.RolloutState,
		RolloutStateReason: d.RolloutStateReason,
		DesiredCount:       d.DesiredCount,
		PendingCount:       d.PendingCount,
		RunningCount:       d.RunningCount,
		FailedTasks:        d.FailedTasks,
	}
}

// ----- Converters -----

// toDeploymentConfiguration converts handler input to backend type.
func toDeploymentConfiguration(in *deploymentConfigurationInput) *DeploymentConfiguration {
	if in == nil {
		return nil
	}

	dc := &DeploymentConfiguration{
		MinimumHealthyPercent: in.MinimumHealthyPercent,
		MaximumPercent:        in.MaximumPercent,
	}

	if in.DeploymentCircuitBreaker != nil {
		dc.DeploymentCircuitBreaker = &DeploymentCircuitBreaker{
			Enable:   in.DeploymentCircuitBreaker.Enable,
			Rollback: in.DeploymentCircuitBreaker.Rollback,
		}
	}

	return dc
}

// toDeploymentController converts handler input to backend type.
func toDeploymentController(in *deploymentControllerInput) *DeploymentController {
	if in == nil {
		return nil
	}

	return &DeploymentController{Type: in.Type}
}

// toNetworkConfiguration converts handler input to backend type.
func toNetworkConfiguration(in *networkConfigurationInput) *NetworkConfiguration {
	if in == nil {
		return nil
	}

	nc := &NetworkConfiguration{}

	if in.AwsvpcConfiguration != nil {
		nc.AwsvpcConfiguration = &AwsvpcConfiguration{
			Subnets:        in.AwsvpcConfiguration.Subnets,
			SecurityGroups: in.AwsvpcConfiguration.SecurityGroups,
			AssignPublicIP: in.AwsvpcConfiguration.AssignPublicIP,
		}
	}

	return nc
}

// toNetworkConfigurationView converts backend type to handler view.
func toNetworkConfigurationView(nc *NetworkConfiguration) *networkConfigurationView {
	if nc == nil {
		return nil
	}

	v := &networkConfigurationView{}

	if nc.AwsvpcConfiguration != nil {
		v.AwsvpcConfiguration = &awsvpcConfigurationView{
			Subnets:        nc.AwsvpcConfiguration.Subnets,
			SecurityGroups: nc.AwsvpcConfiguration.SecurityGroups,
			AssignPublicIP: nc.AwsvpcConfiguration.AssignPublicIP,
		}
	}

	return v
}

// toLoadBalancers converts handler input to backend type.
func toLoadBalancers(in []loadBalancerInput) []LoadBalancer {
	if len(in) == 0 {
		return nil
	}

	out := make([]LoadBalancer, len(in))
	for i, lb := range in {
		out[i] = LoadBalancer(lb)
	}

	return out
}

// toServiceRegistries converts handler input to backend type.
func toServiceRegistries(in []serviceRegistryInput) []ServiceRegistry {
	if len(in) == 0 {
		return nil
	}

	out := make([]ServiceRegistry, len(in))
	for i, sr := range in {
		out[i] = ServiceRegistry(sr)
	}

	return out
}

// toPlacementStrategies converts handler input to backend type.
func toPlacementStrategies(in []placementStrategyInput) []PlacementStrategy {
	if len(in) == 0 {
		return nil
	}

	out := make([]PlacementStrategy, len(in))
	for i, s := range in {
		out[i] = PlacementStrategy(s)
	}

	return out
}

// toServiceConnectConfiguration converts handler input to backend type.
func toServiceConnectConfiguration(
	in *serviceConnectConfigurationInput,
) *ServiceConnectConfiguration {
	if in == nil {
		return nil
	}

	out := &ServiceConnectConfiguration{
		Namespace: in.Namespace,
		Enabled:   in.Enabled,
	}

	for _, s := range in.Services {
		svc := ServiceConnectService{
			PortName:      s.PortName,
			DiscoveryName: s.DiscoveryName,
		}

		for _, a := range s.ClientAliases {
			svc.ClientAliases = append(svc.ClientAliases, ServiceConnectClientAlias(a))
		}

		out.Services = append(out.Services, svc)
	}

	return out
}

// toServiceConnectConfigurationView converts backend type to view.
func toServiceConnectConfigurationView(
	in *ServiceConnectConfiguration,
) *serviceConnectConfigurationView {
	if in == nil {
		return nil
	}

	out := &serviceConnectConfigurationView{
		Namespace: in.Namespace,
		Enabled:   in.Enabled,
	}

	for _, s := range in.Services {
		svc := serviceConnectServiceView{
			PortName:      s.PortName,
			DiscoveryName: s.DiscoveryName,
		}

		for _, a := range s.ClientAliases {
			svc.ClientAliases = append(svc.ClientAliases, serviceConnectClientAliasView(a))
		}

		out.Services = append(out.Services, svc)
	}

	return out
}
