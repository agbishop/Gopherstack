package iot

import (
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// AggregationType is the field aggregation type for a fleet metric
// (types.AggregationType, aws-sdk-go-v2/service/iot@v1.77.4).
type AggregationType struct {
	Name   string   `json:"name"`
	Values []string `json:"values,omitempty"`
}

// FleetMetric represents an IoT fleet metric.
type FleetMetric struct {
	Tags             map[string]string `json:"tags,omitempty"`
	MetricName       string            `json:"metricName"`
	MetricARN        string            `json:"metricArn"`
	QueryString      string            `json:"queryString,omitempty"`
	IndexName        string            `json:"indexName,omitempty"`
	QueryVersion     string            `json:"queryVersion,omitempty"`
	Description      string            `json:"description,omitempty"`
	AggregationField string            `json:"aggregationField,omitempty"`
	AggregationType  *AggregationType  `json:"aggregationType,omitempty"`
	Unit             string            `json:"unit,omitempty"`
	Period           int32             `json:"period,omitempty"`
	Version          int64             `json:"version"`
	CreationDate     float64           `json:"creationDate,omitempty"`
	LastModified     float64           `json:"lastModifiedDate,omitempty"`
}

func cloneFleetMetric(fm *FleetMetric) *FleetMetric {
	cp := *fm

	return &cp
}

func (b *InMemoryBackend) fleetMetricARN(name string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("fleetmetric/%s", name))
}

// CreateFleetMetricInput holds input for CreateFleetMetric.
type CreateFleetMetricInput struct {
	MetricName       string           `json:"metricName"`
	QueryString      string           `json:"queryString,omitempty"`
	IndexName        string           `json:"indexName,omitempty"`
	QueryVersion     string           `json:"queryVersion,omitempty"`
	Description      string           `json:"description,omitempty"`
	AggregationField string           `json:"aggregationField,omitempty"`
	AggregationType  *AggregationType `json:"aggregationType,omitempty"`
	Unit             string           `json:"unit,omitempty"`
	// []types.Tag on the wire, not a map (serializers.go:2724, aws-sdk-go-v2/service/iot@v1.77.4).
	Tags   []tags.KV `json:"tags,omitempty"`
	Period int32     `json:"period,omitempty"`
}

func (b *InMemoryBackend) CreateFleetMetric(input *CreateFleetMetricInput) (*FleetMetric, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.fleetMetrics.Has(input.MetricName) {
		return nil, fmt.Errorf("fleet metric %q already exists: %w", input.MetricName, ErrAlreadyExists)
	}
	now := float64(time.Now().Unix())
	fm := &FleetMetric{
		MetricName:       input.MetricName,
		MetricARN:        b.fleetMetricARN(input.MetricName),
		QueryString:      input.QueryString,
		IndexName:        input.IndexName,
		QueryVersion:     input.QueryVersion,
		Description:      input.Description,
		AggregationField: input.AggregationField,
		AggregationType:  input.AggregationType,
		Unit:             input.Unit,
		Period:           input.Period,
		Tags:             tags.MapFromKV(input.Tags),
		Version:          1,
		CreationDate:     now,
		LastModified:     now,
	}
	b.fleetMetrics.Put(fm)
	b.putResourceTagsLocked(fm.MetricARN, fm.Tags)

	return cloneFleetMetric(fm), nil
}

func (b *InMemoryBackend) DescribeFleetMetric(name string) (*FleetMetric, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	fm, ok := b.fleetMetrics.Get(name)
	if !ok {
		return nil, fmt.Errorf("fleet metric %q not found: %w", name, ErrResourceNotFound)
	}

	return cloneFleetMetric(fm), nil
}

func (b *InMemoryBackend) ListFleetMetrics() []*FleetMetric {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*FleetMetric, 0, b.fleetMetrics.Len())
	for _, v := range b.fleetMetrics.Snapshot() {
		out = append(out, cloneFleetMetric(v))
	}

	return out
}

// UpdateFleetMetricInput holds input for UpdateFleetMetric.
type UpdateFleetMetricInput struct {
	QueryString      string           `json:"queryString,omitempty"`
	IndexName        string           `json:"indexName,omitempty"`
	QueryVersion     string           `json:"queryVersion,omitempty"`
	Description      string           `json:"description,omitempty"`
	AggregationField string           `json:"aggregationField,omitempty"`
	AggregationType  *AggregationType `json:"aggregationType,omitempty"`
	Unit             string           `json:"unit,omitempty"`
	Period           int32            `json:"period,omitempty"`
	ExpectedVersion  int64            `json:"expectedVersion,omitempty"`
}

func (b *InMemoryBackend) UpdateFleetMetric(name string, input *UpdateFleetMetricInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	fm, ok := b.fleetMetrics.Get(name)
	if !ok {
		return fmt.Errorf("fleet metric %q not found: %w", name, ErrResourceNotFound)
	}

	if input.ExpectedVersion != 0 && input.ExpectedVersion != fm.Version {
		return fmt.Errorf("%w: expected version %d but current is %d",
			ErrVersionConflict, input.ExpectedVersion, fm.Version)
	}
	if input.QueryString != "" {
		fm.QueryString = input.QueryString
	}
	if input.IndexName != "" {
		fm.IndexName = input.IndexName
	}
	if input.QueryVersion != "" {
		fm.QueryVersion = input.QueryVersion
	}
	if input.Description != "" {
		fm.Description = input.Description
	}
	if input.AggregationField != "" {
		fm.AggregationField = input.AggregationField
	}
	if input.AggregationType != nil {
		fm.AggregationType = input.AggregationType
	}
	if input.Unit != "" {
		fm.Unit = input.Unit
	}
	if input.Period > 0 {
		fm.Period = input.Period
	}
	fm.Version++
	fm.LastModified = float64(time.Now().Unix())

	return nil
}

func (b *InMemoryBackend) DeleteFleetMetric(name string, expectedVersion int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	fm, ok := b.fleetMetrics.Get(name)
	if !ok {
		return fmt.Errorf("fleet metric %q not found: %w", name, ErrResourceNotFound)
	}

	if expectedVersion != 0 && expectedVersion != fm.Version {
		return fmt.Errorf("%w: expected version %d, current version %d",
			ErrVersionConflict, expectedVersion, fm.Version)
	}
	b.fleetMetrics.Delete(name)
	delete(b.resourceTags, fm.MetricARN)

	return nil
}

// CustomMetric represents an IoT custom metric.
type CustomMetric struct {
	Tags             map[string]string `json:"tags,omitempty"`
	MetricName       string            `json:"metricName"`
	MetricARN        string            `json:"metricArn"`
	MetricType       string            `json:"metricType"`
	DisplayName      string            `json:"displayName,omitempty"`
	Version          int64             `json:"version"`
	CreationDate     float64           `json:"creationDate,omitempty"`
	LastModifiedDate float64           `json:"lastModifiedDate,omitempty"`
}

func cloneCustomMetric(cm *CustomMetric) *CustomMetric {
	cp := *cm

	return &cp
}

func (b *InMemoryBackend) customMetricARN(name string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("custommetric/%s", name))
}

// CreateCustomMetricInput holds input for CreateCustomMetric.
type CreateCustomMetricInput struct {
	MetricName         string `json:"metricName"`
	MetricType         string `json:"metricType"`
	DisplayName        string `json:"displayName,omitempty"`
	ClientRequestToken string `json:"clientRequestToken,omitempty"`
	// []types.Tag on the wire, not a map (serializers.go:2226, aws-sdk-go-v2/service/iot@v1.77.4).
	Tags []tags.KV `json:"tags,omitempty"`
}

func (b *InMemoryBackend) CreateCustomMetric(input *CreateCustomMetricInput) (*CustomMetric, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.customMetrics.Has(input.MetricName) {
		return nil, fmt.Errorf("custom metric %q already exists: %w", input.MetricName, ErrAlreadyExists)
	}
	now := float64(time.Now().Unix())
	cm := &CustomMetric{
		MetricName:       input.MetricName,
		MetricARN:        b.customMetricARN(input.MetricName),
		MetricType:       input.MetricType,
		DisplayName:      input.DisplayName,
		Tags:             tags.MapFromKV(input.Tags),
		Version:          1,
		CreationDate:     now,
		LastModifiedDate: now,
	}
	b.customMetrics.Put(cm)
	b.putResourceTagsLocked(cm.MetricARN, cm.Tags)

	return cloneCustomMetric(cm), nil
}

func (b *InMemoryBackend) DescribeCustomMetric(name string) (*CustomMetric, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	cm, ok := b.customMetrics.Get(name)
	if !ok {
		return nil, fmt.Errorf("custom metric %q not found: %w", name, ErrResourceNotFound)
	}

	return cloneCustomMetric(cm), nil
}

func (b *InMemoryBackend) ListCustomMetrics() []*CustomMetric {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*CustomMetric, 0, b.customMetrics.Len())
	for _, v := range b.customMetrics.Snapshot() {
		out = append(out, cloneCustomMetric(v))
	}

	return out
}

// UpdateCustomMetricInput holds input for UpdateCustomMetric.
type UpdateCustomMetricInput struct {
	DisplayName string `json:"displayName"`
}

func (b *InMemoryBackend) UpdateCustomMetric(name, displayName string) (*CustomMetric, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cm, ok := b.customMetrics.Get(name)
	if !ok {
		return nil, fmt.Errorf("custom metric %q not found: %w", name, ErrResourceNotFound)
	}
	if displayName != "" {
		cm.DisplayName = displayName
	}
	cm.Version++
	cm.LastModifiedDate = float64(time.Now().Unix())

	return cloneCustomMetric(cm), nil
}

func (b *InMemoryBackend) DeleteCustomMetric(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.customMetrics.Has(name) {
		return fmt.Errorf("custom metric %q not found: %w", name, ErrResourceNotFound)
	}
	b.customMetrics.Delete(name)
	delete(b.resourceTags, b.customMetricARN(name))

	return nil
}

// Dimension represents an IoT dimension.
type Dimension struct {
	Tags             map[string]string `json:"tags,omitempty"`
	Name             string            `json:"name"`
	ARN              string            `json:"arn"`
	Type             string            `json:"type"`
	StringValues     []string          `json:"stringValues,omitempty"`
	CreationDate     float64           `json:"creationDate,omitempty"`
	LastModifiedDate float64           `json:"lastModifiedDate,omitempty"`
}

func cloneDimension(d *Dimension) *Dimension {
	cp := *d
	cp.StringValues = append([]string(nil), d.StringValues...)

	return &cp
}

func (b *InMemoryBackend) dimensionARN(name string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("dimension/%s", name))
}

// CreateDimensionInput holds input for CreateDimension.
type CreateDimensionInput struct {
	// []types.Tag on the wire, not a map (serializers.go:2337, aws-sdk-go-v2/service/iot@v1.77.4).
	Tags               []tags.KV `json:"tags,omitempty"`
	Name               string    `json:"name"`
	Type               string    `json:"type"`
	ClientRequestToken string    `json:"clientRequestToken,omitempty"`
	StringValues       []string  `json:"stringValues"`
}

func (b *InMemoryBackend) CreateDimension(input *CreateDimensionInput) (*Dimension, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.dimensions.Has(input.Name) {
		return nil, fmt.Errorf("dimension %q already exists: %w", input.Name, ErrAlreadyExists)
	}
	now := float64(time.Now().Unix())
	d := &Dimension{
		Name:             input.Name,
		ARN:              b.dimensionARN(input.Name),
		Type:             input.Type,
		StringValues:     append([]string(nil), input.StringValues...),
		Tags:             tags.MapFromKV(input.Tags),
		CreationDate:     now,
		LastModifiedDate: now,
	}
	b.dimensions.Put(d)
	b.putResourceTagsLocked(d.ARN, d.Tags)

	return cloneDimension(d), nil
}

func (b *InMemoryBackend) DescribeDimension(name string) (*Dimension, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	d, ok := b.dimensions.Get(name)
	if !ok {
		return nil, fmt.Errorf("dimension %q not found: %w", name, ErrResourceNotFound)
	}

	return cloneDimension(d), nil
}

func (b *InMemoryBackend) ListDimensions() []*Dimension {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*Dimension, 0, b.dimensions.Len())
	for _, v := range b.dimensions.Snapshot() {
		out = append(out, cloneDimension(v))
	}

	return out
}

// UpdateDimensionInput holds input for UpdateDimension.
type UpdateDimensionInput struct {
	StringValues []string `json:"stringValues"`
}

func (b *InMemoryBackend) UpdateDimension(name string, stringValues []string) (*Dimension, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	d, ok := b.dimensions.Get(name)
	if !ok {
		return nil, fmt.Errorf("dimension %q not found: %w", name, ErrResourceNotFound)
	}
	if len(stringValues) > 0 {
		d.StringValues = append([]string(nil), stringValues...)
	}
	d.LastModifiedDate = float64(time.Now().Unix())

	return cloneDimension(d), nil
}

func (b *InMemoryBackend) DeleteDimension(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.dimensions.Has(name) {
		return fmt.Errorf("dimension %q not found: %w", name, ErrResourceNotFound)
	}
	b.dimensions.Delete(name)
	delete(b.resourceTags, b.dimensionARN(name))

	return nil
}

// MetricDatapoint is one reported value of a security-profile metric for a
// thing at a point in time.
type MetricDatapoint struct {
	Value     *MetricValueData `json:"value,omitempty"`
	Timestamp float64          `json:"timestamp"`
}

// MetricValueData is the polymorphic value of a metric datapoint. Mirrors
// types.MetricValue (v1.77.4), which also has Numbers/Strings beyond the
// four members previously modeled here.
type MetricValueData struct {
	Count   *int64    `json:"count,omitempty"`
	Number  *float64  `json:"number,omitempty"`
	Cidrs   []string  `json:"cidrs,omitempty"`
	Ports   []int32   `json:"ports,omitempty"`
	Numbers []float64 `json:"numbers,omitempty"`
	Strings []string  `json:"strings,omitempty"`
}

// metricValueKey builds the composite key for a thing/metric pair.
func metricValueKey(thingName, metricName string) string {
	return thingName + "/" + metricName
}

// ListMetricValues returns stored metric datapoints for a thing/metric over
// [startTime, endTime], paginated.
func (b *InMemoryBackend) ListMetricValues(
	thingName, metricName string, startTime, endTime float64, maxResults int32, nextToken string,
) ([]*MetricDatapoint, string, error) {
	if thingName == "" || metricName == "" {
		return nil, "", fmt.Errorf("%w: thingName and metricName are required", ErrValidation)
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.things.Has(thingName) {
		return nil, "", fmt.Errorf("%w: %s", ErrThingNotFound, thingName)
	}

	all := b.metricValues[metricValueKey(thingName, metricName)]
	filtered := make([]*MetricDatapoint, 0, len(all))

	for _, dp := range all {
		if startTime > 0 && dp.Timestamp < startTime {
			continue
		}

		if endTime > 0 && dp.Timestamp > endTime {
			continue
		}

		cp := *dp
		filtered = append(filtered, &cp)
	}

	page, next := paginateMaps(filtered, searchPageSize(maxResults), searchStartOffset(nextToken))

	return page, next, nil
}

// AddMetricValueInternal seeds a metric datapoint for a thing/metric pair
// (there is no public PutMetricValue control-plane operation; values are
// normally reported by the device SDK's Device Defender metrics agent).
func (b *InMemoryBackend) AddMetricValueInternal(thingName, metricName string, dp MetricDatapoint) {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := metricValueKey(thingName, metricName)
	cp := dp
	b.metricValues[key] = append(b.metricValues[key], &cp)
}
