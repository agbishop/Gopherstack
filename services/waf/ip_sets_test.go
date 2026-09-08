package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func wafCreateIPSet(t *testing.T, h *waf.Handler, name string) string {
	t.Helper()

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateIPSet", map[string]any{
		"ChangeToken": token,
		"Name":        name,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	m := resp["IPSet"].(map[string]any)
	id := m["IPSetId"].(string)
	require.NotEmpty(t, id)

	return id
}

func TestWAF_IPSet_CreateGetUpdateDeleteList(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	id := wafCreateIPSet(t, h, "my-ips")
	assert.Equal(t, 1, waf.IPSetCount(h.Backend.(*waf.InMemoryBackend)))

	// Get - empty descriptors
	rec := wafDo(t, h, "GetIPSet", map[string]any{"IPSetId": id})
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ipSet := resp["IPSet"].(map[string]any)
	assert.Equal(t, "my-ips", ipSet["Name"])
	descs := ipSet["IPSetDescriptors"].([]any)
	assert.Empty(t, descs)

	// Update: insert descriptor
	token := wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateIPSet", map[string]any{
		"ChangeToken": token,
		"IPSetId":     id,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"IPSetDescriptor": map[string]any{
					"Type":  "IPV4",
					"Value": "192.0.2.0/24",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify
	rec = wafDo(t, h, "GetIPSet", map[string]any{"IPSetId": id})
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ipSet = resp["IPSet"].(map[string]any)
	descs = ipSet["IPSetDescriptors"].([]any)
	require.Len(t, descs, 1)
	desc := descs[0].(map[string]any)
	assert.Equal(t, "IPV4", desc["Type"])
	assert.Equal(t, "192.0.2.0/24", desc["Value"])

	// Delete descriptor
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateIPSet", map[string]any{
		"ChangeToken": token,
		"IPSetId":     id,
		"Updates": []map[string]any{
			{
				"Action": "DELETE",
				"IPSetDescriptor": map[string]any{
					"Type":  "IPV4",
					"Value": "192.0.2.0/24",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = wafDo(t, h, "GetIPSet", map[string]any{"IPSetId": id})
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ipSet = resp["IPSet"].(map[string]any)
	descs = ipSet["IPSetDescriptors"].([]any)
	assert.Empty(t, descs)

	// List
	rec = wafDo(t, h, "ListIPSets", map[string]any{"Limit": 100})
	require.Equal(t, http.StatusOK, rec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	sets := listResp["IPSets"].([]any)
	assert.Len(t, sets, 1)

	// Delete
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "DeleteIPSet", map[string]any{
		"ChangeToken": token,
		"IPSetId":     id,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, waf.IPSetCount(h.Backend.(*waf.InMemoryBackend)))
}

func TestWAF_IPSet_IPv6Descriptor(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	id := wafCreateIPSet(t, h, "ipv6-set")

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "UpdateIPSet", map[string]any{
		"ChangeToken": token,
		"IPSetId":     id,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"IPSetDescriptor": map[string]any{
					"Type":  "IPV6",
					"Value": "2001:db8::/32",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = wafDo(t, h, "GetIPSet", map[string]any{"IPSetId": id})
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ipSet := resp["IPSet"].(map[string]any)
	descs := ipSet["IPSetDescriptors"].([]any)
	require.Len(t, descs, 1)
	assert.Equal(t, "IPV6", descs[0].(map[string]any)["Type"])
}

func TestWAF_MultipleIPSets_List(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	wafCreateIPSet(t, h, "ips-1")
	wafCreateIPSet(t, h, "ips-2")

	rec := wafDo(t, h, "ListIPSets", map[string]any{"Limit": 100})
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	sets := resp["IPSets"].([]any)
	assert.Len(t, sets, 2)
}

func TestWAF_IPSet_NoOpUpdatesRejected(t *testing.T) {
	t.Parallel()

	descriptor := map[string]any{"Type": "IPV4", "Value": "203.0.113.0/24"}

	t.Run("insert_duplicate_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateIPSet(t, h, "noop-insert-ipset")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateIPSet", map[string]any{
			"ChangeToken": token,
			"IPSetId":     id,
			"Updates":     []map[string]any{{"Action": "INSERT", "IPSetDescriptor": descriptor}},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		token = wafGetToken(t, h)
		rec = wafDo(t, h, "UpdateIPSet", map[string]any{
			"ChangeToken": token,
			"IPSetId":     id,
			"Updates":     []map[string]any{{"Action": "INSERT", "IPSetDescriptor": descriptor}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})

	t.Run("delete_missing_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateIPSet(t, h, "noop-delete-ipset")

		// The IPSet never contained this descriptor, so this DELETE is itself a no-op.
		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateIPSet", map[string]any{
			"ChangeToken": token,
			"IPSetId":     id,
			"Updates":     []map[string]any{{"Action": "DELETE", "IPSetDescriptor": descriptor}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})
}

func TestIPSet_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   func(token string) map[string]any
		name   string
		action string
	}{
		{
			name:   "GetIPSet",
			action: "GetIPSet",
			body:   func(string) map[string]any { return map[string]any{"IPSetId": "nonexistent"} },
		},
		{
			name:   "UpdateIPSet",
			action: "UpdateIPSet",
			body: func(token string) map[string]any {
				return map[string]any{"ChangeToken": token, "IPSetId": "nonexistent"}
			},
		},
		{
			name:   "DeleteIPSet",
			action: "DeleteIPSet",
			body: func(token string) map[string]any {
				return map[string]any{"ChangeToken": token, "IPSetId": "nonexistent"}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWAFHandler(t)
			token := wafGetToken(t, h)
			rec := wafDo(t, h, tc.action, tc.body(token))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}
