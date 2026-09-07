package efs_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/efs"
)

// TestCreateMountTarget_IPAddressType verifies CreateMountTarget accepts
// IpAddressType (IPV4_ONLY/IPV6_ONLY/DUAL_STACK, default IPV4_ONLY when
// omitted, matching real AWS's documented default) and rejects unknown values,
// per aws-sdk-go-v2/service/efs's types.IpAddressType enum.
//
// The rejection was efs.ErrValidation until this pass; CreateMountTarget
// declares BadRequest, never ValidationException (efs@v1.44.4
// deserializers.go) -- the old assertion locked in the exact wire-code
// defect this pass fixed.
func TestCreateMountTarget_IPAddressType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		ipAddressType string
		wantErr       bool
	}{
		{name: "default_when_omitted"},
		{name: "ipv4_only", ipAddressType: "IPV4_ONLY"},
		{name: "ipv6_only", ipAddressType: "IPV6_ONLY"},
		{name: "dual_stack", ipAddressType: "DUAL_STACK"},
		{name: "unknown_value_rejected", ipAddressType: "BOGUS", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-mt-iat-"+tt.name))
			require.NoError(t, err)

			req := mtReq(fs.FileSystemID, "subnet-"+tt.name)
			req.IPAddressType = tt.ipAddressType

			mt, err := b.CreateMountTarget(context.Background(), req)

			if tt.wantErr {
				require.ErrorIs(t, err, efs.ErrBadRequest)

				return
			}
			require.NoError(t, err)
			require.NotNil(t, mt)
		})
	}
}

// TestCreateMountTarget_Ipv6Address verifies the mount target's Ipv6Address
// round-trips through the HTTP wire response, matching
// aws-sdk-go-v2/service/efs's types.MountTargetDescription.Ipv6Address field.
func TestCreateMountTarget_Ipv6Address(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ipv6Address string
	}{
		{name: "no_ipv6_address_omitted_from_response"},
		{name: "ipv6_address_present", ipv6Address: "2600:1f18:2144:e600::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEFSHandler()
			fsID := createFS(t, h, "tok-mt-ipv6-"+tt.name)

			body := map[string]any{
				"FileSystemId": fsID,
				"SubnetId":     "subnet-" + tt.name,
			}
			if tt.ipv6Address != "" {
				body["IpAddressType"] = "DUAL_STACK"
				body["Ipv6Address"] = tt.ipv6Address
			}

			rec := doREST(t, h, http.MethodPost, "/2015-02-01/mount-targets", body)
			require.Equal(t, http.StatusOK, rec.Code, "CreateMountTarget failed: %s", rec.Body.String())

			resp := parseResp(t, rec)
			if tt.ipv6Address == "" {
				assert.NotContains(t, resp, "Ipv6Address")
			} else {
				assert.Equal(t, tt.ipv6Address, resp["Ipv6Address"])
			}
		})
	}
}
