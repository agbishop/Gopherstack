package ssm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ssmsdk "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/kms"
	"github.com/blackbirdworks/gopherstack/services/ssm"
)

func TestErrNilAppContext(t *testing.T) {
	t.Parallel()

	p := &ssm.Provider{}
	_, err := p.Init(nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ssm.ErrNilAppContext)
}

// TestProviderInit verifies normal provider init.
func TestProviderInit(t *testing.T) {
	t.Parallel()

	p := &ssm.Provider{}
	ctx := &service.AppContext{JanitorCtx: t.Context()}
	reg, err := p.Init(ctx)
	require.NoError(t, err)
	assert.NotNil(t, reg)
}

// TestStorageBackendInterface verifies var_ assertion compiles.
func TestStorageBackendInterface(t *testing.T) {
	t.Parallel()

	var _ ssm.StorageBackend = (*ssm.InMemoryBackend)(nil)
}

// TestHandlerOpsLen verifies the number of supported operations.
func TestHandlerOpsLen(t *testing.T) {
	t.Parallel()

	h := ssm.NewHandler(ssm.NewInMemoryBackend())
	assert.Len(t, h.GetSupportedOperations(), 152)
}

// TestSDKOpsSorted verifies GetSupportedOperations is sorted.
func TestSDKOpsSorted(t *testing.T) {
	t.Parallel()

	h := ssm.NewHandler(ssm.NewInMemoryBackend())
	ops := h.GetSupportedOperations()

	require.NotEmpty(t, ops)

	for i := 1; i < len(ops); i++ {
		assert.LessOrEqual(t, ops[i-1], ops[i],
			"ops not sorted at index %d: %s > %s", i, ops[i-1], ops[i])
	}
}

type testKMSAdapter struct {
	b *kms.InMemoryBackend
}

func (a *testKMSAdapter) EncryptSSM(keyID string, plaintext []byte) ([]byte, error) {
	out, err := a.b.Encrypt(context.Background(), &kms.EncryptInput{KeyID: keyID, Plaintext: plaintext})
	if err != nil {
		return nil, err
	}

	return out.CiphertextBlob, nil
}
func (a *testKMSAdapter) DecryptSSM(ciphertext []byte) ([]byte, error) {
	out, err := a.b.Decrypt(context.Background(), &kms.DecryptInput{CiphertextBlob: ciphertext})
	if err != nil {
		return nil, err
	}

	return out.Plaintext, nil
}

// newSSMWithKMS creates an SSM backend wired to a real KMS backend.
// Returns the SSM backend and the created key ID.
func newSSMWithKMS(t *testing.T) (*ssm.InMemoryBackend, string) {
	t.Helper()
	kmsBackend := kms.NewInMemoryBackend()
	keyOut, err := kmsBackend.CreateKey(t.Context(), &kms.CreateKeyInput{Description: "test key"})
	require.NoError(t, err)
	keyID := keyOut.KeyMetadata.KeyID

	ssmBackend := ssm.NewInMemoryBackend()
	ssmBackend.WithKMS(&testKMSAdapter{b: kmsBackend})

	return ssmBackend, keyID
}
func putTestParam(t *testing.T, b *ssm.InMemoryBackend, name string) {
	t.Helper()

	_, err := b.PutParameter(context.Background(), &ssm.PutParameterInput{
		Name:  name,
		Value: "v",
		Type:  "String",
	})
	require.NoError(t, err)
}
func TestParameterExpiration_JanitorEvicts(t *testing.T) {
	t.Parallel()

	t.Run("past-expiry parameter is deleted by janitor", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		// Attach an Expiration policy with a timestamp in the past.
		pastTS := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02T15:04:05.000Z")
		policies := fmt.Sprintf(
			`[{"Type":"Expiration","Version":"1.0","Attributes":{"Timestamp":%q}}]`,
			pastTS,
		)

		_, err := b.PutParameter(ctx, &ssm.PutParameterInput{
			Name:     "/expire/past",
			Value:    "gone-soon",
			Type:     "String",
			Tier:     "Advanced",
			Policies: policies,
		})
		require.NoError(t, err)

		// Confirm it exists.
		_, err = b.GetParameter(ctx, &ssm.GetParameterInput{Name: "/expire/past"})
		require.NoError(t, err)

		// Run the janitor sweep.
		j := ssm.NewJanitor(b, time.Minute)
		j.SweepOnce(ctx)

		// Parameter must be gone.
		_, err = b.GetParameter(ctx, &ssm.GetParameterInput{Name: "/expire/past"})
		assert.ErrorIs(t, err, ssm.ErrParameterNotFound,
			"past-expiry parameter must be evicted by janitor")
	})

	t.Run("future-expiry parameter is kept by janitor", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		futureTS := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
		policies := fmt.Sprintf(
			`[{"Type":"Expiration","Version":"1.0","Attributes":{"Timestamp":%q}}]`,
			futureTS,
		)

		_, err := b.PutParameter(ctx, &ssm.PutParameterInput{
			Name:     "/expire/future",
			Value:    "still-here",
			Type:     "String",
			Tier:     "Advanced",
			Policies: policies,
		})
		require.NoError(t, err)

		j := ssm.NewJanitor(b, time.Minute)
		j.SweepOnce(ctx)

		_, err = b.GetParameter(ctx, &ssm.GetParameterInput{Name: "/expire/future"})
		require.NoError(t, err, "future-expiry parameter must survive janitor sweep")
	})

	t.Run("parameter without policy is not evicted", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		_, err := b.PutParameter(ctx, &ssm.PutParameterInput{
			Name:  "/no-policy",
			Value: "permanent",
			Type:  "String",
		})
		require.NoError(t, err)

		j := ssm.NewJanitor(b, time.Minute)
		j.SweepOnce(ctx)

		_, err = b.GetParameter(ctx, &ssm.GetParameterInput{Name: "/no-policy"})
		require.NoError(t, err, "parameter without expiry policy must not be evicted")
	})

	t.Run("RFC3339 timestamp format also works", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		ctx := context.Background()

		pastTS := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
		policies := fmt.Sprintf(
			`[{"Type":"Expiration","Version":"1.0","Attributes":{"Timestamp":%q}}]`,
			pastTS,
		)

		_, err := b.PutParameter(ctx, &ssm.PutParameterInput{
			Name:     "/expire/rfc3339",
			Value:    "gone",
			Type:     "String",
			Tier:     "Advanced",
			Policies: policies,
		})
		require.NoError(t, err)

		j := ssm.NewJanitor(b, time.Minute)
		j.SweepOnce(ctx)

		_, err = b.GetParameter(ctx, &ssm.GetParameterInput{Name: "/expire/rfc3339"})
		assert.ErrorIs(t, err, ssm.ErrParameterNotFound)
	})
}
func Test_PutParameter_HierarchyLevelLimit(t *testing.T) {
	t.Parallel()

	fifteenLevels := "/" + strings.Join(makeLevels(14), "/") + "/name" // 14 + name = 15
	sixteenLevels := "/" + strings.Join(makeLevels(15), "/") + "/name" // 15 + name = 16

	cases := []struct {
		name      string
		paramName string
		wantErr   bool
	}{
		{name: "exactly 15 levels is valid", paramName: fifteenLevels, wantErr: false},
		{name: "16 levels exceeds the limit", paramName: sixteenLevels, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			ctx := context.Background()

			_, err := b.PutParameter(ctx, &ssm.PutParameterInput{
				Name:  tc.paramName,
				Type:  ssm.StringType,
				Value: "v",
			})

			if tc.wantErr {
				require.ErrorIs(t, err, ssm.ErrHierarchyLevelLimitExceeded)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Test_PutParameter_IntelligentTiering_AutoUpgradesToAdvanced verifies that,
// per AWS docs, Intelligent-Tiering parameters are auto-promoted to the
// Advanced tier whenever the request needs a capability Standard doesn't
// support (value over 4 KiB, or parameter policies attached) instead of the
// PutParameter call failing.
func Test_PutParameter_IntelligentTiering_AutoUpgradesToAdvanced(t *testing.T) {
	t.Parallel()

	smallValue := strings.Repeat("a", 100)
	overStandardValue := strings.Repeat("a", 5000) // >4096, <=8192

	cases := []struct {
		name     string
		tier     string
		value    string
		wantTier string
		wantErr  bool
	}{
		{
			// AWS echoes back the requested "Intelligent-Tiering" tier as-is
			// when no capability forces a promotion — it does not resolve to
			// the concrete underlying "Standard" tier in the response.
			name:     "small value on Intelligent-Tiering reports Intelligent-Tiering",
			tier:     "Intelligent-Tiering",
			value:    smallValue,
			wantTier: "Intelligent-Tiering",
		},
		{
			name:     "value over 4KiB on Intelligent-Tiering auto-upgrades to Advanced",
			tier:     "Intelligent-Tiering",
			value:    overStandardValue,
			wantTier: "Advanced",
		},
		{
			name:    "value over 4KiB on explicit Standard tier fails",
			tier:    "Standard",
			value:   overStandardValue,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			ctx := context.Background()

			out, err := b.PutParameter(ctx, &ssm.PutParameterInput{
				Name:  "/tier/param",
				Type:  ssm.StringType,
				Value: tc.value,
				Tier:  tc.tier,
			})

			if tc.wantErr {
				require.ErrorIs(t, err, ssm.ErrValidationException)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantTier, out.Tier)
		})
	}
}

// Test_PutParameter_PoliciesRequireAdvancedTier verifies AWS's documented
// constraint that parameter policies (Expiration, ExpirationNotification,
// NoChangeNotification) are only supported on Advanced-tier parameters;
// Standard tier rejects them, and Intelligent-Tiering auto-upgrades to
// Advanced to accommodate them.
func Test_PutParameter_PoliciesRequireAdvancedTier(t *testing.T) {
	t.Parallel()

	const policy = `[{"Type":"Expiration","Version":"1.0","Attributes":{"Timestamp":"2099-01-01T00:00:00.000Z"}}]`

	cases := []struct {
		name     string
		tier     string
		wantTier string
		wantErr  bool
	}{
		{name: "default (Standard) tier rejects policies", tier: "", wantErr: true},
		{name: "explicit Standard tier rejects policies", tier: "Standard", wantErr: true},
		{name: "Advanced tier accepts policies", tier: "Advanced", wantTier: "Advanced"},
		{
			name:     "Intelligent-Tiering auto-upgrades for policies",
			tier:     "Intelligent-Tiering",
			wantTier: "Advanced",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			ctx := context.Background()

			out, err := b.PutParameter(ctx, &ssm.PutParameterInput{
				Name:     "/policy/param",
				Type:     ssm.StringType,
				Value:    "v",
				Tier:     tc.tier,
				Policies: policy,
			})

			if tc.wantErr {
				require.ErrorIs(t, err, ssm.ErrValidationException)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantTier, out.Tier)
		})
	}
}

// TestDescribeParameters_PoliciesWireShape proves ParameterMetadata.Policies
// is real, structured types.ParameterInlinePolicy objects (types/types.go:
// 4840-4857), not the raw PutParameterInput.Policies request string echoed
// verbatim -- a real aws-sdk-go-v2 client's json.Unmarshal into
// []types.ParameterInlinePolicy would fail entirely against a JSON string
// where it expects an array of objects.
func TestDescribeParameters_PoliciesWireShape(t *testing.T) {
	t.Parallel()

	client := newTestSSMClient(t, ssm.NewHandler(ssm.NewInMemoryBackend()))

	policy := `[{"Type":"Expiration","Version":"1.0","Attributes":{"Timestamp":"2099-01-01T00:00:00.000Z"}}]`

	_, err := client.PutParameter(t.Context(), &ssmsdk.PutParameterInput{
		Name:     aws.String("/policy/wire"),
		Type:     ssmtypes.ParameterTypeString,
		Value:    aws.String("v"),
		Tier:     ssmtypes.ParameterTierAdvanced,
		Policies: aws.String(policy),
	})
	require.NoError(t, err)

	out, err := client.DescribeParameters(t.Context(), &ssmsdk.DescribeParametersInput{})
	require.NoError(t, err)
	require.Len(t, out.Parameters, 1)
	require.Len(t, out.Parameters[0].Policies, 1)
	assert.Equal(t, "Expiration", *out.Parameters[0].Policies[0].PolicyType)
	assert.Equal(t, "Finished", *out.Parameters[0].Policies[0].PolicyStatus)
	assert.Contains(t, *out.Parameters[0].Policies[0].PolicyText, "Expiration")
}

// TestGetParameterHistory_PoliciesWireShape proves ParameterHistory.Policies
// (api_op_GetParameterHistory.go/types.ParameterHistory) is real, structured
// types.ParameterInlinePolicy objects, not the raw PutParameterInput.Policies
// request string -- GetParameterHistory's real wire member had NO Go struct
// member at all before this fix, so every historical entry silently dropped
// the policies attached to it.
func TestGetParameterHistory_PoliciesWireShape(t *testing.T) {
	t.Parallel()

	client := newTestSSMClient(t, ssm.NewHandler(ssm.NewInMemoryBackend()))

	policy := `[{"Type":"Expiration","Version":"1.0","Attributes":{"Timestamp":"2099-01-01T00:00:00.000Z"}}]`

	_, err := client.PutParameter(t.Context(), &ssmsdk.PutParameterInput{
		Name:     aws.String("/policy/history"),
		Type:     ssmtypes.ParameterTypeString,
		Value:    aws.String("v"),
		Tier:     ssmtypes.ParameterTierAdvanced,
		Policies: aws.String(policy),
	})
	require.NoError(t, err)

	out, err := client.GetParameterHistory(t.Context(), &ssmsdk.GetParameterHistoryInput{
		Name: aws.String("/policy/history"),
	})
	require.NoError(t, err)
	require.Len(t, out.Parameters, 1)
	require.Len(t, out.Parameters[0].Policies, 1)
	assert.Equal(t, "Expiration", *out.Parameters[0].Policies[0].PolicyType)
}

// TestDeleteParameter_RegionCleanup verifies that deleting the last parameter
// in a region removes the empty inner maps so they do not linger indefinitely.
func TestDeleteParameter_RegionCleanup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		deleteVia  string // "single" or "batch"
		paramCount int
	}{
		{name: "single delete cleans region", deleteVia: "single", paramCount: 1},
		{name: "batch delete cleans region", deleteVia: "batch", paramCount: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			b := ssm.NewInMemoryBackend()

			names := make([]string, tc.paramCount)
			for i := range tc.paramCount {
				name := "/cleanup/p"
				if tc.paramCount > 1 {
					name = "/cleanup/p" + string(rune('0'+i))
				}
				names[i] = name
				_, err := b.PutParameter(ctx, &ssm.PutParameterInput{
					Name:  name,
					Type:  ssm.StringType,
					Value: "v",
				})
				require.NoError(t, err)
			}

			switch tc.deleteVia {
			case "single":
				_, err := b.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: names[0]})
				require.NoError(t, err)
			case "batch":
				_, err := b.DeleteParameters(ctx, &ssm.DeleteParametersInput{Names: names})
				require.NoError(t, err)
			}

			// After deleting all params, no tag entry should survive either.
			for _, name := range names {
				assert.False(t, b.HasTagEntry(name),
					"tag entry must be cleaned up after DeleteParameter")
			}

			// HistoryLen must be zero — the history inner map entry was cleaned up.
			for _, name := range names {
				assert.Equal(t, 0, b.HistoryLen(name))
			}
		})
	}
}
func postJSON(t *testing.T, h *ssm.Handler, op string, body any) (int, map[string]any) {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err)

	rec := doRequest(t, h, op, string(payload))

	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)

	return rec.Code, out
}
func newHandler() *ssm.Handler {
	return ssm.NewHandler(ssm.NewInMemoryBackend())
}
func TestPutParameter_Tier_Standard(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, out := postJSON(t, h, "PutParameter", map[string]any{
		"Name":  "/app/tier-standard",
		"Type":  "String",
		"Value": "hello",
		"Tier":  "Standard",
	})

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "Standard", out["Tier"])
}
func TestPutParameter_Tier_Advanced(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, out := postJSON(t, h, "PutParameter", map[string]any{
		"Name":  "/app/tier-advanced",
		"Type":  "String",
		"Value": "hello",
		"Tier":  "Advanced",
	})

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "Advanced", out["Tier"])
}
func TestPutParameter_Tier_IntelligentTiering(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, out := postJSON(t, h, "PutParameter", map[string]any{
		"Name":  "/app/tier-intelligent",
		"Type":  "String",
		"Value": "hello",
		"Tier":  "Intelligent-Tiering",
	})

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "Intelligent-Tiering", out["Tier"])
}
func TestPutParameter_Tier_DefaultIsStandard(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, out := postJSON(t, h, "PutParameter", map[string]any{
		"Name":  "/app/no-tier",
		"Type":  "String",
		"Value": "hello",
	})

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "Standard", out["Tier"])
}
func TestPutParameter_Tier_InvalidRejected(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, _ := postJSON(t, h, "PutParameter", map[string]any{
		"Name":  "/app/bad-tier",
		"Type":  "String",
		"Value": "hello",
		"Tier":  "Gold",
	})

	assert.Equal(t, http.StatusBadRequest, code)
}
func TestPutParameter_Standard_SizeLimit(t *testing.T) {
	t.Parallel()
	h := newHandler()

	bigValue := strings.Repeat("x", 4097)
	code, out := postJSON(t, h, "PutParameter", map[string]any{
		"Name":  "/app/big",
		"Type":  "String",
		"Value": bigValue,
		"Tier":  "Standard",
	})

	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, out["message"], "4096")
}
func TestPutParameter_Advanced_SizeFits(t *testing.T) {
	t.Parallel()
	h := newHandler()

	bigValue := strings.Repeat("x", 5000)
	code, _ := postJSON(t, h, "PutParameter", map[string]any{
		"Name":  "/app/big-advanced",
		"Type":  "String",
		"Value": bigValue,
		"Tier":  "Advanced",
	})

	assert.Equal(t, http.StatusOK, code)
}
func TestPutParameter_Advanced_SizeLimit(t *testing.T) {
	t.Parallel()
	h := newHandler()

	tooBig := strings.Repeat("x", 8193)
	code, _ := postJSON(t, h, "PutParameter", map[string]any{
		"Name":  "/app/too-big-advanced",
		"Type":  "String",
		"Value": tooBig,
		"Tier":  "Advanced",
	})

	assert.Equal(t, http.StatusBadRequest, code)
}
func TestPutParameter_AllowedPattern_Match(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, _ := postJSON(t, h, "PutParameter", map[string]any{
		"Name":           "/app/pattern-ok",
		"Type":           "String",
		"Value":          "abc123",
		"AllowedPattern": "^[a-z0-9]+$",
	})

	assert.Equal(t, http.StatusOK, code)
}
func TestPutParameter_AllowedPattern_Mismatch(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, out := postJSON(t, h, "PutParameter", map[string]any{
		"Name":           "/app/pattern-bad",
		"Type":           "String",
		"Value":          "ABC!!!",
		"AllowedPattern": "^[a-z0-9]+$",
	})

	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, out["message"], "AllowedPattern")
	// InvalidAllowedPatternException, not the generic ValidationException:
	// it is PutParameter's own declared exception for this
	// (ssm@v1.73.4 deserializers.go:13880, gopherstack-jpfk).
	assert.Equal(t, "InvalidAllowedPatternException", out["__type"])
}
func TestPutParameter_AllowedPattern_InvalidRegex(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, out := postJSON(t, h, "PutParameter", map[string]any{
		"Name":           "/app/bad-regex",
		"Type":           "String",
		"Value":          "v",
		"AllowedPattern": "[invalid(",
	})

	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "InvalidAllowedPatternException", out["__type"])
}
func TestPutParameter_Type_Unsupported(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, out := postJSON(t, h, "PutParameter", map[string]any{
		"Name":  "/app/bad-type",
		"Type":  "Bogus",
		"Value": "v",
	})

	assert.Equal(t, http.StatusBadRequest, code)
	// UnsupportedParameterType, not the generic ValidationException: it is
	// PutParameter's own declared exception for this
	// (ssm@v1.73.4 deserializers.go:13910, gopherstack-jpfk).
	assert.Equal(t, "UnsupportedParameterType", out["__type"])
}
func TestPutParameter_DataType_Text(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, _ := postJSON(t, h, "PutParameter", map[string]any{
		"Name":     "/app/dt-text",
		"Type":     "String",
		"Value":    "hello",
		"DataType": "text",
	})

	assert.Equal(t, http.StatusOK, code)
}
func TestPutParameter_DataType_EC2Image(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, _ := postJSON(t, h, "PutParameter", map[string]any{
		"Name":     "/app/dt-ec2",
		"Type":     "String",
		"Value":    "ami-0abcdef1234567890",
		"DataType": "aws:ec2:image",
	})

	assert.Equal(t, http.StatusOK, code)
}
func TestPutParameter_DataType_Invalid(t *testing.T) {
	t.Parallel()
	h := newHandler()

	code, out := postJSON(t, h, "PutParameter", map[string]any{
		"Name":     "/app/dt-bad",
		"Type":     "String",
		"Value":    "v",
		"DataType": "bogus:type",
	})

	assert.Equal(t, http.StatusBadRequest, code)
	// No declared PutParameter exception fits an invalid DataType (checked
	// ssm@v1.73.4 deserializers.go); ValidationException stays deliberately.
	assert.Equal(t, "ValidationException", out["__type"])
}

// TestSSMHandler_Reset verifies the reset method clears state.
func TestSSMHandler_Reset(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	_, err := b.PutParameter(context.TODO(), &ssm.PutParameterInput{
		Name:  "/test/reset-param",
		Type:  "String",
		Value: "value",
	})
	require.NoError(t, err)

	h.Reset()
}
func TestSSMBackend_GetParameters_MissingParam(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	_, _ = b.PutParameter(context.TODO(), &ssm.PutParameterInput{
		Name:  "/exists",
		Type:  "String",
		Value: "val",
	})

	out, err := b.GetParameters(context.TODO(), &ssm.GetParametersInput{
		Names: []string{"/exists", "/does-not-exist"},
	})
	require.NoError(t, err)
	assert.Len(t, out.Parameters, 1)
	assert.Equal(t, "/exists", out.Parameters[0].Name)
	assert.Len(t, out.InvalidParameters, 1)
	assert.Equal(t, "/does-not-exist", out.InvalidParameters[0])
}
func TestSSMBackend_DeleteParameter_NotFound(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	_, err := b.DeleteParameter(context.TODO(), &ssm.DeleteParameterInput{Name: "/nonexistent"})
	require.Error(t, err)
	require.ErrorIs(t, err, ssm.ErrParameterNotFound)
}
func TestSSMBackend_DeleteParameter_Success(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	_, _ = b.PutParameter(context.TODO(), &ssm.PutParameterInput{
		Name:  "/to-delete",
		Type:  "String",
		Value: "val",
	})

	_, err := b.DeleteParameter(context.TODO(), &ssm.DeleteParameterInput{Name: "/to-delete"})
	require.NoError(t, err)

	_, err = b.GetParameter(context.TODO(), &ssm.GetParameterInput{Name: "/to-delete"})
	require.ErrorIs(t, err, ssm.ErrParameterNotFound)
}
