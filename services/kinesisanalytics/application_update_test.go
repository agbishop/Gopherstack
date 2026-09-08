package kinesisanalytics_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kinesisanalytics"
)

func TestUpdateApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup            func(*kinesisanalytics.InMemoryBackend)
		name             string
		appName          string
		codeUpdate       string
		currentVersionID int64
		wantVersionID    int64
		wantErr          bool
	}{
		{
			name:             "updates application code",
			appName:          "updatable",
			currentVersionID: 1,
			codeUpdate:       "SELECT 2",
			setup: func(b *kinesisanalytics.InMemoryBackend) {
				_, _ = kinesisanalytics.CreateApp(b, testRegion, testAccountID, "updatable", "", "SELECT 1", nil)
			},
			wantVersionID: 2,
		},
		{
			name:             "version mismatch returns error",
			appName:          "ver-app",
			currentVersionID: 99,
			setup: func(b *kinesisanalytics.InMemoryBackend) {
				_, _ = kinesisanalytics.CreateApp(b, testRegion, testAccountID, "ver-app", "", "", nil)
			},
			wantErr: true,
		},
		{
			name:             "code update exceeding 100KB limit returns error",
			appName:          "oversized-code-app",
			currentVersionID: 1,
			codeUpdate:       strings.Repeat("a", 102401),
			setup: func(b *kinesisanalytics.InMemoryBackend) {
				_, _ = kinesisanalytics.CreateApp(
					b,
					testRegion,
					testAccountID,
					"oversized-code-app",
					"",
					"SELECT 1",
					nil,
				)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			tt.setup(b)

			app, err := kinesisanalytics.UpdateAppCode(b, tt.appName, tt.currentVersionID, tt.codeUpdate)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, app)
			assert.Equal(t, tt.wantVersionID, app.ApplicationVersionID)

			if tt.codeUpdate != "" {
				assert.Equal(t, tt.codeUpdate, app.ApplicationCode)
			}
		})
	}
}

func TestHandler_UpdateApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*kinesisanalytics.InMemoryBackend)
		input      map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "updates application",
			setup: func(b *kinesisanalytics.InMemoryBackend) {
				_, _ = kinesisanalytics.CreateApp(b, testRegion, testAccountID, "upd-app", "", "SELECT 1", nil)
			},
			input: map[string]any{
				"ApplicationName":             "upd-app",
				"CurrentApplicationVersionId": 1,
				"ApplicationUpdate":           map[string]any{"ApplicationCodeUpdate": "SELECT 2"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not found for missing application",
			input:      map[string]any{"ApplicationName": "ghost", "CurrentApplicationVersionId": 1},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "version mismatch returns error",
			setup: func(b *kinesisanalytics.InMemoryBackend) {
				_, _ = kinesisanalytics.CreateApp(b, testRegion, testAccountID, "ver-app", "", "", nil)
			},
			input: map[string]any{
				"ApplicationName":             "ver-app",
				"CurrentApplicationVersionId": 99,
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandlerWithBackend(t)

			if tt.setup != nil {
				tt.setup(b)
			}

			rec := doRequest(t, h, "UpdateApplication", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_UpdateApplication_NestedWireShapes exercises UpdateApplication's
// InputUpdates/OutputUpdates/ReferenceDataSourceUpdates nested payloads using the real AWS
// "Update"-suffixed field names (e.g. "ResourceARNUpdate", not "ResourceARN"). These nested
// shapes are distinct wire types from their Add* counterparts; reusing the Add* JSON field
// names here previously caused UpdateApplication to silently no-op (or wipe fields to empty
// strings) instead of applying the update.
func TestHandler_UpdateApplication_NestedWireShapes(t *testing.T) {
	t.Parallel()

	h, b := newTestHandlerWithBackend(t)
	app, err := kinesisanalytics.CreateApp(b, testRegion, testAccountID, "wire-upd-app", "", "", nil)
	require.NoError(t, err)

	require.NoError(t, b.AddApplicationInput(
		context.Background(), app.ApplicationName, app.ApplicationVersionID,
		kinesisanalytics.InputDescription{
			NamePrefix:       "IN",
			InputParallelism: &kinesisanalytics.InputParallelism{Count: 1},
			KinesisStreamsInputDescription: &kinesisanalytics.KinesisStreamsInputDesc{
				ResourceARN: "arn:aws:kinesis:us-east-1:000000000000:stream/old-in",
				RoleARN:     "arn:aws:iam::000000000000:role/old-in-role",
			},
		},
	))

	require.NoError(t, b.AddApplicationOutput(
		context.Background(), app.ApplicationName, 2,
		kinesisanalytics.OutputDescription{
			Name: "OUT",
			KinesisStreamsOutputDescription: &kinesisanalytics.KinesisStreamsOutputDesc{
				ResourceARN: "arn:aws:kinesis:us-east-1:000000000000:stream/old-out",
				RoleARN:     "arn:aws:iam::000000000000:role/old-out-role",
			},
			DestinationSchema: &kinesisanalytics.DestinationSchemaDesc{RecordFormatType: "JSON"},
		},
	))

	require.NoError(t, b.AddApplicationReferenceDataSource(
		context.Background(), app.ApplicationName, 3,
		kinesisanalytics.ReferenceDataSourceDescription{
			TableName: "TBL",
			S3ReferenceDataSourceDescription: &kinesisanalytics.S3ReferenceDataSourceDesc{
				BucketARN:        "arn:aws:s3:::old-bucket",
				FileKey:          "old.csv",
				ReferenceRoleARN: "arn:aws:iam::000000000000:role/old-ref-role",
			},
		},
	))

	seeded, err := b.DescribeApplication(context.Background(), app.ApplicationName)
	require.NoError(t, err)
	require.Len(t, seeded.Inputs, 1)
	require.Len(t, seeded.Outputs, 1)
	require.Len(t, seeded.ReferenceDataSources, 1)

	inputID := seeded.Inputs[0].InputID
	outputID := seeded.Outputs[0].OutputID
	referenceID := seeded.ReferenceDataSources[0].ReferenceID

	rec := doRequest(t, h, "UpdateApplication", map[string]any{
		"ApplicationName":             app.ApplicationName,
		"CurrentApplicationVersionId": seeded.ApplicationVersionID,
		"ApplicationUpdate": map[string]any{
			"InputUpdates": []map[string]any{
				{
					"InputId":          inputID,
					"NamePrefixUpdate": "IN2",
					"KinesisStreamsInputUpdate": map[string]any{
						"ResourceARNUpdate": "arn:aws:kinesis:us-east-1:000000000000:stream/new-in",
						"RoleARNUpdate":     "arn:aws:iam::000000000000:role/new-in-role",
					},
					"InputParallelismUpdate": map[string]any{"CountUpdate": 3},
				},
			},
			"OutputUpdates": []map[string]any{
				{
					"OutputId":   outputID,
					"NameUpdate": "OUT2",
					"KinesisStreamsOutputUpdate": map[string]any{
						"ResourceARNUpdate": "arn:aws:kinesis:us-east-1:000000000000:stream/new-out",
						"RoleARNUpdate":     "arn:aws:iam::000000000000:role/new-out-role",
					},
				},
			},
			"ReferenceDataSourceUpdates": []map[string]any{
				{
					"ReferenceId":     referenceID,
					"TableNameUpdate": "TBL2",
					"S3ReferenceDataSourceUpdate": map[string]any{
						"BucketARNUpdate":        "arn:aws:s3:::new-bucket",
						"FileKeyUpdate":          "new.csv",
						"ReferenceRoleARNUpdate": "arn:aws:iam::000000000000:role/new-ref-role",
					},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	describeRec := doRequest(t, h, "DescribeApplication", map[string]any{"ApplicationName": app.ApplicationName})
	require.Equal(t, http.StatusOK, describeRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(describeRec.Body.Bytes(), &resp))
	detail := resp["ApplicationDetail"].(map[string]any)

	inList := detail["InputDescriptions"].([]any)
	require.Len(t, inList, 1)
	in := inList[0].(map[string]any)
	assert.Equal(t, "IN2", in["NamePrefix"])
	assert.InDelta(t, float64(3), in["InputParallelism"].(map[string]any)["Count"], 0)
	assert.Len(t, in["InAppStreamNames"].([]any), 3)
	ksInput := in["KinesisStreamsInputDescription"].(map[string]any)
	assert.Equal(t, "arn:aws:kinesis:us-east-1:000000000000:stream/new-in", ksInput["ResourceARN"])
	assert.Equal(t, "arn:aws:iam::000000000000:role/new-in-role", ksInput["RoleARN"])

	outList := detail["OutputDescriptions"].([]any)
	require.Len(t, outList, 1)
	out := outList[0].(map[string]any)
	assert.Equal(t, "OUT2", out["Name"])
	ksOutput := out["KinesisStreamsOutputDescription"].(map[string]any)
	assert.Equal(t, "arn:aws:kinesis:us-east-1:000000000000:stream/new-out", ksOutput["ResourceARN"])
	assert.Equal(t, "arn:aws:iam::000000000000:role/new-out-role", ksOutput["RoleARN"])

	refList := detail["ReferenceDataSourceDescriptions"].([]any)
	require.Len(t, refList, 1)
	ref := refList[0].(map[string]any)
	assert.Equal(t, "TBL2", ref["TableName"])
	s3Ref := ref["S3ReferenceDataSourceDescription"].(map[string]any)
	assert.Equal(t, "arn:aws:s3:::new-bucket", s3Ref["BucketARN"])
	assert.Equal(t, "new.csv", s3Ref["FileKey"])
	assert.Equal(t, "arn:aws:iam::000000000000:role/new-ref-role", s3Ref["ReferenceRoleARN"])
}

// TestHandler_UpdateApplication_InputSchemaUpdateIsPartialPatch verifies that InputSchemaUpdate
// only overwrites the sub-fields supplied by the caller (RecordFormatUpdate / RecordEncodingUpdate
// / RecordColumnUpdates), unlike ReferenceSchemaUpdate which replaces the whole schema.
func TestHandler_UpdateApplication_InputSchemaUpdateIsPartialPatch(t *testing.T) {
	t.Parallel()

	h, b := newTestHandlerWithBackend(t)
	app, err := kinesisanalytics.CreateApp(b, testRegion, testAccountID, "schema-upd-app", "", "", nil)
	require.NoError(t, err)

	require.NoError(t, b.AddApplicationInput(
		context.Background(), app.ApplicationName, app.ApplicationVersionID,
		kinesisanalytics.InputDescription{
			NamePrefix: "IN",
			InputSchema: &kinesisanalytics.SourceSchema{
				RecordEncoding: "UTF-8",
				RecordFormat:   kinesisanalytics.RecordFormat{RecordFormatType: "CSV"},
				RecordColumns:  []kinesisanalytics.RecordColumn{{Name: "COL1", SQLType: "VARCHAR(4)"}},
			},
		},
	))

	seeded, err := b.DescribeApplication(context.Background(), app.ApplicationName)
	require.NoError(t, err)
	require.Len(t, seeded.Inputs, 1)

	// Only RecordEncodingUpdate is supplied; RecordFormat/RecordColumns must survive untouched.
	rec := doRequest(t, h, "UpdateApplication", map[string]any{
		"ApplicationName":             app.ApplicationName,
		"CurrentApplicationVersionId": seeded.ApplicationVersionID,
		"ApplicationUpdate": map[string]any{
			"InputUpdates": []map[string]any{
				{
					"InputId": seeded.Inputs[0].InputID,
					"InputSchemaUpdate": map[string]any{
						"RecordEncodingUpdate": "UTF-16",
					},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	after, err := b.DescribeApplication(context.Background(), app.ApplicationName)
	require.NoError(t, err)
	require.NotNil(t, after.Inputs[0].InputSchema)
	assert.Equal(t, "UTF-16", after.Inputs[0].InputSchema.RecordEncoding)
	assert.Equal(t, "CSV", after.Inputs[0].InputSchema.RecordFormat.RecordFormatType)
	require.Len(t, after.Inputs[0].InputSchema.RecordColumns, 1)
	assert.Equal(t, "COL1", after.Inputs[0].InputSchema.RecordColumns[0].Name)
}

// TestUpdateApplication_ReturnsApplicationDetail verifies that UpdateApplication
// returns an ApplicationDetail in its response body. Real AWS Kinesis Analytics returns
// the full application detail after an update, not an empty object.
func TestUpdateApplication_ReturnsApplicationDetail(t *testing.T) {
	t.Parallel()

	h, b := newTestHandlerWithBackend(t)

	_, err := kinesisanalytics.CreateApp(b, testRegion, testAccountID, "update-resp-app", "", "old-code", nil)
	require.NoError(t, err)

	rec := doRequest(t, h, "UpdateApplication", map[string]any{
		"ApplicationName":             "update-resp-app",
		"CurrentApplicationVersionID": 1,
		"ApplicationUpdate": map[string]any{
			"ApplicationCodeUpdate": "new-code",
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	detail, ok := resp["ApplicationDetail"].(map[string]any)
	require.True(t, ok, "UpdateApplication response must contain ApplicationDetail, got: %s", rec.Body.String())
	assert.Equal(t, "update-resp-app", detail["ApplicationName"],
		"ApplicationDetail must include ApplicationName")
	assert.InDelta(t, float64(2), detail["ApplicationVersionId"], 0,
		"ApplicationDetail must show incremented version after update")
}

// TestUpdateApplication_CodeVisible verifies that the updated application code
// is visible in the ApplicationDetail returned by UpdateApplication.
func TestUpdateApplication_CodeVisible(t *testing.T) {
	t.Parallel()

	h, b := newTestHandlerWithBackend(t)

	_, err := kinesisanalytics.CreateApp(b, testRegion, testAccountID, "code-visible-app", "", "SELECT 1;", nil)
	require.NoError(t, err)

	rec := doRequest(t, h, "UpdateApplication", map[string]any{
		"ApplicationName":             "code-visible-app",
		"CurrentApplicationVersionID": 1,
		"ApplicationUpdate": map[string]any{
			"ApplicationCodeUpdate": "SELECT 2;",
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	detail, ok := resp["ApplicationDetail"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "SELECT 2;", detail["ApplicationCode"],
		"updated code must be visible in UpdateApplication response")
}

// TestUpdateApplication_BumpsVersion verifies UpdateApplication increments the version.
func TestUpdateApplication_BumpsVersion(t *testing.T) {
	t.Parallel()

	b := newBackend()
	app, _ := kinesisanalytics.CreateApp(b, testRegion, testAccountID, "ver-bump", "", "SELECT 1", nil)
	assert.Equal(t, int64(1), app.ApplicationVersionID)

	_, err := kinesisanalytics.UpdateAppCode(b, "ver-bump", 1, "SELECT 2")
	require.NoError(t, err)

	app2, _ := b.DescribeApplication(context.Background(), "ver-bump")
	assert.Equal(t, int64(2), app2.ApplicationVersionID)
	assert.Equal(t, "SELECT 2", app2.ApplicationCode)
}

// TestUpdateApplication_ConcurrentModificationException verifies version mismatch returns 400.
func TestUpdateApplication_ConcurrentModificationException(t *testing.T) {
	t.Parallel()

	h, b := newTestHandlerWithBackend(t)
	app, _ := kinesisanalytics.CreateApp(b, testRegion, testAccountID, "conc-app", "", "", nil)

	rec := doRequest(t, h, "UpdateApplication", map[string]any{
		"ApplicationName":             app.ApplicationName,
		"CurrentApplicationVersionId": int64(999), // wrong version
		"ApplicationUpdate":           map[string]any{},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ConcurrentModificationException", errResp["__type"])
}
