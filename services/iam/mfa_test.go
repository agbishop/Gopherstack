package iam_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	iamsdk "github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iam"
)

// TestMFADevice_Backend covers EnableMFADevice, DeactivateMFADevice.
func TestMFADevice_Backend(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	_, err := b.CreateUser("mfa-user2", "/", "")
	require.NoError(t, err)

	// Create a virtual MFA device first.
	vmfa, err := b.CreateVirtualMFADevice("mfa-user2-token", "/")
	require.NoError(t, err)
	serialNumber := vmfa.SerialNumber

	// EnableMFADevice.
	rec := callIAM(t, h, "EnableMFADevice", map[string]string{
		"UserName":            "mfa-user2",
		"SerialNumber":        serialNumber,
		"AuthenticationCode1": "123456",
		"AuthenticationCode2": "654321",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// DeactivateMFADevice.
	rec = callIAM(t, h, "DeactivateMFADevice", map[string]string{
		"UserName":     "mfa-user2",
		"SerialNumber": serialNumber,
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestBackendReset_ClearsComprehensiveState covers ResetComprehensiveBackend and related.
func TestBackendReset_ClearsComprehensiveState(t *testing.T) {
	t.Parallel()

	b := iam.NewInMemoryBackend()

	// OIDCProviderExists.
	exists := b.OIDCProviderExists("https://nonexistent.com")
	assert.False(t, exists)

	oidc, err := b.CreateOpenIDConnectProvider(
		"https://testoidc.com",
		[]string{"sts.amazonaws.com"},
		[]string{"990f41981148b53dc7c615a6b0c2a26555cc5d85"},
	)
	require.NoError(t, err)

	exists = b.OIDCProviderExists("https://testoidc.com")
	assert.True(t, exists)

	// ListMFADevicesForUser.
	_, err = b.CreateUser("mfa-device-user", "/", "")
	require.NoError(t, err)

	devicesPage, err := b.ListMFADevicesForUser("mfa-device-user", "", 0)
	require.NoError(t, err)
	assert.Empty(t, devicesPage.Data)

	// GetMFADeviceOwner (should return empty for unknown serial).
	owner := b.GetMFADeviceOwner("arn:aws:iam::000000000000:mfa/nonexistent")
	assert.Empty(t, owner)

	// GenerateServiceLastAccessedDetailsForEntity.
	jobID := b.GenerateServiceLastAccessedDetailsForEntity(oidc.Arn)
	assert.NotEmpty(t, jobID)

	// RecordServiceAccess.
	b.RecordServiceAccess(oidc.Arn, "s3.amazonaws.com", "Amazon S3")

	// ResetComprehensiveBackend only resets SSH/MFA state, not OIDC providers.
	b.ResetComprehensiveBackend()

	// OIDC providers are in the main backend and still exist after comprehensive reset.
	exists = b.OIDCProviderExists("https://testoidc.com")
	assert.True(t, exists)
}

// TestVirtualMFA_Backend tests ListVirtualMFADevices and DeleteVirtualMFADevice.
func TestVirtualMFA_Backend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(*iam.InMemoryBackend) string
		name      string
		action    string
		wantCount int
		wantErr   bool
	}{
		{
			name:   "list_empty",
			action: "list",
			setup:  func(_ *iam.InMemoryBackend) string { return "" },
		},
		{
			name:   "list_one",
			action: "list",
			setup: func(b *iam.InMemoryBackend) string {
				_, _ = b.CreateVirtualMFADevice("MyMFA", "/")

				return ""
			},
			wantCount: 1,
		},
		{
			name:   "delete_existing",
			action: "delete",
			setup: func(b *iam.InMemoryBackend) string {
				d, _ := b.CreateVirtualMFADevice("MyMFA", "/")

				return d.SerialNumber
			},
		},
		{
			name:    "delete_not_found",
			action:  "delete",
			setup:   func(_ *iam.InMemoryBackend) string { return "arn:aws:iam::000000000000:mfa/ghost" },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iam.NewInMemoryBackend()
			serial := tt.setup(b)

			switch tt.action {
			case "list":
				p, err := b.ListVirtualMFADevices("", 0)
				require.NoError(t, err)
				assert.Len(t, p.Data, tt.wantCount)
			case "delete":
				err := b.DeleteVirtualMFADevice(serial)
				if tt.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}

func TestMFADevice_InitialStatus_NotAssigned(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	dev, err := b.CreateVirtualMFADeviceFull("MyDevice", "/")
	require.NoError(t, err)
	assert.Equal(t, iam.MFAStatusNotAssigned, dev.Status)
}

func TestEnableMFADevice_SetsActiveStatus(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("lena", "/", "")
	dev, _ := b.CreateVirtualMFADeviceFull("LenaDevice", "/")

	require.NoError(t, b.EnableMFADevice("lena", dev.SerialNumber, "111111", "222222"))

	devicesPage, err := b.ListMFADevicesForUser("lena", "", 0)
	require.NoError(t, err)
	require.Len(t, devicesPage.Data, 1)
	assert.Equal(t, iam.MFAStatusEnabled, devicesPage.Data[0].Status)
}

func TestEnableMFADevice_RejectsDoubleEnable(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("mike", "/", "")
	dev, _ := b.CreateVirtualMFADeviceFull("MikeDevice", "/")

	require.NoError(t, b.EnableMFADevice("mike", dev.SerialNumber, "111111", "222222"))

	// Second enable on same device must fail.
	err := b.EnableMFADevice("mike", dev.SerialNumber, "333333", "444444")
	require.Error(t, err)
	assert.ErrorIs(t, err, iam.ErrMFADeviceAlreadyEnabled)
}

func TestDeactivateMFADevice_SetsDeactivatedStatus(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("nina", "/", "")
	dev, _ := b.CreateVirtualMFADeviceFull("NinaDevice", "/")
	require.NoError(t, b.EnableMFADevice("nina", dev.SerialNumber, "111111", "222222"))

	require.NoError(t, b.DeactivateMFADevice("nina", dev.SerialNumber))
}

func TestDeactivateMFADevice_RejectsUnenabled(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("omar", "/", "")
	dev, _ := b.CreateVirtualMFADeviceFull("OmarDevice", "/")

	// Device is PENDING — cannot deactivate.
	err := b.DeactivateMFADevice("omar", dev.SerialNumber)
	require.Error(t, err)
	assert.ErrorIs(t, err, iam.ErrInvalidAction)
}

func TestCredentialReport_MFAStatusReflected(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("report-user", "/", "")

	// Before MFA: mfa_active column should be false.
	report := b.GetCredentialReport()
	assert.Contains(t, report, "report-user")
}

func TestCredentialReport_MFAActive_TrueAfterVirtualMFAEnabled(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("mfa-user", "/", "")

	serial := "arn:aws:iam::000000000000:mfa/mfa-user"
	_, mfaErr := b.CreateVirtualMFADevice("mfa-user", "/")
	require.NoError(t, mfaErr)
	require.NoError(t, b.EnableMFADevice("mfa-user", serial, "123456", "654321"))

	csv := b.GetCredentialReport()
	lines := strings.Split(strings.TrimSpace(csv), "\n")

	var userLine string

	for _, l := range lines {
		if strings.HasPrefix(l, "mfa-user,") {
			userLine = l

			break
		}
	}

	require.NotEmpty(t, userLine, "report must contain mfa-user row")
	cols := strings.Split(userLine, ",")
	require.Greater(t, len(cols), 7, "row must have mfa_active column")
	assert.Equal(t, "true", cols[7], "mfa_active must be true after enabling MFA")
}

func TestCredentialReport_MFAActive_Reflected(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("alice", "/", "")

	report := b.GetCredentialReport()
	lines := strings.Split(report, "\n")

	var aliceRow string
	for _, line := range lines {
		if strings.HasPrefix(line, "alice,") {
			aliceRow = line
		}
	}

	require.NotEmpty(t, aliceRow)
	fields := strings.Split(aliceRow, ",")

	// mfa_active is the 8th field (index 7, 0-based).
	require.GreaterOrEqual(t, len(fields), 8)
	assert.Equal(t, "false", fields[7], "mfa_active must be false before MFA is enabled")

	// Enable MFA device.
	dev, err := b.CreateVirtualMFADeviceFull("alice-mfa", "/mfa/")
	require.NoError(t, err)
	require.NoError(t, b.EnableMFADevice("alice", dev.SerialNumber, "123456", "789012"))

	report2 := b.GetCredentialReport()
	lines2 := strings.Split(report2, "\n")

	var aliceRow2 string
	for _, line := range lines2 {
		if strings.HasPrefix(line, "alice,") {
			aliceRow2 = line
		}
	}

	require.NotEmpty(t, aliceRow2)
	fields2 := strings.Split(aliceRow2, ",")
	require.GreaterOrEqual(t, len(fields2), 8)
	assert.Equal(t, "true", fields2[7], "mfa_active must be true after MFA device is enabled")
}

func TestVirtualMFADevice_CRUD(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("alice", "/", "")

	dev, err := b.CreateVirtualMFADeviceFull("alice-token", "/mfa/")
	require.NoError(t, err)
	assert.NotEmpty(t, dev.SerialNumber)
	assert.NotEmpty(t, dev.Base32StringSeed)

	require.NoError(t, b.EnableMFADevice("alice", dev.SerialNumber, "123456", "789012"))

	devicesPage, err := b.ListMFADevicesForUser("alice", "", 0)
	require.NoError(t, err)
	assert.Len(t, devicesPage.Data, 1)

	require.NoError(t, b.DeactivateMFADevice("alice", dev.SerialNumber))

	devicesPage2, err := b.ListMFADevicesForUser("alice", "", 0)
	require.NoError(t, err)
	assert.Empty(t, devicesPage2.Data)

	require.NoError(t, b.DeleteVirtualMFADevice(dev.SerialNumber))
}

func TestDeleteVirtualMFADevice_RequiresDeactivation(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("bob", "/", "")

	dev, err := b.CreateVirtualMFADeviceFull("bob-token", "/mfa/")
	require.NoError(t, err)

	require.NoError(t, b.EnableMFADevice("bob", dev.SerialNumber, "123456", "789012"))

	err = b.DeleteVirtualMFADevice(dev.SerialNumber)
	require.ErrorIs(t, err, iam.ErrDeleteConflict)

	require.NoError(t, b.DeactivateMFADevice("bob", dev.SerialNumber))
	require.NoError(t, b.DeleteVirtualMFADevice(dev.SerialNumber))
}

// TestListVirtualMFADevices_ItemShape_RealClient is a regression test for gopherstack-21my:
// ListVirtualMFADevices' item struct (VirtualMFADeviceXML, models_mfa.go) omitted User and
// Tags entirely, even though the real VirtualMFADevice deserializer
// (awsAwsquery_deserializeDocumentVirtualMFADevice) reads both and they are backed by real
// state: the user link is tracked in the same map GetMFADeviceOwner/EnableMFADevice already
// read and write, and tags are stored under the same "mfa:"-prefixed key
// TagMFADevice/ListMFADeviceTags already use. Seeds a device, enables it for a user with
// tags, and asserts both round-trip through the real SDK client.
func TestListVirtualMFADevices_ItemShape_RealClient(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	client := newTestIAMClient(t, h)

	_, err := client.CreateUser(t.Context(), &iamsdk.CreateUserInput{UserName: aws.String("mfa-owner")})
	require.NoError(t, err)

	created, err := client.CreateVirtualMFADevice(t.Context(), &iamsdk.CreateVirtualMFADeviceInput{
		VirtualMFADeviceName: aws.String("list-shape-mfa"),
		Tags: []types.Tag{
			{Key: aws.String("env"), Value: aws.String("prod")},
		},
	})
	require.NoError(t, err)
	serial := aws.ToString(created.VirtualMFADevice.SerialNumber)

	_, err = client.EnableMFADevice(t.Context(), &iamsdk.EnableMFADeviceInput{
		UserName:            aws.String("mfa-owner"),
		SerialNumber:        aws.String(serial),
		AuthenticationCode1: aws.String("123456"),
		AuthenticationCode2: aws.String("789012"),
	})
	require.NoError(t, err)

	listed, err := client.ListVirtualMFADevices(t.Context(), &iamsdk.ListVirtualMFADevicesInput{})
	require.NoError(t, err)

	var found *types.VirtualMFADevice
	for i := range listed.VirtualMFADevices {
		if aws.ToString(listed.VirtualMFADevices[i].SerialNumber) == serial {
			found = &listed.VirtualMFADevices[i]
		}
	}
	require.NotNil(t, found, "created device must appear in the list")

	require.NotNil(t, found.User, "User must round-trip, not decode nil")
	assert.Equal(t, "mfa-owner", aws.ToString(found.User.UserName))

	require.Len(t, found.Tags, 1)
	assert.Equal(t, "env", aws.ToString(found.Tags[0].Key))
	assert.Equal(t, "prod", aws.ToString(found.Tags[0].Value))
}

func TestListMFADevices_Pagination(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)
	client := newTestIAMClient(t, h)

	_, err := b.CreateUser("paula", "/", "")
	require.NoError(t, err)

	for i := range 3 {
		dev, devErr := b.CreateVirtualMFADeviceFull(fmt.Sprintf("paula-device-%d", i), "/")
		require.NoError(t, devErr)
		require.NoError(t, b.EnableMFADevice("paula", dev.SerialNumber, "111111", "222222"))
	}

	page1, err := client.ListMFADevices(t.Context(), &iamsdk.ListMFADevicesInput{
		UserName: aws.String("paula"),
		MaxItems: aws.Int32(2),
	})
	require.NoError(t, err)
	require.Len(t, page1.MFADevices, 2)
	assert.True(t, page1.IsTruncated)
	require.NotNil(t, page1.Marker)
	assert.NotEmpty(t, *page1.Marker)

	page2, err := client.ListMFADevices(t.Context(), &iamsdk.ListMFADevicesInput{
		UserName: aws.String("paula"),
		MaxItems: aws.Int32(2),
		Marker:   page1.Marker,
	})
	require.NoError(t, err)
	require.Len(t, page2.MFADevices, 1)
	assert.False(t, page2.IsTruncated)
	assert.Nil(t, page2.Marker)
}
