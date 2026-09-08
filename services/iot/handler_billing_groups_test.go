package iot_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewOps_BillingGroup tests BillingGroup CRUD.
func TestBillingGroup(t *testing.T) {
	t.Parallel()
	h := newIoTHandlerBatch1(t)

	// CreateBillingGroup
	out := iotOK(t, h, http.MethodPost, "/billing-groups/my-group", map[string]any{
		"billingGroupProperties": map[string]any{
			"billingGroupDescription": "test group",
		},
	})
	if out["billingGroupName"] != "my-group" {
		t.Errorf("billingGroupName mismatch: %v", out)
	}

	// DescribeBillingGroup
	out2 := iotOK(t, h, http.MethodGet, "/billing-groups/my-group", nil)
	if out2["billingGroupName"] != "my-group" {
		t.Errorf("describe mismatch: %v", out2)
	}

	// ListBillingGroups
	out3 := iotOK(t, h, http.MethodGet, "/billing-groups", nil)
	groups, _ := out3["billingGroups"].([]any)
	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}

	// UpdateBillingGroup
	out4 := iotOK(t, h, http.MethodPatch, "/billing-groups/my-group", map[string]any{
		"billingGroupProperties": map[string]any{
			"billingGroupDescription": "updated",
		},
	})
	if out4["version"] == nil {
		t.Error("expected version in update response")
	}

	// DeleteBillingGroup
	iotOK(t, h, http.MethodDelete, "/billing-groups/my-group", nil)

	iotExpectError(t, h, "/billing-groups/my-group")
}

// UpdateBillingGroupInput.ExpectedVersion (iot@v1.77.4/api_op_UpdateBillingGroup.go:38-41):
// "If the version of the billing group does not match the expected version
// specified in the request, the UpdateBillingGroup request is rejected with
// a VersionConflictException." expectedVersion is a BODY field
// (awsRestjson1_serializeOpDocumentUpdateBillingGroupInput).
func TestBillingGroup_UpdateVersionConflict_Rejected(t *testing.T) {
	t.Parallel()
	h := newIoTHandlerBatch1(t)

	iotOK(t, h, http.MethodPost, "/billing-groups/ver-group", map[string]any{
		"billingGroupProperties": map[string]any{"billingGroupDescription": "original"},
	})

	rec := iotRequest(t, h, http.MethodPatch, "/billing-groups/ver-group", map[string]any{
		"billingGroupProperties": map[string]any{"billingGroupDescription": "should not apply"},
		"expectedVersion":        99,
	})
	require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())

	var errBody struct {
		Type string `json:"__type"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	assert.Equal(t, "VersionConflictException", errBody.Type)

	out := iotOK(t, h, http.MethodGet, "/billing-groups/ver-group", nil)
	props, _ := out["billingGroupProperties"].(map[string]any)
	assert.Equal(t, "original", props["billingGroupDescription"],
		"rejected update must not have applied")
}

// DeleteBillingGroupInput.ExpectedVersion (iot@v1.77.4/api_op_DeleteBillingGroup.go:38-41),
// expectedVersion is a QUERY parameter
// (awsRestjson1_serializeOpHttpBindingsDeleteBillingGroupInput).
func TestBillingGroup_DeleteVersionConflict_Rejected(t *testing.T) {
	t.Parallel()
	h := newIoTHandlerBatch1(t)

	iotOK(t, h, http.MethodPost, "/billing-groups/del-ver-group", map[string]any{
		"billingGroupProperties": map[string]any{"billingGroupDescription": "keep-me"},
	})

	rec := iotRequest(t, h, http.MethodDelete, "/billing-groups/del-ver-group?expectedVersion=99", nil)
	require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())

	iotOK(t, h, http.MethodGet, "/billing-groups/del-ver-group", nil)
}
