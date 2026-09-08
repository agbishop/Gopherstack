package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/devtls"
)

func TestTLSConfigFromCLI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantCert    string
		cli         CLI
		wantEnabled bool
	}{
		{
			name:        "disabled by default",
			cli:         CLI{},
			wantEnabled: false,
		},
		{
			name:        "explicit --tls enables self-signed",
			cli:         CLI{TLS: true},
			wantEnabled: true,
			wantCert:    "",
		},
		{
			name:        "cert+key pair enables file-based TLS",
			cli:         CLI{TLSCertFile: "/tmp/c.pem", TLSKeyFile: "/tmp/k.pem"},
			wantEnabled: true,
			wantCert:    "/tmp/c.pem",
		},
		{
			name:        "cert without key does not enable",
			cli:         CLI{TLSCertFile: "/tmp/c.pem"},
			wantEnabled: false,
			wantCert:    "/tmp/c.pem",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tlsConfigFromCLI(&tc.cli)
			if got.enabled != tc.wantEnabled {
				t.Fatalf("enabled = %v, want %v", got.enabled, tc.wantEnabled)
			}

			if got.certFile != tc.wantCert {
				t.Fatalf("certFile = %q, want %q", got.certFile, tc.wantCert)
			}
		})
	}
}

func TestGenerateSelfSignedCert(t *testing.T) {
	t.Parallel()

	cert, err := devtls.GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("devtls.GenerateSelfSignedCert: %v", err)
	}

	if len(cert.Certificate) == 0 {
		t.Fatal("expected at least one certificate in the chain")
	}

	if cert.PrivateKey == nil {
		t.Fatal("expected a private key")
	}
}

// TestServeHTTPS starts the HTTPS listener with an in-memory self-signed cert
// and verifies it actually serves a request over a TLS connection. The cert is
// trusted via a RootCAs pool so the test needs no InsecureSkipVerify.
func TestServeHTTPS(t *testing.T) {
	t.Parallel()

	cert, err := devtls.GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("devtls.GenerateSelfSignedCert: %v", err)
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	e := echo.New()
	e.GET("/ping", func(c *echo.Context) error {
		return c.String(http.StatusOK, "pong")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	addr := ln.Addr().String()
	_ = ln.Close()

	server := &http.Server{
		Addr:              addr,
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}

	errCh := make(chan error, 1)
	go func() {
		if sErr := server.ListenAndServeTLS("", ""); sErr != nil && !errors.Is(sErr, http.ErrServerClosed) {
			errCh <- sErr
		}
	}()

	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS12},
		},
		Timeout: 5 * time.Second,
	}

	resp := getWithRetry(t, client, "https://"+addr+"/ping", errCh)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if resp.TLS == nil {
		t.Fatal("expected connection state to report TLS")
	}
}

// TestServeHTTP_FileBasedTLS exercises serveHTTP's file-based TLS branch end to
// end: it writes a self-signed cert/key to disk, serves with those paths, and
// confirms an HTTPS request succeeds.
func TestServeHTTP_FileBasedTLS(t *testing.T) {
	t.Parallel()

	cert, err := devtls.GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("devtls.GenerateSelfSignedCert: %v", err)
	}

	dir := t.TempDir()
	certPath := dir + "/cert.pem"
	keyPath := dir + "/key.pem"
	writeCertKeyFiles(t, cert, certPath, keyPath)

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	e := echo.New()
	e.GET("/ping", func(c *echo.Context) error {
		return c.String(http.StatusOK, "pong")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	addr := ln.Addr().String()
	_ = ln.Close()

	server := &http.Server{Addr: addr, Handler: e, ReadHeaderTimeout: 5 * time.Second}
	tlsCfg := tlsSettings{enabled: true, certFile: certPath, keyFile: keyPath}

	errCh := make(chan error, 1)
	go func() {
		if sErr := serveHTTP(server, tlsCfg); sErr != nil && !errors.Is(sErr, http.ErrServerClosed) {
			errCh <- sErr
		}
	}()

	defer func() { _ = server.Shutdown(context.Background()) }()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS12},
		},
		Timeout: 5 * time.Second,
	}

	resp := getWithRetry(t, client, "https://"+addr+"/ping", errCh)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// writeCertKeyFiles writes the cert chain and private key of cert to PEM files.
func writeCertKeyFiles(t *testing.T, cert tls.Certificate, certPath, keyPath string) {
	t.Helper()

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if writeErr := os.WriteFile(keyPath, keyPEM, 0o600); writeErr != nil {
		t.Fatalf("write key: %v", writeErr)
	}
}

// getWithRetry issues a GET, retrying briefly while the listener goroutine
// finishes binding. It fails the test if the server goroutine reports an error.
func getWithRetry(t *testing.T, client *http.Client, url string, errCh <-chan error) *http.Response {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}

		resp, err := client.Do(req)
		if err == nil {
			return resp
		}

		select {
		case sErr := <-errCh:
			t.Fatalf("server error: %v", sErr)
		default:
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatal("HTTPS server did not become reachable")

	return nil
}
