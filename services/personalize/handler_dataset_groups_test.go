package personalize_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/personalize"
)

func TestPersonalize_DatasetGroup_CRUD(t *testing.T) {
	t.Parallel()

	h := personalizeHandler(t)

	// Create
	rec := personalizeDo(t, h, "CreateDatasetGroup", map[string]any{
		"name":   "my-group",
		"domain": "ECOMMERCE",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	created := personalizeUnmarshal(t, rec)
	dgArn, _ := created["datasetGroupArn"].(string)
	assert.NotEmpty(t, dgArn)
	assert.Equal(t, "ECOMMERCE", created["domain"])

	// Describe
	rec = personalizeDo(t, h, "DescribeDatasetGroup", map[string]any{"datasetGroupArn": dgArn})
	require.Equal(t, http.StatusOK, rec.Code)
	described := personalizeUnmarshal(t, rec)
	dg := described["datasetGroup"].(map[string]any)
	assert.Equal(t, "my-group", dg["name"])
	assert.Equal(t, "ECOMMERCE", dg["domain"])
	assert.Equal(t, "ACTIVE", dg["status"])
	assert.NotEmpty(t, dg["creationDateTime"])
	assert.NotEmpty(t, dg["lastUpdatedDateTime"])

	// List
	rec = personalizeDo(t, h, "ListDatasetGroups", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)
	listed := personalizeUnmarshal(t, rec)
	groups := listed["datasetGroups"].([]any)
	assert.Len(t, groups, 1)

	// Delete
	rec = personalizeDo(t, h, "DeleteDatasetGroup", map[string]any{"datasetGroupArn": dgArn})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify deleted
	rec = personalizeDo(t, h, "DescribeDatasetGroup", map[string]any{"datasetGroupArn": dgArn})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	errBody := personalizeUnmarshal(t, rec)
	assert.Equal(t, "ResourceNotFoundException", errBody["__type"])
}

func TestPersonalize_DatasetGroup_AlreadyExists(t *testing.T) {
	t.Parallel()

	h := personalizeHandler(t)
	personalizeDo(t, h, "CreateDatasetGroup", map[string]any{"name": "dup-group"})
	rec := personalizeDo(t, h, "CreateDatasetGroup", map[string]any{"name": "dup-group"})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	m := personalizeUnmarshal(t, rec)
	assert.Equal(t, "ResourceAlreadyExistsException", m["__type"])
}

// TestPersonalize_DatasetGroup_InvalidDomain locks that CreateDatasetGroup
// rejects a domain outside the real types.Domain enum
// (ECOMMERCE/VIDEO_ON_DEMAND). An empty/omitted domain remains valid (it
// creates a Custom, rather than Domain, dataset group).
func TestPersonalize_DatasetGroup_InvalidDomain(t *testing.T) {
	t.Parallel()

	h := personalizeHandler(t)

	rec := personalizeDo(t, h, "CreateDatasetGroup", map[string]any{
		"name":   "bad-domain-group",
		"domain": "NOT_A_REAL_DOMAIN",
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	m := personalizeUnmarshal(t, rec)
	assert.Equal(t, "InvalidInputException", m["__type"])
}

// TestPersonalize_DatasetGroup_DeleteInUse locks that DeleteDatasetGroup
// rejects a group that still has a dataset, a solution, or an event tracker
// (api_op_DeleteDatasetGroup.go: "Before you delete a dataset group, you
// must delete the following: All associated event trackers. All associated
// solutions. All datasets in the dataset group.").
func TestPersonalize_DatasetGroup_DeleteInUse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		seed func(t *testing.T, h *personalize.Handler, dgArn string)
		name string
	}{
		{
			name: "dataset",
			seed: func(t *testing.T, h *personalize.Handler, dgArn string) {
				t.Helper()
				schemaArn := personalizeCreateSchema(t, h, "dg-in-use-ds-schema")
				rec := personalizeDo(t, h, "CreateDataset", map[string]any{
					"name":            "dg-in-use-ds",
					"datasetGroupArn": dgArn,
					"datasetType":     "INTERACTIONS",
					"schemaArn":       schemaArn,
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "solution",
			seed: func(t *testing.T, h *personalize.Handler, dgArn string) {
				t.Helper()
				rec := personalizeDo(t, h, "CreateSolution", map[string]any{
					"name":            "dg-in-use-sol",
					"datasetGroupArn": dgArn,
					"recipeArn":       "arn:aws:personalize:::recipe/aws-user-personalization",
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "event_tracker",
			seed: func(t *testing.T, h *personalize.Handler, dgArn string) {
				t.Helper()
				rec := personalizeDo(t, h, "CreateEventTracker", map[string]any{
					"name":            "dg-in-use-et",
					"datasetGroupArn": dgArn,
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := personalizeHandler(t)
			dgArn := personalizeCreateDatasetGroup(t, h, "dg-in-use-"+tt.name)
			tt.seed(t, h, dgArn)

			rec := personalizeDo(t, h, "DeleteDatasetGroup", map[string]any{"datasetGroupArn": dgArn})
			require.Equal(t, http.StatusBadRequest, rec.Code)
			m := personalizeUnmarshal(t, rec)
			assert.Equal(t, "ResourceInUseException", m["__type"])
		})
	}
}

// --- Dataset ---
