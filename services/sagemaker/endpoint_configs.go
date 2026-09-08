package sagemaker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrEndpointConfigNotFound is returned when an endpoint config does not exist.
	ErrEndpointConfigNotFound = awserr.New("ValidationException", awserr.ErrNotFound)
	// ErrEndpointConfigAlreadyExists is returned when an endpoint config already exists.
	ErrEndpointConfigAlreadyExists = awserr.New("ResourceInUse", awserr.ErrConflict)
	// ErrEndpointConfigInUse is returned when deleting an endpoint config still used by an endpoint.
	ErrEndpointConfigInUse = awserr.New("ResourceInUse", awserr.ErrConflict)
)

// CreateEndpointConfig creates a new SageMaker endpoint configuration.
func (b *InMemoryBackend) CreateEndpointConfig(
	ctx context.Context,
	name string,
	productionVariants []ProductionVariant,
	tags map[string]string,
) (*EndpointConfig, error) {
	b.mu.Lock("CreateEndpointConfig")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	ecStore := b.endpointConfigsStore(region)

	if _, ok := ecStore.Get(name); ok {
		return nil, fmt.Errorf(
			"%w: endpoint config %s already exists",
			ErrEndpointConfigAlreadyExists,
			name,
		)
	}

	configARN := arn.Build("sagemaker", region, b.accountID, "endpoint-config/"+name)

	storedVariants := make([]ProductionVariant, len(productionVariants))
	copy(storedVariants, productionVariants)

	ec := &EndpointConfig{
		EndpointConfigName: name,
		EndpointConfigARN:  configARN,
		ProductionVariants: storedVariants,
		CreationTime:       time.Now(),
		Tags:               mergeTags(nil, tags),
	}
	ecStore.Put(ec)
	b.endpointConfigARNIndexStore(region)[configARN] = name

	return cloneEndpointConfig(ec), nil
}

// DescribeEndpointConfig returns an endpoint config by name.
func (b *InMemoryBackend) DescribeEndpointConfig(ctx context.Context, name string) (*EndpointConfig, error) {
	b.mu.RLock("DescribeEndpointConfig")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	ec, ok := b.endpointConfigsStoreRO(region).Get(name)
	if !ok {
		return nil, fmt.Errorf(
			"%w: could not find endpoint configuration %q",
			ErrEndpointConfigNotFound,
			name,
		)
	}

	return cloneEndpointConfig(ec), nil
}

// ListEndpointConfigs returns endpoint configurations matching filter, sorted
// per filter.SortBy/SortOrder (default CreationTime/Descending,
// api_op_ListEndpointConfigs.go), with pagination capped at filter.MaxResults.
func (b *InMemoryBackend) ListEndpointConfigs(
	ctx context.Context, nextToken string, filter nameTimeFilter,
) ([]*EndpointConfig, string) {
	b.mu.RLock("ListEndpointConfigs")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	all := make([]*EndpointConfig, 0, b.endpointConfigsStoreRO(region).Len())
	for _, ec := range b.endpointConfigsStoreRO(region).All() {
		all = append(all, cloneEndpointConfig(ec))
	}

	return filterSortPaginateByName(all, nextToken, filter, true,
		func(ec *EndpointConfig) string { return ec.EndpointConfigName },
		func(ec *EndpointConfig) time.Time { return ec.CreationTime })
}

// DeleteEndpointConfig deletes an endpoint configuration by name.
func (b *InMemoryBackend) DeleteEndpointConfig(ctx context.Context, name string) error {
	b.mu.Lock("DeleteEndpointConfig")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	ecStore := b.endpointConfigsStore(region)

	ec, ok := ecStore.Get(name)
	if !ok {
		return fmt.Errorf(
			"%w: could not find endpoint configuration %q",
			ErrEndpointConfigNotFound,
			name,
		)
	}

	for _, ep := range b.endpointsStore(region).All() {
		if ep.EndpointConfigName == name {
			return fmt.Errorf(
				"%w: endpoint configuration %q is in use by endpoint %q",
				ErrEndpointConfigInUse,
				name,
				ep.EndpointName,
			)
		}
	}

	arnIndex := b.endpointConfigARNIndexStore(region)
	delete(arnIndex, ec.EndpointConfigARN)
	ecStore.Delete(name)

	return nil
}

// SetEndpointConfigExtras sets optional fields on an existing endpoint config that were not
// included in the original CreateEndpointConfig signature.
func (b *InMemoryBackend) SetEndpointConfigExtras(
	ctx context.Context,
	name string,
	dataCaptureConfig *DataCaptureConfig,
	asyncInferenceConfig *AsyncInferenceConfig,
	vpcConfig *VpcConfig,
	executionRoleArn string,
	kmsKeyID string,
	shadowProductionVariants []ProductionVariant,
	enableNetworkIsolation bool,
	explainerConfig, metricsConfig json.RawMessage,
) error {
	b.mu.Lock("SetEndpointConfigExtras")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	ec, ok := b.endpointConfigsStore(region).Get(name)
	if !ok {
		return fmt.Errorf(
			"%w: could not find endpoint configuration %q",
			ErrEndpointConfigNotFound,
			name,
		)
	}

	if dataCaptureConfig != nil {
		dcc := *dataCaptureConfig
		ec.DataCaptureConfig = &dcc
	}

	if asyncInferenceConfig != nil {
		aic := *asyncInferenceConfig
		ec.AsyncInferenceConfig = &aic
	}

	if vpcConfig != nil {
		vpc := *vpcConfig
		vpc.SecurityGroupIDs = append([]string(nil), vpcConfig.SecurityGroupIDs...)
		vpc.Subnets = append([]string(nil), vpcConfig.Subnets...)
		ec.VpcConfig = &vpc
	}

	ec.ExecutionRoleArn = executionRoleArn
	ec.KmsKeyID = kmsKeyID
	ec.EnableNetworkIsolation = enableNetworkIsolation

	if len(explainerConfig) > 0 {
		ec.ExplainerConfig = append(json.RawMessage(nil), explainerConfig...)
	}

	if len(metricsConfig) > 0 {
		ec.MetricsConfig = append(json.RawMessage(nil), metricsConfig...)
	}

	if len(shadowProductionVariants) > 0 {
		stored := make([]ProductionVariant, len(shadowProductionVariants))
		for i, pv := range shadowProductionVariants {
			stored[i] = cloneProductionVariant(pv)
		}
		ec.ShadowProductionVariants = stored
	}

	return nil
}
