package macie2

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// categoryPolicy is the real FindingCategory enum value for a policy
// finding ("POLICY"), produced by "Policy:"-prefixed finding types.
const categoryPolicy = "POLICY"

// sampleFindingSchemaVersion is the schema version CreateSampleFindings
// stamps onto its fabricated findings.
const sampleFindingSchemaVersion = "1.0"

// sampleObjectSizeBytes is the fabricated size CreateSampleFindings reports
// for its sample S3 object's classification result.
const sampleObjectSizeBytes = 1024

// GetFindings retrieves findings by ID.
func (b *InMemoryBackend) GetFindings(findingIDs []string) ([]*Finding, error) {
	b.mu.RLock("GetFindings")
	defer b.mu.RUnlock()

	result := make([]*Finding, 0, len(findingIDs))

	for _, id := range findingIDs {
		f, ok := b.findings.Get(id)
		if !ok {
			return nil, ErrFindingNotFound
		}

		cp := f.Finding
		result = append(result, &cp)
	}

	return result, nil
}

// FindingSortCriteria mirrors types.SortCriteria for ListFindings. Only the
// AttributeName values backed by this model's fields are honored --
// resourcesAffected and policyDetails.action.apiCallDetails.firstSeen/
// lastSeen are documented AttributeName values (types.SortCriteria doc
// comment) this backend has no comparable scalar for, so sorting by them is
// left a no-op rather than inventing an ordering.
type FindingSortCriteria struct {
	AttributeName string
	OrderBy       string
}

func sortFindings(findings []*storedFinding, sortBy *FindingSortCriteria) {
	if sortBy == nil {
		sort.Slice(findings, func(i, k int) bool { return findings[i].ID < findings[k].ID })

		return
	}

	desc := sortBy.OrderBy == sortOrderDesc

	sort.Slice(findings, func(i, k int) bool {
		var less, tied bool

		switch sortBy.AttributeName {
		case "count":
			less, tied = findings[i].Count < findings[k].Count, findings[i].Count == findings[k].Count
		case keyCreatedAt:
			less = findings[i].CreatedAt.Before(findings[k].CreatedAt)
			tied = findings[i].CreatedAt.Equal(findings[k].CreatedAt)
		case keyUpdatedAt:
			less = findings[i].UpdatedAt.Before(findings[k].UpdatedAt)
			tied = findings[i].UpdatedAt.Equal(findings[k].UpdatedAt)
		case "type":
			less, tied = findings[i].Type < findings[k].Type, findings[i].Type == findings[k].Type
		case "severity.score":
			less = findings[i].Severity.Score < findings[k].Severity.Score
			tied = findings[i].Severity.Score == findings[k].Severity.Score
		default:
			return false
		}

		if tied {
			// ID is this table's unique key (storedFindingKeyFn); breaking ties on it
			// keeps a total order so the offset-based page.NewHMAC cursor can't drop
			// or duplicate findings that tie on the requested attribute.
			return findings[i].ID < findings[k].ID
		}

		if desc {
			return !less
		}

		return less
	})
}

// ListFindings returns finding IDs, filtered by criteria and sorted by
// sortBy.
func (b *InMemoryBackend) ListFindings(
	criteria map[string]any, sortBy *FindingSortCriteria, limit int, token string,
) ([]string, string, error) {
	b.mu.RLock("ListFindings")
	defer b.mu.RUnlock()

	var filtered []*storedFinding

	for _, finding := range b.findings.All() {
		if matchesFindingCriteria(finding, criteria) {
			filtered = append(filtered, finding)
		}
	}

	sortFindings(filtered, sortBy)

	ids := make([]string, len(filtered))
	for i, f := range filtered {
		ids[i] = f.ID
	}

	data, next := paginate(ids, token, b.paginationSecret, limit)

	return data, next, nil
}

func getFindingFieldValue(finding *storedFinding, key string) string {
	switch key {
	case keyType:
		return finding.Type
	case "category":
		return finding.Category
	case keyUpdatedAt:
		return finding.UpdatedAt.Format(time.RFC3339)
	case "severity.description":
		return finding.Severity.Description
	case bucketFieldAccountID:
		return finding.AccountID
	case bucketFieldRegion:
		return finding.Region
	}

	return ""
}

func matchEq(fVal string, eqVals []any) bool {
	for _, eqV := range eqVals {
		if strV, sOk := eqV.(string); sOk && strV == fVal {
			return true
		}
	}

	return false
}

func matchNeq(fVal string, neqVals []any) bool {
	for _, neqV := range neqVals {
		if strV, sOk := neqV.(string); sOk && strV == fVal {
			return false
		}
	}

	return true
}

func matchesFindingCriteria(finding *storedFinding, criteria map[string]any) bool {
	if len(criteria) == 0 {
		return true
	}

	criterion, ok := criteria["criterion"].(map[string]any)
	if !ok || len(criterion) == 0 {
		return true
	}

	for k, v := range criterion {
		cond, cOk := v.(map[string]any)
		if !cOk {
			continue
		}

		fVal := getFindingFieldValue(finding, k)

		if eqVals, eqOk := cond["eq"].([]any); eqOk {
			if !matchEq(fVal, eqVals) {
				return false
			}
		}
		if neqVals, neqOk := cond["neq"].([]any); neqOk {
			if !matchNeq(fVal, neqVals) {
				return false
			}
		}
	}

	return true
}

// sampleBucketName is the placeholder S3 bucket CreateSampleFindings
// attributes its fabricated findings to.
const sampleBucketName = "DOC-EXAMPLE-BUCKET"

// CreateSampleFindings creates sample findings. Real Macie's sample findings
// use realistic example resource/classification data rather than empty
// fields, so this populates the same category-appropriate ResourcesAffected/
// ClassificationDetails shape a live scan would produce.
func (b *InMemoryBackend) CreateSampleFindings(findingTypes []string) error {
	b.mu.Lock("CreateSampleFindings")
	defer b.mu.Unlock()

	types := findingTypes
	if len(types) == 0 {
		types = []string{"SensitiveData:S3Object/Personal"}
	}

	now := time.Now().UTC()
	bucketArn := arn.BuildS3(sampleBucketName)

	for _, ft := range types {
		id := uuid.New().String()
		category := categoryClassification

		if strings.HasPrefix(ft, "Policy:") {
			category = categoryPolicy
		}

		finding := Finding{
			AccountID:     b.accountID,
			Archived:      false,
			Category:      category,
			Count:         1,
			CreatedAt:     now,
			Description:   "Sample finding of type " + ft,
			ID:            id,
			Partition:     "aws",
			Region:        b.region,
			Sample:        true,
			SchemaVersion: sampleFindingSchemaVersion,
			ResourcesAffected: &ResourcesAffected{
				S3Bucket: &AffectedS3Bucket{Arn: bucketArn, Name: sampleBucketName},
			},
			Severity:  Severity{Description: "Medium", Score: defaultFindingScore},
			Title:     "Sample: " + ft,
			Type:      ft,
			UpdatedAt: now,
		}

		switch category {
		case categoryClassification:
			finding.ResourcesAffected.S3Object = &AffectedS3Object{
				BucketArn: bucketArn,
				Key:       "sample-object.txt",
				Path:      sampleBucketName + "/sample-object.txt",
			}
			finding.ClassificationDetails = &ClassificationDetails{
				OriginType: "SENSITIVE_DATA_DISCOVERY_JOB",
				Result: &ClassificationResult{
					MimeType:       "text/plain",
					SizeClassified: sampleObjectSizeBytes,
					Status:         &ClassificationResultStatus{Code: "COMPLETE"},
				},
			}
		case categoryPolicy:
			finding.PolicyDetails = &PolicyDetails{
				Action: &FindingAction{
					ActionType: "AWS_API_CALL",
				},
				Actor: &FindingActor{
					UserIdentity: &UserIdentity{
						Type:        "IAMUser",
						UserName:    "SampleUser",
						PrincipalID: "AIDAEXAMPLEUSERID",
					},
				},
			}
		}

		sf := &storedFinding{Finding: finding}
		if b.matchesArchiveFilter(sf) {
			sf.Finding.Archived = true
		}

		b.findings.Put(sf)
	}

	return nil
}

// GetFindingStatistics returns statistics grouped by the given field.
func (b *InMemoryBackend) GetFindingStatistics(
	groupBy string, criteria map[string]any,
) ([]FindingStatisticsGroup, error) {
	b.mu.RLock("GetFindingStatistics")
	defer b.mu.RUnlock()

	counts := make(map[string]int64)

	for _, f := range b.findings.All() {
		if !matchesFindingCriteria(f, criteria) {
			continue
		}

		var key string

		switch groupBy {
		case "type":
			key = f.Type
		case "severity.description":
			key = f.Severity.Description
		case "resourcesAffected.s3Bucket.name":
			key = bucketNameOf(f.Finding)
		case "classificationDetails.jobId":
			key = jobIDOf(f.Finding)
		default:
			key = f.Type
		}

		counts[key]++
	}

	result := make([]FindingStatisticsGroup, 0, len(counts))

	for k, v := range counts {
		result = append(result, FindingStatisticsGroup{GroupKey: k, Count: v})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].GroupKey < result[j].GroupKey })

	return result, nil
}

func bucketNameOf(f Finding) string {
	if f.ResourcesAffected != nil && f.ResourcesAffected.S3Bucket != nil && f.ResourcesAffected.S3Bucket.Name != "" {
		return f.ResourcesAffected.S3Bucket.Name
	}

	return "unknown-bucket"
}

func jobIDOf(f Finding) string {
	if f.ClassificationDetails != nil && f.ClassificationDetails.JobID != "" {
		return f.ClassificationDetails.JobID
	}

	return "unknown-job"
}

// GetFindingsPublicationConfiguration returns the findings publication config.
func (b *InMemoryBackend) GetFindingsPublicationConfiguration() (*FindingsPublicationConfig, error) {
	b.mu.RLock("GetFindingsPublicationConfiguration")
	defer b.mu.RUnlock()

	if b.findingsPubConfig == nil {
		return &FindingsPublicationConfig{}, nil
	}

	cp := *b.findingsPubConfig

	return &cp, nil
}

// PutFindingsPublicationConfiguration stores the findings publication config.
func (b *InMemoryBackend) PutFindingsPublicationConfiguration(cfg *FindingsPublicationConfig) error {
	b.mu.Lock("PutFindingsPublicationConfiguration")
	defer b.mu.Unlock()

	if cfg == nil {
		b.findingsPubConfig = nil

		return nil
	}

	cp := *cfg
	b.findingsPubConfig = &cp

	return nil
}
