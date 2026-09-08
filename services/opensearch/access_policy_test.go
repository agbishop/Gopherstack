package opensearch_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/opensearch"
)

const explicitDenyPolicy = `{
	"Version": "2012-10-17",
	"Statement": [
		{"Effect": "Deny", "Action": "es:*", "Resource": "*"}
	]
}`

const grantsNothingPolicy = `{
	"Version": "2012-10-17",
	"Statement": []
}`

const allowAllPolicy = `{
	"Version": "2012-10-17",
	"Statement": [
		{"Effect": "Allow", "Action": "es:*", "Resource": "*"}
	]
}`

// TestDocumentDataPlaneHonoursAccessPolicies proves that a domain's
// resource-based AccessPolicies actually gates the document data plane
// (IndexDocument/GetDocument/DeleteDocument/SearchIndex), not just Get/Describe
// echo. Before the fix, every case here indexed, fetched, searched and deleted
// documents regardless of AccessPolicies -- gopherstack-5hsd.
func TestDocumentDataPlaneHonoursAccessPolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		accessPolicies string
		wantDenied     bool
	}{
		{name: "explicit_deny_blocks", accessPolicies: explicitDenyPolicy, wantDenied: true},
		{name: "grants_nothing_blocks", accessPolicies: grantsNothingPolicy, wantDenied: true},
		{name: "allow_all_permits", accessPolicies: allowAllPolicy, wantDenied: false},
		{name: "unset_permits", accessPolicies: "", wantDenied: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := opensearch.NewInMemoryBackend(testAccountID, testRegion)
			_, err := b.CreateDomain(opensearch.CreateDomainInput{
				Name:           "dom",
				AccessPolicies: tt.accessPolicies,
			})
			require.NoError(t, err)
			_, err = b.CreateIndex("dom", "products", nil, nil, nil, nil)
			require.NoError(t, err)

			_, _, err = b.IndexDocument("dom", "products", "p1", map[string]any{"name": "widget"})
			assertAccessOutcome(t, err, tt.wantDenied)

			if tt.wantDenied {
				// The index gate fires before document existence is checked,
				// so these are still observable even though indexing above
				// was blocked and no document exists.
				_, getErr := b.GetDocument("dom", "products", "p1")
				assertAccessOutcome(t, getErr, true)

				_, searchErr := b.SearchIndex("dom", "products", nil, 0)
				assertAccessOutcome(t, searchErr, true)

				deleteErr := b.DeleteDocument("dom", "products", "p1")
				assertAccessOutcome(t, deleteErr, true)

				return
			}

			_, err = b.GetDocument("dom", "products", "p1")
			assertAccessOutcome(t, err, tt.wantDenied)

			_, err = b.SearchIndex("dom", "products", nil, 0)
			assertAccessOutcome(t, err, tt.wantDenied)

			err = b.DeleteDocument("dom", "products", "p1")
			assertAccessOutcome(t, err, tt.wantDenied)
		})
	}
}

func assertAccessOutcome(t *testing.T, err error, wantDenied bool) {
	t.Helper()

	if wantDenied {
		require.Error(t, err)
		assert.ErrorIs(t, err, opensearch.ErrAccessDenied)

		return
	}

	require.NoError(t, err)
}
