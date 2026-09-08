// Package devtls provides a shared, in-memory self-signed TLS certificate
// generator for gopherstack's opt-in/mandatory dev HTTPS listeners.
//
// It is lifted out of cli.go's original generateSelfSignedCert (used by the
// main port's opt-in TLS mode) so that services/azurearm -- which must serve
// its ARM/AAD-discovery listener over HTTPS unconditionally, see AZURE.md
// section 10.8 -- can share the exact same cert-generation logic instead of
// duplicating it. cli.go now calls GenerateSelfSignedCert instead of
// defining its own copy.
package devtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// DefaultValidity is the validity window of a generated certificate.
const DefaultValidity = 365 * 24 * time.Hour

// serialBits is the bit-length of the random certificate serial.
const serialBits = 128

// GenerateSelfSignedCert creates an in-memory self-signed certificate valid
// for localhost/127.0.0.1/::1, plus any extra DNS names or IP addresses
// passed in hosts (each entry is tried as an IP first, falling back to a DNS
// name). Suitable for an opt-in or mandatory dev HTTPS listener -- never for
// production use.
func GenerateSelfSignedCert(hosts ...string) (tls.Certificate, error) {
	certPEM, keyPEM, err := GenerateSelfSignedCertPEM(hosts...)
	if err != nil {
		return tls.Certificate{}, err
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("devtls: build key pair: %w", err)
	}

	return cert, nil
}

// GenerateSelfSignedCertPEM is the PEM-returning variant of
// GenerateSelfSignedCert, used by callers (e.g. the terraform test harness)
// that need to write the certificate to disk for a child process (such as
// `tofu`/`terraform`) to trust via SSL_CERT_FILE.
func GenerateSelfSignedCertPEM(hosts ...string) (certPEM, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("devtls: generate key: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), serialBits)

	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("devtls: generate serial: %w", err)
	}

	dnsNames, ipAddrs := hostsToNamesAndIPs(hosts)

	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "gopherstack", Organization: []string{"gopherstack"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(DefaultValidity),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("devtls: create certificate: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("devtls: marshal key: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

// hostsToNamesAndIPs builds the DNSNames/IPAddresses lists: localhost,
// 127.0.0.1, and ::1 always, plus any extra dedup'd hosts/IPs from extra.
func hostsToNamesAndIPs(extra []string) ([]string, []net.IP) {
	dnsNames := []string{"localhost"}
	ipAddrs := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}

	seenNames := map[string]struct{}{"localhost": {}}
	seenIPs := map[string]struct{}{"127.0.0.1": {}, "::1": {}}

	for _, h := range extra {
		if h == "" {
			continue
		}

		if ip := net.ParseIP(h); ip != nil {
			if _, dup := seenIPs[ip.String()]; dup {
				continue
			}

			seenIPs[ip.String()] = struct{}{}
			ipAddrs = append(ipAddrs, ip)

			continue
		}

		if _, dup := seenNames[h]; dup {
			continue
		}

		seenNames[h] = struct{}{}
		dnsNames = append(dnsNames, h)
	}

	return dnsNames, ipAddrs
}
