package cloudwatchlogs

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateDelivery creates a delivery between a delivery source and destination.
// fieldDelimiter, recordFields, and s3Config are set at creation time, matching
// real CreateDeliveryInput (confirmed via serializers.go's
// awsAwsjson11_serializeOpDocumentCreateDeliveryInput: fieldDelimiter/
// recordFields/s3DeliveryConfiguration are all top-level CreateDeliveryInput
// members) -- an earlier revision only exposed these via the separate
// UpdateDeliveryConfiguration op, so a real client that set them on create
// (as the real API allows) had them silently dropped.
func (b *InMemoryBackend) CreateDelivery(
	deliverySourceName, deliveryDestinationArn, fieldDelimiter string,
	recordFields []string,
	s3Config *DeliveryS3Configuration,
	tags map[string]string,
) (*Delivery, error) {
	if deliverySourceName == "" {
		return nil, fmt.Errorf("%w: deliverySourceName is required", ErrValidationException)
	}

	if deliveryDestinationArn == "" {
		return nil, fmt.Errorf("%w: deliveryDestinationArn is required", ErrValidationException)
	}

	id := uuid.New().String()
	deliveryArn := arn.Build("logs", b.region, b.accountID, "delivery:"+id)
	now := time.Now().UnixMilli()

	d := &Delivery{
		ID:                      id,
		Arn:                     deliveryArn,
		DeliverySourceName:      deliverySourceName,
		DeliveryDestinationArn:  deliveryDestinationArn,
		FieldDelimiter:          fieldDelimiter,
		RecordFields:            recordFields,
		S3DeliveryConfiguration: s3Config,
		Tags:                    maps.Clone(tags),
		CreationTime:            now,
	}

	b.mu.Lock("CreateDelivery")
	defer b.mu.Unlock()

	if dest := b.deliveryDestinationByArnLocked(deliveryDestinationArn); dest != nil {
		d.DeliveryDestinationType = dest.DeliveryDestinationType
	}

	b.deliveries.Put(d)

	cp := *d
	cp.Tags = maps.Clone(d.Tags)

	return &cp, nil
}

// DescribeDeliveries lists deliveries with pagination.
func (b *InMemoryBackend) DescribeDeliveries(
	limit int,
	nextToken string,
) ([]Delivery, string, error) {
	b.mu.RLock("DescribeDeliveries")
	defer b.mu.RUnlock()

	all := make([]Delivery, 0, b.deliveries.Len())
	for _, d := range b.deliveries.All() {
		cp := *d
		cp.Tags = maps.Clone(d.Tags)
		all = append(all, cp)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreationTime != all[j].CreationTime {
			return all[i].CreationTime < all[j].CreationTime
		}

		return all[i].ID < all[j].ID
	})

	startIdx := parseNextToken(nextToken)
	if startIdx >= len(all) {
		return []Delivery{}, "", nil
	}
	if limit <= 0 {
		limit = defaultDescribeLimit
	}
	end := startIdx + limit
	var outToken string
	if end < len(all) {
		outToken = encodeNextToken(end)
	} else {
		end = len(all)
	}

	return all[startIdx:end], outToken, nil
}

// GetDelivery returns a single delivery by ID.
func (b *InMemoryBackend) GetDelivery(id string) (*Delivery, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: id is required", ErrValidationException)
	}

	b.mu.RLock("GetDelivery")
	defer b.mu.RUnlock()

	d, ok := b.deliveries.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: delivery %s not found", ErrDeliveryNotFound, id)
	}
	cp := *d
	cp.Tags = maps.Clone(d.Tags)

	return &cp, nil
}

// DeleteDelivery deletes a delivery by ID.
func (b *InMemoryBackend) DeleteDelivery(id string) error {
	if id == "" {
		return fmt.Errorf("%w: id is required", ErrValidationException)
	}

	b.mu.Lock("DeleteDelivery")
	defer b.mu.Unlock()

	if !b.deliveries.Delete(id) {
		return fmt.Errorf("%w: delivery %s not found", ErrDeliveryNotFound, id)
	}

	return nil
}

// AddDeliveryInternal seeds a Delivery directly into the store for testing.
// It overwrites any existing delivery with the same ID.
func (b *InMemoryBackend) AddDeliveryInternal(delivery Delivery) {
	b.mu.Lock("AddDeliveryInternal")
	defer b.mu.Unlock()

	d := delivery
	d.Tags = maps.Clone(delivery.Tags)
	b.deliveries.Put(&d)
}

// validDeliveryDestinationTypes returns the allowed values for
// deliveryDestinationType (aws-sdk-go-v2 types.DeliveryDestinationType: S3,
// CWL, FH, XRAY).
func validDeliveryDestinationTypes() map[string]struct{} {
	return map[string]struct{}{
		"S3":   {},
		"CWL":  {},
		"FH":   {},
		"XRAY": {},
	}
}

// PutDeliveryDestination creates or updates a delivery destination.
// destinationType, when non-empty, must be one of S3/CWL/FH/XRAY.
func (b *InMemoryBackend) PutDeliveryDestination(
	name, targetArn, outputFormat, destinationType string,
	tags map[string]string,
) (*DeliveryDestination, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidationException)
	}

	if destinationType != "" {
		if _, ok := validDeliveryDestinationTypes()[destinationType]; !ok {
			return nil, fmt.Errorf(
				"%w: invalid deliveryDestinationType %q, must be one of S3, CWL, FH, XRAY",
				ErrValidationException, destinationType,
			)
		}
	}

	b.mu.Lock("PutDeliveryDestination")
	defer b.mu.Unlock()

	existing, exists := b.deliveryDestinations.Get(name)
	if exists {
		existing.TargetArn = targetArn
		existing.OutputFormat = outputFormat
		if destinationType != "" {
			existing.DeliveryDestinationType = destinationType
		}
		if tags != nil {
			existing.Tags = tags
		}
		cp := *existing

		return &cp, nil
	}

	dest := DeliveryDestination{
		Name:                    name,
		Arn:                     "arn:aws:logs:" + b.region + ":" + b.accountID + ":delivery-destination:" + name,
		TargetArn:               targetArn,
		OutputFormat:            outputFormat,
		DeliveryDestinationType: destinationType,
		Tags:                    tags,
		CreatedAt:               time.Now().UTC(),
	}
	stored := dest
	b.deliveryDestinations.Put(&stored)

	return &dest, nil
}

// deliveryDestinationByArnLocked finds a DeliveryDestination by its ARN.
// deliveryDestinations is keyed by name, not ARN, so this scans; the table is
// small (one entry per configured destination) and this is only called on
// CreateDelivery. Caller must hold at least a read lock.
func (b *InMemoryBackend) deliveryDestinationByArnLocked(destArn string) *DeliveryDestination {
	for _, d := range b.deliveryDestinations.All() {
		if d.Arn == destArn {
			return d
		}
	}

	return nil
}

// GetDeliveryDestination returns a delivery destination by name.
func (b *InMemoryBackend) GetDeliveryDestination(name string) (*DeliveryDestination, error) {
	b.mu.RLock("GetDeliveryDestination")
	defer b.mu.RUnlock()

	dest, ok := b.deliveryDestinations.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: delivery destination %q not found", ErrDeliveryDestinationNotFound, name)
	}
	cp := *dest

	return &cp, nil
}

// DescribeDeliveryDestinations returns delivery destinations with Limit/NextToken
// pagination (real DescribeDeliveryDestinationsInput members, api_op_DescribeDeliveryDestinations.go
// -- no documented default/max page size, so this follows the same defaultDescribeLimit
// fallback the rest of this package's undocumented-default ops use).
func (b *InMemoryBackend) DescribeDeliveryDestinations(nextToken string, limit int) ([]DeliveryDestination, string) {
	b.mu.RLock("DescribeDeliveryDestinations")
	defer b.mu.RUnlock()

	out := make([]DeliveryDestination, 0, b.deliveryDestinations.Len())
	for _, d := range b.deliveryDestinations.All() {
		out = append(out, *d)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	start, end, outToken := paginateRange(len(out), nextToken, limit)

	return out[start:end], outToken
}

// DeleteDeliveryDestination removes a delivery destination by name. Real
// AWS: "You can't delete a delivery destination if any current deliveries
// are associated with it".
func (b *InMemoryBackend) DeleteDeliveryDestination(name string) error {
	b.mu.Lock("DeleteDeliveryDestination")
	defer b.mu.Unlock()

	dest, ok := b.deliveryDestinations.Get(name)
	if !ok {
		return fmt.Errorf("%w: delivery destination %q not found", ErrDeliveryDestinationNotFound, name)
	}

	for _, d := range b.deliveries.All() {
		if d.DeliveryDestinationArn == dest.Arn {
			return fmt.Errorf(
				"%w: delivery destination %q has associated deliveries",
				ErrDeliveryDestinationInUse, name,
			)
		}
	}

	b.deliveryDestinations.Delete(name)

	return nil
}

// PutDeliveryDestinationPolicy stores a policy on a delivery destination.
func (b *InMemoryBackend) PutDeliveryDestinationPolicy(name, policy string) error {
	b.mu.Lock("PutDeliveryDestinationPolicy")
	defer b.mu.Unlock()

	dest, ok := b.deliveryDestinations.Get(name)
	if !ok {
		return fmt.Errorf("%w: delivery destination %q not found", ErrDeliveryDestinationNotFound, name)
	}

	dest.Policy = policy

	return nil
}

// GetDeliveryDestinationPolicy returns the policy for a delivery destination.
func (b *InMemoryBackend) GetDeliveryDestinationPolicy(name string) (string, error) {
	b.mu.RLock("GetDeliveryDestinationPolicy")
	defer b.mu.RUnlock()

	dest, ok := b.deliveryDestinations.Get(name)
	if !ok {
		return "", fmt.Errorf("%w: delivery destination %q not found", ErrDeliveryDestinationNotFound, name)
	}

	return dest.Policy, nil
}

// DeleteDeliveryDestinationPolicy removes the policy from a delivery destination.
func (b *InMemoryBackend) DeleteDeliveryDestinationPolicy(name string) error {
	b.mu.Lock("DeleteDeliveryDestinationPolicy")
	defer b.mu.Unlock()

	dest, ok := b.deliveryDestinations.Get(name)
	if !ok {
		return fmt.Errorf("%w: delivery destination %q not found", ErrDeliveryDestinationNotFound, name)
	}

	dest.Policy = ""

	return nil
}

// serviceFromARN extracts the service segment (index 2) of a standard
// 6-colon-field ARN (arn:partition:service:region:account-id:resource).
// Returns "" for anything that doesn't parse as an ARN with enough fields,
// rather than erroring: this is best-effort metadata derivation, not
// input validation.
func serviceFromARN(resourceArn string) string {
	const (
		arnFieldCount = 6
		serviceField  = 2
	)

	parts := strings.SplitN(resourceArn, ":", arnFieldCount)
	if len(parts) < arnFieldCount || parts[0] != "arn" {
		return ""
	}

	return parts[serviceField]
}

// PutDeliverySource creates or updates a delivery source. service is derived
// from the first resource ARN (see serviceFromARN), matching real AWS, which
// does not accept it as client input.
func (b *InMemoryBackend) PutDeliverySource(
	name, logType string,
	resourceArns []string,
	tags map[string]string,
) (*DeliverySource, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidationException)
	}

	var service string
	if len(resourceArns) > 0 {
		service = serviceFromARN(resourceArns[0])
	}

	b.mu.Lock("PutDeliverySource")
	defer b.mu.Unlock()

	existing, exists := b.deliverySources.Get(name)
	if exists {
		existing.LogType = logType
		if len(resourceArns) > 0 {
			existing.ResourceArns = resourceArns
			existing.Service = service
		}
		if tags != nil {
			existing.Tags = tags
		}
		cp := *existing

		return &cp, nil
	}

	src := DeliverySource{
		Name:         name,
		Arn:          "arn:aws:logs:" + b.region + ":" + b.accountID + ":delivery-source:" + name,
		LogType:      logType,
		ResourceArns: resourceArns,
		Service:      service,
		Tags:         tags,
		CreatedAt:    time.Now().UTC(),
	}
	stored := src
	b.deliverySources.Put(&stored)

	return &src, nil
}

// GetDeliverySource returns a delivery source by name.
func (b *InMemoryBackend) GetDeliverySource(name string) (*DeliverySource, error) {
	b.mu.RLock("GetDeliverySource")
	defer b.mu.RUnlock()

	src, ok := b.deliverySources.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: delivery source %q not found", ErrDeliverySourceNotFound, name)
	}
	cp := *src

	return &cp, nil
}

// DescribeDeliverySources returns delivery sources with Limit/NextToken
// pagination (real DescribeDeliverySourcesInput members, api_op_DescribeDeliverySources.go
// -- no documented default/max page size, so this follows the same defaultDescribeLimit
// fallback the rest of this package's undocumented-default ops use).
func (b *InMemoryBackend) DescribeDeliverySources(nextToken string, limit int) ([]DeliverySource, string) {
	b.mu.RLock("DescribeDeliverySources")
	defer b.mu.RUnlock()

	out := make([]DeliverySource, 0, b.deliverySources.Len())
	for _, s := range b.deliverySources.All() {
		out = append(out, *s)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	start, end, outToken := paginateRange(len(out), nextToken, limit)

	return out[start:end], outToken
}

// DeleteDeliverySource removes a delivery source by name. Real AWS: "You
// can't delete a delivery source if any current deliveries are associated
// with it".
func (b *InMemoryBackend) DeleteDeliverySource(name string) error {
	b.mu.Lock("DeleteDeliverySource")
	defer b.mu.Unlock()

	if !b.deliverySources.Has(name) {
		return fmt.Errorf("%w: delivery source %q not found", ErrDeliverySourceNotFound, name)
	}

	for _, d := range b.deliveries.All() {
		if d.DeliverySourceName == name {
			return fmt.Errorf("%w: delivery source %q has associated deliveries", ErrDeliverySourceInUse, name)
		}
	}

	b.deliverySources.Delete(name)

	return nil
}

// UpdateDeliveryConfiguration updates the field delimiter, record fields,
// and/or S3 delivery configuration for a delivery. s3Config matches real
// UpdateDeliveryConfigurationInput.S3DeliveryConfiguration (confirmed via
// api_op_UpdateDeliveryConfiguration.go) -- an earlier revision only updated
// fieldDelimiter/recordFields, silently dropping any S3DeliveryConfiguration
// a real client sent.
func (b *InMemoryBackend) UpdateDeliveryConfiguration(
	id, fieldDelimiter string,
	recordFields []string,
	s3Config *DeliveryS3Configuration,
) error {
	b.mu.Lock("UpdateDeliveryConfiguration")
	defer b.mu.Unlock()

	delivery, ok := b.deliveries.Get(id)
	if !ok {
		return fmt.Errorf("%w: delivery %q not found", ErrDeliveryNotFound, id)
	}

	if fieldDelimiter != "" {
		delivery.FieldDelimiter = fieldDelimiter
	}

	if len(recordFields) > 0 {
		delivery.RecordFields = recordFields
	}

	if s3Config != nil {
		delivery.S3DeliveryConfiguration = s3Config
	}

	return nil
}
