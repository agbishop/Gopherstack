package shield_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/shield"
)

// wireErrorType decodes the __type field from an error response body.
func wireErrorType(t *testing.T, body []byte) string {
	t.Helper()

	var env struct {
		Type string `json:"__type"`
	}
	require.NoError(t, json.Unmarshal(body, &env))

	return env.Type
}

// TestHandler_ErrorWireType_SubscriptionRequired verifies that an operation requiring an active
// Shield Advanced subscription reports the real Shield "InvalidOperationException" __type, not
// "ResourceAlreadyExistsException". ErrSubscriptionRequired wraps awserr.ErrConflict internally
// for backward-compatible errors.Is matching, which previously caused handleError's generic
// ErrConflict rule to shadow it and misreport the wire type.
func TestHandler_ErrorWireType_SubscriptionRequired(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doShieldRequest(t, h, "CreateProtection", map[string]any{
		"Name":        "prot",
		"ResourceArn": eipARN("1"),
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "InvalidOperationException", wireErrorType(t, rec.Body.Bytes()))
}

// TestHandler_ErrorWireType_InvalidPaginationToken verifies a malformed NextToken reports the
// real Shield "InvalidPaginationTokenException" __type at 400, not a 500 InternalErrorException.
func TestHandler_ErrorWireType_InvalidPaginationToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.NoError(t, h.Backend.CreateSubscription())

	rec := doShieldRequest(t, h, "ListProtections", map[string]any{
		"NextToken": "not-valid-base64url!!!",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "InvalidPaginationTokenException", wireErrorType(t, rec.Body.Bytes()))
}

// TestHandler_ErrorWireType_LimitsExceeded verifies a quota violation reports the real Shield
// "LimitsExceededException" __type.
func TestHandler_ErrorWireType_LimitsExceeded(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.NoError(t, h.Backend.CreateSubscription())

	const maxPerType = 100

	for i := range maxPerType {
		rec := doShieldRequest(t, h, "CreateProtection", map[string]any{
			"Name":        fmt.Sprintf("prot-%d", i),
			"ResourceArn": eipARN(strconv.Itoa(i)),
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec := doShieldRequest(t, h, "CreateProtection", map[string]any{
		"Name":        "one-too-many",
		"ResourceArn": "arn:aws:ec2:us-east-1::eip-allocation/eipalloc-over-the-limit",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "LimitsExceededException", wireErrorType(t, rec.Body.Bytes()))
}

// TestHandler_ErrorWireType_NoAssociatedRole verifies AssociateDRTLogBucket without a prior
// AssociateDRTRole reports the real Shield "NoAssociatedRoleException" __type.
func TestHandler_ErrorWireType_NoAssociatedRole(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.NoError(t, h.Backend.CreateSubscription())

	rec := doShieldRequest(t, h, "AssociateDRTLogBucket", map[string]any{"LogBucket": "my-bucket"})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "NoAssociatedRoleException", wireErrorType(t, rec.Body.Bytes()))
}

// TestHandler_ErrorWireType_CreateProtectionGroupSubscriptionRequired verifies that, unlike
// CreateProtection, CreateProtectionGroup without an active subscription reports the real Shield
// "ResourceNotFoundException" __type, not "InvalidOperationException" -- CreateProtectionGroup's
// error catalog (deserializers.go's deserializeOpErrorCreateProtectionGroup) declares
// ResourceAlreadyExistsException/ResourceNotFoundException/LimitsExceededException/
// OptimisticLockException/InvalidParameterException, but no InvalidOperationException at all
// (gopherstack-g2l5).
func TestHandler_ErrorWireType_CreateProtectionGroupSubscriptionRequired(t *testing.T) {
	t.Parallel()

	b := shield.NewInMemoryBackend(testAccountID, testRegion)
	h := shield.NewHandler(b)

	rec := doShieldRequest(t, h, "CreateProtectionGroup", map[string]any{
		"ProtectionGroupId": "grp-1",
		"Aggregation":       "SUM",
		"Pattern":           "ALL",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	wireType := wireErrorType(t, rec.Body.Bytes())
	assert.Equal(t, "ResourceNotFoundException", wireType)
	assert.NotEqual(t, "InvalidOperationException", wireType)
	assert.Equal(t, 0, shield.ProtectionGroupCount(b), "no protection group should be created")
}

// TestHandler_ErrorWireType_TagResourceSubscriptionRequired verifies that, unlike CreateProtection,
// TagResource without an active subscription reports the real Shield "ResourceNotFoundException"
// __type, not "InvalidOperationException" -- TagResource's error catalog (deserializers.go's
// deserializeOpErrorTagResource) declares InvalidResourceException/InvalidParameterException/
// ResourceNotFoundException, but no InvalidOperationException at all (gopherstack-g2l5). Also
// verifies the target protection's tags are left unmutated by the rejected request.
func TestHandler_ErrorWireType_TagResourceSubscriptionRequired(t *testing.T) {
	t.Parallel()

	b := shield.NewInMemoryBackend(testAccountID, testRegion)
	h := shield.NewHandler(b)
	p := b.AddProtectionInternal("prot", eipARN("1"))

	rec := doShieldRequest(t, h, "TagResource", map[string]any{
		"ResourceARN": p.ProtectionArn,
		"Tags":        []map[string]string{{"Key": "k", "Value": "v"}},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	wireType := wireErrorType(t, rec.Body.Bytes())
	assert.Equal(t, "ResourceNotFoundException", wireType)
	assert.NotEqual(t, "InvalidOperationException", wireType)

	got, err := b.DescribeProtection(p.ID, "")
	require.NoError(t, err)
	assert.Empty(t, got.Tags, "tags should not be applied when the request is rejected")
}

// TestHandler_ErrorWireType_ListAttacksInvalidPaginationToken verifies that, unlike ListProtections,
// a malformed NextToken on ListAttacks reports the real Shield "InvalidParameterException" __type,
// not "InvalidPaginationTokenException" -- ListAttacks's error catalog (deserializers.go's
// deserializeOpErrorListAttacks) declares InvalidOperationException/InvalidParameterException, but
// no InvalidPaginationTokenException at all, unlike ListProtections/ListProtectionGroups/
// ListResourcesInProtectionGroup which do declare it (gopherstack-g2l5).
func TestHandler_ErrorWireType_ListAttacksInvalidPaginationToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.NoError(t, h.Backend.CreateSubscription())

	rec := doShieldRequest(t, h, "ListAttacks", map[string]any{
		"NextToken": "not-valid-base64url!!!",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	wireType := wireErrorType(t, rec.Body.Bytes())
	assert.Equal(t, "InvalidParameterException", wireType)
	assert.NotEqual(t, "InvalidPaginationTokenException", wireType)
}

// TestHandler_ErrorWireType_UpdateProtectionGroupMembersLimit verifies that, unlike
// CreateProtectionGroup, exceeding the ARBITRARY-pattern member cap on UpdateProtectionGroup
// reports the real Shield "InvalidParameterException" __type, not "LimitsExceededException" --
// UpdateProtectionGroup's error catalog (deserializers.go's
// deserializeOpErrorUpdateProtectionGroup) declares InvalidParameterException/
// OptimisticLockException/ResourceNotFoundException, but no LimitsExceededException at all
// (gopherstack-g2l5). Also verifies the group's members are left unmutated by the rejected update.
func TestHandler_ErrorWireType_UpdateProtectionGroupMembersLimit(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.NoError(t, h.Backend.CreateSubscription())

	rec := doShieldRequest(t, h, "CreateProtectionGroup", map[string]any{
		"ProtectionGroupId": "grp-1",
		"Aggregation":       "SUM",
		"Pattern":           "ARBITRARY",
		"Members":           []string{eipARN("1")},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	const maxMembers = 10000

	members := make([]string, maxMembers+1)
	for i := range members {
		members[i] = eipARN(strconv.Itoa(i))
	}

	rec = doShieldRequest(t, h, "UpdateProtectionGroup", map[string]any{
		"ProtectionGroupId": "grp-1",
		"Aggregation":       "SUM",
		"Pattern":           "ARBITRARY",
		"Members":           members,
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	wireType := wireErrorType(t, rec.Body.Bytes())
	assert.Equal(t, "InvalidParameterException", wireType)
	assert.NotEqual(t, "LimitsExceededException", wireType)

	pg, err := h.Backend.DescribeProtectionGroup("grp-1")
	require.NoError(t, err)
	assert.Equal(t, []string{eipARN("1")}, pg.Members, "members should not be updated when the request is rejected")
}
