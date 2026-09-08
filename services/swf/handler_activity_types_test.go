package swf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/swf"
)

func TestHandler_DescribeActivityType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		setupFn  func(*swf.InMemoryBackend)
		name     string
		wantType string
		wantCode int
	}{
		{
			name: "found",
			setupFn: func(b *swf.InMemoryBackend) {
				b.AddActivityTypeInternal("d1", "act1", "1.0", "REGISTERED")
			},
			body:     map[string]any{"domain": "d1", "activityType": map[string]any{"name": "act1", "version": "1.0"}},
			wantCode: http.StatusOK,
			wantType: "REGISTERED",
		},
		{
			name: "not_found",
			body: map[string]any{
				"domain":       "d1",
				"activityType": map[string]any{"name": "missing", "version": "1.0"},
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := swf.NewInMemoryBackend()
			if tt.setupFn != nil {
				tt.setupFn(b)
			}

			h := swf.NewHandler(b)
			rec := doSWFRequest(t, h, "DescribeActivityType", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantType != "" {
				resp := parseSWFResp(t, rec)
				typeInfo, ok := resp["typeInfo"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantType, typeInfo["status"])
			}
		})
	}
}

func TestHandler_DeprecateActivityType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		setupFn  func(*swf.InMemoryBackend)
		name     string
		wantType string
		wantCode int
	}{
		{
			name: "success",
			setupFn: func(b *swf.InMemoryBackend) {
				b.AddActivityTypeInternal("d1", "act1", "1.0", "REGISTERED")
			},
			body:     map[string]any{"domain": "d1", "activityType": map[string]any{"name": "act1", "version": "1.0"}},
			wantCode: http.StatusOK,
		},
		{
			name: "not_found",
			body: map[string]any{
				"domain":       "d1",
				"activityType": map[string]any{"name": "missing", "version": "1.0"},
			},
			wantCode: http.StatusNotFound,
			wantType: "UnknownResourceFault",
		},
		{
			name: "already_deprecated",
			setupFn: func(b *swf.InMemoryBackend) {
				b.AddActivityTypeInternal("d1", "act-dep", "1.0", "DEPRECATED")
			},
			body: map[string]any{
				"domain":       "d1",
				"activityType": map[string]any{"name": "act-dep", "version": "1.0"},
			},
			wantCode: http.StatusBadRequest,
			wantType: "TypeDeprecatedFault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := swf.NewInMemoryBackend()
			if tt.setupFn != nil {
				tt.setupFn(b)
			}

			h := swf.NewHandler(b)
			rec := doSWFRequest(t, h, "DeprecateActivityType", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantType != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantType, resp["__type"])
			}
		})
	}
}

func TestHandler_DeprecateActivityType_ThenDescribeShowsDeprecated(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	b.AddActivityTypeInternal("d1", "act1", "1.0", "REGISTERED")
	h := swf.NewHandler(b)

	rec := doSWFRequest(t, h, "DeprecateActivityType", map[string]any{
		"domain": "d1", "activityType": map[string]any{"name": "act1", "version": "1.0"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSWFRequest(t, h, "DescribeActivityType", map[string]any{
		"domain": "d1", "activityType": map[string]any{"name": "act1", "version": "1.0"},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	resp := parseSWFResp(t, rec2)
	typeInfo := resp["typeInfo"].(map[string]any)
	assert.Equal(t, "DEPRECATED", typeInfo["status"])
}

func TestHandler_RegisterActivityType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		wantCode int
	}{
		{
			name:     "success",
			body:     map[string]any{"domain": "d1", "name": "act1", "version": "1.0", "description": "my activity"},
			wantCode: http.StatusOK,
		},
		{
			name:     "missing_name",
			body:     map[string]any{"domain": "d1", "name": "", "version": "1.0"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSWFHandler(t)
			doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "d1"})
			rec := doSWFRequest(t, h, "RegisterActivityType", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_RegisterActivityType_Duplicate(t *testing.T) {
	t.Parallel()

	h := newTestSWFHandler(t)
	doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "d1"})

	rec1 := doSWFRequest(t, h, "RegisterActivityType", map[string]any{
		"domain": "d1", "name": "act1", "version": "1.0",
	})
	require.Equal(t, http.StatusOK, rec1.Code)

	rec2 := doSWFRequest(t, h, "RegisterActivityType", map[string]any{
		"domain": "d1", "name": "act1", "version": "1.0",
	})
	assert.Equal(t, http.StatusBadRequest, rec2.Code)

	resp := parseSWFResp(t, rec2)
	assert.Equal(t, "TypeAlreadyExistsFault", resp["__type"])
}

func TestHandler_ListActivityTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body      any
		name      string
		setupOps  []setupAction
		wantCode  int
		wantCount int
	}{
		{
			name: "lists_registered",
			setupOps: []setupAction{
				{
					action: "RegisterActivityType",
					body:   map[string]any{"domain": "d1", "name": "act1", "version": "1.0"},
				},
				{
					action: "RegisterActivityType",
					body:   map[string]any{"domain": "d1", "name": "act2", "version": "1.0"},
				},
			},
			body:      map[string]any{"domain": "d1", "registrationStatus": "REGISTERED"},
			wantCode:  http.StatusOK,
			wantCount: 2,
		},
		{
			name:      "empty",
			body:      map[string]any{"domain": "d1"},
			wantCode:  http.StatusOK,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSWFHandler(t)
			doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "d1"})
			for _, s := range tt.setupOps {
				doSWFRequest(t, h, s.action, s.body)
			}

			rec := doSWFRequest(t, h, "ListActivityTypes", tt.body)
			require.Equal(t, tt.wantCode, rec.Code)

			resp := parseSWFResp(t, rec)

			typeInfos, _ := resp["typeInfos"].([]any)
			assert.Len(t, typeInfos, tt.wantCount)
		})
	}
}

func TestHandler_UndeprecateActivityType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		setupFn  func(*swf.InMemoryBackend)
		name     string
		wantCode int
	}{
		{
			name: "success",
			setupFn: func(b *swf.InMemoryBackend) {
				b.AddActivityTypeInternal("d1", "act1", "1.0", "DEPRECATED")
			},
			body:     map[string]any{"domain": "d1", "activityType": map[string]any{"name": "act1", "version": "1.0"}},
			wantCode: http.StatusOK,
		},
		{
			name: "not_found",
			body: map[string]any{
				"domain":       "d1",
				"activityType": map[string]any{"name": "missing", "version": "1.0"},
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := swf.NewInMemoryBackend()
			if tt.setupFn != nil {
				tt.setupFn(b)
			}

			h := swf.NewHandler(b)
			rec := doSWFRequest(t, h, "UndeprecateActivityType", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestHandler_DeleteActivityType_RequiresDeprecatedFirst verifies
// DeleteActivityType rejects deleting a type that hasn't been deprecated yet
// (real AWS: "Prior to deletion, activity types must first be deprecated"),
// succeeds once deprecated, and 404s on an unknown type.
func TestHandler_DeleteActivityType_RequiresDeprecatedFirst(t *testing.T) {
	t.Parallel()

	h := newTestSWFHandler(t)
	doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "d1"})
	doSWFRequest(t, h, "RegisterActivityType", map[string]any{"domain": "d1", "name": "act1", "version": "1.0"})

	body := map[string]any{
		"domain":       "d1",
		"activityType": map[string]any{"name": "act1", "version": "1.0"},
	}

	// Not yet deprecated -> TypeNotDeprecatedFault.
	rec := doSWFRequest(t, h, "DeleteActivityType", body)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	errResp := parseSWFResp(t, rec)
	assert.Equal(t, "TypeNotDeprecatedFault", errResp["__type"])

	// Deprecate, then delete should succeed.
	doSWFRequest(t, h, "DeprecateActivityType", map[string]any{
		"domain": "d1", "activityType": map[string]any{"name": "act1", "version": "1.0"},
	})
	rec = doSWFRequest(t, h, "DeleteActivityType", body)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Unknown type -> UnknownResourceFault.
	rec = doSWFRequest(t, h, "DeleteActivityType", body)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
