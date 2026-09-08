package devtls_test

import (
	"crypto/x509"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/devtls"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		extraHosts  []string
		wantDNS     []string
		wantIPCount int
	}{
		{
			name:        "no extra hosts still covers localhost/127.0.0.1/::1",
			extraHosts:  nil,
			wantDNS:     []string{"localhost"},
			wantIPCount: 2,
		},
		{
			name:        "extra DNS name and IP are added",
			extraHosts:  []string{"arm.gopherstack.local", "10.0.0.5"},
			wantDNS:     []string{"localhost", "arm.gopherstack.local"},
			wantIPCount: 3,
		},
		{
			name:        "duplicate extra host is deduped",
			extraHosts:  []string{"localhost", "127.0.0.1"},
			wantDNS:     []string{"localhost"},
			wantIPCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cert, err := devtls.GenerateSelfSignedCert(tt.extraHosts...)
			require.NoError(t, err)
			require.NotEmpty(t, cert.Certificate)

			leaf, err := x509.ParseCertificate(cert.Certificate[0])
			require.NoError(t, err)

			for _, dns := range tt.wantDNS {
				assert.Contains(t, leaf.DNSNames, dns)
			}

			assert.Len(t, leaf.IPAddresses, tt.wantIPCount)

			var haveLoopback, haveLoopback6 bool

			for _, ip := range leaf.IPAddresses {
				if ip.Equal(net.IPv4(127, 0, 0, 1)) {
					haveLoopback = true
				}

				if ip.Equal(net.IPv6loopback) {
					haveLoopback6 = true
				}
			}

			assert.True(t, haveLoopback, "expected 127.0.0.1 in IPAddresses")
			assert.True(t, haveLoopback6, "expected ::1 in IPAddresses")
		})
	}
}

func TestGenerateSelfSignedCertPEM(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM, err := devtls.GenerateSelfSignedCertPEM("example.gopherstack.local")
	require.NoError(t, err)
	assert.NotEmpty(t, certPEM)
	assert.NotEmpty(t, keyPEM)
	assert.Contains(t, string(certPEM), "BEGIN CERTIFICATE")
	assert.Contains(t, string(keyPEM), "BEGIN PRIVATE KEY")
}
