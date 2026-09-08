package fsx_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	fsxsdk "github.com/aws/aws-sdk-go-v2/service/fsx"
	"github.com/aws/aws-sdk-go-v2/service/fsx/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testFileCacheLustreConfig builds a minimal valid CreateFileCache
// LustreConfiguration block for the typed SDK client (fsx@v1.68.4
// types/types.go:574 -- DeploymentType/MetadataConfiguration/
// PerUnitStorageThroughput required).
func testFileCacheLustreConfig() *types.CreateFileCacheLustreConfiguration {
	return &types.CreateFileCacheLustreConfiguration{
		DeploymentType:           types.FileCacheLustreDeploymentTypeCache1,
		MetadataConfiguration:    &types.FileCacheLustreMetadataConfiguration{StorageCapacity: aws.Int32(2400)},
		PerUnitStorageThroughput: aws.Int32(1000),
	}
}

// TestFileCache_TagsWireShape proves gopherstack's FileCache wire shape follows the
// real SDK's split between two distinct types for the same resource
// (fsx@v1.68.4 types/types.go:2264 FileCache vs types/types.go:2349
// FileCacheCreating): CreateFileCacheOutput.FileCache is a *FileCacheCreating,
// which HAS a Tags member (deserializers.go:9984
// awsAwsjson11_deserializeDocumentFileCacheCreating, case "Tags") --
// but DescribeFileCachesOutput.FileCaches and UpdateFileCacheOutput.FileCache are
// plain FileCache, which has NO Tags case at all
// (deserializers.go:9818 awsAwsjson11_deserializeDocumentFileCache has no
// case "Tags"; a real client's deserializer silently drops that key into its
// default branch, and types.FileCache doesn't even have a field to hold it).
func TestFileCache_TagsWireShape(t *testing.T) {
	t.Parallel()

	t.Run("create returns tags via typed client", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		client := newTestFSxClient(t, h)

		out, err := client.CreateFileCache(t.Context(), &fsxsdk.CreateFileCacheInput{
			FileCacheType:        types.FileCacheTypeLustre,
			FileCacheTypeVersion: aws.String("2.12"),
			StorageCapacity:      aws.Int32(1200),
			SubnetIds:            []string{"subnet-0123abcd"},
			Tags:                 []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
			LustreConfiguration:  testFileCacheLustreConfig(),
		})
		require.NoError(t, err)
		require.Len(t, out.FileCache.Tags, 1)
		assert.Equal(t, "env", aws.ToString(out.FileCache.Tags[0].Key))
		assert.Equal(t, "prod", aws.ToString(out.FileCache.Tags[0].Value))
	})

	t.Run("describe response has no Tags key", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		createRec := doFSxRequest(t, h, "CreateFileCache", map[string]any{
			"FileCacheType":        "LUSTRE",
			"FileCacheTypeVersion": "2.12",
			"SubnetIds":            []string{"subnet-1"},
			"StorageCapacity":      1200,
			"Tags":                 []map[string]string{{"Key": "env", "Value": "prod"}},
			"LustreConfiguration":  fileCacheLustreConfigBody(),
		})
		require.Equal(t, http.StatusOK, createRec.Code)

		var createOut map[string]any
		require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
		fcID := createOut["FileCache"].(map[string]any)["FileCacheId"].(string)

		describeRec := doFSxRequest(t, h, "DescribeFileCaches", map[string]any{
			"FileCacheIds": []string{fcID},
		})
		require.Equal(t, http.StatusOK, describeRec.Code)

		var describeOut map[string]any
		require.NoError(t, json.Unmarshal(describeRec.Body.Bytes(), &describeOut))

		caches, ok := describeOut["FileCaches"].([]any)
		require.True(t, ok)
		require.Len(t, caches, 1)

		fc, ok := caches[0].(map[string]any)
		require.True(t, ok)

		_, hasTags := fc["Tags"]
		assert.False(t, hasTags, "DescribeFileCaches' FileCache shape has no Tags case in the real "+
			"deserializer (deserializers.go:9818); emitting the key here is wire-inaccurate")
	})

	t.Run("update response has no Tags key", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		createRec := doFSxRequest(t, h, "CreateFileCache", map[string]any{
			"FileCacheType":        "LUSTRE",
			"FileCacheTypeVersion": "2.12",
			"SubnetIds":            []string{"subnet-1"},
			"StorageCapacity":      1200,
			"Tags":                 []map[string]string{{"Key": "env", "Value": "prod"}},
			"LustreConfiguration":  fileCacheLustreConfigBody(),
		})
		require.Equal(t, http.StatusOK, createRec.Code)

		var createOut map[string]any
		require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
		fcID := createOut["FileCache"].(map[string]any)["FileCacheId"].(string)

		updateRec := doFSxRequest(t, h, "UpdateFileCache", map[string]any{
			"FileCacheId":        fcID,
			"StorageCapacityGiB": 2400,
		})
		require.Equal(t, http.StatusOK, updateRec.Code)

		var updateOut map[string]any
		require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updateOut))

		fc, ok := updateOut["FileCache"].(map[string]any)
		require.True(t, ok)

		_, hasTags := fc["Tags"]
		assert.False(t, hasTags, "UpdateFileCache's FileCache shape has no Tags case in the real "+
			"deserializer (deserializers.go:9818); emitting the key here is wire-inaccurate")
	})
}
