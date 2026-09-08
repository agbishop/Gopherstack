package shield_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/shield"
)

func TestHandler_CreateSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "success",
			wantStatus: http.StatusOK,
		},
		{
			name:       "idempotent when already subscribed",
			wantStatus: http.StatusOK,
		},
	}

	h := newTestHandler(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doShieldRequest(t, h, "CreateSubscription", map[string]any{})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_DescribeSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*shield.Handler)
		name       string
		wantField  string
		wantStatus int
	}{
		{
			name: "no subscription returns not found",
			setup: func(_ *shield.Handler) {
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "after subscription returns details",
			setup: func(h *shield.Handler) {
				_ = h.Backend.CreateSubscription()
			},
			wantStatus: http.StatusOK,
			wantField:  "Subscription",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.setup(h)

			rec := doShieldRequest(t, h, "DescribeSubscription", map[string]any{})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantField != "" {
				var result map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
				assert.Contains(t, result, tt.wantField)
			}
		})
	}
}

// TestHandler_DescribeSubscription_TimeCommitmentInSeconds verifies the wire
// field is TimeCommitmentInSeconds (seconds), matching aws-sdk-go-v2's
// types.Subscription.TimeCommitmentInSeconds -- not the fabricated
// TimeCommitmentInDays key/unit gopherstack used to emit.
func TestHandler_DescribeSubscription_TimeCommitmentInSeconds(t *testing.T) {
	t.Parallel()

	const wantSeconds = 365 * 86400

	h := newTestHandler(t)
	require.NoError(t, h.Backend.CreateSubscription())

	rec := doShieldRequest(t, h, "DescribeSubscription", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var result map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))

	sub, ok := result["Subscription"].(map[string]any)
	require.True(t, ok, "response must include a Subscription object")

	_, hasDaysField := sub["TimeCommitmentInDays"]
	assert.False(t, hasDaysField, "wire response must not use the fabricated TimeCommitmentInDays key")

	got, ok := sub["TimeCommitmentInSeconds"]
	require.True(t, ok, "wire response must include TimeCommitmentInSeconds")
	assert.InDelta(t, float64(wantSeconds), got, 0)
}

func TestHandler_GetSubscriptionState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(*shield.Handler)
		wantState string
	}{
		{
			name:      "inactive when no subscription",
			setup:     func(_ *shield.Handler) {},
			wantState: "INACTIVE",
		},
		{
			name: "active after subscription",
			setup: func(h *shield.Handler) {
				_ = h.Backend.CreateSubscription()
			},
			wantState: "ACTIVE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.setup(h)

			rec := doShieldRequest(t, h, "GetSubscriptionState", map[string]any{})
			assert.Equal(t, http.StatusOK, rec.Code)

			var result map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			assert.Equal(t, tt.wantState, result["SubscriptionState"])
		})
	}
}

// TestHandler_DeleteSubscription tests DeleteSubscription.
func TestHandler_DeleteSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*shield.Handler)
		name       string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *shield.Handler) {
				require.NoError(t, h.Backend.CreateSubscription())
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "no active subscription",
			setup:      func(_ *shield.Handler) {},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.setup(h)

			rec := doShieldRequest(t, h, "DeleteSubscription", map[string]any{})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestAudit_Gap2_DescribeSubscriptionFields verifies extended subscription response fields.
func TestHandler_DescribeSubscriptionFields(t *testing.T) {
	t.Parallel()

	b := shield.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b.CreateSubscription())
	h := shield.NewHandler(b)

	rec := doShieldRequest(t, h, "DescribeSubscription", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	sub := resp["Subscription"].(map[string]any)
	assert.NotNil(t, sub["ProactiveEngagementStatus"])
	assert.NotNil(t, sub["Limits"])
	assert.NotNil(t, sub["SubscriptionLimits"])
	assert.NotNil(t, sub["SubscriptionArn"])

	// Gap 22: SubscriptionArn must NOT have trailing path.
	subArn, _ := sub["SubscriptionArn"].(string)
	assert.True(
		t,
		strings.HasSuffix(subArn, ":subscription"),
		"SubscriptionArn %q should end with :subscription",
		subArn,
	)
	assert.NotContains(t, subArn, "subscription/", "SubscriptionArn %q should not have trailing path", subArn)
}

// TestAudit_Gap2_SubscriptionLimitsStructure verifies SubscriptionLimits structure.
func TestHandler_SubscriptionLimitsStructure(t *testing.T) {
	t.Parallel()

	b := shield.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b.CreateSubscription())
	h := shield.NewHandler(b)

	rec := doShieldRequest(t, h, "DescribeSubscription", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	sub := resp["Subscription"].(map[string]any)
	limits := sub["SubscriptionLimits"].(map[string]any)
	assert.NotNil(t, limits["ProtectionLimits"])
	assert.NotNil(t, limits["ProtectionGroupLimits"])
}

// --- Gap 3: CreateSubscription sets ProactiveEngagementStatus to DISABLED ---

// TestAudit_Gap13_SubscriptionTimestampIsFloat verifies subscription timestamps are floats.
func TestHandler_SubscriptionTimestampIsFloat(t *testing.T) {
	t.Parallel()

	b := shield.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b.CreateSubscription())
	h := shield.NewHandler(b)

	rec := doShieldRequest(t, h, "DescribeSubscription", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

	sub := raw["Subscription"].(map[string]any)
	st, ok := sub["StartTime"].(float64)
	assert.True(t, ok, "StartTime should be float64")
	assert.Greater(t, st, float64(1e9), "StartTime should be a real epoch second")
}

// TestAudit_Gap22_SubscriptionArnFormat verifies correct ARN format (no trailing path).
func TestHandler_SubscriptionArnFormat(t *testing.T) {
	t.Parallel()

	b := shield.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b.CreateSubscription())
	h := shield.NewHandler(b)

	rec := doShieldRequest(t, h, "DescribeSubscription", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	sub := resp["Subscription"].(map[string]any)
	arn := sub["SubscriptionArn"].(string)

	assert.Equal(t, "arn:aws:shield::000000000000:subscription", arn)
}

// --- Gap 23: Reset is atomic ---

// TestRefinement1_HTTPCreateSubscription tests via HTTP.
func TestHandler_CreateSubscriptionBasic(t *testing.T) {
	t.Parallel()

	h := shield.NewHandler(shield.NewInMemoryBackend("000000000000", "us-east-1"))
	rec := doShieldRequest(t, h, "CreateSubscription", nil)
	assert.Equal(t, 200, rec.Code)
}

// TestRefinement1_HTTPGetSubscriptionState tests via HTTP.
func TestHandler_GetSubscriptionStateBasic(t *testing.T) {
	t.Parallel()

	h := shield.NewHandler(shield.NewInMemoryBackend("000000000000", "us-east-1"))
	rec := doShieldRequest(t, h, "GetSubscriptionState", nil)
	assert.Equal(t, 200, rec.Code)
}

// TestRefinement1_HTTPDescribeSubscriptionNotFound tests 404 when no subscription.
func TestHandler_DescribeSubscriptionNotFound(t *testing.T) {
	t.Parallel()

	h := shield.NewHandler(shield.NewInMemoryBackend("000000000000", "us-east-1"))
	rec := doShieldRequest(t, h, "DescribeSubscription", nil)
	assert.Equal(t, 400, rec.Code)
}

// TestRefinement1_HTTPUpdateSubscription tests via HTTP.
func TestHandler_UpdateSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*shield.InMemoryBackend)
		body       map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "disable_auto_renew",
			setup: func(b *shield.InMemoryBackend) {
				require.NoError(t, b.CreateSubscription())
			},
			body:       map[string]any{"AutoRenew": shield.AutoRenewDisabled},
			wantStatus: 200,
		},
		{
			name:       "invalid_auto_renew_value",
			setup:      func(*shield.InMemoryBackend) {},
			body:       map[string]any{"AutoRenew": "MAYBE"},
			wantStatus: 400,
		},
		{
			// api_op_UpdateSubscription.go: "Only enter values for parameters you want to
			// change. Empty parameters are not updated." An omitted AutoRenew must succeed
			// as a no-op, not a validation error.
			name: "omit_auto_renew_succeeds",
			setup: func(b *shield.InMemoryBackend) {
				require.NoError(t, b.CreateSubscription())
			},
			body:       map[string]any{},
			wantStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := shield.NewInMemoryBackend("000000000000", "us-east-1")
			tt.setup(b)
			h := shield.NewHandler(b)
			rec := doShieldRequest(t, h, "UpdateSubscription", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_UpdateSubscriptionOmitAutoRenewPreservesExistingValue proves the omitted-field
// no-op leaves the prior AutoRenew value intact rather than clobbering it with the empty string.
func TestHandler_UpdateSubscriptionOmitAutoRenewPreservesExistingValue(t *testing.T) {
	t.Parallel()

	b := shield.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b.CreateSubscription())
	h := shield.NewHandler(b)

	rec := doShieldRequest(t, h, "UpdateSubscription", map[string]any{"AutoRenew": shield.AutoRenewDisabled})
	require.Equal(t, 200, rec.Code)

	rec = doShieldRequest(t, h, "UpdateSubscription", map[string]any{})
	require.Equal(t, 200, rec.Code)

	rec = doShieldRequest(t, h, "DescribeSubscription", nil)
	require.Equal(t, 200, rec.Code)

	var result map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	sub, ok := result["Subscription"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, shield.AutoRenewDisabled, sub["AutoRenew"])
}

// TestRefinement1_DescribeSubscriptionIncludesArn verifies SubscriptionArn in response.
func TestHandler_DescribeSubscriptionIncludesArn(t *testing.T) {
	t.Parallel()

	b := shield.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b.CreateSubscription())

	h := shield.NewHandler(b)
	rec := doShieldRequest(t, h, "DescribeSubscription", nil)
	require.Equal(t, 200, rec.Code)

	var result map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	sub, ok := result["Subscription"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, sub, "SubscriptionArn")
}
