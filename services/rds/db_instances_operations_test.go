package rds_test

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

// TestRDSBackend_ModifyDBInstance_NewFields tests ModifyDBInstance with new fields.
func TestRDSBackend_ModifyDBInstance_NewFields(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDBInstance("mod-db", "postgres", "db.t3.micro", "", "", "", 20, rds.DBInstanceOptions{})
	require.NoError(t, err)

	opts := rds.DBInstanceOptions{
		StorageType:           "io1",
		BackupRetentionPeriod: 14,
		MultiAZ:               true,
		ApplyImmediately:      true,
	}

	inst, err := b.ModifyDBInstance("mod-db", "db.r5.large", 100, opts)
	require.NoError(t, err)
	assert.Equal(t, "db.r5.large", inst.DBInstanceClass)
	assert.Equal(t, 100, inst.AllocatedStorage)
	assert.Equal(t, "io1", inst.StorageType)
	assert.Equal(t, 14, inst.BackupRetentionPeriod)
	assert.True(t, inst.MultiAZ)
}

func TestRDSBackend_InstanceModifyTransitionAndDeletePublishesEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "create modify delete transitions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := rds.NewInMemoryBackend("000000000000", "us-east-1")
			const instanceID = "transition-db"

			created, err := b.CreateDBInstance(instanceID, "postgres", "", "", "", "", 20, rds.DBInstanceOptions{})
			require.NoError(t, err)
			assert.Equal(t, "creating", created.DBInstanceStatus)

			modified, err := b.ModifyDBInstance(instanceID, "db.r5.large", 100, rds.DBInstanceOptions{})
			require.NoError(t, err)
			assert.Equal(t, "modifying", modified.DBInstanceStatus)

			require.Eventually(t, func() bool {
				instances, describeErr := b.DescribeDBInstances(instanceID)
				if describeErr != nil || len(instances) != 1 {
					return false
				}

				return instances[0].DBInstanceStatus == "available" && instances[0].DBInstanceClass == "db.r5.large"
			}, 3*time.Second, 20*time.Millisecond)

			deleted, err := b.DeleteDBInstance(instanceID)
			require.NoError(t, err)
			assert.Equal(t, "deleting", deleted.DBInstanceStatus)
			_, err = b.DescribeDBInstances(instanceID)
			require.ErrorIs(t, err, rds.ErrInstanceNotFound)

			messages := rds.EventMessagesForSource(b, instanceID)
			assert.Contains(t, messages, "DB instance created")
			assert.Contains(t, messages, "DB instance is now available")
			assert.Contains(t, messages, "DB instance modification started")
			assert.Contains(t, messages, "DB instance deletion started")
			assert.Contains(t, messages, "DB instance deleted")
		})
	}
}

// TestCreateDBInstance_IdentifierValidation verifies that
// CreateDBInstance enforces AWS identifier constraints:
// must start with a letter, contain only alphanumeric/hyphens, 1–63 chars.
func TestCreateDBInstance_IdentifierValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{
			name:       "valid_simple",
			id:         "my-db-instance",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid_single_letter",
			id:         "a",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid_63_chars",
			id:         "a123456789012345678901234567890123456789012345678901234567890ab",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid_starts_with_digit",
			id:         "1mydb",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_starts_with_hyphen",
			id:         "-mydb",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_underscore",
			id:         "my_db",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_space",
			id:         "my db",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_64_chars",
			id:         "a1234567890123456789012345678901234567890123456789012345678901234",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAccuracyRDSHandler()
			rec := doAccuracyRDS(t, h, url.Values{
				"Action":               {"CreateDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {tt.id},
				"Engine":               {"postgres"},
				"MasterUsername":       {"admin"},
			})
			assert.Equal(t, tt.wantStatus, rec.Code, "id=%q", tt.id)
		})
	}
}

// TestCreateDBInstance_EngineValidation verifies that CreateDBInstance
// rejects an Engine value that isn't one of the values
// aws-sdk-go-v2's CreateDBInstanceInput documents as valid (real AWS
// returns InvalidParameterValue for an unsupported engine), while every
// documented value -- including the less common RDS Custom / Db2 / SQL
// Server engines, not just the handful this emulator's DescribeDBEngineVersions
// catalog seeds -- is accepted.
func TestCreateDBInstance_EngineValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		engine     string
		wantStatus int
	}{
		{name: "valid_postgres", engine: "postgres", wantStatus: http.StatusOK},
		{name: "valid_mysql", engine: "mysql", wantStatus: http.StatusOK},
		{name: "valid_mariadb", engine: "mariadb", wantStatus: http.StatusOK},
		{name: "valid_aurora_mysql", engine: "aurora-mysql", wantStatus: http.StatusOK},
		{name: "valid_aurora_postgresql", engine: "aurora-postgresql", wantStatus: http.StatusOK},
		{name: "valid_oracle_ee", engine: "oracle-ee", wantStatus: http.StatusOK},
		{name: "valid_sqlserver_ee", engine: "sqlserver-ee", wantStatus: http.StatusOK},
		{name: "valid_db2_se", engine: "db2-se", wantStatus: http.StatusOK},
		{name: "invalid_made_up_engine", engine: "not-a-real-engine", wantStatus: http.StatusBadRequest},
		{name: "invalid_engine_class_confusion", engine: "db.t3.micro", wantStatus: http.StatusBadRequest},
		{name: "invalid_cluster_only_engine_neptune", engine: "neptune", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAccuracyRDSHandler()
			rec := doAccuracyRDS(t, h, url.Values{
				"Action":               {"CreateDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"engine-validation-db"},
				"Engine":               {tt.engine},
				"MasterUsername":       {"admin"},
			})
			assert.Equal(t, tt.wantStatus, rec.Code, "engine=%q body=%s", tt.engine, rec.Body.String())
			if tt.wantStatus == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "InvalidParameterValue")
			}
		})
	}
}

// TestCreateDBInstance_AllocatedStorageBound verifies that CreateDBInstance
// rejects an out-of-range AllocatedStorage (AWS bound: 20–65536 GiB) and
// accepts in-range values.
func TestCreateDBInstance_AllocatedStorageBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		id         string
		storage    string
		wantStatus int
	}{
		{name: "below min", id: "as-below-min", storage: "10", wantStatus: http.StatusBadRequest},
		{name: "at min", id: "as-at-min", storage: "20", wantStatus: http.StatusOK},
		{name: "mid range", id: "as-mid-range", storage: "100", wantStatus: http.StatusOK},
		{name: "at max", id: "as-at-max", storage: "65536", wantStatus: http.StatusOK},
		{name: "above max", id: "as-above-max", storage: "65537", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newAccuracyRDSHandler()
			rec := doAccuracyRDS(t, h, url.Values{
				"Action":               {"CreateDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {tc.id},
				"DBInstanceClass":      {"db.t3.micro"},
				"Engine":               {"postgres"},
				"MasterUsername":       {"admin"},
				"AllocatedStorage":     {tc.storage},
			})
			assert.Equal(t, tc.wantStatus, rec.Code, "AllocatedStorage=%s", tc.storage)
		})
	}
}

// TestRDS_CreateDBInstance_IdentifierValidation asserts that invalid identifiers are rejected.
func TestRDS_CreateDBInstance_IdentifierValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		dbInstanceID     string
		dbInstanceClass  string
		engine           string
		wantErrorContain string
		wantCode         int
	}{
		{
			name:            "valid_identifier",
			dbInstanceID:    "my-db-instance",
			dbInstanceClass: "db.t3.micro",
			engine:          "mysql",
			wantCode:        http.StatusOK,
		},
		{
			name:             "empty_identifier",
			dbInstanceID:     "",
			dbInstanceClass:  "db.t3.micro",
			engine:           "mysql",
			wantCode:         http.StatusBadRequest,
			wantErrorContain: "Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRDSHandler()
			body := "Action=CreateDBInstance" +
				"&DBInstanceIdentifier=" + tt.dbInstanceID +
				"&DBInstanceClass=" + tt.dbInstanceClass +
				"&Engine=" + tt.engine

			rec := postRDSForm(t, h, body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantErrorContain != "" {
				assert.Contains(t, rec.Body.String(), tt.wantErrorContain)
			}
		})
	}
}

// TestCreateDBInstance_CaseInsensitiveIdentifier asserts that
// DBInstanceIdentifier is treated as a case-insensitive persistent handle,
// matching real AWS (which lower-cases identifiers internally): creating
// "MyCaseDB" then "mycasedb" must collide with DBInstanceAlreadyExistsFault,
// and every subsequent lookup (Describe, Delete) must find the resource
// regardless of the casing used, while the wire response keeps echoing the
// identifier's original, as-created casing.
func TestCreateDBInstance_CaseInsensitiveIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		setupID   string
		actionID  string
		action    string
		wantErr   bool
	}{
		{
			name:      "create_collides_on_lowercased_duplicate",
			setupID:   "MyCaseDB",
			actionID:  "mycasedb",
			action:    "create",
			wantErr:   true,
			wantErrIs: rds.ErrInstanceAlreadyExists,
		},
		{
			name:      "create_collides_on_uppercased_duplicate",
			setupID:   "lowercase-db",
			actionID:  "LOWERCASE-DB",
			action:    "create",
			wantErr:   true,
			wantErrIs: rds.ErrInstanceAlreadyExists,
		},
		{
			name:      "create_collides_on_mixed_case_duplicate",
			setupID:   "Mixed-Case-DB",
			actionID:  "MIXED-case-db",
			action:    "create",
			wantErr:   true,
			wantErrIs: rds.ErrInstanceAlreadyExists,
		},
		{
			name:     "create_distinct_id_does_not_collide",
			setupID:  "some-db",
			actionID: "some-other-db",
			action:   "create",
		},
		{
			name:     "describe_finds_resource_under_different_case",
			setupID:  "FindMe-DB",
			actionID: "findme-db",
			action:   "describe",
		},
		{
			name:     "delete_removes_resource_under_different_case",
			setupID:  "DeleteMe-DB",
			actionID: "deleteme-db",
			action:   "delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			_, err := b.CreateDBInstance(
				tt.setupID, "postgres", "db.t3.micro", "", "admin", "", 20, rds.DBInstanceOptions{},
			)
			require.NoError(t, err)

			switch tt.action {
			case "create":
				_, err = b.CreateDBInstance(
					tt.actionID, "postgres", "db.t3.micro", "", "admin", "", 20, rds.DBInstanceOptions{},
				)
			case "describe":
				var insts []rds.DBInstance
				insts, err = b.DescribeDBInstances(tt.actionID)
				if err == nil {
					require.Len(t, insts, 1)
					// The wire response must keep echoing the ORIGINAL
					// caller-supplied casing from creation time, not the
					// (lowercased) lookup key or the differently-cased
					// identifier this Describe call was invoked with.
					assert.Equal(t, tt.setupID, insts[0].DBInstanceIdentifier)
				}
			case "delete":
				_, err = b.DeleteDBInstance(tt.actionID)
				if err == nil {
					_, describeErr := b.DescribeDBInstances(tt.setupID)
					require.ErrorIs(t, describeErr, rds.ErrInstanceNotFound)
				}
			}

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSwitchoverReadReplica(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErrIs  error
		name       string
		instanceID string
		wantErr    bool
	}{
		{
			name:       "success",
			instanceID: "my-instance",
		},
		{
			name:       "not found",
			instanceID: "missing",
			wantErr:    true,
			wantErrIs:  rds.ErrInstanceNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend(t)
			if !tt.wantErr {
				_, err := b.CreateDBInstance(
					tt.instanceID,
					"mysql",
					"db.t3.micro",
					"",
					"admin",
					"",
					20,
					rds.DBInstanceOptions{},
				)
				require.NoError(t, err)
			}
			got, err := b.SwitchoverReadReplica(tt.instanceID)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.instanceID, got.DBInstanceIdentifier)
			assert.Empty(t, got.ReplicaSourceDBInstanceIdentifier)
		})
	}
}

// TestRestoreDBInstanceFromS3 covers the seven required members of
// RestoreDBInstanceFromS3Input: DBInstanceClass, DBInstanceIdentifier,
// Engine, S3BucketName, S3IngestionRoleArn, SourceEngine and
// SourceEngineVersion. S3IngestionRoleArn/SourceEngine/SourceEngineVersion
// were previously dropped entirely by the handler.
func TestRestoreDBInstanceFromS3(t *testing.T) {
	t.Parallel()

	type restoreParams struct {
		id                  string
		engine              string
		dbInstanceClass     string
		s3Bucket            string
		s3IngestionRoleArn  string
		sourceEngine        string
		sourceEngineVersion string
	}

	valid := func() restoreParams {
		return restoreParams{
			id:                  "restored-db",
			engine:              "mysql",
			dbInstanceClass:     "db.t3.micro",
			s3Bucket:            "my-backup-bucket",
			s3IngestionRoleArn:  "arn:aws:iam::000000000000:role/rds-s3-ingestion",
			sourceEngine:        "mysql",
			sourceEngineVersion: "5.7.40",
		}
	}

	tests := []struct {
		wantErrIs error
		mutate    func(restoreParams) restoreParams
		name      string
		wantErr   bool
	}{
		{
			name: "success",
		},
		{
			name: "empty bucket",
			mutate: func(p restoreParams) restoreParams {
				p.s3Bucket = ""

				return p
			},
			wantErr:   true,
			wantErrIs: rds.ErrInvalidParameter,
		},
		{
			name: "empty id",
			mutate: func(p restoreParams) restoreParams {
				p.id = ""

				return p
			},
			wantErr:   true,
			wantErrIs: rds.ErrInvalidParameter,
		},
		{
			name: "empty s3 ingestion role arn",
			mutate: func(p restoreParams) restoreParams {
				p.s3IngestionRoleArn = ""

				return p
			},
			wantErr:   true,
			wantErrIs: rds.ErrInvalidParameter,
		},
		{
			name: "empty source engine",
			mutate: func(p restoreParams) restoreParams {
				p.sourceEngine = ""

				return p
			},
			wantErr:   true,
			wantErrIs: rds.ErrInvalidParameter,
		},
		{
			name: "empty source engine version",
			mutate: func(p restoreParams) restoreParams {
				p.sourceEngineVersion = ""

				return p
			},
			wantErr:   true,
			wantErrIs: rds.ErrInvalidParameter,
		},
		{
			name: "already exists",
			mutate: func(p restoreParams) restoreParams {
				p.id = "existing-db"

				return p
			},
			wantErr:   true,
			wantErrIs: rds.ErrInstanceAlreadyExists,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := valid()
			if tt.mutate != nil {
				p = tt.mutate(p)
			}

			b := newTestBackend(t)
			if tt.name == "already exists" {
				_, err := b.CreateDBInstance(
					p.id, p.engine, p.dbInstanceClass, "", "admin", "", 20, rds.DBInstanceOptions{},
				)
				require.NoError(t, err)
			}

			got, err := b.RestoreDBInstanceFromS3(
				p.id, p.engine, p.dbInstanceClass, p.s3Bucket,
				p.s3IngestionRoleArn, p.sourceEngine, p.sourceEngineVersion,
				"", "",
			)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, p.id, got.DBInstanceIdentifier)
			assert.Equal(t, p.engine, got.Engine)
		})
	}
}

// TestInstanceCreateTime verifies that InstanceCreateTime is set on CreateDBInstance.
func TestInstanceCreateTime(t *testing.T) {
	t.Parallel()
	b := newTestBackend(t)
	before := time.Now().UTC()
	inst, err := b.CreateDBInstance("db-ts", "postgres", "db.t3.micro", "", "admin", "", 20, rds.DBInstanceOptions{})
	require.NoError(t, err)
	after := time.Now().UTC()
	assert.False(t, inst.InstanceCreateTime.IsZero(), "InstanceCreateTime should be set")
	assert.False(t, inst.InstanceCreateTime.Before(before), "InstanceCreateTime should be after test start")
	assert.False(t, inst.InstanceCreateTime.After(after), "InstanceCreateTime should be before test end")
}

// TestDescribeDBInstancesSorted verifies deterministic sort order.
func TestDescribeDBInstancesSorted(t *testing.T) {
	t.Parallel()
	b := newTestBackend(t)
	for _, id := range []string{"db-z", "db-a", "db-m"} {
		_, err := b.CreateDBInstance(id, "postgres", "db.t3.micro", "", "admin", "", 20, rds.DBInstanceOptions{})
		require.NoError(t, err)
	}
	got, err := b.DescribeDBInstances("")
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "db-a", got[0].DBInstanceIdentifier)
	assert.Equal(t, "db-m", got[1].DBInstanceIdentifier)
	assert.Equal(t, "db-z", got[2].DBInstanceIdentifier)
}

// TestIopsAndStorageThroughputPersisted verifies Iops and StorageThroughput are stored and returned.
func TestIopsAndStorageThroughputPersisted(t *testing.T) {
	t.Parallel()

	h := newAccuracyRDSHandler()

	rec := doAccuracyRDS(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"iops-test"},
		"DBInstanceClass":      {"db.r6g.large"},
		"Engine":               {"postgres"},
		"MasterUsername":       {"admin"},
		"AllocatedStorage":     {"100"},
		"StorageType":          {"io1"},
		"Iops":                 {"3000"},
		"StorageThroughput":    {"125"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			DBInstance struct {
				Iops              int `xml:"Iops"`
				StorageThroughput int `xml:"StorageThroughput"`
			} `xml:"DBInstance"`
		} `xml:"CreateDBInstanceResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 3000, resp.Result.DBInstance.Iops)
	assert.Equal(t, 125, resp.Result.DBInstance.StorageThroughput)
}

// TestVpcSecurityGroupsPersisted verifies VpcSecurityGroups are stored and returned.
func TestVpcSecurityGroupsPersisted(t *testing.T) {
	t.Parallel()

	h := newAccuracyRDSHandler()

	rec := doAccuracyRDS(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"sg-test"},
		"DBInstanceClass":      {"db.t3.micro"},
		"Engine":               {"postgres"},
		"MasterUsername":       {"admin"},
		"AllocatedStorage":     {"20"},
		"VpcSecurityGroupIds.VpcSecurityGroupID.1": {"sg-11111111"},
		"VpcSecurityGroupIds.VpcSecurityGroupID.2": {"sg-22222222"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			DBInstance struct {
				VpcSecurityGroups struct {
					Members []struct {
						VpcSecurityGroupID string `xml:"VpcSecurityGroupId"`
						Status             string `xml:"Status"`
					} `xml:"VpcSecurityGroupMembership"`
				} `xml:"VpcSecurityGroups"`
			} `xml:"DBInstance"`
		} `xml:"CreateDBInstanceResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Result.DBInstance.VpcSecurityGroups.Members, 2)
	assert.Equal(t, "sg-11111111", resp.Result.DBInstance.VpcSecurityGroups.Members[0].VpcSecurityGroupID)
	assert.Equal(t, "active", resp.Result.DBInstance.VpcSecurityGroups.Members[0].Status)
}

// TestDBSecurityGroupsPersisted verifies DBSecurityGroups (the classic,
// non-VPC association) is stored and returned. CreateDBInstanceInput.
// DBSecurityGroups is a real, documented field (rds@v1.124.1
// api_op_CreateDBInstance.go) that the backend previously had no field to
// store at all.
func TestDBSecurityGroupsPersisted(t *testing.T) {
	t.Parallel()

	h := newAccuracyRDSHandler()

	rec := doAccuracyRDS(t, h, url.Values{
		"Action":                     {"CreateDBSecurityGroup"},
		"Version":                    {"2014-10-31"},
		"DBSecurityGroupName":        {"dbsg-test"},
		"DBSecurityGroupDescription": {"test group"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doAccuracyRDS(t, h, url.Values{
		"Action":                                 {"CreateDBInstance"},
		"Version":                                {"2014-10-31"},
		"DBInstanceIdentifier":                   {"dbsg-inst"},
		"DBInstanceClass":                        {"db.t3.micro"},
		"Engine":                                 {"postgres"},
		"MasterUsername":                         {"admin"},
		"AllocatedStorage":                       {"20"},
		"DBSecurityGroups.DBSecurityGroupName.1": {"dbsg-test"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			DBInstance struct {
				DBSecurityGroups struct {
					Members []struct {
						DBSecurityGroupName string `xml:"DBSecurityGroupName"`
						Status              string `xml:"Status"`
					} `xml:"DBSecurityGroup"`
				} `xml:"DBSecurityGroups"`
			} `xml:"DBInstance"`
		} `xml:"CreateDBInstanceResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Result.DBInstance.DBSecurityGroups.Members, 1)
	assert.Equal(t, "dbsg-test", resp.Result.DBInstance.DBSecurityGroups.Members[0].DBSecurityGroupName)
	assert.Equal(t, "active", resp.Result.DBInstance.DBSecurityGroups.Members[0].Status)

	// Real AWS: CreateDBInstance's own declared error switch includes
	// DBSecurityGroupNotFound for a group that doesn't exist.
	rec = doAccuracyRDS(t, h, url.Values{
		"Action":                                 {"CreateDBInstance"},
		"Version":                                {"2014-10-31"},
		"DBInstanceIdentifier":                   {"dbsg-inst-2"},
		"DBInstanceClass":                        {"db.t3.micro"},
		"Engine":                                 {"postgres"},
		"MasterUsername":                         {"admin"},
		"AllocatedStorage":                       {"20"},
		"DBSecurityGroups.DBSecurityGroupName.1": {"no-such-group"},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "DBSecurityGroupNotFound")
}

// TestLicenseModelPersisted verifies LicenseModel is stored and returned.
func TestLicenseModelPersisted(t *testing.T) {
	t.Parallel()

	h := newAccuracyRDSHandler()

	rec := doAccuracyRDS(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"license-test"},
		"DBInstanceClass":      {"db.t3.micro"},
		"Engine":               {"oracle-ee"},
		"MasterUsername":       {"admin"},
		"AllocatedStorage":     {"100"},
		"LicenseModel":         {"bring-your-own-license"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			DBInstance struct {
				LicenseModel string `xml:"LicenseModel"`
			} `xml:"DBInstance"`
		} `xml:"CreateDBInstanceResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "bring-your-own-license", resp.Result.DBInstance.LicenseModel)
}

// TestMonitoringFieldsPersisted verifies monitoring fields are stored and returned.
func TestMonitoringFieldsPersisted(t *testing.T) {
	t.Parallel()

	h := newAccuracyRDSHandler()

	rec := doAccuracyRDS(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"monitoring-test"},
		"DBInstanceClass":      {"db.t3.micro"},
		"Engine":               {"postgres"},
		"MasterUsername":       {"admin"},
		"AllocatedStorage":     {"20"},
		"MonitoringInterval":   {"60"},
		"MonitoringRoleArn":    {"arn:aws:iam::123456789012:role/rds-monitoring"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			DBInstance struct {
				MonitoringRoleArn  string `xml:"MonitoringRoleArn"`
				MonitoringInterval int    `xml:"MonitoringInterval"`
			} `xml:"DBInstance"`
		} `xml:"CreateDBInstanceResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 60, resp.Result.DBInstance.MonitoringInterval)
	assert.Equal(t, "arn:aws:iam::123456789012:role/rds-monitoring", resp.Result.DBInstance.MonitoringRoleArn)
}

// TestPreferredWindowsPersisted verifies preferred windows are stored and returned.
func TestPreferredWindowsPersisted(t *testing.T) {
	t.Parallel()

	h := newAccuracyRDSHandler()

	rec := doAccuracyRDS(t, h, url.Values{
		"Action":                     {"CreateDBInstance"},
		"Version":                    {"2014-10-31"},
		"DBInstanceIdentifier":       {"windows-test"},
		"DBInstanceClass":            {"db.t3.micro"},
		"Engine":                     {"postgres"},
		"MasterUsername":             {"admin"},
		"AllocatedStorage":           {"20"},
		"PreferredMaintenanceWindow": {"mon:03:00-mon:04:00"},
		"PreferredBackupWindow":      {"02:00-03:00"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			DBInstance struct {
				PreferredMaintenanceWindow string `xml:"PreferredMaintenanceWindow"`
				PreferredBackupWindow      string `xml:"PreferredBackupWindow"`
			} `xml:"DBInstance"`
		} `xml:"CreateDBInstanceResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "mon:03:00-mon:04:00", resp.Result.DBInstance.PreferredMaintenanceWindow)
	assert.Equal(t, "02:00-03:00", resp.Result.DBInstance.PreferredBackupWindow)
}
