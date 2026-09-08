package servicediscovery_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gopherDNS "github.com/blackbirdworks/gopherstack/pkgs/dns"
	"github.com/blackbirdworks/gopherstack/services/servicediscovery"
)

// startTestDNSServer starts a real embedded DNS server on a free UDP port,
// mirroring pkgs/dns/dns_test.go's helper of the same shape.
func startTestDNSServer(t *testing.T) (*gopherDNS.Server, string) {
	t.Helper()

	const maxAttempts = 5

	for range maxAttempts {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		require.NoError(t, err)

		port := pc.LocalAddr().(*net.UDPAddr).Port
		_ = pc.Close()

		addr := fmt.Sprintf("127.0.0.1:%d", port)

		srv, err := gopherDNS.New(gopherDNS.Config{ListenAddr: addr, ResolveIP: "127.0.0.1"})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())

		if err = srv.Start(ctx); err != nil {
			cancel()
			_ = srv.Stop()

			continue
		}

		t.Cleanup(func() {
			cancel()
			_ = srv.Stop()
		})

		return srv, addr
	}

	t.Fatal("failed to start DNS test server after retries")

	return nil, ""
}

// queryA performs a real UDP DNS A query and returns the first answer's IP and the response code.
func queryA(t *testing.T, addr, hostname string) (string, int) {
	t.Helper()

	c := new(dns.Client)
	c.Net = "udp"
	c.Timeout = 2 * time.Second

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
	m.RecursionDesired = false

	r, _, err := c.Exchange(m, addr)
	require.NoError(t, err)

	if len(r.Answer) == 0 {
		return "", r.Rcode
	}

	a, ok := r.Answer[0].(*dns.A)
	if !ok {
		return "", r.Rcode
	}

	return a.A.String(), r.Rcode
}

// createPublicDNSNamespace creates a public DNS namespace and returns its ID.
func createPublicDNSNamespace(t *testing.T, h *servicediscovery.Handler, name string) string {
	t.Helper()

	rec := doSDRequest(t, h, "CreatePublicDnsNamespace", map[string]any{"Name": name})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	opID, ok := resp["OperationId"].(string)
	require.True(t, ok)

	opRec := doSDRequest(t, h, "GetOperation", map[string]any{"OperationId": opID})
	require.Equal(t, http.StatusOK, opRec.Code, opRec.Body.String())

	var opResp map[string]any
	require.NoError(t, json.Unmarshal(opRec.Body.Bytes(), &opResp))

	targets := opResp["Operation"].(map[string]any)["Targets"].(map[string]any)

	return targets["NAMESPACE"].(string)
}

// TestRegisterInstance_DNS_RealResolution proves gopherstack-qy2b end to end:
// registering an instance in a service belonging to a DNS namespace makes it
// actually resolvable via a real DNS query against the embedded server, and
// deregistering it removes the record again. Before the fix, no DNSRegistrar
// was ever wired into servicediscovery's backend, so this query returned
// NXDOMAIN regardless of what RegisterInstance recorded internally.
func TestRegisterInstance_DNS_RealResolution(t *testing.T) {
	t.Parallel()

	srv, addr := startTestDNSServer(t)

	backend := servicediscovery.NewInMemoryBackend("000000000000", "us-east-1")
	backend.SetDNSRegistrar(srv)
	h := servicediscovery.NewHandler(backend)

	nsID := createPublicDNSNamespace(t, h, "ns-dns-resolve")

	svcRec := doSDRequest(t, h, "CreateService", map[string]any{
		"Name":        "svc-dns-resolve",
		"NamespaceId": nsID,
		"DnsConfig": map[string]any{
			"DnsRecords": []map[string]any{{"Type": "A", "TTL": 60}},
		},
	})
	require.Equal(t, http.StatusOK, svcRec.Code, svcRec.Body.String())

	var svcResp map[string]any
	require.NoError(t, json.Unmarshal(svcRec.Body.Bytes(), &svcResp))
	svcID := svcResp["Service"].(map[string]any)["Id"].(string)

	hostname := "svc-dns-resolve.ns-dns-resolve"

	_, rcode := queryA(t, addr, hostname)
	assert.Equal(t, dns.RcodeNameError, rcode, "unregistered instance must not resolve")

	regRec := doSDRequest(t, h, "RegisterInstance", map[string]any{
		"ServiceId":  svcID,
		"InstanceId": "i-1",
		"Attributes": map[string]string{"AWS_INSTANCE_IPV4": "10.1.2.3"},
	})
	require.Equal(t, http.StatusOK, regRec.Code, regRec.Body.String())

	ip, rcode := queryA(t, addr, hostname)
	assert.Equal(t, dns.RcodeSuccess, rcode)
	assert.Equal(t, "10.1.2.3", ip)

	deregRec := doSDRequest(t, h, "DeregisterInstance", map[string]any{
		"ServiceId":  svcID,
		"InstanceId": "i-1",
	})
	require.Equal(t, http.StatusOK, deregRec.Code, deregRec.Body.String())

	_, rcode = queryA(t, addr, hostname)
	assert.Equal(t, dns.RcodeNameError, rcode, "deregistered instance must stop resolving")
}

// TestRegisterInstance_HTTPNamespace_NoDNSRegistration verifies that an HTTP
// namespace -- which has no DNS at all -- never gets a DNS registration, even
// when an instance happens to carry an AWS_INSTANCE_IPV4 attribute.
// Registering DNS for an HTTP namespace would be a fabrication.
func TestRegisterInstance_HTTPNamespace_NoDNSRegistration(t *testing.T) {
	t.Parallel()

	registrar := &mockSDDNSRegistrar{registered: make(map[string]bool), records: make(map[string][]string)}

	backend := servicediscovery.NewInMemoryBackend("000000000000", "us-east-1")
	backend.SetDNSRegistrar(registrar)
	h := servicediscovery.NewHandler(backend)

	nsRec := doSDRequest(t, h, "CreateHttpNamespace", map[string]any{"Name": "ns-http"})
	require.Equal(t, http.StatusOK, nsRec.Code, nsRec.Body.String())

	var nsResp map[string]any
	require.NoError(t, json.Unmarshal(nsRec.Body.Bytes(), &nsResp))
	opID := nsResp["OperationId"].(string)

	opRec := doSDRequest(t, h, "GetOperation", map[string]any{"OperationId": opID})
	var opResp map[string]any
	require.NoError(t, json.Unmarshal(opRec.Body.Bytes(), &opResp))
	nsID := opResp["Operation"].(map[string]any)["Targets"].(map[string]any)["NAMESPACE"].(string)

	svcRec := doSDRequest(t, h, "CreateService", map[string]any{"Name": "svc-http", "NamespaceId": nsID})
	require.Equal(t, http.StatusOK, svcRec.Code, svcRec.Body.String())

	var svcResp map[string]any
	require.NoError(t, json.Unmarshal(svcRec.Body.Bytes(), &svcResp))
	svcID := svcResp["Service"].(map[string]any)["Id"].(string)

	regRec := doSDRequest(t, h, "RegisterInstance", map[string]any{
		"ServiceId":  svcID,
		"InstanceId": "i-http-1",
		"Attributes": map[string]string{"AWS_INSTANCE_IPV4": "10.9.9.9"},
	})
	require.Equal(t, http.StatusOK, regRec.Code, regRec.Body.String())

	assert.Empty(t, registrar.registered, "an HTTP namespace must never receive a DNS registration")
}

// TestDeregisterInstance_DNS_ResyncKeepsSibling verifies that deregistering
// one of several instances registered to the same DNS service removes only
// that instance's record value, leaving the sibling instance resolvable --
// the same resync-not-blind-deregister discipline route53 uses for record
// sets sharing a name.
func TestDeregisterInstance_DNS_ResyncKeepsSibling(t *testing.T) {
	t.Parallel()

	registrar := &mockSDDNSRegistrar{registered: make(map[string]bool), records: make(map[string][]string)}

	backend := servicediscovery.NewInMemoryBackend("000000000000", "us-east-1")
	backend.SetDNSRegistrar(registrar)
	h := servicediscovery.NewHandler(backend)

	nsID := createPublicDNSNamespace(t, h, "ns-multi")

	svcRec := doSDRequest(t, h, "CreateService", map[string]any{
		"Name":        "svc-multi",
		"NamespaceId": nsID,
		"DnsConfig": map[string]any{
			"RoutingPolicy": "MULTIVALUE",
			"DnsRecords":    []map[string]any{{"Type": "A", "TTL": 60}},
		},
	})
	require.Equal(t, http.StatusOK, svcRec.Code, svcRec.Body.String())

	var svcResp map[string]any
	require.NoError(t, json.Unmarshal(svcRec.Body.Bytes(), &svcResp))
	svcID := svcResp["Service"].(map[string]any)["Id"].(string)

	hostname := "svc-multi.ns-multi."

	doSDRequest(t, h, "RegisterInstance", map[string]any{
		"ServiceId":  svcID,
		"InstanceId": "i-1",
		"Attributes": map[string]string{"AWS_INSTANCE_IPV4": "10.0.0.1"},
	})
	doSDRequest(t, h, "RegisterInstance", map[string]any{
		"ServiceId":  svcID,
		"InstanceId": "i-2",
		"Attributes": map[string]string{"AWS_INSTANCE_IPV4": "10.0.0.2"},
	})

	assert.ElementsMatch(t, []string{"10.0.0.1", "10.0.0.2"}, registrar.records[hostname])

	deregRec := doSDRequest(t, h, "DeregisterInstance", map[string]any{
		"ServiceId":  svcID,
		"InstanceId": "i-1",
	})
	require.Equal(t, http.StatusOK, deregRec.Code, deregRec.Body.String())

	assert.True(t, registrar.registered[hostname], "sibling instance must remain registered")
	assert.Equal(t, []string{"10.0.0.2"}, registrar.records[hostname])
}

// mockSDDNSRegistrar is a minimal in-memory DNSRegistrar for servicediscovery tests.
type mockSDDNSRegistrar struct {
	registered map[string]bool
	records    map[string][]string // fqdn -> values, across every recordType
}

func (m *mockSDDNSRegistrar) RegisterRecord(hostname, _ string, values []string) {
	fqdn := dns.Fqdn(hostname)
	m.registered[fqdn] = true
	m.records[fqdn] = append(m.records[fqdn], values...)
}

func (m *mockSDDNSRegistrar) Deregister(hostname string) {
	fqdn := dns.Fqdn(hostname)
	delete(m.registered, fqdn)
	delete(m.records, fqdn)
}
