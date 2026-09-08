package appsync_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/appsync"
)

func TestInMemoryBackend_StartSchemaCreation(t *testing.T) {
	t.Parallel()

	validSchema := `type Query { hello: String }`
	invalidSchema := `type { broken schema }`

	tests := []struct {
		name       string
		sdl        string
		wantStatus appsync.SchemaStatus
		wantErr    bool
	}{
		{
			name:       "valid_schema_is_accepted",
			sdl:        validSchema,
			wantStatus: appsync.SchemaStatusActive,
		},
		{
			name:    "invalid_schema_returns_error",
			sdl:     invalidSchema,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
			schema, err := b.StartSchemaCreation(api.APIID, tt.sdl)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, schema.Status)
		})
	}
}

func TestInMemoryBackend_GetSchemaCreationStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(*appsync.InMemoryBackend, string)
		apiID      string
		wantStatus appsync.SchemaStatus
		wantErr    bool
	}{
		{
			name:       "returns_not_applicable_when_no_schema",
			setup:      func(_ *appsync.InMemoryBackend, _ string) {},
			wantStatus: appsync.SchemaStatusNotApplicable,
		},
		{
			name: "returns_active_after_valid_schema_upload",
			setup: func(b *appsync.InMemoryBackend, apiID string) {
				_, _ = b.StartSchemaCreation(apiID, `type Query { hello: String }`)
			},
			wantStatus: appsync.SchemaStatusActive,
		},
		{
			name:    "returns_not_found_for_nonexistent_api",
			setup:   func(_ *appsync.InMemoryBackend, _ string) {},
			apiID:   "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

			apiID := tt.apiID
			if apiID == "" {
				apiID = api.APIID
			}

			tt.setup(b, api.APIID)
			schema, err := b.GetSchemaCreationStatus(apiID)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, awserr.ErrNotFound)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, schema.Status)
		})
	}
}

func TestInMemoryBackend_GetIntrospectionSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs  error
		name       string
		format     string
		wantSDL    string
		hasSchema  bool
		invalidSDL bool
		wantErr    bool
	}{
		{
			name:      "sdl_format_returns_schema_sdl_verbatim",
			format:    "SDL",
			hasSchema: true,
			wantSDL:   `type Query { hello: String }`,
		},
		{
			name:      "empty_format_defaults_to_sdl",
			format:    "",
			hasSchema: true,
			wantSDL:   `type Query { hello: String }`,
		},
		{
			name:      "error_when_no_schema",
			format:    "SDL",
			hasSchema: false,
			wantErr:   true,
			wantErrIs: awserr.ErrNotFound,
		},
		{
			name:      "unrecognized_format_rejected",
			format:    "XML",
			hasSchema: true,
			wantErr:   true,
			wantErrIs: appsync.ErrValidation,
		},
		{
			// gopherstack-w4kf: JSON introspection of an unparsed schema must
			// raise ErrGraphQLSchemaInvalid, not the ErrInvalidSchema sentinel
			// StartSchemaCreation uses -- this op's declared set has no
			// BadRequestException.
			name:       "invalid_schema_json_format_rejected",
			format:     "JSON",
			hasSchema:  true,
			invalidSDL: true,
			wantErr:    true,
			wantErrIs:  appsync.ErrGraphQLSchemaInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

			if tt.hasSchema {
				sdl := `type Query { hello: String }`
				if tt.invalidSDL {
					sdl = `type { broken schema`
				}

				_, _ = b.StartSchemaCreation(api.APIID, sdl)
			}

			sdl, err := b.GetIntrospectionSchema(api.APIID, tt.format, true)

			if tt.wantErr {
				require.Error(t, err)

				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSDL, string(sdl))
		})
	}
}
