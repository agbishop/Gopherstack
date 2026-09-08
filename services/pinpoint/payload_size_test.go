package pinpoint_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Mirrors the unexported limits in payload_size.go (7 MB / 4 MB / 15 KB from
// https://docs.aws.amazon.com/pinpoint/latest/developerguide/quotas.html),
// duplicated here because this is an external test package.
const (
	generalPayloadLimitBytes   = 7 * 1024 * 1024
	putEventsPayloadLimitBytes = 4 * 1024 * 1024
	updateEndpointLimitBytes   = 15 * 1024
)

// jsonBodyOfSize builds a syntactically valid `{prefix<padding>suffix}`-shaped
// JSON document of exactly targetLen bytes by padding with 'a' characters.
func jsonBodyOfSize(t *testing.T, prefix, suffix string, targetLen int) []byte {
	t.Helper()

	padLen := targetLen - len(prefix) - len(suffix)
	require.Positive(t, padLen, "target size %d too small for fixed prefix/suffix", targetLen)

	body := prefix + strings.Repeat("a", padLen) + suffix
	require.Len(t, body, targetLen)
	require.True(t, json.Valid([]byte(body)), "constructed body must be valid JSON")

	return []byte(body)
}

func requirePayloadTooLarge(t *testing.T, body []byte) {
	t.Helper()

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.Equal(t, "PayloadTooLargeException", envelope["__type"])
}

func TestCreateApp_PayloadSizeLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		size       int
		wantStatus int
	}{
		{name: "at limit accepted", size: generalPayloadLimitBytes, wantStatus: http.StatusCreated},
		{
			name:       "one byte over rejected",
			size:       generalPayloadLimitBytes + 1,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerForTest(t)
			body := jsonBodyOfSize(t, `{"Name":"`, `"}`, tt.size)

			rec := doRawPinpointRequest(t, h, http.MethodPost, "/v1/apps", body)

			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusRequestEntityTooLarge {
				requirePayloadTooLarge(t, rec.Body.Bytes())
			}
		})
	}
}

func TestPutEvents_PayloadSizeLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		size       int
		wantStatus int
	}{
		{
			name:       "at limit accepted",
			size:       putEventsPayloadLimitBytes,
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "one byte over rejected",
			size:       putEventsPayloadLimitBytes + 1,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "under general limit but over event limit rejected",
			size:       5 * 1024 * 1024, // > 4 MB PutEvents limit, < 7 MB general ceiling
			wantStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerForTest(t)
			appID := createTestApp(t, h, "app")

			body := jsonBodyOfSize(t, `{"BatchItem":{},"Pad":"`, `"}`, tt.size)

			rec := doRawPinpointRequest(t, h, http.MethodPost, "/v1/apps/"+appID+"/events", body)

			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusRequestEntityTooLarge {
				requirePayloadTooLarge(t, rec.Body.Bytes())
			}
		})
	}
}

func TestUpdateEndpoint_PayloadSizeLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		size       int
		wantStatus int
	}{
		{
			name:       "at limit accepted",
			size:       updateEndpointLimitBytes,
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "one byte over rejected",
			size:       updateEndpointLimitBytes + 1,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "under general limit but over endpoint limit rejected",
			size:       1024 * 1024, // 1 MB > 15 KB endpoint limit, well under 7 MB general ceiling
			wantStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerForTest(t)
			appID := createTestApp(t, h, "app")

			body := jsonBodyOfSize(t, `{"Address":"x","Pad":"`, `"}`, tt.size)

			rec := doRawPinpointRequest(
				t,
				h,
				http.MethodPut,
				"/v1/apps/"+appID+"/endpoints/ep-1",
				body,
			)

			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusRequestEntityTooLarge {
				requirePayloadTooLarge(t, rec.Body.Bytes())
			}
		})
	}
}
