package guardduty

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// validFindingPublishingFrequencies matches the real
// types.FindingPublishingFrequency enum.
//
//nolint:gochecknoglobals // static lookup table
var validFindingPublishingFrequencies = map[string]bool{
	"FIFTEEN_MINUTES": true,
	"ONE_HOUR":        true,
	"SIX_HOURS":       true,
}

// validDetectorFeatureNames matches the real types.DetectorFeature enum.
//
//nolint:gochecknoglobals // static lookup table
var validDetectorFeatureNames = map[string]bool{
	"S3_DATA_EVENTS":         true,
	"EKS_AUDIT_LOGS":         true,
	"EBS_MALWARE_PROTECTION": true,
	"RDS_LOGIN_EVENTS":       true,
	"LAMBDA_NETWORK_LOGS":    true,
	"EKS_RUNTIME_MONITORING": true,
	"RUNTIME_MONITORING":     true,
	"AI_PROTECTION":          true,
	"AI_ANALYST":             true,
}

// validFeatureStatuses matches the real types.FeatureStatus enum.
//
//nolint:gochecknoglobals // static lookup table
var validFeatureStatuses = map[string]bool{
	statusEnabled:  true,
	statusDisabled: true,
}

// validateDetectorConfig rejects a findingPublishingFrequency or
// features[].name/status outside the real SDK enums (types.
// FindingPublishingFrequency / types.DetectorFeature / types.FeatureStatus)
// -- this backend previously accepted and stored any string here, more
// permissive than the real service, which rejects an unrecognized enum
// value with a validation error rather than persisting it verbatim.
func validateDetectorConfig(frequency string, features []DetectorFeature) error {
	if frequency != "" && !validFindingPublishingFrequencies[frequency] {
		return ErrValidation
	}

	for _, f := range features {
		if !validDetectorFeatureNames[f.Name] {
			return ErrValidation
		}

		if f.Status != "" && !validFeatureStatuses[f.Status] {
			return ErrValidation
		}
	}

	return nil
}

// CreateDetector creates a new GuardDuty detector for this account+region.
func (b *InMemoryBackend) CreateDetector(
	enable bool,
	frequency string,
	tags map[string]string,
	features []DetectorFeature,
) (*Detector, error) {
	b.mu.Lock("CreateDetector")
	defer b.mu.Unlock()

	if err := validateDetectorConfig(frequency, features); err != nil {
		return nil, err
	}

	if b.detectors.Len() > 0 {
		return nil, ErrDetectorAlreadyExists
	}

	if frequency == "" {
		frequency = freqSixHours
	}

	status := statusDisabled
	if enable {
		status = statusEnabled
	}

	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	now := time.Now().UTC()
	d := &Detector{
		DetectorID:                 id,
		Status:                     status,
		FindingPublishingFrequency: frequency,
		ServiceRole: fmt.Sprintf(
			"arn:aws:iam::%s:role/aws-service-role/guardduty.amazonaws.com/AWSServiceRoleForAmazonGuardDuty",
			b.accountID,
		),
		CreatedAt: now,
		UpdatedAt: now,
		Tags:      tags,
		Features:  features,
	}
	b.detectors.Put(d)

	arn := b.detectorARN(id)
	if tags != nil {
		b.tags[arn] = maps.Clone(tags)
	}

	return d, nil
}

// GetDetector retrieves a detector by ID.
func (b *InMemoryBackend) GetDetector(detectorID string) (*Detector, error) {
	b.mu.RLock("GetDetector")
	defer b.mu.RUnlock()

	d, ok := b.detectors.Get(detectorID)
	if !ok {
		return nil, ErrDetectorNotFound
	}

	return d, nil
}

// UpdateDetector updates a detector's configuration.
func (b *InMemoryBackend) UpdateDetector(
	detectorID string,
	enable *bool,
	frequency string,
	features []DetectorFeature,
) error {
	b.mu.Lock("UpdateDetector")
	defer b.mu.Unlock()

	d, ok := b.detectors.Get(detectorID)
	if !ok {
		return ErrDetectorNotFound
	}

	// Validate before mutating anything -- an invalid frequency/feature must
	// not leave the detector's Status half-updated from enable/disable.
	if err := validateDetectorConfig(frequency, features); err != nil {
		return err
	}

	if enable != nil {
		if *enable {
			d.Status = statusEnabled
		} else {
			d.Status = statusDisabled
		}
	}

	if frequency != "" {
		d.FindingPublishingFrequency = frequency
	}

	if features != nil {
		d.Features = features
	}

	d.UpdatedAt = time.Now().UTC()

	return nil
}

// DeleteDetector removes a detector.
func (b *InMemoryBackend) DeleteDetector(detectorID string) error {
	b.mu.Lock("DeleteDetector")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return ErrDetectorNotFound
	}

	b.detectors.Delete(detectorID)

	// slices.Clone: Index.Get's returned slice mutates under Delete, so it
	// must be cloned before the delete loop below.
	for _, f := range slices.Clone(b.filtersByDetector.Get(detectorID)) {
		b.filters.Delete(detectorKey(detectorID, f.Name))
	}

	for _, f := range slices.Clone(b.findingsByDetector.Get(detectorID)) {
		b.findings.Delete(detectorKey(detectorID, f.ID))
	}

	for _, s := range slices.Clone(b.ipSetsByDetector.Get(detectorID)) {
		b.ipSets.Delete(detectorKey(detectorID, s.IPSetID))
	}

	for _, s := range slices.Clone(b.threatIntelSetsByDetector.Get(detectorID)) {
		b.threatIntelSets.Delete(detectorKey(detectorID, s.ThreatIntelSetID))
	}

	for _, m := range slices.Clone(b.membersByDetector.Get(detectorID)) {
		b.members.Delete(detectorKey(detectorID, m.AccountID))
	}

	for _, d := range slices.Clone(b.publishingDestinationsByDetector.Get(detectorID)) {
		b.publishingDestinations.Delete(detectorKey(detectorID, d.DestinationID))
	}

	for _, s := range slices.Clone(b.threatEntitySetsByDetector.Get(detectorID)) {
		b.threatEntitySets.Delete(detectorKey(detectorID, s.ThreatEntitySetID))
	}

	for _, s := range slices.Clone(b.trustedEntitySetsByDetector.Get(detectorID)) {
		b.trustedEntitySets.Delete(detectorKey(detectorID, s.TrustedEntitySetID))
	}

	for _, inv := range slices.Clone(b.investigationsByDetector.Get(detectorID)) {
		b.investigations.Delete(detectorKey(detectorID, inv.InvestigationID))
	}

	// malwareScans has no byDetector index (it's keyed by ScanID, not a
	// composite key), so filter b.malwareScans.All() directly -- All()
	// returns a fresh slice, safe to range over while deleting.
	for _, s := range b.malwareScans.All() {
		if s.DetectorID == detectorID {
			b.malwareScans.Delete(s.ScanID)
		}
	}

	b.malwareScanSettings.Delete(detectorID)

	delete(b.tags, b.detectorARN(detectorID))

	return nil
}

// ListDetectors returns all detector IDs.
func (b *InMemoryBackend) ListDetectors() []string {
	b.mu.RLock("ListDetectors")
	defer b.mu.RUnlock()

	items := b.detectors.Snapshot()
	ids := make([]string, len(items))

	for i, d := range items {
		ids[i] = d.DetectorID
	}

	return ids
}
