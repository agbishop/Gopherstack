package fsx_test

import (
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	fsxsdk "github.com/aws/aws-sdk-go-v2/service/fsx"
	"github.com/aws/aws-sdk-go-v2/service/fsx/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/fsx"
)

const tagsRTRegion = "us-east-1"

// newTestFSxClient stands up the real aws-sdk-go-v2 FSx client against an
// httptest server running this package's Handler, wired through the same
// pkgs/service registry/router used in production. FSx's Create* backend
// methods take unexported *createXInput structs (gopherstack-23ti), so this
// full HTTP round trip -- not a direct backend call -- is the only way to
// drive creation from _test.go.
func newTestFSxClient(t *testing.T, h *fsx.Handler) *fsxsdk.Client {
	t.Helper()

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(h))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion(tagsRTRegion),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return fsxsdk.NewFromConfig(cfg, func(o *fsxsdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

// TestCreateOpsWithTags_RoundTrip drives every fsx Create* op whose real
// Input struct accepts Tags (fsx@v1.68.4: api_op_CreateFileSystem.go:235,
// CreateBackup.go:94, CreateFileCache.go:121, CreateDataRepositoryAssociation.go:102,
// CreateDataRepositoryTask.go:126, CreateSnapshot.go:74,
// CreateStorageVirtualMachine.go:75, CreateVolume.go:54) through the real SDK
// client and asserts ListTagsForResource sees what was supplied at creation
// (gopherstack-2mwl). CreateAndAttachS3AccessPoint takes no Tags in the real
// SDK and is excluded; CreateFileSystemFromBackup/CreateVolumeFromBackup tag
// the same resource kinds as CreateFileSystem/CreateVolume and aren't
// separately covered.
func TestCreateOpsWithTags_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(t *testing.T, client *fsxsdk.Client) string
		name  string
	}{
		{
			name: "file system",
			setup: func(t *testing.T, client *fsxsdk.Client) string {
				t.Helper()
				out, err := client.CreateFileSystem(t.Context(), &fsxsdk.CreateFileSystemInput{
					FileSystemType:  types.FileSystemTypeLustre,
					SubnetIds:       []string{"subnet-0123abcd"},
					StorageCapacity: aws.Int32(1200),
					LustreConfiguration: &types.CreateFileSystemLustreConfiguration{
						DeploymentType: types.LustreDeploymentTypeScratch2,
					},
					Tags: []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
				})
				require.NoError(t, err)

				return aws.ToString(out.FileSystem.ResourceARN)
			},
		},
		{
			name: "backup",
			setup: func(t *testing.T, client *fsxsdk.Client) string {
				t.Helper()
				fsOut := createTestLustreFS(t, client)

				out, err := client.CreateBackup(t.Context(), &fsxsdk.CreateBackupInput{
					FileSystemId: fsOut.FileSystem.FileSystemId,
					Tags:         []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
				})
				require.NoError(t, err)

				return aws.ToString(out.Backup.ResourceARN)
			},
		},
		{
			name: "file cache",
			setup: func(t *testing.T, client *fsxsdk.Client) string {
				t.Helper()
				out, err := client.CreateFileCache(t.Context(), &fsxsdk.CreateFileCacheInput{
					FileCacheType:        types.FileCacheTypeLustre,
					FileCacheTypeVersion: aws.String("2.12"),
					StorageCapacity:      aws.Int32(1200),
					SubnetIds:            []string{"subnet-0123abcd"},
					Tags:                 []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
					LustreConfiguration: &types.CreateFileCacheLustreConfiguration{
						DeploymentType: types.FileCacheLustreDeploymentTypeCache1,
						MetadataConfiguration: &types.FileCacheLustreMetadataConfiguration{
							StorageCapacity: aws.Int32(2400),
						},
						PerUnitStorageThroughput: aws.Int32(1000),
					},
				})
				require.NoError(t, err)

				return aws.ToString(out.FileCache.ResourceARN)
			},
		},
		{
			name: "data repository association",
			setup: func(t *testing.T, client *fsxsdk.Client) string {
				t.Helper()
				fsOut := createTestLustreFS(t, client)

				out, err := client.CreateDataRepositoryAssociation(
					t.Context(),
					&fsxsdk.CreateDataRepositoryAssociationInput{
						FileSystemId:       fsOut.FileSystem.FileSystemId,
						DataRepositoryPath: aws.String("s3://tagged-bucket"),
						FileSystemPath:     aws.String("/data"),
						Tags:               []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
					},
				)
				require.NoError(t, err)

				return aws.ToString(out.Association.ResourceARN)
			},
		},
		{
			name: "data repository task",
			setup: func(t *testing.T, client *fsxsdk.Client) string {
				t.Helper()
				fsOut := createTestLustreFS(t, client)

				out, err := client.CreateDataRepositoryTask(t.Context(), &fsxsdk.CreateDataRepositoryTaskInput{
					FileSystemId: fsOut.FileSystem.FileSystemId,
					Type:         types.DataRepositoryTaskTypeExport,
					Report: &types.CompletionReport{
						Enabled: aws.Bool(false),
					},
					Tags: []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
				})
				require.NoError(t, err)

				return aws.ToString(out.DataRepositoryTask.ResourceARN)
			},
		},
		{
			name: "snapshot",
			setup: func(t *testing.T, client *fsxsdk.Client) string {
				t.Helper()
				fsOut := createTestOntapFS(t, client)
				svmOut, err := client.CreateStorageVirtualMachine(t.Context(), &fsxsdk.CreateStorageVirtualMachineInput{
					FileSystemId: fsOut.FileSystem.FileSystemId,
					Name:         aws.String("snap-source-svm"),
				})
				require.NoError(t, err)

				volOut, err := client.CreateVolume(t.Context(), &fsxsdk.CreateVolumeInput{
					Name:       aws.String("snap-source-volume"),
					VolumeType: types.VolumeTypeOntap,
					OntapConfiguration: &types.CreateOntapVolumeConfiguration{
						JunctionPath:            aws.String("/snap-source"),
						SizeInBytes:             aws.Int64(1024 * 1024 * 1024),
						StorageVirtualMachineId: svmOut.StorageVirtualMachine.StorageVirtualMachineId,
					},
				})
				require.NoError(t, err)

				out, err := client.CreateSnapshot(t.Context(), &fsxsdk.CreateSnapshotInput{
					Name:     aws.String("tagged-snapshot"),
					VolumeId: volOut.Volume.VolumeId,
					Tags:     []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
				})
				require.NoError(t, err)

				return aws.ToString(out.Snapshot.ResourceARN)
			},
		},
		{
			name: "storage virtual machine",
			setup: func(t *testing.T, client *fsxsdk.Client) string {
				t.Helper()
				fsOut := createTestOntapFS(t, client)

				out, err := client.CreateStorageVirtualMachine(t.Context(), &fsxsdk.CreateStorageVirtualMachineInput{
					FileSystemId: fsOut.FileSystem.FileSystemId,
					Name:         aws.String("tagged-svm"),
					Tags:         []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
				})
				require.NoError(t, err)

				return aws.ToString(out.StorageVirtualMachine.ResourceARN)
			},
		},
		{
			name: "volume",
			setup: func(t *testing.T, client *fsxsdk.Client) string {
				t.Helper()
				fsOut := createTestOntapFS(t, client)
				svmOut, err := client.CreateStorageVirtualMachine(t.Context(), &fsxsdk.CreateStorageVirtualMachineInput{
					FileSystemId: fsOut.FileSystem.FileSystemId,
					Name:         aws.String("tagged-volume-svm"),
				})
				require.NoError(t, err)

				out, err := client.CreateVolume(t.Context(), &fsxsdk.CreateVolumeInput{
					Name:       aws.String("tagged-volume"),
					VolumeType: types.VolumeTypeOntap,
					OntapConfiguration: &types.CreateOntapVolumeConfiguration{
						JunctionPath:            aws.String("/tagged-vol"),
						SizeInBytes:             aws.Int64(1024 * 1024 * 1024),
						StorageVirtualMachineId: svmOut.StorageVirtualMachine.StorageVirtualMachineId,
					},
					Tags: []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
				})
				require.NoError(t, err)

				return aws.ToString(out.Volume.ResourceARN)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := fsx.NewInMemoryBackend("000000000000", tagsRTRegion)
			h := fsx.NewHandler(backend)
			client := newTestFSxClient(t, h)

			resourceARN := tt.setup(t, client)
			require.NotEmpty(t, resourceARN)

			out, err := client.ListTagsForResource(t.Context(), &fsxsdk.ListTagsForResourceInput{
				ResourceARN: aws.String(resourceARN),
			})
			require.NoError(t, err)

			require.Len(t, out.Tags, 1)
			assert.Equal(t, "env", aws.ToString(out.Tags[0].Key))
			assert.Equal(t, "prod", aws.ToString(out.Tags[0].Value))
		})
	}
}

func createTestLustreFS(t *testing.T, client *fsxsdk.Client) *fsxsdk.CreateFileSystemOutput {
	t.Helper()

	out, err := client.CreateFileSystem(t.Context(), &fsxsdk.CreateFileSystemInput{
		FileSystemType:  types.FileSystemTypeLustre,
		SubnetIds:       []string{"subnet-0123abcd"},
		StorageCapacity: aws.Int32(1200),
		LustreConfiguration: &types.CreateFileSystemLustreConfiguration{
			DeploymentType: types.LustreDeploymentTypeScratch2,
		},
	})
	require.NoError(t, err)

	return out
}

func createTestOntapFS(t *testing.T, client *fsxsdk.Client) *fsxsdk.CreateFileSystemOutput {
	t.Helper()

	out, err := client.CreateFileSystem(t.Context(), &fsxsdk.CreateFileSystemInput{
		FileSystemType:  types.FileSystemTypeOntap,
		SubnetIds:       []string{"subnet-0123abcd", "subnet-0456efab"},
		StorageCapacity: aws.Int32(1024),
		OntapConfiguration: &types.CreateFileSystemOntapConfiguration{
			DeploymentType:     types.OntapDeploymentTypeMultiAz1,
			PreferredSubnetId:  aws.String("subnet-0123abcd"),
			ThroughputCapacity: aws.Int32(128),
		},
	})
	require.NoError(t, err)

	return out
}

// createTestOntapVolume creates a fresh ONTAP file system, a storage virtual
// machine on it, and an ONTAP volume anchored at that SVM -- real
// CreateVolumeInput has no top-level FileSystemId/StorageVirtualMachineId at
// all (fsx@v1.68.4 api_op_CreateVolume.go); the SVM is the only real anchor
// for an ONTAP volume's parent file system.
func createTestOntapVolume(t *testing.T, client *fsxsdk.Client, name string) *fsxsdk.CreateVolumeOutput {
	t.Helper()

	fsOut := createTestOntapFS(t, client)

	svmOut, err := client.CreateStorageVirtualMachine(t.Context(), &fsxsdk.CreateStorageVirtualMachineInput{
		FileSystemId: fsOut.FileSystem.FileSystemId,
		Name:         aws.String(name + "-svm"),
	})
	require.NoError(t, err)

	volOut, err := client.CreateVolume(t.Context(), &fsxsdk.CreateVolumeInput{
		VolumeType: types.VolumeTypeOntap,
		Name:       aws.String(name),
		OntapConfiguration: &types.CreateOntapVolumeConfiguration{
			StorageVirtualMachineId: svmOut.StorageVirtualMachine.StorageVirtualMachineId,
		},
	})
	require.NoError(t, err)

	return volOut
}
