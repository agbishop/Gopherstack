package rds_test

import (
	"encoding/xml"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

// TestRDSErrorCodes_FaultSuffix is a regression test for a wire-code bug
// found by field-diffing gopherstack's error-code table against
// aws-sdk-go-v2/service/rds@v1.116.2's types/errors.go ErrorCode() methods
// (the ground truth for what a real RDS server puts on the wire). AWS is
// inconsistent about the "Fault" suffix across error codes (e.g.
// DBInstanceNotFound has none, but DBClusterNotFoundFault does), so each
// case here is individually confirmed against the real SDK rather than
// assumed from a uniform convention. Every RDS error uses HTTP 400 (the
// Query/EC2 protocol convention — unlike REST/JSON protocols, HTTP status
// does not vary by fault type; only the <Code> element does).
func TestRDSErrorCodes_FaultSuffix(t *testing.T) {
	t.Parallel()

	type errResp struct {
		XMLName xml.Name `xml:"ErrorResponse"`
		Error   struct {
			Code string `xml:"Code"`
		} `xml:"Error"`
	}

	tests := []struct {
		name           string
		setup          func(t *testing.T, h *rds.Handler)
		query          string
		wantCode       string
		wantCodeAbsent string
	}{
		{
			name:     "DeleteDBCluster not found",
			query:    "Action=DeleteDBCluster&Version=2014-10-31&DBClusterIdentifier=missing&SkipFinalSnapshot=true",
			wantCode: "DBClusterNotFoundFault",
		},
		{
			name: "CreateDBCluster already exists",
			setup: func(t *testing.T, h *rds.Handler) {
				t.Helper()
				postRDSForm(t, h, "Action=CreateDBCluster&Version=2014-10-31"+
					"&DBClusterIdentifier=dup-cluster&Engine=aurora-postgresql"+
					"&MasterUsername=admin&MasterUserPassword=password123")
			},
			query: "Action=CreateDBCluster&Version=2014-10-31" +
				"&DBClusterIdentifier=dup-cluster&Engine=aurora-postgresql" +
				"&MasterUsername=admin&MasterUserPassword=password123",
			wantCode: "DBClusterAlreadyExistsFault",
		},
		{
			name:     "DeleteDBClusterSnapshot not found",
			query:    "Action=DeleteDBClusterSnapshot&Version=2014-10-31&DBClusterSnapshotIdentifier=missing",
			wantCode: "DBClusterSnapshotNotFoundFault",
		},
		{
			name: "CreateDBClusterSnapshot already exists",
			setup: func(t *testing.T, h *rds.Handler) {
				t.Helper()
				postRDSForm(t, h, "Action=CreateDBCluster&Version=2014-10-31"+
					"&DBClusterIdentifier=csnap-src&Engine=aurora-postgresql"+
					"&MasterUsername=admin&MasterUserPassword=password123")
				postRDSForm(t, h, "Action=CreateDBClusterSnapshot&Version=2014-10-31"+
					"&DBClusterSnapshotIdentifier=dup-csnap&DBClusterIdentifier=csnap-src")
			},
			query: "Action=CreateDBClusterSnapshot&Version=2014-10-31" +
				"&DBClusterSnapshotIdentifier=dup-csnap&DBClusterIdentifier=csnap-src",
			wantCode: "DBClusterSnapshotAlreadyExistsFault",
		},
		{
			name:     "DeleteDBClusterEndpoint not found",
			query:    "Action=DeleteDBClusterEndpoint&Version=2014-10-31&DBClusterEndpointIdentifier=missing",
			wantCode: "DBClusterEndpointNotFoundFault",
		},
		{
			name: "CreateDBClusterEndpoint already exists",
			setup: func(t *testing.T, h *rds.Handler) {
				t.Helper()
				postRDSForm(t, h, "Action=CreateDBCluster&Version=2014-10-31"+
					"&DBClusterIdentifier=cep-src&Engine=aurora-postgresql"+
					"&MasterUsername=admin&MasterUserPassword=password123")
				postRDSForm(t, h, "Action=CreateDBClusterEndpoint&Version=2014-10-31"+
					"&DBClusterEndpointIdentifier=dup-cep&DBClusterIdentifier=cep-src")
			},
			query: "Action=CreateDBClusterEndpoint&Version=2014-10-31" +
				"&DBClusterEndpointIdentifier=dup-cep&DBClusterIdentifier=cep-src",
			wantCode: "DBClusterEndpointAlreadyExistsFault",
		},
		{
			name:     "DeleteGlobalCluster not found",
			query:    "Action=DeleteGlobalCluster&Version=2014-10-31&GlobalClusterIdentifier=missing",
			wantCode: "GlobalClusterNotFoundFault",
		},
		{
			name: "CreateGlobalCluster already exists",
			setup: func(t *testing.T, h *rds.Handler) {
				t.Helper()
				postRDSForm(t, h, "Action=CreateGlobalCluster&Version=2014-10-31"+
					"&GlobalClusterIdentifier=dup-gc&Engine=aurora-postgresql")
			},
			query:    "Action=CreateGlobalCluster&Version=2014-10-31&GlobalClusterIdentifier=dup-gc&Engine=aurora-postgresql",
			wantCode: "GlobalClusterAlreadyExistsFault",
		},
		{
			name:     "DeleteBlueGreenDeployment not found",
			query:    "Action=DeleteBlueGreenDeployment&Version=2014-10-31&BlueGreenDeploymentIdentifier=missing",
			wantCode: "BlueGreenDeploymentNotFoundFault",
		},
		{
			name: "CreateBlueGreenDeployment already exists",
			setup: func(t *testing.T, h *rds.Handler) {
				t.Helper()
				postRDSForm(t, h,
					"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=bgd-src&Engine=mysql")
				postRDSForm(t, h, "Action=CreateBlueGreenDeployment&Version=2014-10-31"+
					"&BlueGreenDeploymentName=dup-bgd"+
					"&Source=arn:aws:rds:us-east-1:000000000000:db:bgd-src")
			},
			query: "Action=CreateBlueGreenDeployment&Version=2014-10-31" +
				"&BlueGreenDeploymentName=dup-bgd" +
				"&Source=arn:aws:rds:us-east-1:000000000000:db:bgd-src",
			wantCode: "BlueGreenDeploymentAlreadyExistsFault",
		},
		{
			name:     "DeleteIntegration not found",
			query:    "Action=DeleteIntegration&Version=2014-10-31&IntegrationIdentifier=missing",
			wantCode: "IntegrationNotFoundFault",
		},
		{
			name: "CreateIntegration already exists",
			setup: func(t *testing.T, h *rds.Handler) {
				t.Helper()
				postRDSForm(t, h, "Action=CreateIntegration&Version=2014-10-31"+
					"&IntegrationName=dup-integration"+
					"&SourceArn=arn:aws:rds:us-east-1:000000000000:cluster:src"+
					"&TargetArn=arn:aws:redshift-serverless:us-east-1:000000000000:namespace/tgt")
			},
			query: "Action=CreateIntegration&Version=2014-10-31" +
				"&IntegrationName=dup-integration" +
				"&SourceArn=arn:aws:rds:us-east-1:000000000000:cluster:src" +
				"&TargetArn=arn:aws:redshift-serverless:us-east-1:000000000000:namespace/tgt",
			wantCode: "IntegrationAlreadyExistsFault",
		},
		{
			name:     "DeleteOptionGroup not found",
			query:    "Action=DeleteOptionGroup&Version=2014-10-31&OptionGroupName=missing",
			wantCode: "OptionGroupNotFoundFault",
		},
		{
			name: "CreateOptionGroup already exists",
			setup: func(t *testing.T, h *rds.Handler) {
				t.Helper()
				postRDSForm(t, h, "Action=CreateOptionGroup&Version=2014-10-31"+
					"&OptionGroupName=dup-og&EngineName=mysql&MajorEngineVersion=8.0"+
					"&OptionGroupDescription=d")
			},
			query: "Action=CreateOptionGroup&Version=2014-10-31" +
				"&OptionGroupName=dup-og&EngineName=mysql&MajorEngineVersion=8.0" +
				"&OptionGroupDescription=d",
			wantCode: "OptionGroupAlreadyExistsFault",
		},
		{
			// Regression test: before this fix, DBProxy-already-exists had
			// no entry in the error-code mapping table at all, so it fell
			// through to an unmapped "" code and a 500 InternalFailure
			// instead of a client-facing 400 DBProxyAlreadyExistsFault.
			name: "CreateDBProxy already exists",
			setup: func(t *testing.T, h *rds.Handler) {
				t.Helper()
				postRDSForm(t, h, "Action=CreateDBProxy&Version=2014-10-31"+
					"&DBProxyName=dup-proxy&EngineFamily=MYSQL"+
					"&RoleArn=arn:aws:iam::000000000000:role/proxy-role"+
					"&Auth.member.1.AuthScheme=SECRETS")
			},
			query: "Action=CreateDBProxy&Version=2014-10-31" +
				"&DBProxyName=dup-proxy&EngineFamily=MYSQL" +
				"&RoleArn=arn:aws:iam::000000000000:role/proxy-role" +
				"&Auth.member.1.AuthScheme=SECRETS",
			wantCode: "DBProxyAlreadyExistsFault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRDSHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postRDSForm(t, h, tt.query)
			require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())

			var resp errResp
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantCode, resp.Error.Code)
			if tt.wantCodeAbsent != "" {
				assert.NotEqual(t, tt.wantCodeAbsent, resp.Error.Code)
			}
		})
	}
}

// TestRDSErrorCodes_ClassASweep is a regression suite for gopherstack-33jc:
// six root causes where a handler emitted a code that
// aws-sdk-go-v2/service/rds@v1.124.1/deserializers.go's declared error set
// for the op does not contain, found by cmd/errtargetaudit. Each case pins
// the correct declared code and, via wantCodeAbsent, proves the previously
// emitted (wrong) code no longer appears.
func TestRDSErrorCodes_ClassASweep(t *testing.T) {
	t.Parallel()

	type errResp struct {
		XMLName xml.Name `xml:"ErrorResponse"`
		Error   struct {
			Code string `xml:"Code"`
		} `xml:"Error"`
	}

	tests := []struct {
		name           string
		setup          func(t *testing.T, h *rds.Handler)
		query          string
		wantCode       string
		wantCodeAbsent string
	}{
		{
			name: "ApplyPendingMaintenanceAction resource not found",
			query: "Action=ApplyPendingMaintenanceAction&Version=2014-10-31" +
				"&ResourceIdentifier=arn:aws:rds:us-east-1:000000000000:db:missing" +
				"&ApplyAction=system-update&OptInType=immediate",
			wantCode:       "ResourceNotFoundFault",
			wantCodeAbsent: "DBInstanceNotFound",
		},
		{
			name: "EnableHttpEndpoint resource not found",
			query: "Action=EnableHttpEndpoint&Version=2014-10-31" +
				"&ResourceArn=arn:aws:rds:us-east-1:000000000000:cluster:missing",
			wantCode:       "ResourceNotFoundFault",
			wantCodeAbsent: "DBClusterNotFoundFault",
		},
		{
			name: "DisableHttpEndpoint resource not found",
			query: "Action=DisableHttpEndpoint&Version=2014-10-31" +
				"&ResourceArn=arn:aws:rds:us-east-1:000000000000:cluster:missing",
			wantCode:       "ResourceNotFoundFault",
			wantCodeAbsent: "DBClusterNotFoundFault",
		},
		{
			name: "CreateCustomDBEngineVersion already exists",
			setup: func(t *testing.T, h *rds.Handler) {
				t.Helper()
				postRDSForm(t, h, "Action=CreateCustomDBEngineVersion&Version=2014-10-31"+
					"&Engine=custom-oracle-ee&EngineVersion=19.0.0")
			},
			query: "Action=CreateCustomDBEngineVersion&Version=2014-10-31" +
				"&Engine=custom-oracle-ee&EngineVersion=19.0.0",
			wantCode:       "CustomDBEngineVersionAlreadyExistsFault",
			wantCodeAbsent: "DBInstanceAlreadyExists",
		},
		{
			name: "DeleteCustomDBEngineVersion not found",
			query: "Action=DeleteCustomDBEngineVersion&Version=2014-10-31" +
				"&Engine=custom-oracle-ee&EngineVersion=missing",
			wantCode:       "CustomDBEngineVersionNotFoundFault",
			wantCodeAbsent: "DBInstanceNotFound",
		},
		{
			name: "ModifyCustomDBEngineVersion not found",
			query: "Action=ModifyCustomDBEngineVersion&Version=2014-10-31" +
				"&Engine=custom-oracle-ee&EngineVersion=missing&Status=inactive",
			wantCode:       "CustomDBEngineVersionNotFoundFault",
			wantCodeAbsent: "DBInstanceNotFound",
		},
		{
			name: "DescribeDBClusterSnapshotAttributes not found",
			query: "Action=DescribeDBClusterSnapshotAttributes&Version=2014-10-31" +
				"&DBClusterSnapshotIdentifier=missing",
			wantCode:       "DBClusterSnapshotNotFoundFault",
			wantCodeAbsent: "DBSnapshotNotFound",
		},
		{
			name: "ModifyDBClusterSnapshotAttribute not found",
			query: "Action=ModifyDBClusterSnapshotAttribute&Version=2014-10-31" +
				"&DBClusterSnapshotIdentifier=missing&AttributeName=restore",
			wantCode:       "DBClusterSnapshotNotFoundFault",
			wantCodeAbsent: "DBSnapshotNotFound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRDSHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postRDSForm(t, h, tt.query)
			require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())

			var resp errResp
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantCode, resp.Error.Code)
			assert.NotEqual(t, tt.wantCodeAbsent, resp.Error.Code)
		})
	}
}
