package rds_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

func TestDBSecurityGroup_CRUD(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()

	sg, err := b.CreateDBSecurityGroup("my-sg", "legacy security group")
	require.NoError(t, err)
	assert.Equal(t, "my-sg", sg.DBSecurityGroupName)

	groups, err := b.DescribeDBSecurityGroups("my-sg")
	require.NoError(t, err)
	require.Len(t, groups, 1)

	err = b.DeleteDBSecurityGroup("my-sg")
	require.NoError(t, err)

	_, err = b.DescribeDBSecurityGroups("my-sg")
	require.Error(t, err)
	assert.ErrorIs(t, err, rds.ErrDBSecurityGroupNotFound)
}

func TestDBSecurityGroup_AuthorizeRevoke(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBSecurityGroup("auth-sg", "test")
	require.NoError(t, err)

	sg, err := b.AuthorizeDBSecurityGroupIngress("auth-sg", "10.0.0.0/8")
	require.NoError(t, err)
	require.Len(t, sg.IPRanges, 1)
	assert.Equal(t, "10.0.0.0/8", sg.IPRanges[0].CIDRIP)

	sg, err = b.RevokeDBSecurityGroupIngress("auth-sg", "10.0.0.0/8")
	require.NoError(t, err)
	assert.Empty(t, sg.IPRanges)
}

func TestDBSecurityGroup_Duplicate(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBSecurityGroup("dup-sg", "first")
	require.NoError(t, err)

	_, err = b.CreateDBSecurityGroup("dup-sg", "second")
	require.Error(t, err)
	assert.ErrorIs(t, err, rds.ErrDBSecurityGroupAlreadyExists)
}

func TestDBSecurityGroup_NotFound(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()

	// AuthorizeDBSecurityGroupIngress auto-creates the group (EC2-Classic behaviour)
	sg, err := b.AuthorizeDBSecurityGroupIngress("auto-created-sg", "0.0.0.0/0")
	require.NoError(t, err)
	assert.Equal(t, "auto-created-sg", sg.DBSecurityGroupName)

	err = b.DeleteDBSecurityGroup("noexist")
	require.Error(t, err)
	assert.ErrorIs(t, err, rds.ErrDBSecurityGroupNotFound)
}

func TestDBSecurityGroup_HTTP(t *testing.T) {
	t.Parallel()

	h := newBatch2Handler()

	rec := postRDSForm(t, h, url.Values{
		"Action":                     {"CreateDBSecurityGroup"},
		"Version":                    {"2014-10-31"},
		"DBSecurityGroupName":        {"http-legacy-sg"},
		"DBSecurityGroupDescription": {"legacy sg for test"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":              {"DescribeDBSecurityGroups"},
		"Version":             {"2014-10-31"},
		"DBSecurityGroupName": {"http-legacy-sg"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "http-legacy-sg")

	rec = postRDSForm(t, h, url.Values{
		"Action":              {"AuthorizeDBSecurityGroupIngress"},
		"Version":             {"2014-10-31"},
		"DBSecurityGroupName": {"http-legacy-sg"},
		"CIDRIP":              {"192.168.0.0/16"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":              {"RevokeDBSecurityGroupIngress"},
		"Version":             {"2014-10-31"},
		"DBSecurityGroupName": {"http-legacy-sg"},
		"CIDRIP":              {"192.168.0.0/16"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":              {"DeleteDBSecurityGroup"},
		"Version":             {"2014-10-31"},
		"DBSecurityGroupName": {"http-legacy-sg"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestRDSBackend_AuthorizeDBSecurityGroupIngress tests AuthorizeDBSecurityGroupIngress.
func TestRDSBackend_AuthorizeDBSecurityGroupIngress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(b *rds.InMemoryBackend)
		name      string
		groupName string
		cidrIP    string
		wantErrIs error
		wantCIDRs []string
		wantErr   bool
	}{
		{
			name:      "creates_new_group",
			setup:     func(_ *rds.InMemoryBackend) {},
			groupName: "my-sg",
			cidrIP:    "10.0.0.0/8",
			wantCIDRs: []string{"10.0.0.0/8"},
		},
		{
			name: "adds_to_existing",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.AuthorizeDBSecurityGroupIngress("my-sg", "10.0.0.0/8")
			},
			groupName: "my-sg",
			cidrIP:    "192.168.0.0/16",
			wantCIDRs: []string{"10.0.0.0/8", "192.168.0.0/16"},
		},
		{
			name: "idempotent_duplicate",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.AuthorizeDBSecurityGroupIngress("my-sg", "10.0.0.0/8")
			},
			groupName: "my-sg",
			cidrIP:    "10.0.0.0/8",
			wantCIDRs: []string{"10.0.0.0/8"},
		},
		{
			name:      "empty_group_name",
			setup:     func(_ *rds.InMemoryBackend) {},
			groupName: "",
			cidrIP:    "10.0.0.0/8",
			wantErr:   true,
			wantErrIs: rds.ErrInvalidParameter,
		},
		{
			name:      "empty_cidr",
			setup:     func(_ *rds.InMemoryBackend) {},
			groupName: "my-sg",
			cidrIP:    "",
			wantErr:   true,
			wantErrIs: rds.ErrInvalidParameter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := rds.NewInMemoryBackend("000000000000", "us-east-1")
			tt.setup(b)

			sg, err := b.AuthorizeDBSecurityGroupIngress(tt.groupName, tt.cidrIP)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrIs)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.groupName, sg.DBSecurityGroupName)

			cidrs := make([]string, 0, len(sg.IPRanges))
			for _, r := range sg.IPRanges {
				cidrs = append(cidrs, r.CIDRIP)
			}

			assert.Equal(t, tt.wantCIDRs, cidrs)
		})
	}
}

func TestDescribeDBSecurityGroups(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErrIs error
		name      string
		filter    string
		wantCount int
		wantErr   bool
	}{
		{name: "all", filter: "", wantCount: 2},
		{name: "by name", filter: "sg-1", wantCount: 1},
		{name: "not found", filter: "missing", wantErr: true, wantErrIs: rds.ErrDBSecurityGroupNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend(t)
			b.AddSecurityGroupInternal("sg-1", "Security group 1")
			b.AddSecurityGroupInternal("sg-2", "Security group 2")
			got, err := b.DescribeDBSecurityGroups(tt.filter)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.wantCount)
		})
	}
}

// TestDeleteDBSecurityGroup_RejectsDefault locks real AWS's
// DeleteDBSecurityGroupInput.DBSecurityGroupName doc comment: "You can't
// delete the default DB security group" ("Must not be \"Default\"").
func TestDeleteDBSecurityGroup_RejectsDefault(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	b.AddSecurityGroupInternal("Default", "default security group")

	err := b.DeleteDBSecurityGroup("Default")
	require.ErrorIs(t, err, rds.ErrDBSecurityGroupInvalidState)

	groups, descErr := b.DescribeDBSecurityGroups("Default")
	require.NoError(t, descErr)
	require.Len(t, groups, 1, "default security group must survive the rejected delete")
}

func TestDeleteDBSecurityGroup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		setup     func(t *testing.T, b *rds.InMemoryBackend)
		wantErrIs error
		name      string
		sgName    string
		wantErr   bool
	}{
		{
			name:   "success",
			sgName: "sg-1",
			setup: func(_ *testing.T, b *rds.InMemoryBackend) {
				b.AddSecurityGroupInternal("sg-1", "desc")
			},
		},
		{name: "not found", sgName: "missing", wantErr: true, wantErrIs: rds.ErrDBSecurityGroupNotFound},
		{name: "empty name", sgName: "", wantErr: true, wantErrIs: rds.ErrInvalidParameter},
		{
			// Real AWS: "The specified DB security group must not be
			// associated with any DB instances" (api_op_DeleteDBSecurityGroup.go).
			name:      "associated with instance",
			sgName:    "assoc-sg",
			wantErr:   true,
			wantErrIs: rds.ErrDBSecurityGroupInvalidState,
			setup: func(t *testing.T, b *rds.InMemoryBackend) {
				t.Helper()
				b.AddSecurityGroupInternal("assoc-sg", "desc")
				_, err := b.CreateDBInstance("assoc-inst", "postgres", "db.t3.micro", "", "admin", "", 20,
					rds.DBInstanceOptions{DBSecurityGroupNames: []string{"assoc-sg"}})
				require.NoError(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend(t)
			if tt.setup != nil {
				tt.setup(t, b)
			}
			err := b.DeleteDBSecurityGroup(tt.sgName)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
		})
	}
}

// TestDeleteDBSecurityGroup_ClearsTags guards against the ghost-row class:
// a security group deleted and recreated under the same name must not
// inherit the deleted group's tags, since DBSecurityGroupArn is a
// deterministic function of the name.
func TestDeleteDBSecurityGroup_ClearsTags(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	sg, err := b.CreateDBSecurityGroup("ghost-sg", "original")
	require.NoError(t, err)
	require.NotEmpty(t, sg.DBSecurityGroupArn)

	b.AddTagsToResource(sg.DBSecurityGroupArn, []rds.Tag{{Key: "env", Value: "prod"}})
	require.Len(t, b.ListTagsForResource(sg.DBSecurityGroupArn), 1)

	require.NoError(t, b.DeleteDBSecurityGroup("ghost-sg"))

	recreated, err := b.CreateDBSecurityGroup("ghost-sg", "recreated")
	require.NoError(t, err)
	require.Equal(t, sg.DBSecurityGroupArn, recreated.DBSecurityGroupArn, "ARN is deterministic by name")

	assert.Empty(t, b.ListTagsForResource(recreated.DBSecurityGroupArn),
		"recreated security group must not inherit tags from the deleted one")
}

func TestRevokeDBSecurityGroupIngress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErrIs error
		name      string
		groupName string
		cidrIP    string
		wantErr   bool
	}{
		{name: "success", groupName: "sg-1", cidrIP: "10.0.0.0/8"},
		{
			name:      "group not found",
			groupName: "missing",
			cidrIP:    "10.0.0.0/8",
			wantErr:   true,
			wantErrIs: rds.ErrDBSecurityGroupNotFound,
		},
		{
			name:      "CIDR not found",
			groupName: "sg-1",
			cidrIP:    "192.168.0.0/16",
			wantErr:   true,
			wantErrIs: rds.ErrInvalidParameter,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend(t)
			if tt.name != "group not found" {
				b.AddSecurityGroupInternal("sg-1", "desc")
				_, err := b.AuthorizeDBSecurityGroupIngress("sg-1", "10.0.0.0/8")
				require.NoError(t, err)
			}
			got, err := b.RevokeDBSecurityGroupIngress(tt.groupName, tt.cidrIP)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)

			for _, r := range got.IPRanges {
				assert.NotEqual(t, tt.cidrIP, r.CIDRIP, "revoked CIDR must not remain in IPRanges")
			}
		})
	}
}

func TestCreateDBSecurityGroupR2(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErrIs   error
		name        string
		groupName   string
		description string
		wantErr     bool
	}{
		{
			name:        "success",
			groupName:   "my-sg",
			description: "test group",
		},
		{
			name:      "empty name",
			groupName: "",
			wantErr:   true,
			wantErrIs: rds.ErrInvalidParameter,
		},
		{
			name:      "already exists",
			groupName: "dup-sg",
			wantErr:   true,
			wantErrIs: rds.ErrDBSecurityGroupAlreadyExists,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend(t)
			if tt.name == "already exists" {
				_, err := b.CreateDBSecurityGroup(tt.groupName, tt.description)
				require.NoError(t, err)
			}
			got, err := b.CreateDBSecurityGroup(tt.groupName, tt.description)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.groupName, got.DBSecurityGroupName)
			assert.Equal(t, tt.description, got.DBSecurityGroupDescription)
		})
	}
}
