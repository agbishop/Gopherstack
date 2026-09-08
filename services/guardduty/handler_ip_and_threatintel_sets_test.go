package guardduty_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardDuty_CreateIPSet_FormatValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{name: "rejects unknown format", format: "BOGUS", wantErr: true},
		{name: "rejects lowercase of a valid format", format: "txt", wantErr: true},
		{name: "accepts TXT", format: "TXT"},
		{name: "accepts STIX", format: "STIX"},
		{name: "accepts OTX_CSV", format: "OTX_CSV"},
		{name: "accepts ALIEN_VAULT", format: "ALIEN_VAULT"},
		{name: "accepts PROOF_POINT", format: "PROOF_POINT"},
		{name: "accepts FIRE_EYE", format: "FIRE_EYE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			detectorID := createTestDetector(t, h)

			rec := doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/ipset", map[string]any{
				"name":     "my-ipset",
				"format":   tt.format,
				"location": "s3://bucket/ipset.txt",
			})

			if tt.wantErr {
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

				var errOut map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errOut))
				assert.Equal(t, "BadRequestException", errOut["__type"])

				return
			}

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}
}

func TestGuardDuty_CreateThreatIntelSet_FormatValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{name: "rejects unknown format", format: "BOGUS", wantErr: true},
		{name: "rejects lowercase of a valid format", format: "stix", wantErr: true},
		{name: "accepts TXT", format: "TXT"},
		{name: "accepts STIX", format: "STIX"},
		{name: "accepts OTX_CSV", format: "OTX_CSV"},
		{name: "accepts ALIEN_VAULT", format: "ALIEN_VAULT"},
		{name: "accepts PROOF_POINT", format: "PROOF_POINT"},
		{name: "accepts FIRE_EYE", format: "FIRE_EYE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			detectorID := createTestDetector(t, h)

			rec := doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/threatintelset", map[string]any{
				"name":     "my-tiset",
				"format":   tt.format,
				"location": "s3://bucket/tiset.txt",
			})

			if tt.wantErr {
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

				var errOut map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errOut))
				assert.Equal(t, "BadRequestException", errOut["__type"])

				return
			}

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}
}
