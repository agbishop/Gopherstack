package azureservicebus_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azureservicebus"
)

func TestParseSASAuthorization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		header   string
		wantAuth azureservicebus.Authorization
		wantOK   bool
	}{
		{
			name: "well-formed header",
			header: "SharedAccessSignature sr=https%3A%2F%2Fns.servicebus.windows.net%2Fq&" +
				"sig=abc123&se=1893456000&skn=RootManageSharedAccessKey",
			wantOK: true,
			wantAuth: azureservicebus.Authorization{
				Resource:  "https://ns.servicebus.windows.net/q",
				Signature: "abc123",
				Expiry:    1893456000,
				KeyName:   "RootManageSharedAccessKey",
			},
		},
		{
			name:   "missing scheme prefix",
			header: "Bearer sometoken",
			wantOK: false,
		},
		{
			name:   "empty header",
			header: "",
			wantOK: false,
		},
		{
			name:   "malformed key=value pairs are skipped, not fatal",
			header: "SharedAccessSignature garbage&skn=key1",
			wantOK: true,
			wantAuth: azureservicebus.Authorization{
				KeyName: "key1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			auth, ok := azureservicebus.ParseSASAuthorization(tt.header)
			assert.Equal(t, tt.wantOK, ok)

			if tt.wantOK {
				assert.Equal(t, tt.wantAuth, auth)
			}
		})
	}
}

func TestSignSAS(t *testing.T) {
	t.Parallel()

	sig1 := azureservicebus.SignSAS("https://ns.servicebus.windows.net/q", 1893456000, "key")
	sig2 := azureservicebus.SignSAS("https://ns.servicebus.windows.net/q", 1893456000, "key")
	require.Equal(t, sig1, sig2, "signing is deterministic")

	sig3 := azureservicebus.SignSAS("https://ns.servicebus.windows.net/other", 1893456000, "key")
	assert.NotEqual(t, sig1, sig3, "different resource yields different signature")
}

func TestVerifySAS(t *testing.T) {
	t.Parallel()

	const key = "dGVzdC1rZXktdmFsdWU="

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	resource := "https://ns.servicebus.windows.net/q"
	validExpiry := now.Add(time.Hour).Unix()
	sig := azureservicebus.SignSAS(resource, validExpiry, key)

	tests := []struct {
		name string
		key  string
		auth azureservicebus.Authorization
		want bool
	}{
		{
			name: "valid signature and not expired",
			auth: azureservicebus.Authorization{Resource: resource, Signature: sig, Expiry: validExpiry},
			key:  key,
			want: true,
		},
		{
			name: "wrong key fails",
			auth: azureservicebus.Authorization{Resource: resource, Signature: sig, Expiry: validExpiry},
			key:  "d3Jvbmcta2V5",
			want: false,
		},
		{
			name: "expired token fails even with correct signature",
			auth: azureservicebus.Authorization{
				Resource:  resource,
				Signature: azureservicebus.SignSAS(resource, now.Add(-time.Hour).Unix(), key),
				Expiry:    now.Add(-time.Hour).Unix(),
			},
			key:  key,
			want: false,
		},
		{
			name: "tampered signature fails",
			auth: azureservicebus.Authorization{Resource: resource, Signature: "tampered", Expiry: validExpiry},
			key:  key,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := azureservicebus.VerifySAS(tt.auth, tt.key, now)
			assert.Equal(t, tt.want, got)
		})
	}
}
