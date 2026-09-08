package servicediscovery_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/servicediscovery"
)

func TestBackend_Region(t *testing.T) {
	t.Parallel()

	b := servicediscovery.NewInMemoryBackend("000000000000", "eu-west-1")
	assert.Equal(t, "eu-west-1", b.Region())
}

func TestBackend_ListNamespaces(t *testing.T) {
	t.Parallel()

	b := servicediscovery.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateHTTPNamespace("ns-b", "", nil)
	require.NoError(t, err)

	_, err = b.CreateHTTPNamespace("ns-a", "", nil)
	require.NoError(t, err)

	list := b.ListNamespaces(servicediscovery.ListNamespacesFilter{})
	require.Len(t, list, 2)
	assert.Equal(t, "ns-a", list[0].Name, "namespaces should be sorted by name")
}

// TestExportCountHelpers verifies that count helpers work correctly.
func TestExportCountHelpers(t *testing.T) {
	t.Parallel()

	b, h := newBackendAndHandler(t)

	assert.Equal(t, 0, servicediscovery.NamespaceCount(b))
	assert.Equal(t, 0, servicediscovery.ServiceCount(b))
	assert.Equal(t, 0, servicediscovery.InstanceCount(b))
	assert.Equal(t, 0, servicediscovery.OperationCount(b))
	assert.Equal(t, 0, servicediscovery.ServiceAttributeCount(b))
	assert.Equal(t, 30, servicediscovery.HandlerOpsLen(h))
}

// TestStorageBackendInterfaceCompiles verifies that Handler.Backend
// is the StorageBackend interface type (compile-time check via assignment).
func TestStorageBackendInterfaceCompiles(t *testing.T) {
	t.Parallel()

	b := servicediscovery.NewInMemoryBackend("000000000000", "us-east-1")
	var _ servicediscovery.StorageBackend = b // compile-time assertion

	h := servicediscovery.NewHandler(b)
	assert.NotNil(t, h.Backend)
}

// TestAddSeedHelpers verifies AddNamespaceInternal / AddServiceInternal work.
func TestAddSeedHelpers(t *testing.T) {
	t.Parallel()

	b, _ := newBackendAndHandler(t)

	ns := servicediscovery.NewNamespaceForTest("ns-seed-001", "seeded-ns", "HTTP")
	servicediscovery.AddNamespaceInternal(b, ns)
	assert.Equal(t, 1, servicediscovery.NamespaceCount(b))

	svc := servicediscovery.NewServiceForTest("svc-seed-001", "seeded-svc", "ns-seed-001")
	servicediscovery.AddServiceInternal(b, svc)
	assert.Equal(t, 1, servicediscovery.ServiceCount(b))

	inst := servicediscovery.NewInstanceForTest("inst-001", "svc-seed-001", map[string]string{"ip": "1.2.3.4"})
	servicediscovery.AddInstanceInternal(b, inst)
	assert.Equal(t, 1, servicediscovery.InstanceCount(b))
}

// TestBackendReset clears everything via Handler.Reset().
func TestBackendReset(t *testing.T) {
	t.Parallel()

	b, h := newBackendAndHandler(t)

	ns := servicediscovery.NewNamespaceForTest("ns-r", "reset-ns", "HTTP")
	servicediscovery.AddNamespaceInternal(b, ns)
	assert.Equal(t, 1, servicediscovery.NamespaceCount(b))

	h.Reset()
	assert.Equal(t, 0, servicediscovery.NamespaceCount(b))
}

// mockHostedZoneCreator simulates the Route 53 backend a DNS namespace's hosted zone is
// created against via SetHostedZoneCreator.
type mockHostedZoneCreator struct {
	err    error
	nextID string
	calls  []mockHostedZoneCall
}

type mockHostedZoneCall struct {
	name      string
	callerRef string
	vpcID     string
	vpcRegion string
	private   bool
}

func (m *mockHostedZoneCreator) CreateHostedZone(
	name, callerRef, _ string,
	private bool,
	vpcID, vpcRegion string,
) (string, error) {
	m.calls = append(m.calls, mockHostedZoneCall{
		name: name, callerRef: callerRef, private: private, vpcID: vpcID, vpcRegion: vpcRegion,
	})

	if m.err != nil {
		return "", m.err
	}

	return m.nextID, nil
}

// TestCreatePrivateDNSNamespace_UsesRealHostedZone verifies that a private DNS namespace's
// HostedZoneId comes from the wired Route 53 backend rather than a synthetic value that
// matches no real zone (gopherstack-chmx).
func TestCreatePrivateDNSNamespace_UsesRealHostedZone(t *testing.T) {
	t.Parallel()

	b := servicediscovery.NewInMemoryBackend("000000000000", "us-east-1")
	hzMock := &mockHostedZoneCreator{nextID: "Z1REALZONE0001"}
	b.SetHostedZoneCreator(hzMock)

	_, err := b.CreatePrivateDNSNamespace("private.local", "", "vpc-1234", 0, nil)
	require.NoError(t, err)

	list := b.ListNamespaces(servicediscovery.ListNamespacesFilter{})
	require.Len(t, list, 1)

	ns, err := b.GetNamespace(list[0].ID)
	require.NoError(t, err)
	require.NotNil(t, ns.Properties)
	require.NotNil(t, ns.Properties.DNSProperties)
	assert.Equal(t, "Z1REALZONE0001", ns.Properties.DNSProperties.HostedZoneID)

	require.Len(t, hzMock.calls, 1)
	assert.Equal(t, "private.local", hzMock.calls[0].name)
	assert.True(t, hzMock.calls[0].private)
	assert.Equal(t, "vpc-1234", hzMock.calls[0].vpcID)
}

// TestCreatePublicDNSNamespace_UsesRealHostedZone mirrors the private-namespace case for
// DNS_PUBLIC, verifying private=false is passed through and no VPC is associated.
func TestCreatePublicDNSNamespace_UsesRealHostedZone(t *testing.T) {
	t.Parallel()

	b := servicediscovery.NewInMemoryBackend("000000000000", "us-east-1")
	hzMock := &mockHostedZoneCreator{nextID: "Z2REALZONE0002"}
	b.SetHostedZoneCreator(hzMock)

	_, err := b.CreatePublicDNSNamespace("public.example.com", "", 0, nil)
	require.NoError(t, err)

	list := b.ListNamespaces(servicediscovery.ListNamespacesFilter{})
	require.Len(t, list, 1)

	ns, err := b.GetNamespace(list[0].ID)
	require.NoError(t, err)
	require.NotNil(t, ns.Properties)
	require.NotNil(t, ns.Properties.DNSProperties)
	assert.Equal(t, "Z2REALZONE0002", ns.Properties.DNSProperties.HostedZoneID)

	require.Len(t, hzMock.calls, 1)
	assert.False(t, hzMock.calls[0].private)
	assert.Empty(t, hzMock.calls[0].vpcID)
}

// TestCreateDNSNamespace_UnwiredRoute53StaysPermissive verifies that namespace creation
// still succeeds, falling back to a synthetic HostedZoneId, when Route 53 has not been
// wired in via SetHostedZoneCreator -- an unwired hook must be a silent no-op, never a
// rejection.
func TestCreateDNSNamespace_UnwiredRoute53StaysPermissive(t *testing.T) {
	t.Parallel()

	b := servicediscovery.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreatePrivateDNSNamespace("unwired.local", "", "vpc-9999", 0, nil)
	require.NoError(t, err)

	list := b.ListNamespaces(servicediscovery.ListNamespacesFilter{})
	require.Len(t, list, 1)

	ns, err := b.GetNamespace(list[0].ID)
	require.NoError(t, err)
	require.NotNil(t, ns.Properties)
	require.NotNil(t, ns.Properties.DNSProperties)
	assert.NotEmpty(t, ns.Properties.DNSProperties.HostedZoneID)
}
