package redshift_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// ---- ModifyCluster ----

func TestRedshiftHandler_ModifyCluster(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=mod-cluster")
			},
			body: "Action=ModifyCluster&Version=2012-12-01" +
				"&ClusterIdentifier=mod-cluster&NodeType=ra3.xlplus&NumberOfNodes=3",
			wantCode:     http.StatusOK,
			wantContains: []string{"ModifyClusterResponse", "mod-cluster"},
		},
		{
			name:         "not_found",
			body:         "Action=ModifyCluster&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
		},
		{
			name:         "missing_id",
			body:         "Action=ModifyCluster&Version=2012-12-01",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "invalid_number_of_nodes",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=invalid-cluster")
			},
			body: "Action=ModifyCluster&Version=2012-12-01" +
				"&ClusterIdentifier=invalid-cluster&NumberOfNodes=notanumber",
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

// ---- RebootCluster ----

func TestRedshiftHandler_RebootCluster(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=reboot-cluster")
			},
			body:         "Action=RebootCluster&Version=2012-12-01&ClusterIdentifier=reboot-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"RebootClusterResponse", "reboot-cluster"},
		},
		{
			name:         "not_found",
			body:         "Action=RebootCluster&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
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

// ---- PauseCluster ----

func TestRedshiftHandler_PauseCluster(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=pause-cluster")
			},
			body:         "Action=PauseCluster&Version=2012-12-01&ClusterIdentifier=pause-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"PauseClusterResponse", "pause-cluster"},
		},
		{
			name:         "not_found",
			body:         "Action=PauseCluster&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
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

// ---- ResumeCluster ----

func TestRedshiftHandler_ResumeCluster(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=resume-cluster")
				postRedshiftForm(t, h, "Action=PauseCluster&Version=2012-12-01&ClusterIdentifier=resume-cluster")
			},
			body:         "Action=ResumeCluster&Version=2012-12-01&ClusterIdentifier=resume-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"ResumeClusterResponse", "resume-cluster"},
		},
		{
			name:         "not_found",
			body:         "Action=ResumeCluster&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
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

// ---- ResizeCluster ----

func TestRedshiftHandler_ResizeCluster(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=resize-cluster")
			},
			body: "Action=ResizeCluster&Version=2012-12-01" +
				"&ClusterIdentifier=resize-cluster&NodeType=ra3.4xlarge&NumberOfNodes=4",
			wantCode:     http.StatusOK,
			wantContains: []string{"ResizeClusterResponse", "resize-cluster"},
		},
		{
			name:         "not_found",
			body:         "Action=ResizeCluster&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
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

// ---- RotateEncryptionKey ----

func TestRedshiftHandler_RotateEncryptionKey(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=encrypt-cluster")
			},
			body:         "Action=RotateEncryptionKey&Version=2012-12-01&ClusterIdentifier=encrypt-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"RotateEncryptionKeyResponse", "encrypt-cluster"},
		},
		{
			name:         "not_found",
			body:         "Action=RotateEncryptionKey&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
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

// ---- ModifyClusterIamRoles ----

func TestRedshiftHandler_ModifyClusterIamRoles(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=iam-cluster")
			},
			body: "Action=ModifyClusterIamRoles&Version=2012-12-01" +
				"&ClusterIdentifier=iam-cluster" +
				"&AddIamRoles.IamRoleArn.1=arn:aws:iam::123:role/myrole",
			wantCode:     http.StatusOK,
			wantContains: []string{"ModifyClusterIamRolesResponse", "iam-cluster"},
		},
		{
			name:         "not_found",
			body:         "Action=ModifyClusterIamRoles&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
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

// ---- ModifyClusterMaintenance ----

func TestRedshiftHandler_ModifyClusterMaintenance(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=maint-cluster")
			},
			body: "Action=ModifyClusterMaintenance&Version=2012-12-01" +
				"&ClusterIdentifier=maint-cluster&MaintenanceTrackName=current",
			wantCode:     http.StatusOK,
			wantContains: []string{"ModifyClusterMaintenanceResponse", "maint-cluster"},
		},
		{
			name:         "not_found",
			body:         "Action=ModifyClusterMaintenance&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
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

// ---- CreateCluster identifier validation ----

// TestCreateCluster_IdentifierValidation verifies that CreateCluster
// enforces the AWS ClusterIdentifier naming rules:
//   - must begin with a lowercase letter
//   - only lowercase letters, digits, and hyphens
//   - must not end with a hyphen
//   - must not contain consecutive hyphens
//   - 1–63 characters
//
// Real AWS returns InvalidParameterCombination / ClusterIdentifierConstraint for
// violations; the emulator previously accepted any non-empty string.
func TestCreateCluster_IdentifierValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		id       string
		wantCode int
	}{
		{
			name:     "starts_with_digit_rejected",
			id:       "1cluster",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "starts_with_hyphen_rejected",
			id:       "-cluster",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "ends_with_hyphen_rejected",
			id:       "cluster-",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "consecutive_hyphens_rejected",
			id:       "my--cluster",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "uppercase_letter_rejected",
			id:       "MyCluster",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "valid_simple_name_accepted",
			id:       "mycluster",
			wantCode: http.StatusOK,
		},
		{
			name:     "valid_with_hyphens_accepted",
			id:       "my-cluster-1",
			wantCode: http.StatusOK,
		},
		{
			name:     "valid_min_length_accepted",
			id:       "a",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			body := "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=" + tt.id

			rec := postRedshiftForm(t, h, body)
			assert.Equal(t, tt.wantCode, rec.Code,
				"CreateCluster ClusterIdentifier=%q", tt.id)

			if tt.wantCode == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "InvalidParameterValue",
					"expected InvalidParameterValue error for ClusterIdentifier=%q", tt.id)
			}
		})
	}
}

// TestCreateCluster_IdentifierMaxLength verifies that a 63-character
// identifier is accepted and a 64-character one is rejected.
func TestCreateCluster_IdentifierMaxLength(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()

	// 63 chars: 'a' + 62 'b's = valid max
	validID := "a" + strings.Repeat("b", 62)

	rec := postRedshiftForm(t, h,
		"Action=CreateCluster&Version=2012-12-01&ClusterIdentifier="+validID)
	require.Equal(t, http.StatusOK, rec.Code, "63-char identifier should be accepted")

	// 64 chars: 'a' + 63 'b's = too long
	tooLongID := "a" + strings.Repeat("b", 63)
	rec2 := postRedshiftForm(t, h,
		"Action=CreateCluster&Version=2012-12-01&ClusterIdentifier="+tooLongID)
	assert.Equal(t, http.StatusBadRequest, rec2.Code, "64-char identifier should be rejected")
}

// ----- CreateCluster MasterUserPassword validation -----

// TestCreateCluster_PasswordValidation verifies that MasterUserPassword is validated
// when provided. Real AWS enforces password complexity rules.
func TestCreateCluster_PasswordValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		wantCode int
	}{
		{
			name:     "valid_password_accepted",
			password: "ValidPass1",
			wantCode: http.StatusOK,
		},
		{
			name:     "too_short_rejected",
			password: "Ab1",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "too_long_rejected",
			password: "Abcdef1" + strings.Repeat("x", 58),
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "no_uppercase_rejected",
			password: "validpass1",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "no_lowercase_rejected",
			password: "VALIDPASS1",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "no_digit_rejected",
			password: "ValidPassword",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "at_sign_rejected",
			password: "Valid@Pass1",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "slash_rejected",
			password: "Valid/Pass1",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "double_quote_rejected",
			password: `Valid"Pass1`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "space_rejected",
			password: "Valid Pass1",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "no_password_accepted",
			password: "",
			wantCode: http.StatusOK,
		},
		{
			name:     "exactly_8_chars_accepted",
			password: "Validx1a",
			wantCode: http.StatusOK,
		},
		{
			name:     "exactly_64_chars_accepted",
			password: "Validx1" + strings.Repeat("a", 57),
			wantCode: http.StatusOK,
		},
		{
			name:     "65_chars_rejected",
			password: "Validx1" + strings.Repeat("a", 58),
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			clID := "pw-" + strings.ReplaceAll(tt.name, "_", "-")
			body := "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=" + clID
			if tt.password != "" {
				body += "&MasterUserPassword=" + tt.password
			}

			rec := postRedshiftForm(t, h, body)
			assert.Equal(t, tt.wantCode, rec.Code,
				"CreateCluster with MasterUserPassword=%q", tt.password)

			if tt.wantCode == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "InvalidParameterValue",
					"expected InvalidParameterValue for invalid password %q", tt.password)
			}
		})
	}
}

// TestModifyClusterIamRoles_Persists verifies that IAM roles are stored and returned.
func TestModifyClusterIamRoles_Persists(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=iam-cluster")

	// Add two roles.
	rec := postRedshiftForm(t, h,
		"Action=ModifyClusterIamRoles&Version=2012-12-01&ClusterIdentifier=iam-cluster"+
			"&AddIamRoles.IamRoleArn.1=arn:aws:iam::123456789012:role/Role1"+
			"&AddIamRoles.IamRoleArn.2=arn:aws:iam::123456789012:role/Role2")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Role1")
	assert.Contains(t, rec.Body.String(), "Role2")

	// Describe cluster — roles must be present.
	rec2 := postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=iam-cluster")
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "Role1")
	assert.Contains(t, rec2.Body.String(), "Role2")

	// Remove one role.
	rec3 := postRedshiftForm(t, h,
		"Action=ModifyClusterIamRoles&Version=2012-12-01&ClusterIdentifier=iam-cluster"+
			"&RemoveIamRoles.IamRoleArn.1=arn:aws:iam::123456789012:role/Role1")
	require.Equal(t, http.StatusOK, rec3.Code)

	// Describe cluster — only Role2 remains.
	rec4 := postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=iam-cluster")
	require.Equal(t, http.StatusOK, rec4.Code)
	assert.NotContains(t, rec4.Body.String(), "Role1")
	assert.Contains(t, rec4.Body.String(), "Role2")
}

// TestModifyClusterIamRoles_RejectsWhenClusterNotAvailable verifies real
// AWS's InvalidClusterStateFault precondition on ModifyClusterIamRoles
// (confirmed against this op's declared error set in botocore's
// redshift/2012-12-01/service-2.json -- only InvalidClusterStateFault and
// ClusterNotFoundFault -- and awsAwsquery_deserializeOpErrorModifyClusterIamRoles
// in aws-sdk-go-v2/service/redshift@v1.65.4/deserializers.go).
func TestModifyClusterIamRoles_RejectsWhenClusterNotAvailable(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=iam-paused-cluster")

	recPause := postRedshiftForm(t, h,
		"Action=PauseCluster&Version=2012-12-01&ClusterIdentifier=iam-paused-cluster")
	require.Equal(t, http.StatusOK, recPause.Code)

	rec := postRedshiftForm(t, h,
		"Action=ModifyClusterIamRoles&Version=2012-12-01&ClusterIdentifier=iam-paused-cluster"+
			"&AddIamRoles.IamRoleArn.1=arn:aws:iam::123456789012:role/Role1")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidClusterState")

	recDescribe := postRedshiftForm(t, h,
		"Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=iam-paused-cluster")
	require.Equal(t, http.StatusOK, recDescribe.Code)
	assert.NotContains(t, recDescribe.Body.String(), "Role1")
}

// TestModifyClusterMaintenance_Persists verifies that maintenance window is stored.
func TestModifyClusterMaintenance_Persists(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=maint-cluster")

	rec := postRedshiftForm(t, h,
		"Action=ModifyClusterMaintenance&Version=2012-12-01&ClusterIdentifier=maint-cluster"+
			"&MaintenanceTrackName=current")
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=maint-cluster")
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "current")
}

// TestModifyCluster_ApplyImmediately verifies PendingModifiedValues semantics.
func TestModifyCluster_ApplyImmediately(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantNodeTypeInCluster string
		name                  string
		applyImmediately      string
		newNodeType           string
	}{
		{
			name:                  "apply_immediately_true_changes_cluster",
			applyImmediately:      "true",
			newNodeType:           "ds2.xlarge",
			wantNodeTypeInCluster: "ds2.xlarge",
		},
		{
			name:                  "apply_immediately_false_stores_pending",
			applyImmediately:      "false",
			newNodeType:           "ds2.xlarge",
			wantNodeTypeInCluster: "dc2.large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=apply-cluster")

			postRedshiftForm(t, h,
				"Action=ModifyCluster&Version=2012-12-01&ClusterIdentifier=apply-cluster"+
					"&NodeType="+tt.newNodeType+
					"&ApplyImmediately="+tt.applyImmediately)

			rec := postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=apply-cluster")
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantNodeTypeInCluster)
		})
	}
}

// TestModifyCluster_RejectsWhenClusterNotAvailable verifies real AWS's
// InvalidClusterStateFault precondition ("The specified cluster is not in
// the available state", confirmed against InvalidClusterStateFault's
// documentation in botocore's redshift/2012-12-01/service-2.json and its
// presence in ModifyCluster's declared error set, awsAwsquery_deserializeOpErrorModifyCluster
// in aws-sdk-go-v2/service/redshift@v1.65.4/deserializers.go). A paused
// cluster stays paused indefinitely in this backend (PauseCluster sets
// Status="paused" with no reconciler transition back), so this is reachable
// without any activation-delay configuration.
func TestModifyCluster_RejectsWhenClusterNotAvailable(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=paused-cluster")

	recPause := postRedshiftForm(t, h, "Action=PauseCluster&Version=2012-12-01&ClusterIdentifier=paused-cluster")
	require.Equal(t, http.StatusOK, recPause.Code)

	rec := postRedshiftForm(t, h,
		"Action=ModifyCluster&Version=2012-12-01&ClusterIdentifier=paused-cluster&NodeType=ra3.xlplus")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidClusterState")

	// The rejected modify must not have mutated the cluster: NodeType stays
	// at its original value and status stays "paused", not silently flipped
	// back to "available" by the rejected call.
	recDescribe := postRedshiftForm(t, h,
		"Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=paused-cluster")
	require.Equal(t, http.StatusOK, recDescribe.Code)
	assert.Contains(t, recDescribe.Body.String(), "dc2.large")
	assert.NotContains(t, recDescribe.Body.String(), "ra3.xlplus")
	assert.Contains(t, recDescribe.Body.String(), "<ClusterStatus>paused</ClusterStatus>")
}

// TestModifyCluster_EncryptedTriState verifies that Encrypted and
// EnhancedVpcRouting are tri-state on the wire: omitting the field leaves the
// setting unchanged, while explicitly sending "false" turns it off (e.g.
// decrypting a cluster). A plain bool previously could not distinguish
// "not specified" from "explicitly false", making it impossible to ever
// disable either setting via ModifyCluster.
func TestModifyCluster_EncryptedTriState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		modifyBody    string
		wantEncrypted string
	}{
		{
			name:          "unset_leaves_encrypted_true_unchanged",
			modifyBody:    "&NodeType=ra3.xlplus",
			wantEncrypted: "<Encrypted>true</Encrypted>",
		},
		{
			name:          "explicit_false_decrypts",
			modifyBody:    "&Encrypted=false",
			wantEncrypted: "<Encrypted>false</Encrypted>",
		},
		{
			name:          "explicit_true_stays_encrypted",
			modifyBody:    "&Encrypted=true",
			wantEncrypted: "<Encrypted>true</Encrypted>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=enc-cluster")
			postRedshiftForm(t, h,
				"Action=ModifyCluster&Version=2012-12-01&ClusterIdentifier=enc-cluster&Encrypted=true")

			rec := postRedshiftForm(t, h,
				"Action=ModifyCluster&Version=2012-12-01&ClusterIdentifier=enc-cluster"+tt.modifyBody)
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantEncrypted)
		})
	}
}

// ---- FailoverPrimaryCompute ----

func TestHandler_FailoverPrimaryCompute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=fo-cluster")
			},
			body:         "Action=FailoverPrimaryCompute&Version=2012-12-01&ClusterIdentifier=fo-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"FailoverPrimaryComputeResponse", "fo-cluster"},
		},
		{
			name:     "cluster_not_found",
			body:     "Action=FailoverPrimaryCompute&Version=2012-12-01&ClusterIdentifier=missing",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}
