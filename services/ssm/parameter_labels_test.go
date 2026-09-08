package ssm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ssm"
)

func TestParameterLabels(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	// Create a parameter first
	_, err := b.PutParameter(
		context.TODO(),
		&ssm.PutParameterInput{Name: "/my/param", Value: "val", Type: "String"},
	)
	require.NoError(t, err)

	// Label it
	rec := doRequest(t, h, "LabelParameterVersion", `{"Name":"/my/param","Labels":["v1","stable"]}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assertBodyContains(t, rec, "ParameterVersion")

	// Unlabel
	rec = doRequest(t, h, "UnlabelParameterVersion", `{"Name":"/my/param","Labels":["v1"]}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assertBodyContains(t, rec, "RemovedLabels")
}
func TestLabelParameterVersion_MaxTenLabels(t *testing.T) {
	t.Parallel()

	t.Run("exactly 10 labels accepted", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		putTestParam(t, b, "/p/max10")

		labels := make([]string, 10)
		for i := range labels {
			labels[i] = fmt.Sprintf("label%02d", i)
		}

		out, err := b.LabelParameterVersion(context.Background(), &ssm.LabelParameterVersionInput{
			Name:   "/p/max10",
			Labels: labels,
		})
		require.NoError(t, err)
		assert.Empty(t, out.InvalidLabels, "no labels should be invalid at exactly 10")

		hist, err := b.GetParameterHistory(
			context.Background(),
			&ssm.GetParameterHistoryInput{Name: "/p/max10"},
		)
		require.NoError(t, err)
		require.Len(t, hist.Parameters, 1)
		assert.ElementsMatch(t, labels, hist.Parameters[0].Labels)
	})

	t.Run("11th label rejected as InvalidLabel", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		putTestParam(t, b, "/p/overflow")

		// Add 10 labels first.
		first10 := make([]string, 10)
		for i := range first10 {
			first10[i] = fmt.Sprintf("lab%02d", i)
		}

		_, err := b.LabelParameterVersion(context.Background(), &ssm.LabelParameterVersionInput{
			Name:   "/p/overflow",
			Labels: first10,
		})
		require.NoError(t, err)

		// Adding one more must come back as InvalidLabel.
		out, err := b.LabelParameterVersion(context.Background(), &ssm.LabelParameterVersionInput{
			Name:   "/p/overflow",
			Labels: []string{"overflow"},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"overflow"}, out.InvalidLabels)

		hist, err := b.GetParameterHistory(
			context.Background(),
			&ssm.GetParameterHistoryInput{Name: "/p/overflow"},
		)
		require.NoError(t, err)
		require.Len(t, hist.Parameters, 1)
		assert.NotContains(
			t,
			hist.Parameters[0].Labels,
			"overflow",
			"overflow label must not be attached",
		)
	})

	t.Run("duplicate labels not double-counted toward limit", func(t *testing.T) {
		t.Parallel()

		b := ssm.NewInMemoryBackend()
		putTestParam(t, b, "/p/dup")

		_, err := b.LabelParameterVersion(context.Background(), &ssm.LabelParameterVersionInput{
			Name:   "/p/dup",
			Labels: []string{"stable", "prod"},
		})
		require.NoError(t, err)

		// Re-applying the same labels must not cause InvalidLabels.
		out, err := b.LabelParameterVersion(context.Background(), &ssm.LabelParameterVersionInput{
			Name:   "/p/dup",
			Labels: []string{"stable", "prod"},
		})
		require.NoError(t, err)
		assert.Empty(t, out.InvalidLabels, "re-applying existing labels must not overflow")
	})

	t.Run("handler response omits the invented AddedLabels field", func(t *testing.T) {
		t.Parallel()

		h, b := newTestHandler(t)
		putTestParam(t, b, "/p/handler")

		rec := doRequest(t, h, "LabelParameterVersion",
			`{"Name":"/p/handler","Labels":["alpha","beta"]}`)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "ParameterVersion")
		assert.NotContains(
			t,
			rec.Body.String(),
			"AddedLabels",
			"AddedLabels is not a real LabelParameterVersionOutput field (see aws-sdk-go-v2 api_op_LabelParameterVersion.go)",
		)
	})
}

// Test_PutParameter_MaxVersionLimit_LabeledOldestBlocksEviction verifies AWS
// behavior: Parameter Store keeps only the most recent MaxHistoryCap (100)
// versions, auto-deleting the oldest on each new PutParameter. But if the
// oldest version carries a label, AWS refuses to delete it and the PutParameter
// call fails with ParameterMaxVersionLimitExceeded instead of silently
// evicting a labeled version.
func Test_PutParameter_MaxVersionLimit_LabeledOldestBlocksEviction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		labelOldest bool
		wantErr     bool
	}{
		{name: "unlabeled oldest version is evicted normally", labelOldest: false, wantErr: false},
		{name: "labeled oldest version blocks the new write", labelOldest: true, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			ctx := context.Background()

			const paramName = "/history/param"

			// Fill history to exactly MaxHistoryCap versions.
			for range ssm.MaxHistoryCap {
				_, err := b.PutParameter(ctx, &ssm.PutParameterInput{
					Name:      paramName,
					Type:      ssm.StringType,
					Value:     "v",
					Overwrite: true,
				})
				require.NoError(t, err)
			}

			if tc.labelOldest {
				_, err := b.LabelParameterVersion(ctx, &ssm.LabelParameterVersionInput{
					Name:             paramName,
					ParameterVersion: 1, // the oldest surviving version
					Labels:           []string{"pinned"},
				})
				require.NoError(t, err)
			}

			// One more write would push total versions to MaxHistoryCap+1,
			// requiring eviction of version 1.
			_, err := b.PutParameter(ctx, &ssm.PutParameterInput{
				Name:      paramName,
				Type:      ssm.StringType,
				Value:     "one-more",
				Overwrite: true,
			})

			if tc.wantErr {
				require.ErrorIs(t, err, ssm.ErrParameterMaxVersionLimitExceeded)

				// The rejected write must not have mutated state: history stays
				// at MaxHistoryCap (paginated at 50/page) and the current
				// version is unchanged.
				assert.Equal(t, ssm.MaxHistoryCap, countParameterHistory(ctx, t, b, paramName))
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestLabelParameterVersion_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		paramName string
	}{
		{
			name:      "non_existent_parameter_returns_error",
			paramName: "/does/not/exist",
		},
		{
			name:      "another_non_existent_parameter",
			paramName: "ghost-param",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			_, err := b.LabelParameterVersion(context.TODO(), &ssm.LabelParameterVersionInput{
				Name:   tt.paramName,
				Labels: []string{"test-label"},
			})
			require.ErrorIs(t, err, ssm.ErrParameterNotFound,
				"labelling non-existent parameter must return ErrParameterNotFound")
		})
	}
}
func TestLabelParameterVersion_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		labels []string
	}{
		{
			name:   "single_label_applied",
			labels: []string{"my-label"},
		},
		{
			name:   "multiple_labels_applied",
			labels: []string{"alpha", "beta"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)

			_, err := b.PutParameter(context.TODO(), &ssm.PutParameterInput{
				Name:  "/test/label-param",
				Value: "myvalue",
				Type:  "String",
			})
			require.NoError(t, err)

			out, err := b.LabelParameterVersion(context.TODO(), &ssm.LabelParameterVersionInput{
				Name:   "/test/label-param",
				Labels: tt.labels,
			})
			require.NoError(t, err)
			assert.Empty(t, out.InvalidLabels)

			hist, err := b.GetParameterHistory(
				context.TODO(), &ssm.GetParameterHistoryInput{Name: "/test/label-param"},
			)
			require.NoError(t, err)
			require.Len(t, hist.Parameters, 1)
			assert.ElementsMatch(t, tt.labels, hist.Parameters[0].Labels)
		})
	}
}
func TestLabelParameterVersion_Handler_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "non_existent_param_returns_400",
			body: `{"Name":"/no/such/param","Labels":["lbl"]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			rec := doRequest(t, h, "LabelParameterVersion", tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "ParameterNotFound")
		})
	}
}
func TestFull_ParameterStore_LabelParameterVersion(t *testing.T) {
	t.Parallel()
	h := newHandler()

	postJSON(
		t,
		h,
		"PutParameter",
		map[string]any{"Name": "/lbl/p", "Type": "String", "Value": "v1"},
	)

	code, out := postJSON(t, h, "LabelParameterVersion", map[string]any{
		"Name":             "/lbl/p",
		"ParameterVersion": int64(1),
		"Labels":           []string{"prod", "stable"},
	})
	assert.Equal(t, http.StatusOK, code)
	assert.InDelta(t, float64(1), out["ParameterVersion"], 0)
	_, hasAddedLabels := out["AddedLabels"]
	assert.False(
		t,
		hasAddedLabels,
		"AddedLabels is not a real LabelParameterVersionOutput field (see aws-sdk-go-v2 api_op_LabelParameterVersion.go)",
	)

	_, hist := postJSON(t, h, "GetParameterHistory", map[string]any{"Name": "/lbl/p"})
	params := hist["Parameters"].([]any)
	require.Len(t, params, 1)
	assert.ElementsMatch(t, []any{"prod", "stable"}, params[0].(map[string]any)["Labels"])
}
func TestFull_ParameterStore_UnlabelParameterVersion(t *testing.T) {
	t.Parallel()
	h := newHandler()

	postJSON(
		t,
		h,
		"PutParameter",
		map[string]any{"Name": "/unlbl/p", "Type": "String", "Value": "v"},
	)
	postJSON(t, h, "LabelParameterVersion", map[string]any{
		"Name":             "/unlbl/p",
		"ParameterVersion": int64(1),
		"Labels":           []string{"prod", "stable"},
	})

	code, out := postJSON(t, h, "UnlabelParameterVersion", map[string]any{
		"Name":             "/unlbl/p",
		"ParameterVersion": int64(1),
		"Labels":           []string{"prod"},
	})
	assert.Equal(t, http.StatusOK, code)
	removed := out["RemovedLabels"].([]any)
	assert.Len(t, removed, 1)
	assert.Equal(t, "prod", removed[0])
}
func TestParameterLabels_VersionSpecific(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	// Create parameter (version 1).
	_, err := b.PutParameter(
		context.TODO(),
		&ssm.PutParameterInput{Name: "/app/key", Value: "v1", Type: "String"},
	)
	require.NoError(t, err)

	// Update to create version 2.
	_, err = b.PutParameter(
		context.TODO(),
		&ssm.PutParameterInput{Name: "/app/key", Value: "v2", Type: "String", Overwrite: true},
	)
	require.NoError(t, err)

	// Label version 1 with "stable".
	body, _ := json.Marshal(map[string]any{
		"Name":             "/app/key",
		"Labels":           []string{"stable"},
		"ParameterVersion": 1,
	})
	rec := doRequest(t, h, "LabelParameterVersion", string(body))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ParameterVersion")

	// Label version 2 with "latest".
	body, _ = json.Marshal(map[string]any{
		"Name":             "/app/key",
		"Labels":           []string{"latest"},
		"ParameterVersion": 2,
	})
	rec = doRequest(t, h, "LabelParameterVersion", string(body))
	require.Equal(t, http.StatusOK, rec.Code)

	// History should show labels per version.
	rec = doRequest(t, h, "GetParameterHistory", `{"Name":"/app/key"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "stable")
	assert.Contains(t, rec.Body.String(), "latest")
}
func TestParameterLabels_GhostAfterDeleteAndRecreate(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	ctx := context.Background()

	_, err := b.PutParameter(ctx, &ssm.PutParameterInput{
		Name: "/app/ghost", Value: "v1", Type: "String",
	})
	require.NoError(t, err)

	_, err = b.LabelParameterVersion(ctx, &ssm.LabelParameterVersionInput{
		Name:   "/app/ghost",
		Labels: []string{"prod"},
	})
	require.NoError(t, err)

	_, err = b.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: "/app/ghost"})
	require.NoError(t, err)

	assert.False(t, b.HasParameterLabelEntry("/app/ghost"),
		"parameter label entry must be cleaned up after DeleteParameter")

	_, err = b.PutParameter(ctx, &ssm.PutParameterInput{
		Name: "/app/ghost", Value: "v2", Type: "String",
	})
	require.NoError(t, err)

	// "prod" was never labeled on this new incarnation of the parameter --
	// resolving it must fail like any other unknown label, not silently
	// return version 1 of the recreated parameter.
	_, err = b.GetParameter(ctx, &ssm.GetParameterInput{Name: "/app/ghost:prod"})
	require.ErrorIs(t, err, ssm.ErrParameterNotFound)
}

func TestParameterLabels_UnlabelSpecificVersion(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	_, err := b.PutParameter(
		context.TODO(),
		&ssm.PutParameterInput{Name: "/app/ver", Value: "v1", Type: "String"},
	)
	require.NoError(t, err)

	// Add labels to version 1.
	body, _ := json.Marshal(map[string]any{
		"Name":             "/app/ver",
		"Labels":           []string{"prod", "stable"},
		"ParameterVersion": 1,
	})
	doRequest(t, h, "LabelParameterVersion", string(body))

	// Remove one label.
	body, _ = json.Marshal(map[string]any{
		"Name":             "/app/ver",
		"Labels":           []string{"prod"},
		"ParameterVersion": 1,
	})
	rec := doRequest(t, h, "UnlabelParameterVersion", string(body))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "RemovedLabels")

	// History should still have "stable" but not "prod".
	rec = doRequest(t, h, "GetParameterHistory", `{"Name":"/app/ver"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "stable")
	assert.NotContains(t, rec.Body.String(), `"prod"`)
}
