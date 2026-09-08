package efs_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/efs"
)

// TestReplicationConfiguration exercises CreateReplicationConfiguration,
// DescribeReplicationConfigurations and DeleteReplicationConfiguration.
func TestReplicationConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *efs.Handler)
		name string
	}{
		{
			name: "create_describe_delete",
			ops: func(t *testing.T, h *efs.Handler) {
				t.Helper()

				rec := doREST(t, h, http.MethodPost, "/2015-02-01/file-systems", map[string]any{
					"CreationToken": "repl-token",
				})
				require.Equal(t, http.StatusCreated, rec.Code)
				fsID := parseResp(t, rec)["FileSystemId"].(string)

				// Create replication configuration.
				rec2 := doREST(
					t, h, http.MethodPost,
					"/2015-02-01/file-systems/"+fsID+"/replication-configuration",
					map[string]any{
						"Destinations": []map[string]any{
							{"Region": "us-west-2"},
						},
					},
				)
				assert.Equal(t, http.StatusOK, rec2.Code)
				resp := parseResp(t, rec2)
				assert.Equal(t, fsID, resp["SourceFileSystemId"])
				assert.Equal(t, "us-east-1", resp["SourceFileSystemRegion"])
				dests := resp["Destinations"].([]any)
				assert.Len(t, dests, 1)

				// Describe by filesystem ID.
				rec3 := doREST(
					t, h, http.MethodGet,
					"/2015-02-01/file-systems/replication-configurations?FileSystemId="+fsID,
					nil,
				)
				assert.Equal(t, http.StatusOK, rec3.Code)
				desc := parseResp(t, rec3)
				replications := desc["Replications"].([]any)
				assert.Len(t, replications, 1)

				// Delete.
				rec4 := doREST(
					t, h, http.MethodDelete,
					"/2015-02-01/file-systems/"+fsID+"/replication-configuration",
					nil,
				)
				assert.Equal(t, http.StatusNoContent, rec4.Code)

				// Describe after delete returns empty.
				rec5 := doREST(
					t, h, http.MethodGet,
					"/2015-02-01/file-systems/replication-configurations?FileSystemId="+fsID,
					nil,
				)
				assert.Equal(t, http.StatusOK, rec5.Code)
				desc2 := parseResp(t, rec5)
				replications2 := desc2["Replications"].([]any)
				assert.Empty(t, replications2)
			},
		},
		{
			name: "create_on_missing_fs_returns_404",
			ops: func(t *testing.T, h *efs.Handler) {
				t.Helper()

				rec := doREST(
					t, h, http.MethodPost,
					"/2015-02-01/file-systems/fs-notexist/replication-configuration",
					map[string]any{"Destinations": []map[string]any{{"Region": "us-west-2"}}},
				)
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "create_duplicate_returns_409",
			ops: func(t *testing.T, h *efs.Handler) {
				t.Helper()

				rec := doREST(t, h, http.MethodPost, "/2015-02-01/file-systems", map[string]any{
					"CreationToken": "repl-dup-token",
				})
				require.Equal(t, http.StatusCreated, rec.Code)
				fsID := parseResp(t, rec)["FileSystemId"].(string)

				body := map[string]any{"Destinations": []map[string]any{{"Region": "us-west-2"}}}
				rec2 := doREST(t, h, http.MethodPost,
					"/2015-02-01/file-systems/"+fsID+"/replication-configuration", body)
				require.Equal(t, http.StatusOK, rec2.Code)

				rec3 := doREST(t, h, http.MethodPost,
					"/2015-02-01/file-systems/"+fsID+"/replication-configuration", body)
				assert.Equal(t, http.StatusConflict, rec3.Code)
			},
		},
		{
			name: "delete_non_existent_returns_404",
			ops: func(t *testing.T, h *efs.Handler) {
				t.Helper()

				rec := doREST(t, h, http.MethodPost, "/2015-02-01/file-systems", map[string]any{
					"CreationToken": "repl-del-token",
				})
				require.Equal(t, http.StatusCreated, rec.Code)
				fsID := parseResp(t, rec)["FileSystemId"].(string)

				rec2 := doREST(
					t, h, http.MethodDelete,
					"/2015-02-01/file-systems/"+fsID+"/replication-configuration",
					nil,
				)
				assert.Equal(t, http.StatusNotFound, rec2.Code)
			},
		},
		{
			name: "describe_all_without_filter",
			ops: func(t *testing.T, h *efs.Handler) {
				t.Helper()

				for _, token := range []string{"repl-all-1", "repl-all-2"} {
					r := doREST(t, h, http.MethodPost, "/2015-02-01/file-systems", map[string]any{
						"CreationToken": token,
					})
					require.Equal(t, http.StatusCreated, r.Code)
					fsID := parseResp(t, r)["FileSystemId"].(string)

					doREST(
						t, h, http.MethodPost,
						"/2015-02-01/file-systems/"+fsID+"/replication-configuration",
						map[string]any{"Destinations": []map[string]any{{"Region": "eu-west-1"}}},
					)
				}

				rec := doREST(t, h, http.MethodGet,
					"/2015-02-01/file-systems/replication-configurations", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				replications := parseResp(t, rec)["Replications"].([]any)
				assert.Len(t, replications, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestEFSHandler()
			tt.ops(t, h)
		})
	}
}

// TestSortedDescribeReplicationConfigurations verifies sorted replication configs.
func TestSortedDescribeReplicationConfigurations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count int
	}{
		{name: "multiple_sorted", count: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEFSHandler()
			for i := range tt.count {
				fsID := createFS(t, h, "tok-rep-sort-"+tt.name+"-"+string(rune('a'+i)))
				rec := doREST(t, h, http.MethodPost,
					"/2015-02-01/file-systems/"+fsID+"/replication-configuration",
					map[string]any{
						"Destinations": []map[string]any{
							{"Region": "us-west-2"},
						},
					})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doREST(t, h, http.MethodGet,
				"/2015-02-01/file-systems/replication-configurations", nil)
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			list, ok := resp["Replications"].([]any)
			require.True(t, ok)
			require.Len(t, list, tt.count)

			for i := 1; i < len(list); i++ {
				prev := list[i-1].(map[string]any)["SourceFileSystemId"].(string)
				curr := list[i].(map[string]any)["SourceFileSystemId"].(string)
				assert.LessOrEqual(t, prev, curr)
			}
		})
	}
}

// TestFileSystemProtection verifies protection is emitted in responses.
func TestFileSystemProtection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		protection     string
		wantProtection string
		wantHTTPStatus int
	}{
		{
			name:           "default_protection_is_disabled",
			protection:     "",
			wantHTTPStatus: http.StatusOK,
			wantProtection: "DISABLED",
		},
		{
			name:           "set_to_enabled",
			protection:     "ENABLED",
			wantHTTPStatus: http.StatusOK,
			wantProtection: "ENABLED",
		},
		{
			name:           "set_to_replicating",
			protection:     "REPLICATING",
			wantHTTPStatus: http.StatusOK,
			wantProtection: "REPLICATING",
		},
		{
			name:           "invalid_value_returns_400",
			protection:     "UNKNOWN",
			wantHTTPStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEFSHandler()
			fsID := createFS(t, h, "tok-prot-"+tt.name)

			if tt.protection != "" {
				rec := doREST(t, h, http.MethodPut,
					"/2015-02-01/file-systems/"+fsID+"/protection",
					map[string]any{"ReplicationOverwriteProtection": tt.protection})
				assert.Equal(t, tt.wantHTTPStatus, rec.Code)

				if tt.wantHTTPStatus != http.StatusOK {
					return
				}
			}

			// Verify via describe.
			rec2 := doREST(t, h, http.MethodGet, "/2015-02-01/file-systems/"+fsID, nil)
			require.Equal(t, http.StatusOK, rec2.Code)

			resp2 := parseResp(t, rec2)
			fsList := resp2["FileSystems"].([]any)
			require.Len(t, fsList, 1)
			fsResp := fsList[0].(map[string]any)
			prot := fsResp["FileSystemProtection"].(map[string]any)
			assert.Equal(t, tt.wantProtection, prot["ReplicationOverwriteProtection"])
		})
	}
}

// TestCreateReplicationConfiguration_DestinationCount verifies exactly one destination
// is required, matching AWS EFS behavior.
func TestCreateReplicationConfiguration_DestinationCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		destinations []map[string]any
		wantStatus   int
	}{
		{
			name:         "one_destination_accepted",
			destinations: []map[string]any{{"Region": "us-west-2"}},
			wantStatus:   http.StatusOK,
		},
		{
			name:         "zero_destinations_rejected",
			destinations: []map[string]any{},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name: "two_destinations_rejected",
			destinations: []map[string]any{
				{"Region": "us-west-2"},
				{"Region": "eu-west-1"},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEFSHandler()
			fsID := createFS(t, h, "repl-dest-"+tt.name)

			rec := doREST(
				t, h, http.MethodPost,
				"/2015-02-01/file-systems/"+fsID+"/replication-configuration",
				map[string]any{"Destinations": tt.destinations},
			)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusBadRequest {
				resp := parseResp(t, rec)
				assert.Contains(t, resp["ErrorCode"], "ValidationException")
			}
		})
	}
}

// TestReplicationConfiguration_SourceOwnerInResponse verifies that
// CreateReplicationConfiguration and DescribeReplicationConfigurations return
// SourceFileSystemOwnerId. Real AWS includes the owner account ID.
func TestReplicationConfiguration_SourceOwnerInResponse(t *testing.T) {
	t.Parallel()

	h := newTestEFSHandler()

	fsRec := doREST(t, h, http.MethodPost, "/2015-02-01/file-systems", map[string]any{
		"CreationToken": "rc-owner-test",
	})
	require.Equal(t, http.StatusCreated, fsRec.Code)

	var fsOut struct {
		FileSystemID string `json:"FileSystemId"`
	}
	require.NoError(t, json.Unmarshal(fsRec.Body.Bytes(), &fsOut))

	rcRec := doREST(t, h, http.MethodPost,
		"/2015-02-01/file-systems/"+fsOut.FileSystemID+"/replication-configuration",
		map[string]any{
			"Destinations": []map[string]any{
				{"Region": "us-west-2"},
			},
		})
	require.Equal(t, http.StatusOK, rcRec.Code, "CreateReplicationConfiguration failed: %s", rcRec.Body.String())

	var rcOut struct {
		SourceFileSystemOwnerID string `json:"SourceFileSystemOwnerId"`
	}
	require.NoError(t, json.Unmarshal(rcRec.Body.Bytes(), &rcOut))

	assert.NotEmpty(t, rcOut.SourceFileSystemOwnerID,
		"SourceFileSystemOwnerId must be present in CreateReplicationConfiguration response")
}

// TestReplicationConfiguration_DestinationHasOwnerNoArn verifies that
// destination entries in a replication configuration include FileSystemId and
// OwnerId, but never a FileSystemArn. Real AWS's response-side
// types.Destination (aws-sdk-go-v2/service/efs@v1.44.4 types/types.go) has no
// ARN field at all -- only the request-side types.DestinationToCreate does --
// and deserializers.go's awsRestjson1_deserializeDocumentDestination declares
// no "FileSystemArn" case, so a real SDK client would silently drop it. A
// prior version of this test asserted the opposite and locked in that
// fabricated field.
func TestReplicationConfiguration_DestinationHasOwnerNoArn(t *testing.T) {
	t.Parallel()

	h := newTestEFSHandler()

	fsRec := doREST(t, h, http.MethodPost, "/2015-02-01/file-systems", map[string]any{
		"CreationToken": "rc-dest-arn-test",
	})
	require.Equal(t, http.StatusCreated, fsRec.Code)

	var fsOut struct {
		FileSystemID string `json:"FileSystemId"`
	}
	require.NoError(t, json.Unmarshal(fsRec.Body.Bytes(), &fsOut))

	rcRec := doREST(t, h, http.MethodPost,
		"/2015-02-01/file-systems/"+fsOut.FileSystemID+"/replication-configuration",
		map[string]any{
			"Destinations": []map[string]any{
				{"Region": "eu-west-1"},
			},
		})
	require.Equal(t, http.StatusOK, rcRec.Code, "CreateReplicationConfiguration failed: %s", rcRec.Body.String())

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rcRec.Body.Bytes(), &raw))
	dests, ok := raw["Destinations"].([]any)
	require.True(t, ok)
	require.Len(t, dests, 1)

	dest, ok := dests[0].(map[string]any)
	require.True(t, ok)

	var rcOut struct {
		Destinations []struct {
			FileSystemID string `json:"FileSystemId"`
			OwnerID      string `json:"OwnerId"`
			Status       string `json:"Status"`
		} `json:"Destinations"`
	}
	require.NoError(t, json.Unmarshal(rcRec.Body.Bytes(), &rcOut))
	require.Len(t, rcOut.Destinations, 1)

	d := rcOut.Destinations[0]
	assert.NotEmpty(t, d.FileSystemID, "destination FileSystemId must be assigned")
	assert.NotEmpty(t, d.OwnerID, "destination OwnerId must be present")
	assert.Equal(t, "ENABLED", d.Status)

	_, hasArn := dest["FileSystemArn"]
	assert.False(t, hasArn, "destination FileSystemArn must not appear on the wire")
}

// TestDescribeReplicationConfigurations_ByDestinationFileSystemID verifies that
// DescribeReplicationConfigurations with FileSystemId matching a destination (not source)
// returns the replication configuration. Real AWS matches both source and destination.
func TestDescribeReplicationConfigurations_ByDestinationFileSystemID(t *testing.T) {
	t.Parallel()

	h := newTestEFSHandler()

	fsRec := doREST(t, h, http.MethodPost, "/2015-02-01/file-systems", map[string]any{
		"CreationToken": "rc-dest-lookup",
	})
	require.Equal(t, http.StatusCreated, fsRec.Code)

	var fsOut struct {
		FileSystemID string `json:"FileSystemId"`
	}
	require.NoError(t, json.Unmarshal(fsRec.Body.Bytes(), &fsOut))

	rcRec := doREST(t, h, http.MethodPost,
		"/2015-02-01/file-systems/"+fsOut.FileSystemID+"/replication-configuration",
		map[string]any{
			"Destinations": []map[string]any{
				{"Region": "ap-southeast-1"},
			},
		})
	require.Equal(t, http.StatusOK, rcRec.Code)

	var rcOut struct {
		Destinations []struct {
			FileSystemID string `json:"FileSystemId"`
		} `json:"Destinations"`
	}
	require.NoError(t, json.Unmarshal(rcRec.Body.Bytes(), &rcOut))
	require.Len(t, rcOut.Destinations, 1)

	destFSID := rcOut.Destinations[0].FileSystemID
	require.NotEmpty(t, destFSID)

	// Now query using the destination FS ID.
	listRec := doREST(t, h, http.MethodGet,
		"/2015-02-01/file-systems/replication-configurations?FileSystemId="+destFSID, nil)
	require.Equal(
		t, http.StatusOK, listRec.Code,
		"DescribeReplicationConfigurations by destination failed: %s", listRec.Body.String(),
	)

	var listOut struct {
		Replications []struct {
			SourceFileSystemID string `json:"SourceFileSystemId"`
		} `json:"Replications"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listOut))

	assert.Len(t, listOut.Replications, 1,
		"DescribeReplicationConfigurations must return result when filtering by destination FS ID")
	assert.Equal(t, fsOut.FileSystemID, listOut.Replications[0].SourceFileSystemID)
}

// TestDescribeReplicationConfigurations_Pagination verifies NextToken/MaxResults
// pagination over the unfiltered list, matching
// aws-sdk-go-v2/service/efs's DescribeReplicationConfigurationsInput.MaxResults /
// .NextToken and DescribeReplicationConfigurationsOutput.NextToken.
func TestDescribeReplicationConfigurations_Pagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		count     int
		maxResult int
		wantFirst int
		wantNext  bool
	}{
		{
			name:      "fits_in_one_page",
			count:     2,
			maxResult: 5,
			wantFirst: 2,
			wantNext:  false,
		},
		{
			name:      "spans_multiple_pages",
			count:     4,
			maxResult: 2,
			wantFirst: 2,
			wantNext:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEFSHandler()
			for i := range tt.count {
				fsID := createFS(t, h, "repl-page-"+tt.name+"-"+string(rune('a'+i)))
				rec := doREST(t, h, http.MethodPost,
					"/2015-02-01/file-systems/"+fsID+"/replication-configuration",
					map[string]any{"Destinations": []map[string]any{{"Region": "us-west-2"}}})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doREST(t, h, http.MethodGet,
				"/2015-02-01/file-systems/replication-configurations?MaxResults="+strconv.Itoa(tt.maxResult), nil)
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			replications, ok := resp["Replications"].([]any)
			require.True(t, ok)
			assert.Len(t, replications, tt.wantFirst)

			if tt.wantNext {
				assert.NotEmpty(t, resp["NextToken"])

				// Follow the token and confirm the remaining items come back.
				rec2 := doREST(t, h, http.MethodGet,
					"/2015-02-01/file-systems/replication-configurations?MaxResults="+
						strconv.Itoa(tt.maxResult)+"&NextToken="+resp["NextToken"].(string), nil)
				require.Equal(t, http.StatusOK, rec2.Code)
				resp2 := parseResp(t, rec2)
				replications2 := resp2["Replications"].([]any)
				assert.Len(t, replications2, tt.count-tt.wantFirst)
			} else {
				assert.NotContains(t, resp, "NextToken")
			}
		})
	}
}

// TestReplicationConfiguration_DestinationLastReplicatedTimestamp verifies that
// Destinations[].LastReplicatedTimestamp is populated as an epoch-seconds number
// (matching aws-sdk-go-v2/service/efs's types.Destination.LastReplicatedTimestamp
// *time.Time, which serializes as a JSON number under restjson1) once the
// replication configuration is created.
func TestReplicationConfiguration_DestinationLastReplicatedTimestamp(t *testing.T) {
	t.Parallel()

	h := newTestEFSHandler()
	fsID := createFS(t, h, "repl-lrt")

	rec := doREST(t, h, http.MethodPost,
		"/2015-02-01/file-systems/"+fsID+"/replication-configuration",
		map[string]any{"Destinations": []map[string]any{{"Region": "us-west-2"}}})
	require.Equal(t, http.StatusOK, rec.Code, "CreateReplicationConfiguration failed: %s", rec.Body.String())

	resp := parseResp(t, rec)
	dests := resp["Destinations"].([]any)
	require.Len(t, dests, 1)

	d := dests[0].(map[string]any)
	lrt, ok := d["LastReplicatedTimestamp"].(float64)
	require.True(t, ok, "LastReplicatedTimestamp must be a JSON number, got %T", d["LastReplicatedTimestamp"])
	assert.Positive(t, lrt)
}

// TestCreateReplicationConfiguration_AssignsDestinationFileSystemID verifies that
// a destination entry without an explicit FileSystemId gets one assigned automatically.
// Real AWS always creates a destination file system.
func TestCreateReplicationConfiguration_AssignsDestinationFileSystemID(t *testing.T) {
	t.Parallel()

	h := newTestEFSHandler()

	fsRec := doREST(t, h, http.MethodPost, "/2015-02-01/file-systems", map[string]any{
		"CreationToken": "rc-auto-dest-id",
	})
	require.Equal(t, http.StatusCreated, fsRec.Code)

	var fsOut struct {
		FileSystemID string `json:"FileSystemId"`
	}
	require.NoError(t, json.Unmarshal(fsRec.Body.Bytes(), &fsOut))

	rcRec := doREST(t, h, http.MethodPost,
		"/2015-02-01/file-systems/"+fsOut.FileSystemID+"/replication-configuration",
		map[string]any{
			"Destinations": []map[string]any{
				{"Region": "ca-central-1"},
			},
		})
	require.Equal(t, http.StatusOK, rcRec.Code)

	var rcOut struct {
		Destinations []struct {
			FileSystemID string `json:"FileSystemId"`
		} `json:"Destinations"`
	}
	require.NoError(t, json.Unmarshal(rcRec.Body.Bytes(), &rcOut))
	require.Len(t, rcOut.Destinations, 1)

	assert.NotEmpty(t, rcOut.Destinations[0].FileSystemID,
		"destination FileSystemId must be auto-assigned when not provided")
}

// TestCreateReplicationConfiguration_Duplicate_WireErrorType verifies that
// creating a second replication configuration for the same source file
// system returns ConflictException, not FileSystemAlreadyExists.
//
// CreateReplicationConfiguration's own deserializeOpError (aws-sdk-go-v2/
// service/efs@v1.44.4 deserializers.go) recognizes BadRequest, ConflictException,
// FileSystemLimitExceeded, FileSystemNotFound, IncorrectFileSystemLifeCycleState,
// InsufficientThroughputCapacity, InternalServerError, ReplicationNotFound,
// ThroughputLimitExceeded, UnsupportedAvailabilityZone, ValidationException --
// no FileSystemAlreadyExists case, unlike CreateFileSystem.
func TestCreateReplicationConfiguration_Duplicate_WireErrorType(t *testing.T) {
	t.Parallel()

	h := newTestEFSHandler()
	fsID := createFS(t, h, "rc-dup-token")

	first := doREST(t, h, http.MethodPost,
		"/2015-02-01/file-systems/"+fsID+"/replication-configuration",
		map[string]any{"Destinations": []map[string]any{{"Region": "us-west-2"}}})
	require.Equal(t, http.StatusOK, first.Code)

	second := doREST(t, h, http.MethodPost,
		"/2015-02-01/file-systems/"+fsID+"/replication-configuration",
		map[string]any{"Destinations": []map[string]any{{"Region": "us-west-2"}}})
	require.Equal(t, http.StatusConflict, second.Code)
	assert.Equal(t, "ConflictException", second.Header().Get("X-Amzn-Errortype"))

	body := parseResp(t, second)
	assert.Equal(t, "ConflictException", body["ErrorCode"])
	assert.NotEqual(t, "FileSystemAlreadyExists", body["ErrorCode"])
}
