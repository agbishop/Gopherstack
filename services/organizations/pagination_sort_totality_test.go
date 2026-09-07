package organizations_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/organizations"
)

// walkAttempts mirrors cloudwatchlogs' pagination_sort_totality_test.go: Go
// randomises map iteration order per range, not per map instance, so a
// non-total sort over a store.Table map walk can (and reliably does)
// disagree with itself across separate calls with nothing changed in
// between. One walk can pass by luck; the bug is about instability *across*
// calls, so each case is repeated many times against the same, unchanged
// backend state.
const walkAttempts = 30

// walkAndVerify repeats a small-page paginated walk walkAttempts times,
// failing if any attempt drops or duplicates an item relative to want, or
// returns the same id on two different pages within one walk.
func walkAndVerify(t *testing.T, want map[string]bool, listPage func(token string) (ids []string, next string)) {
	t.Helper()

	for attempt := range walkAttempts {
		got := make(map[string]bool, len(want))
		token := ""

		for {
			ids, next := listPage(token)
			for _, id := range ids {
				require.Falsef(t, got[id], "attempt %d: id %q returned on more than one page", attempt, id)
				got[id] = true
			}

			if next == "" {
				break
			}

			token = next
		}

		require.Equalf(t, want, got, "attempt %d: paginated walk did not reproduce the created set exactly", attempt)
	}
}

// TestListPoliciesSortIsTotal covers ListPolicies, which sources its
// unsorted candidate set from InMemoryBackend.policies.All() (a
// store.Table map walk) and then sorts only by PolicySummary.Name.
// CreatePolicy enforces no name-uniqueness constraint (verified: real AWS
// Organizations does not require policy names to be unique either), so two
// policies of the same type can legitimately share a Name -- a tie the sort
// does not break, leaving relative order to depend on map-walk order, which
// varies across calls. page.New's index-based cursor assumes "all" is a
// fully sorted (i.e. stably ordered across calls) slice, so an unstable tie
// drops or duplicates a policy across a paginated walk's page boundary.
func TestListPoliciesSortIsTotal(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)
	h := organizations.NewHandler(b)

	want := make(map[string]bool, 4)

	// The default FullAWSAccess SCP every organization is created with also
	// shows up under this filter.
	existing, err := b.ListPolicies("SERVICE_CONTROL_POLICY")
	require.NoError(t, err)

	for _, p := range existing {
		want[p.PolicySummary.ID] = true
	}

	for range 3 {
		p, createErr := b.CreatePolicy("dup-name", "", `{"Version":"2012-10-17"}`, "SERVICE_CONTROL_POLICY", nil)
		require.NoError(t, createErr)
		want[p.PolicySummary.ID] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		rec := doRequest(t, h, "ListPolicies", map[string]any{
			"Filter":     "SERVICE_CONTROL_POLICY",
			"MaxResults": 1,
			"NextToken":  token,
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			NextToken string `json:"NextToken"`
			Policies  []struct {
				ID string `json:"Id"`
			} `json:"Policies"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		ids := make([]string, len(resp.Policies))
		for i, p := range resp.Policies {
			ids[i] = p.ID
		}

		return ids, resp.NextToken
	})
}

// TestListDelegatedAdministratorsOrderIsStableAcrossCalls covers the
// unfiltered branch of ListDelegatedAdministrators (ServicePrincipal == ""),
// which sources its candidate set from InMemoryBackend.delegatedAdmins.All()
// (a store.Table map walk keyed by servicePrincipal+accountID, NOT by
// AccountID alone) and sorted only by AccountID. A single account
// registered as delegated admin for multiple different service principals
// is real, reachable AWS behavior (RegisterDelegatedAdministrator only
// rejects a duplicate servicePrincipal+accountID pair, never a repeat
// AccountID across services) and produces multiple DelegatedAdmin rows tied
// on AccountID -- the same unstable-tie-across-map-walk class as
// TestListPoliciesSortIsTotal.
//
// The real DelegatedAdministrator wire type (organizations@v1.53.5
// types/types.go:192) has no ServicePrincipal member at all, so this can't
// be proven through the paginated wire response the way
// TestListPoliciesSortIsTotal proves its bug (every row for the same
// account is wire-indistinguishable by AccountID alone -- a client-visible
// duplicate-vs-drop count is unaffected either way, since the table always
// holds the same number of entries regardless of their random relative
// order). What page.New's index-based cursor actually requires -- "all" is
// a stably-ordered slice across separate calls -- is a backend-internal
// property, so this test asserts it directly against
// InMemoryBackend.ListDelegatedAdministrators's own return order (via the
// exported, if wire-excluded, ServicePrincipal field) rather than through
// HTTP pagination.
func TestListDelegatedAdministratorsOrderIsStableAcrossCalls(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	acct, err := b.CreateAccount("delegated-admin", "delegated-admin@example.com", "", "", nil)
	require.NoError(t, err)

	servicePrincipals := []string{
		"ram.amazonaws.com", "config.amazonaws.com", "guardduty.amazonaws.com", "securityhub.amazonaws.com",
	}

	for _, sp := range servicePrincipals {
		require.NoError(t, b.EnableAWSServiceAccess(sp))
		require.NoError(t, b.RegisterDelegatedAdministrator(acct.AccountID, sp))
	}

	first, err := b.ListDelegatedAdministrators("")
	require.NoError(t, err)
	require.Len(t, first, len(servicePrincipals))

	wantOrder := make([]string, len(first))
	for i, a := range first {
		wantOrder[i] = a.ServicePrincipal
	}

	for attempt := range walkAttempts {
		admins, errList := b.ListDelegatedAdministrators("")
		require.NoError(t, errList)
		require.Len(t, admins, len(servicePrincipals))

		gotOrder := make([]string, len(admins))
		for i, a := range admins {
			gotOrder[i] = a.ServicePrincipal
		}

		require.Equalf(t, wantOrder, gotOrder,
			"attempt %d: ListDelegatedAdministrators(\"\") returned a different relative order "+
				"than the first call with nothing changed in between -- page.New's index-based "+
				"cursor assumes this slice is stably sorted across calls", attempt)
	}
}
