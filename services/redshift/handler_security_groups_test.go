package redshift_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// ---- CreateClusterSecurityGroup ----

func TestRedshiftHandler_CreateClusterSecurityGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			body: "Action=CreateClusterSecurityGroup&Version=2012-12-01" +
				"&ClusterSecurityGroupName=my-sg&Description=my+sg+description",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateClusterSecurityGroupResponse", "my-sg"},
		},
		{
			name:         "missing_name",
			body:         "Action=CreateClusterSecurityGroup&Version=2012-12-01&Description=desc",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "duplicate",
			body: "Action=CreateClusterSecurityGroup&Version=2012-12-01" +
				"&ClusterSecurityGroupName=dup-sg",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterSecurityGroupAlreadyExists"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.name == "duplicate" {
				postRedshiftForm(t, h, "Action=CreateClusterSecurityGroup&Version=2012-12-01"+
					"&ClusterSecurityGroupName=dup-sg")
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DeleteClusterSecurityGroup ----

func TestRedshiftHandler_DeleteClusterSecurityGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateClusterSecurityGroup&Version=2012-12-01"+
					"&ClusterSecurityGroupName=del-sg")
			},
			body:         "Action=DeleteClusterSecurityGroup&Version=2012-12-01&ClusterSecurityGroupName=del-sg",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteClusterSecurityGroupResponse"},
		},
		{
			name:         "not_found",
			body:         "Action=DeleteClusterSecurityGroup&Version=2012-12-01&ClusterSecurityGroupName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterSecurityGroupNotFound"},
		},
		{
			name:         "missing_name",
			body:         "Action=DeleteClusterSecurityGroup&Version=2012-12-01",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			// Real AWS: "You cannot delete the default security group."
			name:         "default_security_group_rejected",
			body:         "Action=DeleteClusterSecurityGroup&Version=2012-12-01&ClusterSecurityGroupName=default",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidClusterSecurityGroupState"},
		},
		{
			// Real AWS: "You cannot delete a security group that is
			// associated with any clusters" (api_op_DeleteClusterSecurityGroup.go).
			name: "associated_with_cluster_rejected",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateClusterSecurityGroup&Version=2012-12-01"+
					"&ClusterSecurityGroupName=assoc-sg")
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01"+
					"&ClusterIdentifier=assoc-sg-cluster&NodeType=dc2.large"+
					"&MasterUsername=admin&MasterUserPassword=Password1"+
					"&ClusterSecurityGroups.ClusterSecurityGroupName.1=assoc-sg")
			},
			body:         "Action=DeleteClusterSecurityGroup&Version=2012-12-01&ClusterSecurityGroupName=assoc-sg",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidClusterSecurityGroupState"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DescribeClusterSecurityGroups ----

func TestRedshiftHandler_DescribeClusterSecurityGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "list_empty",
			body:         "Action=DescribeClusterSecurityGroups&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeClusterSecurityGroupsResponse"},
		},
		{
			name: "list_with_sg",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateClusterSecurityGroup&Version=2012-12-01"+
					"&ClusterSecurityGroupName=my-sg2")
			},
			body:         "Action=DescribeClusterSecurityGroups&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeClusterSecurityGroupsResponse", "my-sg2"},
		},
		{
			name: "describe_specific",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateClusterSecurityGroup&Version=2012-12-01"+
					"&ClusterSecurityGroupName=specific-sg")
			},
			body:         "Action=DescribeClusterSecurityGroups&Version=2012-12-01&ClusterSecurityGroupName=specific-sg",
			wantCode:     http.StatusOK,
			wantContains: []string{"specific-sg"},
		},
		{
			name:         "not_found",
			body:         "Action=DescribeClusterSecurityGroups&Version=2012-12-01&ClusterSecurityGroupName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterSecurityGroupNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- RevokeClusterSecurityGroupIngress ----

func TestRedshiftHandler_RevokeClusterSecurityGroupIngress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "revoke_cidr",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateClusterSecurityGroup&Version=2012-12-01"+
					"&ClusterSecurityGroupName=revoke-sg")
				postRedshiftForm(t, h, "Action=AuthorizeClusterSecurityGroupIngress&Version=2012-12-01"+
					"&ClusterSecurityGroupName=revoke-sg&CIDRIP=10.0.0.0/8")
			},
			body: "Action=RevokeClusterSecurityGroupIngress&Version=2012-12-01" +
				"&ClusterSecurityGroupName=revoke-sg&CIDRIP=10.0.0.0/8",
			wantCode:     http.StatusOK,
			wantContains: []string{"RevokeClusterSecurityGroupIngressResponse", "revoke-sg"},
		},
		{
			name: "not_found",
			body: "Action=RevokeClusterSecurityGroupIngress&Version=2012-12-01" +
				"&ClusterSecurityGroupName=nonexistent&CIDRIP=10.0.0.0/8",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterSecurityGroupNotFound"},
		},
		{
			name: "cidr_never_authorized",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateClusterSecurityGroup&Version=2012-12-01"+
					"&ClusterSecurityGroupName=unauth-sg")
			},
			body: "Action=RevokeClusterSecurityGroupIngress&Version=2012-12-01" +
				"&ClusterSecurityGroupName=unauth-sg&CIDRIP=10.0.0.0/8",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"AuthorizationNotFound"},
		},
		{
			name:         "missing_cidr_and_ec2_group",
			body:         "Action=RevokeClusterSecurityGroupIngress&Version=2012-12-01&ClusterSecurityGroupName=some-sg",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- Backend: SecurityGroupCount ----

func TestRedshiftBackend_SecurityGroupCount(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	require.Equal(t, 0, redshift.SecurityGroupCount(b))

	h := redshift.NewHandler(b)
	postRedshiftForm(t, h, "Action=CreateClusterSecurityGroup&Version=2012-12-01"+
		"&ClusterSecurityGroupName=count-sg")
	require.Equal(t, 1, redshift.SecurityGroupCount(b))

	postRedshiftForm(t, h, "Action=DeleteClusterSecurityGroup&Version=2012-12-01"+
		"&ClusterSecurityGroupName=count-sg")
	require.Equal(t, 0, redshift.SecurityGroupCount(b))
}

// ---- AuthorizeClusterSecurityGroupIngress ----

func TestHandler_AuthorizeClusterSecurityGroupIngress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success_cidr",
			setup: func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend) {
				b.AddSecurityGroupInternal(&redshift.ClusterSecurityGroup{
					ClusterSecurityGroupName: "my-sg",
					Description:              "test sg",
				})
			},
			body: "Action=AuthorizeClusterSecurityGroupIngress&Version=2012-12-01" +
				"&ClusterSecurityGroupName=my-sg&CIDRIP=10.0.0.0%2F8",
			wantCode:     http.StatusOK,
			wantContains: []string{"AuthorizeClusterSecurityGroupIngressResponse", "10.0.0.0/8"},
		},
		{
			name: "success_ec2_sg",
			setup: func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend) {
				b.AddSecurityGroupInternal(&redshift.ClusterSecurityGroup{
					ClusterSecurityGroupName: "my-sg2",
				})
			},
			body: "Action=AuthorizeClusterSecurityGroupIngress&Version=2012-12-01" +
				"&ClusterSecurityGroupName=my-sg2&EC2SecurityGroupName=sg-12345&EC2SecurityGroupOwnerId=111111111111",
			wantCode:     http.StatusOK,
			wantContains: []string{"AuthorizeClusterSecurityGroupIngressResponse", "sg-12345"},
		},
		{
			name:         "missing_group_name",
			body:         "Action=AuthorizeClusterSecurityGroupIngress&Version=2012-12-01&CIDRIP=10.0.0.0/8",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "missing_cidr_and_ec2_sg",
			body:         "Action=AuthorizeClusterSecurityGroupIngress&Version=2012-12-01&ClusterSecurityGroupName=my-sg",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "security_group_not_found",
			body: "Action=AuthorizeClusterSecurityGroupIngress&Version=2012-12-01" +
				"&ClusterSecurityGroupName=nonexistent&CIDRIP=10.0.0.1/32",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterSecurityGroupNotFound"},
		},
		{
			name: "duplicate_cidr",
			setup: func(_ *testing.T, _ *redshift.Handler, b *redshift.InMemoryBackend) {
				b.AddSecurityGroupInternal(&redshift.ClusterSecurityGroup{
					ClusterSecurityGroupName: "dup-sg",
					IPRanges:                 []redshift.IPRange{{CIDRIP: "10.0.0.0/8", Status: "authorized"}},
				})
			},
			body: "Action=AuthorizeClusterSecurityGroupIngress&Version=2012-12-01" +
				"&ClusterSecurityGroupName=dup-sg&CIDRIP=10.0.0.0%2F8",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"AuthorizationAlreadyExists"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)
			if tt.setup != nil {
				tt.setup(t, h, b)
			}

			rec := postRedshiftForm(t, h, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- AuthorizeClusterSecurityGroupIngress: missing both CIDRIP and EC2GroupName ----

func TestAuthorizeClusterSecurityGroupIngress_MissingBothParams(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(b)

	b.AddSecurityGroupInternal(&redshift.ClusterSecurityGroup{ClusterSecurityGroupName: "sg-test"})

	rec := postRedshiftForm(t, h,
		"Action=AuthorizeClusterSecurityGroupIngress&Version=2012-12-01"+
			"&ClusterSecurityGroupName=sg-test")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterValue")
}

// ---- AuthorizeClusterSecurityGroupIngress: security group not found ----

func TestAuthorizeClusterSecurityGroupIngress_SGNotFound(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	rec := postRedshiftForm(t, h,
		"Action=AuthorizeClusterSecurityGroupIngress&Version=2012-12-01"+
			"&ClusterSecurityGroupName=nonexistent"+
			"&CIDRIP=10.0.0.0/8")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ClusterSecurityGroupNotFound")
}

// ---- AuthorizeClusterSecurityGroupIngress: EC2 security group ----

func TestAuthorizeClusterSecurityGroupIngress_EC2Group(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(b)

	b.AddSecurityGroupInternal(&redshift.ClusterSecurityGroup{
		ClusterSecurityGroupName: "ec2-sg-test",
		Description:              "Test security group",
	})

	rec := postRedshiftForm(t, h,
		"Action=AuthorizeClusterSecurityGroupIngress&Version=2012-12-01"+
			"&ClusterSecurityGroupName=ec2-sg-test"+
			"&EC2SecurityGroupName=sg-abc123"+
			"&EC2SecurityGroupOwnerId=999888777666")

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "AuthorizeClusterSecurityGroupIngressResponse")
	assert.Contains(t, body, "sg-abc123")
	assert.Contains(t, body, "authorized")
}
