package ec2_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ec2"
)

// TestCreateNetworkAcl verifies NACL creation.
func TestCreateNetworkAcl(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	acl, err := b.CreateNetworkACL("vpc-default")
	require.NoError(t, err)
	assert.NotEmpty(t, acl.ID)
	assert.Equal(t, "vpc-default", acl.VPCID)
	assert.False(t, acl.IsDefault)
	assert.NotEmpty(t, acl.Entries, "should have default deny-all entries")

	_, err = b.CreateNetworkACL("vpc-nonexistent")
	assert.Error(t, err)
}

// TestDeleteNetworkAcl verifies NACL deletion.

// TestDeleteNetworkAcl verifies NACL deletion.
func TestDeleteNetworkAcl(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	acl, err := b.CreateNetworkACL("vpc-default")
	require.NoError(t, err)

	require.NoError(t, b.DeleteNetworkACL(acl.ID))
	assert.Error(t, b.DeleteNetworkACL(acl.ID))
}

// TestDeleteNetworkAcl_DependencyViolation verifies AWS's DeleteNetworkAcl
// guard: an ACL still associated with a subnet cannot be deleted.
func TestDeleteNetworkAcl_DependencyViolation(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	vpc, err := b.CreateVpc("10.0.0.0/16", "default")
	require.NoError(t, err)

	acl1, err := b.CreateNetworkACL(vpc.ID)
	require.NoError(t, err)

	acl2, err := b.CreateNetworkACL(vpc.ID)
	require.NoError(t, err)

	subnet, err := b.CreateSubnet(vpc.ID, "10.0.1.0/24", "us-east-1a")
	require.NoError(t, err)

	_, err = b.ReplaceNetworkACLAssociation(acl1.ID, subnet.ID)
	require.NoError(t, err)

	err = b.DeleteNetworkACL(acl1.ID)
	require.ErrorIs(t, err, ec2.ErrDependencyViolation)

	// Move the subnet to the other ACL; deletion then succeeds.
	_, err = b.ReplaceNetworkACLAssociation(acl2.ID, subnet.ID)
	require.NoError(t, err)

	require.NoError(t, b.DeleteNetworkACL(acl1.ID))
}

// TestCreateNetworkAclEntry verifies NACL rule creation.

// TestCreateNetworkAclEntry verifies NACL rule creation.
func TestCreateNetworkAclEntry(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	acl, err := b.CreateNetworkACL("vpc-default")
	require.NoError(t, err)

	require.NoError(
		t,
		b.CreateNetworkACLEntry(acl.ID, 100, "6", "allow", "0.0.0.0/0", false, 80, 80),
	)

	// duplicate should fail
	assert.Error(t, b.CreateNetworkACLEntry(acl.ID, 100, "6", "allow", "0.0.0.0/0", false, 80, 80))
}

// TestDeleteNetworkAclEntry verifies NACL rule deletion.

// TestDeleteNetworkAclEntry verifies NACL rule deletion.
func TestDeleteNetworkAclEntry(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	acl, err := b.CreateNetworkACL("vpc-default")
	require.NoError(t, err)

	require.NoError(
		t,
		b.CreateNetworkACLEntry(acl.ID, 200, "6", "allow", "0.0.0.0/0", false, 443, 443),
	)
	require.NoError(t, b.DeleteNetworkACLEntry(acl.ID, 200, false))

	// non-existent should fail
	assert.Error(t, b.DeleteNetworkACLEntry(acl.ID, 200, false))
}

// TestNetworkAclPersistence verifies NACL Snapshot/Restore round-trip.

// TestNetworkAclPersistence verifies NACL Snapshot/Restore round-trip.
func TestNetworkAclPersistence(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	acl, err := b.CreateNetworkACL("vpc-default")
	require.NoError(t, err)
	require.NoError(
		t,
		b.CreateNetworkACLEntry(acl.ID, 100, "6", "allow", "0.0.0.0/0", false, 80, 80),
	)

	data := b.Snapshot(t.Context())
	require.NotNil(t, data)

	b2 := ec2.NewInMemoryBackend("123456789012", "us-east-1")
	require.NoError(t, b2.Restore(t.Context(), data))

	stored := b2.DescribeStoredNetworkAcls(nil)
	require.Len(t, stored, 1)
	// default deny-all (2) + our rule (1) = 3 entries
	assert.Len(t, stored[0].Entries, 3)
}

// TestGetSupportedOperationsIncludesExpectedOps verifies new ops appear in the list.
func TestGetSupportedOperationsIncludesExpectedOps(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")
	h := ec2.NewHandler(b)

	ops := h.GetSupportedOperations()
	opSet := make(map[string]bool, len(ops))
	for _, op := range ops {
		opSet[op] = true
	}

	for _, expected := range []string{
		"CreateSnapshot", "DescribeSnapshots", "DeleteSnapshot",
		"CopyImage", "DeregisterImage",
		"ModifyVpcAttribute", "ModifySubnetAttribute",
		"CreateNetworkAcl", "DeleteNetworkAcl",
		"CreateNetworkAclEntry", "DeleteNetworkAclEntry",
		"DescribeSecurityGroupRules", "ModifySecurityGroupRules",
		"DeleteLaunchTemplate", "DescribeLaunchTemplateVersions",
		"DeleteVpcEndpoints",
	} {
		assert.True(t, opSet[expected], "expected op %q to be in supported operations", expected)
	}
}

// TestDescribeSubnetsByVPC verifies indexed VPC-based subnet lookup.

// TestReplaceNetworkACLEntry verifies NACL rule replacement.
func TestReplaceNetworkACLEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		aclID      string
		protocol   string
		action     string
		cidr       string
		ruleNumber int
		setupACL   bool
		egress     bool
		wantErr    bool
	}{
		{
			name:       "replace_existing_rule",
			setupACL:   true,
			ruleNumber: 32767,
			protocol:   "-1",
			action:     "deny",
			cidr:       "10.0.0.0/8",
			egress:     false,
			wantErr:    false,
		},
		{
			name:       "missing_acl_id",
			aclID:      "",
			ruleNumber: 100,
			wantErr:    true,
		},
		{
			name:       "non_existent_acl",
			aclID:      "acl-nonexistent",
			ruleNumber: 100,
			wantErr:    true,
		},
		{
			name:       "non_existent_rule",
			setupACL:   true,
			ruleNumber: 999,
			protocol:   "-1",
			action:     "allow",
			cidr:       "0.0.0.0/0",
			egress:     false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

			aclID := tt.aclID

			if tt.setupACL {
				vpc, err := b.CreateVpc("10.0.0.0/16", "default")
				require.NoError(t, err)

				acl, err := b.CreateNetworkACL(vpc.ID)
				require.NoError(t, err)

				aclID = acl.ID
			}

			err := b.ReplaceNetworkACLEntry(
				aclID,
				tt.ruleNumber,
				tt.protocol,
				tt.action,
				tt.cidr,
				tt.egress,
				0,
				0,
			)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			acls := b.DescribeStoredNetworkAcls([]string{aclID})
			require.Len(t, acls, 1)

			var found bool

			for _, e := range acls[0].Entries {
				if e.RuleNumber == tt.ruleNumber && e.Egress == tt.egress {
					assert.Equal(t, tt.cidr, e.CIDRBlock)
					found = true

					break
				}
			}

			assert.True(t, found, "replaced rule should be present in ACL")
		})
	}
}

// TestReplaceNetworkACLAssociation tests subnet reassociation.

// TestReplaceNetworkACLAssociation tests subnet reassociation.
func TestReplaceNetworkACLAssociation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(b *ec2.InMemoryBackend) (aclID, subnetID string)
		name     string
		aclID    string
		subnetID string
		wantErr  bool
	}{
		{
			name: "valid_reassociation",
			setup: func(b *ec2.InMemoryBackend) (string, string) {
				vpc, err := b.CreateVpc("10.0.0.0/16", "default")
				if err != nil {
					return "", ""
				}

				acl, err := b.CreateNetworkACL(vpc.ID)
				if err != nil {
					return "", ""
				}

				subnet, err := b.CreateSubnet(vpc.ID, "10.0.1.0/24", "us-east-1a")
				if err != nil {
					return "", ""
				}

				return acl.ID, subnet.ID
			},
			wantErr: false,
		},
		{
			name:    "missing_acl_id",
			aclID:   "",
			wantErr: true,
		},
		{
			name:     "missing_subnet_id",
			subnetID: "",
			wantErr:  true,
		},
		{
			name:    "non_existent_acl",
			aclID:   "acl-nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

			aclID := tt.aclID
			subnetID := tt.subnetID

			if tt.setup != nil {
				aclID, subnetID = tt.setup(b)
			}

			assocID, err := b.ReplaceNetworkACLAssociation(aclID, subnetID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, assocID)
		})
	}
}

// TestDescribeVpcEndpointServices verifies the endpoint services list.

// TestHandlerReplaceNetworkACLEntry verifies the HTTP handler.
func TestHandlerReplaceNetworkACLEntry(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("000000000000", "us-east-1")

	vpc, err := b.CreateVpc("10.0.0.0/16", "default")
	require.NoError(t, err)

	acl, err := b.CreateNetworkACL(vpc.ID)
	require.NoError(t, err)

	h := ec2.NewHandler(b)
	h.AccountID = "000000000000"
	h.Region = "us-east-1"

	body := "Action=ReplaceNetworkAclEntry&Version=2016-11-15" +
		"&NetworkAclId=" + acl.ID +
		"&RuleNumber=32767" +
		"&Protocol=-1" +
		"&RuleAction=deny" +
		"&CidrBlock=192.168.0.0%2F16" +
		"&Egress=false"

	rec := postForm(t, h, body)
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "ReplaceNetworkAclEntryResponse")
}

// TestHandlerDescribeVpcEndpointServices verifies endpoint services.

// TestHTTP_CreateNetworkAcl verifies the HTTP handler for CreateNetworkAcl.
func TestHTTP_CreateNetworkAcl(t *testing.T) {
	t.Parallel()

	h := newHandler()

	rec := postForm(t, h, "Action=CreateNetworkAcl&Version=2016-11-15&VpcId=vpc-default")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "acl-")
}

// TestDescribeNetworkAcls_DefaultACLCascadesOnVpcDelete pins the cascade
// gopherstack-0o97 questioned: DeleteVpc doesn't store default network ACLs
// to delete, but DescribeNetworkAcls derives them per-VPC (see
// deepdive_ops.go), so a deleted VPC's default ACL stops appearing without
// any explicit cascade step, and an unrelated VPC's default ACL is
// untouched.
func TestDescribeNetworkAcls_DefaultACLCascadesOnVpcDelete(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	vpcA, err := b.CreateVpc("10.10.0.0/16", "default")
	require.NoError(t, err)

	vpcB, err := b.CreateVpc("10.20.0.0/16", "default")
	require.NoError(t, err)

	aclsA := b.DescribeNetworkAcls([]string{vpcA.ID})
	require.Len(t, aclsA, 1)
	assert.True(t, aclsA[0].IsDefault)

	require.NoError(t, b.DeleteVpc(vpcA.ID))

	assert.Empty(t, b.DescribeNetworkAcls([]string{vpcA.ID}),
		"deleted VPC's default ACL must no longer be returned")

	aclsB := b.DescribeNetworkAcls([]string{vpcB.ID})
	require.Len(t, aclsB, 1, "unrelated VPC's default ACL must be unaffected")
	assert.Equal(t, vpcB.ID, aclsB[0].VPCID)
	assert.True(t, aclsB[0].IsDefault)
}

// TestHTTP_DeleteLaunchTemplate verifies the HTTP handler for DeleteLaunchTemplate.
