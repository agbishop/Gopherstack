package dax_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/dax"
)

// ---- Subnet Groups ----

func TestCreateSubnetGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check     func(t *testing.T, sg *dax.SubnetGroup)
		name      string
		sgName    string
		desc      string
		subnetIDs []string
		wantErr   bool
	}{
		{
			name:      "success",
			sgName:    "my-sg",
			desc:      "test subnet group",
			subnetIDs: []string{"subnet-11111111", "subnet-22222222"},
			check: func(t *testing.T, sg *dax.SubnetGroup) {
				t.Helper()
				assert.Equal(t, "my-sg", sg.SubnetGroupName)
				assert.Len(t, sg.Subnets, 2)
				assert.Equal(t, "subnet-11111111", sg.Subnets[0].SubnetID)
				assert.Equal(t, "us-east-1a", sg.Subnets[0].AvailabilityZone)
				// gopherstack does not model per-subnet IPv4/IPv6 CIDR allocation, so
				// SupportedNetworkTypes (types.SubnetGroup.SupportedNetworkTypes) is
				// always reported as IPv4-only.
				assert.Equal(t, []string{dax.NetworkTypeIPv4}, sg.SupportedNetworkTypes)
			},
		},
		{
			// SubnetGroupName carries no required/format check in the real API
			// (CreateSubnetGroup's declared error set has no
			// InvalidParameterValueException case) -- an empty name is accepted,
			// same as any other string.
			name:      "empty name accepted",
			sgName:    "",
			subnetIDs: []string{"subnet-11111111"},
			check: func(t *testing.T, sg *dax.SubnetGroup) {
				t.Helper()
				assert.Empty(t, sg.SubnetGroupName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()

			sg, err := b.CreateSubnetGroup(tt.sgName, tt.desc, tt.subnetIDs)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, sg)
			}
		})
	}
}

func TestCreateSubnetGroup_Duplicate(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateSubnetGroup("sg", "", []string{"subnet-11111111"})
	require.NoError(t, err)
	_, err = b.CreateSubnetGroup("sg", "", []string{"subnet-11111111"})
	require.Error(t, err)
}

// ---- CreateSubnetGroup: requires at least one subnet ----

func TestCreateSubnetGroupRequiresSubnet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		subnetIDs []string
		wantErr   bool
	}{
		{name: "nil subnets rejected", subnetIDs: nil, wantErr: true},
		{name: "empty subnets rejected", subnetIDs: []string{}, wantErr: true},
		{name: "one subnet accepted", subnetIDs: []string{"subnet-abc12345"}, wantErr: false},
		{name: "multiple subnets accepted", subnetIDs: []string{"subnet-aaaaaaaa", "subnet-bbbbbbbb"}, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			_, err := b.CreateSubnetGroup("mysg", "", tt.subnetIDs)

			if tt.wantErr {
				require.Error(t, err)
				// CreateSubnetGroup's own deserializeOpError switch has no
				// InvalidParameterValueException case; InvalidSubnet is the
				// code it actually types (see validateSubnetIDs).
				assert.ErrorIs(t, err, dax.ErrInvalidSubnet)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---- CreateSubnetGroup: VpcID is populated ----

func TestCreateSubnetGroupVpcIDPopulated(t *testing.T) {
	t.Parallel()
	b := newTestBackend()

	sg, err := b.CreateSubnetGroup("test-sg", "", []string{"subnet-abc12345"})
	require.NoError(t, err)
	assert.NotEmpty(t, sg.VpcID, "VpcID should be populated")
	assert.True(t, strings.HasPrefix(sg.VpcID, "vpc-"), "VpcID should start with vpc-")
}

// ---- SubnetGroupName has no format constraint in the real API ----
//
// Unlike ClusterName/ParameterGroupName, SubnetGroupName's real declared error
// set (CreateSubnetGroup's deserializeOpErrorCreateSubnetGroup) has no
// InvalidParameterValueException case, and its client-side validator
// (validateOpCreateSubnetGroupInput) checks only presence, not shape -- so
// none of these are rejected.
func TestCreateSubnetGroupNameNotFormatValidated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		sgName string
	}{
		{name: "alphanumeric-hyphen", sgName: "my-sg"},
		{name: "starts with digit", sgName: "1sg"},
		{name: "ends with hyphen", sgName: "sg-"},
		{name: "consecutive hyphens", sgName: "my--sg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			sg, err := b.CreateSubnetGroup(tt.sgName, "", []string{"subnet-11111111"})

			require.NoError(t, err)
			assert.Equal(t, tt.sgName, sg.SubnetGroupName)
		})
	}
}

func TestUpdateSubnetGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(b *dax.InMemoryBackend)
		check   func(t *testing.T, sg *dax.SubnetGroup)
		name    string
		input   dax.UpdateSubnetGroupInput
		wantErr bool
	}{
		{
			name: "update description",
			setup: func(b *dax.InMemoryBackend) {
				_, _ = b.CreateSubnetGroup("upd-sg", "old desc", []string{"subnet-11111111"})
			},
			input: dax.UpdateSubnetGroupInput{SubnetGroupName: "upd-sg", Description: "new desc"},
			check: func(t *testing.T, sg *dax.SubnetGroup) {
				t.Helper()
				assert.Equal(t, "new desc", sg.Description)
			},
		},
		{
			name: "update subnets",
			setup: func(b *dax.InMemoryBackend) {
				_, _ = b.CreateSubnetGroup("sub-sg", "", []string{"subnet-11111111"})
			},
			input: dax.UpdateSubnetGroupInput{
				SubnetGroupName: "sub-sg",
				SubnetIDs:       []string{"subnet-22222222", "subnet-33333333"},
			},
			check: func(t *testing.T, sg *dax.SubnetGroup) {
				t.Helper()
				assert.Len(t, sg.Subnets, 2)
				assert.Equal(t, "subnet-22222222", sg.Subnets[0].SubnetID)
			},
		},
		{
			name:    "not found",
			setup:   func(_ *dax.InMemoryBackend) {},
			input:   dax.UpdateSubnetGroupInput{SubnetGroupName: "no-such"},
			wantErr: true,
		},
		{
			// UpdateSubnetGroup's own deserializeOpErrorUpdateSubnetGroup switch has
			// no InvalidParameterValueException case; SubnetGroupNotFoundFault is the
			// code it actually types, matching DeleteSubnetGroup's treatment of the
			// same condition.
			name:    "empty name",
			setup:   func(_ *dax.InMemoryBackend) {},
			input:   dax.UpdateSubnetGroupInput{SubnetGroupName: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			tt.setup(b)

			sg, err := b.UpdateSubnetGroup(tt.input)

			if tt.wantErr {
				require.ErrorIs(t, err, dax.ErrSubnetGroupNotFound)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, sg)
			}
		})
	}
}

func TestDeleteSubnetGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(b *dax.InMemoryBackend)
		sgName  string
		wantErr bool
	}{
		{
			name: "success",
			setup: func(b *dax.InMemoryBackend) {
				_, _ = b.CreateSubnetGroup("sg-del", "", []string{"subnet-11111111"})
			},
			sgName: "sg-del",
		},
		{
			name:    "not found",
			setup:   func(_ *dax.InMemoryBackend) {},
			sgName:  "no-such",
			wantErr: true,
		},
		{
			name: "in use",
			setup: func(b *dax.InMemoryBackend) {
				_, _ = b.CreateCluster(validCreateInput("cluster-with-sg"))
			},
			sgName:  dax.DefaultSubnetGroupName,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			tt.setup(b)

			err := b.DeleteSubnetGroup(tt.sgName)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---- SubnetGroupInUseFault ----

func TestDeleteSubnetGroupInUseFault(t *testing.T) {
	t.Parallel()
	b := newTestBackend()

	_, err := b.CreateCluster(validCreateInput("uses-default"))
	require.NoError(t, err)

	err = b.DeleteSubnetGroup(dax.DefaultSubnetGroupName)
	require.Error(t, err)
	assert.ErrorIs(t, err, dax.ErrSubnetGroupInUse)
}

func TestDescribeSubnetGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(b *dax.InMemoryBackend)
		names     []string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "default group exists",
			setup:     func(_ *dax.InMemoryBackend) {},
			wantCount: 1,
		},
		{
			name: "with custom group",
			setup: func(b *dax.InMemoryBackend) {
				_, _ = b.CreateSubnetGroup("custom", "", []string{"subnet-11111111"})
			},
			wantCount: 2,
		},
		{
			name:    "not found",
			setup:   func(_ *dax.InMemoryBackend) {},
			names:   []string{"nonexistent"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			tt.setup(b)

			groups, _, err := b.DescribeSubnetGroups(tt.names, 0, "")

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, groups, tt.wantCount)
		})
	}
}

// ---- DescribeSubnetGroups pagination ----

func TestDescribeSubnetGroupsPagination(t *testing.T) {
	t.Parallel()
	b := newTestBackend()

	// Create additional groups beyond the default.
	for i := range 5 {
		name := []byte{'a' + byte(i)}
		_, err := b.CreateSubnetGroup(string(name)+"-sg", "", []string{"subnet-11111111"})
		require.NoError(t, err)
	}

	// First page of 2.
	page1, tok1, err := b.DescribeSubnetGroups(nil, 2, "")
	require.NoError(t, err)
	assert.Len(t, page1, 2)
	assert.NotEmpty(t, tok1)

	// Second page.
	page2, tok2, err := b.DescribeSubnetGroups(nil, 2, tok1)
	require.NoError(t, err)
	assert.Len(t, page2, 2)
	assert.NotEmpty(t, tok2)

	// Ensure no duplicates across pages.
	seen := make(map[string]bool)
	for _, sg := range append(page1, page2...) {
		assert.False(t, seen[sg.SubnetGroupName], "duplicate %s", sg.SubnetGroupName)
		seen[sg.SubnetGroupName] = true
	}
}
