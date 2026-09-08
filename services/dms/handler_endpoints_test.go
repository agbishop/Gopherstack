package dms_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/dms"
)

// TestEndpointPassword_StoredButNeverOnWire asserts that Password sent on
// CreateEndpoint/ModifyEndpoint is actually stored by the backend (matching
// what a real client would expect -- e.g. a subsequent connection attempt
// should be able to use it) rather than silently dropped, while also never
// appearing in any Describe/Create/Modify JSON response -- the real
// Endpoint wire type has no Password field, matching AWS's documented
// behavior of never echoing credentials back.
func TestEndpointPassword_StoredButNeverOnWire(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	ctx := t.Context()

	createRec := doDMS(t, h, "CreateEndpoint", map[string]any{
		"EndpointIdentifier": "pw-ep",
		"EndpointType":       "source",
		"EngineName":         "mysql",
		"Password":           "s3cr3t",
	})
	require.Equal(t, http.StatusOK, createRec.Code)
	createdEp := parseJSON(t, createRec)["Endpoint"].(map[string]any)
	_, hasPassword := createdEp["Password"]
	assert.False(t, hasPassword, "CreateEndpoint response must never carry Password")

	list, err := h.Backend.DescribeEndpoints(ctx, "pw-ep")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "s3cr3t", list[0].Password, "Password must be stored, not silently dropped")

	epArn := createdEp["EndpointArn"].(string)
	modRec := doDMS(t, h, "ModifyEndpoint", map[string]any{
		"EndpointArn": epArn,
		"Password":    "n3wpass",
	})
	require.Equal(t, http.StatusOK, modRec.Code)
	modEp := parseJSON(t, modRec)["Endpoint"].(map[string]any)
	_, modHasPassword := modEp["Password"]
	assert.False(t, modHasPassword, "ModifyEndpoint response must never carry Password")

	list2, err := h.Backend.DescribeEndpoints(ctx, "pw-ep")
	require.NoError(t, err)
	require.Len(t, list2, 1)
	assert.Equal(t, "n3wpass", list2[0].Password, "ModifyEndpoint must persist the new Password")
}

// TestEndpoint_EngineSettingsRejected locks gopherstack-z79q: CreateEndpoint/
// ModifyEndpoint accept ~19 heterogeneous engine-specific settings blocks
// (MySQLSettings, S3Settings, OracleSettings, ...) that this emulator does
// not model. Rather than silently drop them (the pre-fix behavior --
// encoding/json ignores unknown fields), sending any of them must be
// rejected with a 400 ValidationException naming the field, not accepted
// as if honored.
func TestEndpoint_EngineSettingsRejected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		field string
	}{
		{name: "mysql", field: "MySQLSettings"},
		{name: "postgresql", field: "PostgreSQLSettings"},
		{name: "s3", field: "S3Settings"},
		{name: "oracle", field: "OracleSettings"},
		{name: "mongodb", field: "MongoDbSettings"},
		{name: "kafka", field: "KafkaSettings"},
		{name: "kinesis", field: "KinesisSettings"},
		{name: "redshift", field: "RedshiftSettings"},
		{name: "dynamodb", field: "DynamoDbSettings"},
		{name: "elasticsearch", field: "ElasticsearchSettings"},
		{name: "neptune", field: "NeptuneSettings"},
		{name: "docdb", field: "DocDbSettings"},
		{name: "ibmdb2", field: "IBMDb2Settings"},
		{name: "sqlserver", field: "MicrosoftSQLServerSettings"},
		{name: "sybase", field: "SybaseSettings"},
		{name: "dmstransfer", field: "DmsTransferSettings"},
		{name: "gcpmysql", field: "GcpMySQLSettings"},
		{name: "redis", field: "RedisSettings"},
		{name: "timestream", field: "TimestreamSettings"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestDMSHandler()

			createRec := doDMS(t, h, "CreateEndpoint", map[string]any{
				"EndpointIdentifier": "settings-ep-" + tc.name,
				"EndpointType":       "source",
				"EngineName":         "mysql",
				tc.field:             map[string]any{"Username": "ignored"},
			})
			require.Equal(t, http.StatusBadRequest, createRec.Code)
			createBody := parseJSON(t, createRec)
			assert.Equal(t, "ValidationException", createBody["__type"])
			msg, _ := createBody["message"].(string)
			assert.Contains(t, msg, tc.field)

			// The endpoint must not have been created.
			list, err := h.Backend.DescribeEndpoints(t.Context(), "settings-ep-"+tc.name)
			require.NoError(t, err)
			assert.Empty(t, list)

			// ModifyEndpoint rejects the same field on an existing endpoint.
			h.Backend.AddEndpointInternal("existing-"+tc.name, "source", "mysql")
			descRec := doDMS(t, h, "DescribeEndpoints", map[string]any{
				"Filters": []map[string]any{{"Name": "endpoint-id", "Values": []string{"existing-" + tc.name}}},
			})
			require.Equal(t, http.StatusOK, descRec.Code)
			eps := parseJSON(t, descRec)["Endpoints"].([]any)
			require.Len(t, eps, 1)
			epArn := eps[0].(map[string]any)["EndpointArn"].(string)

			modRec := doDMS(t, h, "ModifyEndpoint", map[string]any{
				"EndpointArn": epArn,
				tc.field:      map[string]any{"Username": "ignored"},
			})
			require.Equal(t, http.StatusBadRequest, modRec.Code)
			modBody := parseJSON(t, modRec)
			assert.Equal(t, "ValidationException", modBody["__type"])
			modMsg, _ := modBody["message"].(string)
			assert.Contains(t, modMsg, tc.field)
		})
	}
}

// TestEndpoint_EngineSettingsNullAccepted asserts an explicit JSON null for an
// engine settings field is treated as absent, matching real SDK clients that
// may serialize an unset pointer field as null rather than omitting it.
func TestEndpoint_EngineSettingsNullAccepted(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()

	rec := doDMS(t, h, "CreateEndpoint", map[string]any{
		"EndpointIdentifier": "null-settings-ep",
		"EndpointType":       "source",
		"EngineName":         "mysql",
		"MySQLSettings":      nil,
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestModifyEndpoint(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	h.Backend.AddEndpointInternal("mod-ep", "source", "mysql")

	// Describe to get ARN.
	descRec := doDMS(t, h, "DescribeEndpoints", map[string]any{
		"Filters": []map[string]any{{"Name": "endpoint-id", "Values": []string{"mod-ep"}}},
	})
	require.Equal(t, http.StatusOK, descRec.Code)
	eps := parseJSON(t, descRec)["Endpoints"].([]any)
	require.Len(t, eps, 1)
	epArn := eps[0].(map[string]any)["EndpointArn"].(string)

	rec := doDMS(t, h, "ModifyEndpoint", map[string]any{
		"EndpointArn": epArn,
		"ServerName":  "new-server",
		"Port":        float64(5432),
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Not found case.
	rec2 := doDMS(t, h, "ModifyEndpoint", map[string]any{
		"EndpointArn": "arn:nonexistent",
	})
	assert.Equal(t, http.StatusNotFound, rec2.Code)

	// EndpointType/EngineName are modifiable, and are validated the same way
	// CreateEndpoint validates them.
	rec3 := doDMS(t, h, "ModifyEndpoint", map[string]any{
		"EndpointArn":  epArn,
		"EndpointType": "target",
		"EngineName":   "postgres",
	})
	require.Equal(t, http.StatusOK, rec3.Code)
	modified := parseJSON(t, rec3)["Endpoint"].(map[string]any)
	assert.Equal(t, "target", modified["EndpointType"])
	assert.Equal(t, "postgres", modified["EngineName"])

	rec4 := doDMS(t, h, "ModifyEndpoint", map[string]any{
		"EndpointArn":  epArn,
		"EndpointType": "SOURCE",
	})
	assert.Equal(t, http.StatusBadRequest, rec4.Code)
}

// TestEndpointType_EnumValidation locks gap #3 from PARITY.md: CreateEndpoint
// previously accepted any string for EndpointType/EngineName. Real AWS models
// EndpointType as a lowercase enum ("source"/"target") and EngineName as a
// documented valid-values list.
func TestEndpointType_EnumValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		endpointType string
		engineName   string
		wantCode     int
	}{
		{name: "valid source/mysql", endpointType: "source", engineName: "mysql", wantCode: http.StatusOK},
		{name: "valid target/s3", endpointType: "target", engineName: "s3", wantCode: http.StatusOK},
		{
			name: "uppercase EndpointType rejected", endpointType: "SOURCE", engineName: "mysql",
			wantCode: http.StatusBadRequest,
		},
		{
			name: "unknown EndpointType rejected", endpointType: "bogus", engineName: "mysql",
			wantCode: http.StatusBadRequest,
		},
		{
			name: "unknown EngineName rejected", endpointType: "source", engineName: "bogus-engine",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestDMSHandler()
			rec := doDMS(t, h, "CreateEndpoint", map[string]any{
				"EndpointIdentifier": "enum-test-ep",
				"EndpointType":       tc.endpointType,
				"EngineName":         tc.engineName,
			})
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

func TestDeleteEndpoint_RejectsIfInUse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		isSource bool
	}{
		{name: "source_endpoint_in_use", isSource: true},
		{name: "target_endpoint_in_use", isSource: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestDMSHandler()

			riRec := doDMS(t, h, "CreateReplicationInstance", map[string]any{
				"ReplicationInstanceIdentifier": "ep-inuse-ri",
				"ReplicationInstanceClass":      "dms.t3.medium",
			})
			require.Equal(t, http.StatusOK, riRec.Code)
			riArn := parseJSON(t, riRec)["ReplicationInstance"].(map[string]any)["ReplicationInstanceArn"].(string)

			srcRec := doDMS(t, h, "CreateEndpoint", map[string]any{
				"EndpointIdentifier": "ep-inuse-src",
				"EndpointType":       "source",
				"EngineName":         "mysql",
			})
			require.Equal(t, http.StatusOK, srcRec.Code)
			srcArn := parseJSON(t, srcRec)["Endpoint"].(map[string]any)["EndpointArn"].(string)

			tgtRec := doDMS(t, h, "CreateEndpoint", map[string]any{
				"EndpointIdentifier": "ep-inuse-tgt",
				"EndpointType":       "target",
				"EngineName":         "s3",
			})
			require.Equal(t, http.StatusOK, tgtRec.Code)
			tgtArn := parseJSON(t, tgtRec)["Endpoint"].(map[string]any)["EndpointArn"].(string)

			taskRec := doDMS(t, h, "CreateReplicationTask", map[string]any{
				"ReplicationTaskIdentifier": "ep-inuse-task",
				"SourceEndpointArn":         srcArn,
				"TargetEndpointArn":         tgtArn,
				"ReplicationInstanceArn":    riArn,
				"MigrationType":             "full-load",
			})
			require.Equal(t, http.StatusOK, taskRec.Code)

			// Delete whichever endpoint is in use — must fail with state error.
			deleteArn := tgtArn
			if tt.isSource {
				deleteArn = srcArn
			}

			delRec := doDMS(t, h, "DeleteEndpoint", map[string]any{
				"EndpointArn": deleteArn,
			})
			require.Equal(t, http.StatusBadRequest, delRec.Code)

			var body map[string]any
			require.NoError(t, json.Unmarshal(delRec.Body.Bytes(), &body))
			assert.Equal(t, "InvalidResourceStateFault", body["__type"],
				"deleting an endpoint used by a task must return InvalidResourceStateFault")
		})
	}
}

func TestDescribeSchemas(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()

	// Create endpoint.
	rec := doDMS(t, h, "CreateEndpoint", map[string]any{
		"EndpointIdentifier": "pg-src",
		"EndpointType":       "source",
		"EngineName":         "postgres",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	epARN := parseJSON(t, rec)["Endpoint"].(map[string]any)["EndpointArn"].(string)

	// Before refresh: DescribeSchemas returns empty.
	rec = doDMS(t, h, "DescribeSchemas", map[string]any{"EndpointArn": epARN})
	require.Equal(t, http.StatusOK, rec.Code)
	pre := parseJSON(t, rec)
	assert.Empty(t, pre["Schemas"])

	// Refresh.
	rec = doDMS(t, h, "RefreshSchemas", map[string]any{
		"EndpointArn":            epARN,
		"ReplicationInstanceArn": "arn:fake",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "successful", parseJSON(t, rec)["RefreshSchemasStatus"].(map[string]any)["Status"])

	// After refresh: schemas are populated.
	rec = doDMS(t, h, "DescribeSchemas", map[string]any{"EndpointArn": epARN})
	require.Equal(t, http.StatusOK, rec.Code)
	post := parseJSON(t, rec)
	schemas, ok := post["Schemas"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, schemas, "postgres endpoint should have schemas after refresh")
	assert.Contains(t, schemas, "public")
}

// TestDescribeSchemas_UnknownEndpoint covers gopherstack-m9uv: DescribeSchemas
// consulted only the endpointSchemas side map, never b.endpointsByARN, so an
// unknown or mistyped ARN returned 200+[] instead of ResourceNotFoundFault.
func TestDescribeSchemas_UnknownEndpoint(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()

	rec := doDMS(t, h, "DescribeSchemas", map[string]any{
		"EndpointArn": "arn:aws:dms:us-east-1:123456789012:endpoint:does-not-exist",
	})
	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "ResourceNotFoundFault", parseJSON(t, rec)["__type"])
}

// TestDeleteEndpoint_ClearsSchemas verifies that DeleteEndpoint removes the
// deleted endpoint's entry from the backend's endpointSchemas side map (keyed
// by EndpointArn) -- guarding commit 6806b0f10's ghost-row fix directly via
// HasEndpointSchemas, independent of DescribeSchemas' own behavior -- and
// that DescribeSchemas now checks the live endpoint table (gopherstack-m9uv)
// -- so a deleted endpoint's ARN gets ResourceNotFoundFault rather than a
// stale 200+[] from the orphaned side-map entry.
func TestDeleteEndpoint_ClearsSchemas(t *testing.T) {
	t.Parallel()

	backend := dms.NewInMemoryBackend("123456789012", config.DefaultRegion)
	h := dms.NewHandler(backend)

	rec := doDMS(t, h, "CreateEndpoint", map[string]any{
		"EndpointIdentifier": "pg-src",
		"EndpointType":       "source",
		"EngineName":         "postgres",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	epARN := parseJSON(t, rec)["Endpoint"].(map[string]any)["EndpointArn"].(string)

	rec = doDMS(t, h, "RefreshSchemas", map[string]any{
		"EndpointArn":            epARN,
		"ReplicationInstanceArn": "arn:fake",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doDMS(t, h, "DescribeSchemas", map[string]any{"EndpointArn": epARN})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, parseJSON(t, rec)["Schemas"])
	require.True(t, backend.HasEndpointSchemas(config.DefaultRegion, epARN))

	rec = doDMS(t, h, "DeleteEndpoint", map[string]any{"EndpointArn": epARN})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.False(t, backend.HasEndpointSchemas(config.DefaultRegion, epARN),
		"DeleteEndpoint must clear the schema entry for the deleted endpoint's ARN")

	rec = doDMS(t, h, "DescribeSchemas", map[string]any{"EndpointArn": epARN})
	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "ResourceNotFoundFault", parseJSON(t, rec)["__type"],
		"DescribeSchemas on a deleted endpoint's ARN must 404, not serve a stale schema list")
}

// TestRefreshSchemas_ReplicationInstanceArnRequired covers gopherstack-4shm's
// class: RefreshSchemasInput.ReplicationInstanceArn is "This member is
// required" (databasemigrationservice@v1.66.4 api_op_RefreshSchemas.go) but
// was decoded and never read at all -- a request omitting it got a 200
// instead of a validation error.
func TestRefreshSchemas_ReplicationInstanceArnRequired(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()

	rec := doDMS(t, h, "CreateEndpoint", map[string]any{
		"EndpointIdentifier": "pg-src-2",
		"EndpointType":       "source",
		"EngineName":         "postgres",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	epARN := parseJSON(t, rec)["Endpoint"].(map[string]any)["EndpointArn"].(string)

	rec = doDMS(t, h, "RefreshSchemas", map[string]any{"EndpointArn": epARN})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDeleteEndpointInUse(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()

	// Create replication instance.
	riResp := parseJSON(t, doDMS(t, h, "CreateReplicationInstance", map[string]any{
		"ReplicationInstanceIdentifier": "ri1",
		"ReplicationInstanceClass":      "dms.t3.micro",
	}))
	riARN := riResp["ReplicationInstance"].(map[string]any)["ReplicationInstanceArn"].(string)

	// Create source and target endpoints.
	srcResp := parseJSON(t, doDMS(t, h, "CreateEndpoint", map[string]any{
		"EndpointIdentifier": "src",
		"EndpointType":       "source",
		"EngineName":         "mysql",
	}))
	srcARN := srcResp["Endpoint"].(map[string]any)["EndpointArn"].(string)

	tgtResp := parseJSON(t, doDMS(t, h, "CreateEndpoint", map[string]any{
		"EndpointIdentifier": "tgt",
		"EndpointType":       "target",
		"EngineName":         "aurora",
	}))
	tgtARN := tgtResp["Endpoint"].(map[string]any)["EndpointArn"].(string)

	// Create task referencing both endpoints.
	rec := doDMS(t, h, "CreateReplicationTask", map[string]any{
		"ReplicationTaskIdentifier": "task1",
		"SourceEndpointArn":         srcARN,
		"TargetEndpointArn":         tgtARN,
		"ReplicationInstanceArn":    riARN,
		"MigrationType":             "full-load",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Attempt to delete the source endpoint while task references it.
	del := doDMS(t, h, "DeleteEndpoint", map[string]any{"EndpointArn": srcARN})
	assert.Equal(t, http.StatusBadRequest, del.Code)
	body := parseJSON(t, del)
	msg, _ := body["message"].(string)
	assert.Contains(t, msg, "in use")
}

func TestHandler_EndpointCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *dms.Handler)
		name string
	}{
		{
			name: "create_success",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "CreateEndpoint", map[string]any{
					"EndpointIdentifier": "src-ep",
					"EndpointType":       "source",
					"EngineName":         "mysql",
					"ServerName":         "db.example.com",
					"Port":               3306,
					"DatabaseName":       "mydb",
					"Username":           "admin",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec)
				ep, ok := resp["Endpoint"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "src-ep", ep["EndpointIdentifier"])
				assert.Equal(t, "source", ep["EndpointType"])
				assert.Equal(t, "mysql", ep["EngineName"])
				assert.Equal(t, "active", ep["Status"])
				assert.NotEmpty(t, ep["EndpointArn"])
			},
		},
		{
			name: "create_duplicate_conflict",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				doDMS(t, h, "CreateEndpoint", map[string]any{
					"EndpointIdentifier": "dup-ep",
					"EndpointType":       "source",
					"EngineName":         "mysql",
				})
				rec := doDMS(t, h, "CreateEndpoint", map[string]any{
					"EndpointIdentifier": "dup-ep",
					"EndpointType":       "source",
					"EngineName":         "mysql",
				})
				assert.Equal(t, http.StatusConflict, rec.Code)
			},
		},
		{
			name: "describe_all",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				doDMS(t, h, "CreateEndpoint", map[string]any{
					"EndpointIdentifier": "ep-a",
					"EndpointType":       "source",
					"EngineName":         "postgres",
				})
				rec := doDMS(t, h, "DescribeEndpoints", map[string]any{})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec)
				list, ok := resp["Endpoints"].([]any)
				require.True(t, ok)
				assert.Len(t, list, 1)
			},
		},
		{
			name: "delete_by_arn",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				create := doDMS(t, h, "CreateEndpoint", map[string]any{
					"EndpointIdentifier": "del-ep",
					"EndpointType":       "target",
					"EngineName":         "s3",
				})
				require.Equal(t, http.StatusOK, create.Code)
				createResp := parseJSON(t, create)
				ep := createResp["Endpoint"].(map[string]any)
				arn := ep["EndpointArn"].(string)

				rec := doDMS(t, h, "DeleteEndpoint", map[string]any{
					"EndpointArn": arn,
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				deleteResp := parseJSON(t, rec)
				delEp, ok := deleteResp["Endpoint"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "del-ep", delEp["EndpointIdentifier"])
			},
		},
		{
			name: "delete_not_found",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "DeleteEndpoint", map[string]any{
					"EndpointArn": "arn:aws:dms:us-east-1:123:endpoint:nonexistent",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "create_missing_identifier",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "CreateEndpoint", map[string]any{
					"EndpointType": "source",
					"EngineName":   "mysql",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "create_missing_engine",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "CreateEndpoint", map[string]any{
					"EndpointIdentifier": "ep-no-engine",
					"EndpointType":       "source",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "describe_by_arn_filter",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				create := doDMS(t, h, "CreateEndpoint", map[string]any{
					"EndpointIdentifier": "arn-ep",
					"EndpointType":       "source",
					"EngineName":         "mysql",
				})
				require.Equal(t, http.StatusOK, create.Code)
				createResp := parseJSON(t, create)
				ep := createResp["Endpoint"].(map[string]any)
				arnVal := ep["EndpointArn"].(string)

				rec := doDMS(t, h, "DescribeEndpoints", map[string]any{
					"Filters": []map[string]any{
						{"Name": "endpoint-arn", "Values": []string{arnVal}},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec)
				list, ok := resp["Endpoints"].([]any)
				require.True(t, ok)
				assert.Len(t, list, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestDMSHandler()
			tt.run(t, h)
		})
	}
}

func TestHandler_DescribeEndpointsByFilter(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	doDMS(t, h, "CreateEndpoint", map[string]any{
		"EndpointIdentifier": "ep-filter-1",
		"EndpointType":       "source",
		"EngineName":         "mysql",
	})
	doDMS(t, h, "CreateEndpoint", map[string]any{
		"EndpointIdentifier": "ep-filter-2",
		"EndpointType":       "target",
		"EngineName":         "s3",
	})

	rec := doDMS(t, h, "DescribeEndpoints", map[string]any{
		"Filters": []map[string]any{
			{"Name": "endpoint-id", "Values": []string{"ep-filter-1"}},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	resp := parseJSON(t, rec)
	list := resp["Endpoints"].([]any)
	assert.Len(t, list, 1)
	assert.Equal(t, "ep-filter-1", list[0].(map[string]any)["EndpointIdentifier"])
}

// TestDescribeEndpointsPagination verifies Marker/MaxRecords pagination.
func TestDescribeEndpointsPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		count      int
		maxRecords int
		wantCount  int
		wantMarker bool
	}{
		{
			name:       "first_page_limited",
			count:      3,
			maxRecords: 2,
			wantCount:  2,
			wantMarker: true,
		},
		{
			name:       "all_results_no_marker",
			count:      2,
			maxRecords: 10,
			wantCount:  2,
			wantMarker: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestDMSHandler()

			for i := range tt.count {
				doDMS(t, h, "CreateEndpoint", map[string]any{
					"EndpointIdentifier": "ep-" + strconv.Itoa(i),
					"EndpointType":       "source",
					"EngineName":         "mysql",
				})
			}

			rec := doDMS(t, h, "DescribeEndpoints", map[string]any{"MaxRecords": tt.maxRecords})
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseJSON(t, rec)
			list, ok := resp["Endpoints"].([]any)
			require.True(t, ok)
			assert.Len(t, list, tt.wantCount)

			_, hasMarker := resp["Marker"]
			assert.Equal(t, tt.wantMarker, hasMarker)
		})
	}
}

// TestDescribeEndpointsContinuation verifies a two-page traversal.
func TestDescribeEndpointsContinuation(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()

	for i := range 3 {
		doDMS(t, h, "CreateEndpoint", map[string]any{
			"EndpointIdentifier": "ep-" + strconv.Itoa(i),
			"EndpointType":       "source",
			"EngineName":         "mysql",
		})
	}

	rec1 := doDMS(t, h, "DescribeEndpoints", map[string]any{"MaxRecords": 2})
	require.Equal(t, http.StatusOK, rec1.Code)
	resp1 := parseJSON(t, rec1)
	page1, ok := resp1["Endpoints"].([]any)
	require.True(t, ok)
	assert.Len(t, page1, 2)

	marker, hasMarker := resp1["Marker"].(string)
	require.True(t, hasMarker)
	require.NotEmpty(t, marker)

	rec2 := doDMS(t, h, "DescribeEndpoints", map[string]any{
		"MaxRecords": 2,
		"Marker":     marker,
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	resp2 := parseJSON(t, rec2)
	page2, ok := resp2["Endpoints"].([]any)
	require.True(t, ok)
	assert.Len(t, page2, 1)

	_, stillHasMarker := resp2["Marker"]
	assert.False(t, stillHasMarker)
}

func TestDescribeEndpointsFilterMiss(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	rec := doDMS(t, h, "DescribeEndpoints", map[string]any{
		"Filters": []map[string]any{
			{"Name": "endpoint-id", "Values": []string{"nonexistent"}},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	body := parseJSON(t, rec)
	endpoints := body["Endpoints"].([]any)
	assert.Empty(t, endpoints)
}

func TestHandler_DescribeEngineVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *dms.Handler)
		name string
	}{
		{
			name: "returns_non_empty_list",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "DescribeEngineVersions", map[string]any{})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec)
				versions, ok := resp["EngineVersions"].([]any)
				require.True(t, ok)
				assert.NotEmpty(t, versions)
			},
		},
		{
			name: "versions_have_version_and_lifecycle",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "DescribeEngineVersions", map[string]any{})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec)
				versions := resp["EngineVersions"].([]any)
				require.NotEmpty(t, versions)
				v0 := versions[0].(map[string]any)
				assert.NotEmpty(t, v0["Version"])
				assert.NotEmpty(t, v0["Lifecycle"])
			},
		},
		{
			name: "contains_current_default_engine_version",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "DescribeEngineVersions", map[string]any{})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec)
				versions := resp["EngineVersions"].([]any)

				found := false
				for _, v := range versions {
					if v.(map[string]any)["Version"] == "3.5.3" {
						found = true

						break
					}
				}
				assert.True(t, found, "expected version 3.5.3 in engine versions list")
			},
		},
		{
			name: "supports_pagination",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "DescribeEngineVersions", map[string]any{"MaxRecords": 2})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec)
				versions, ok := resp["EngineVersions"].([]any)
				require.True(t, ok)
				assert.Len(t, versions, 2)
				_, hasMarker := resp["Marker"]
				assert.True(t, hasMarker)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestDMSHandler()
			tt.run(t, h)
		})
	}
}
