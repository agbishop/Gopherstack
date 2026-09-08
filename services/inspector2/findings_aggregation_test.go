package inspector2_test

// Tests for gopherstack-f9vi: ListFindingAggregations aggregation types that
// are computable from Finding.Title or Finding.Resources[] (already present
// on the Finding model) must report real per-group severity counts instead
// of an honest-empty responses list. PACKAGE, AMI, IMAGE_LAYER, LAMBDA_LAYER,
// FINDING_TYPE, CONTAINER_IMAGE, SERVERLESS_FUNCTION and VM_INSTANCE remain
// genuinely unsupported -- this file also pins that they stay empty even
// when other groupable data exists on the same findings.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/inspector2"
)

const testAccountID = "123456789012"

func seedAggregationFindings(t *testing.T, b *inspector2.InMemoryBackend, findings []inspector2.Finding) {
	t.Helper()

	for _, f := range findings {
		_, err := b.SeedFinding(f)
		require.NoError(t, err)
	}
}

// aggregationGroupCounts maps each response entry's group key to its
// severityCounts.all, and asserts the entry carries the expected union
// member, key field, and accountId.
func aggregationGroupCounts(t *testing.T, responses []map[string]any, unionKey, keyField string) map[string]int64 {
	t.Helper()

	counts := make(map[string]int64, len(responses))

	for _, r := range responses {
		entry, ok := r[unionKey].(map[string]any)
		require.True(t, ok, "response entry missing %q union member: %#v", unionKey, r)

		key, ok := entry[keyField].(string)
		require.True(t, ok, "entry missing %q field: %#v", keyField, entry)

		assert.Equal(t, testAccountID, entry["accountId"], "entry accountId")

		sc, ok := entry["severityCounts"].(map[string]any)
		require.True(t, ok, "entry missing severityCounts: %#v", entry)

		all, ok := sc["all"].(int64)
		require.True(t, ok, "severityCounts.all not int64: %#v", sc)

		counts[key] = all
	}

	return counts
}

func TestListFindingAggregations_ResourceKeyedGrouping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		seed            func(t *testing.T, b *inspector2.InMemoryBackend)
		wantGroups      map[string]int64
		name            string
		aggregationType string
		unionKey        string
		keyField        string
	}{
		{
			name:            "title",
			aggregationType: "TITLE",
			unionKey:        "titleAggregation",
			keyField:        "title",
			wantGroups:      map[string]int64{"Title A": 2, "Title B": 1},
			seed: func(t *testing.T, b *inspector2.InMemoryBackend) {
				t.Helper()
				seedAggregationFindings(t, b, []inspector2.Finding{
					{Title: "Title A", Severity: inspector2.FindingSeverity{Label: "CRITICAL"}},
					{Title: "Title A", Severity: inspector2.FindingSeverity{Label: "HIGH"}},
					{Title: "Title B", Severity: inspector2.FindingSeverity{Label: "LOW"}},
					{Severity: inspector2.FindingSeverity{Label: "LOW"}}, // no title: no group
				})
			},
		},
		{
			name:            "repository",
			aggregationType: "REPOSITORY",
			unionKey:        "repositoryAggregation",
			keyField:        "repository",
			wantGroups:      map[string]int64{"repo-a": 2, "repo-b": 1},
			seed: func(t *testing.T, b *inspector2.InMemoryBackend) {
				t.Helper()
				seedAggregationFindings(t, b, []inspector2.Finding{
					{
						Severity:  inspector2.FindingSeverity{Label: "CRITICAL"},
						Resources: []inspector2.FindingResource{{Type: "AWS_ECR_REPOSITORY", ID: "repo-a"}},
					},
					{
						Severity:  inspector2.FindingSeverity{Label: "HIGH"},
						Resources: []inspector2.FindingResource{{Type: "AWS_ECR_REPOSITORY", ID: "repo-a"}},
					},
					{
						Severity:  inspector2.FindingSeverity{Label: "LOW"},
						Resources: []inspector2.FindingResource{{Type: "AWS_ECR_REPOSITORY", ID: "repo-b"}},
					},
					{
						Severity:  inspector2.FindingSeverity{Label: "LOW"},
						Resources: []inspector2.FindingResource{{Type: "AWS_EC2_INSTANCE", ID: "i-unrelated"}},
					},
				})
			},
		},
		{
			name:            "ec2_instance",
			aggregationType: "AWS_EC2_INSTANCE",
			unionKey:        "ec2InstanceAggregation",
			keyField:        "instanceId",
			wantGroups:      map[string]int64{"i-aaa": 2, "i-bbb": 1},
			seed: func(t *testing.T, b *inspector2.InMemoryBackend) {
				t.Helper()
				seedAggregationFindings(t, b, []inspector2.Finding{
					{
						Severity:  inspector2.FindingSeverity{Label: "CRITICAL"},
						Resources: []inspector2.FindingResource{{Type: "AWS_EC2_INSTANCE", ID: "i-aaa"}},
					},
					{
						Severity:  inspector2.FindingSeverity{Label: "MEDIUM"},
						Resources: []inspector2.FindingResource{{Type: "AWS_EC2_INSTANCE", ID: "i-aaa"}},
					},
					{
						Severity:  inspector2.FindingSeverity{Label: "LOW"},
						Resources: []inspector2.FindingResource{{Type: "AWS_EC2_INSTANCE", ID: "i-bbb"}},
					},
				})
			},
		},
		{
			name:            "aws_ecr_container",
			aggregationType: "AWS_ECR_CONTAINER",
			unionKey:        "awsEcrContainerAggregation",
			keyField:        "resourceId",
			wantGroups:      map[string]int64{"sha256:aaa": 2, "sha256:bbb": 1},
			seed: func(t *testing.T, b *inspector2.InMemoryBackend) {
				t.Helper()
				seedAggregationFindings(t, b, []inspector2.Finding{
					{
						Severity:  inspector2.FindingSeverity{Label: "CRITICAL"},
						Resources: []inspector2.FindingResource{{Type: "AWS_ECR_CONTAINER_IMAGE", ID: "sha256:aaa"}},
					},
					{
						Severity:  inspector2.FindingSeverity{Label: "HIGH"},
						Resources: []inspector2.FindingResource{{Type: "AWS_ECR_CONTAINER_IMAGE", ID: "sha256:aaa"}},
					},
					{
						Severity:  inspector2.FindingSeverity{Label: "LOW"},
						Resources: []inspector2.FindingResource{{Type: "AWS_ECR_CONTAINER_IMAGE", ID: "sha256:bbb"}},
					},
				})
			},
		},
		{
			name:            "aws_lambda_function",
			aggregationType: "AWS_LAMBDA_FUNCTION",
			unionKey:        "lambdaFunctionAggregation",
			keyField:        "resourceId",
			wantGroups:      map[string]int64{"fn-a": 2, "fn-b": 1},
			seed: func(t *testing.T, b *inspector2.InMemoryBackend) {
				t.Helper()
				seedAggregationFindings(t, b, []inspector2.Finding{
					{
						Severity:  inspector2.FindingSeverity{Label: "CRITICAL"},
						Resources: []inspector2.FindingResource{{Type: "AWS_LAMBDA_FUNCTION", ID: "fn-a"}},
					},
					{
						Severity:  inspector2.FindingSeverity{Label: "HIGH"},
						Resources: []inspector2.FindingResource{{Type: "AWS_LAMBDA_FUNCTION", ID: "fn-a"}},
					},
					{
						Severity:  inspector2.FindingSeverity{Label: "LOW"},
						Resources: []inspector2.FindingResource{{Type: "AWS_LAMBDA_FUNCTION", ID: "fn-b"}},
					},
				})
			},
		},
		{
			name:            "code_repository",
			aggregationType: "CODE_REPOSITORY",
			unionKey:        "codeRepositoryAggregation",
			keyField:        "projectNames",
			wantGroups:      map[string]int64{"proj-a": 2, "proj-b": 1},
			seed: func(t *testing.T, b *inspector2.InMemoryBackend) {
				t.Helper()
				seedAggregationFindings(t, b, []inspector2.Finding{
					{
						Severity:  inspector2.FindingSeverity{Label: "CRITICAL"},
						Resources: []inspector2.FindingResource{{Type: "CODE_REPOSITORY", ID: "proj-a"}},
					},
					{
						Severity:  inspector2.FindingSeverity{Label: "HIGH"},
						Resources: []inspector2.FindingResource{{Type: "CODE_REPOSITORY", ID: "proj-a"}},
					},
					{
						Severity:  inspector2.FindingSeverity{Label: "LOW"},
						Resources: []inspector2.FindingResource{{Type: "CODE_REPOSITORY", ID: "proj-b"}},
					},
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := inspector2.NewInMemoryBackend(testAccountID, "us-east-1")

			empty, err := b.ListFindingAggregations(tc.aggregationType, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.aggregationType, empty["aggregationType"])
			assert.Empty(t, empty["responses"], "no findings seeded yet")

			tc.seed(t, b)

			got, err := b.ListFindingAggregations(tc.aggregationType, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.aggregationType, got["aggregationType"])

			responses, ok := got["responses"].([]map[string]any)
			require.True(t, ok, "responses type: %T", got["responses"])
			require.Len(t, responses, len(tc.wantGroups), "group count")

			assert.Equal(t, tc.wantGroups, aggregationGroupCounts(t, responses, tc.unionKey, tc.keyField))
		})
	}
}

// TestListFindingAggregations_UnimplementedTypesIgnoreResourceData pins that
// aggregation types this backend still has no data model for (PACKAGE, AMI,
// IMAGE_LAYER, LAMBDA_LAYER, FINDING_TYPE) stay honestly empty even when the
// seeded findings carry title and resource data that OTHER aggregation types
// now group by -- the unimplemented paths must not accidentally pick up
// that data.
func TestListFindingAggregations_UnimplementedTypesIgnoreResourceData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		aggregationType string
	}{
		{name: "package", aggregationType: "PACKAGE"},
		{name: "ami", aggregationType: "AMI"},
		{name: "image_layer", aggregationType: "IMAGE_LAYER"},
		{name: "lambda_layer", aggregationType: "LAMBDA_LAYER"},
		{name: "finding_type", aggregationType: "FINDING_TYPE"},
		{name: "container_image", aggregationType: "CONTAINER_IMAGE"},
		{name: "serverless_function", aggregationType: "SERVERLESS_FUNCTION"},
		{name: "vm_instance", aggregationType: "VM_INSTANCE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := inspector2.NewInMemoryBackend(testAccountID, "us-east-1")
			seedAggregationFindings(t, b, []inspector2.Finding{
				{
					Title:    "Title A",
					Severity: inspector2.FindingSeverity{Label: "CRITICAL"},
					Resources: []inspector2.FindingResource{
						{Type: "AWS_EC2_INSTANCE", ID: "i-aaa"},
						{Type: "AWS_ECR_REPOSITORY", ID: "repo-a"},
						{Type: "AWS_LAMBDA_FUNCTION", ID: "fn-a"},
						{Type: "CODE_REPOSITORY", ID: "proj-a"},
					},
				},
			})

			got, err := b.ListFindingAggregations(tc.aggregationType, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.aggregationType, got["aggregationType"])
			assert.Empty(t, got["responses"])
		})
	}
}
