package backup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/backup"
)

// fakeBackupS3 is a minimal backup.S3Backend test double: HeadBucket errors
// for any bucket not in the buckets set. Standing in for a wired S3 backend
// without depending on services/s3.
type fakeBackupS3 struct {
	buckets map[string]bool
}

var errFakeBackupNoSuchBucket = errors.New("NoSuchBucket")

func (f *fakeBackupS3) HeadBucket(
	_ context.Context,
	input *sdk_s3.HeadBucketInput,
) (*sdk_s3.HeadBucketOutput, error) {
	if f.buckets[aws.ToString(input.Bucket)] {
		return &sdk_s3.HeadBucketOutput{}, nil
	}

	return nil, errFakeBackupNoSuchBucket
}

// TestStartBackupJob_S3ResourceValidation proves that once S3 is wired via
// SetS3Backend, StartBackupJob rejects an S3 ResourceArn naming a bucket
// that doesn't exist (ResourceNotFoundException) and allows one that does.
// Fails against the pre-fix code: StartBackupJob only checked
// ResourceArn != "" and never checked whether the named resource existed.
func TestStartBackupJob_S3ResourceValidation(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	mustVault(t, b, "vault-s3")
	b.SetS3Backend(&fakeBackupS3{buckets: map[string]bool{"good-bucket": true}})

	tests := []struct {
		name    string
		arn     string
		wantErr bool
	}{
		{name: "missing bucket", arn: "arn:aws:s3:::missing-bucket", wantErr: true},
		{name: "existing bucket", arn: "arn:aws:s3:::good-bucket", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := b.StartBackupJob("vault-s3", tt.arn, "arn:aws:iam::123456789012:role/r", "S3")
			if tt.wantErr {
				require.ErrorIs(t, err, backup.ErrNotFound)

				return
			}
			require.NoError(t, err)
		})
	}
}

// TestStartBackupJob_UnclassifiableARNPermissive proves ResourceArns of a
// type this emulator does not classify -- including an S3 object-key ARN,
// which is not the bucket-level ARN Backup's S3 resource type uses -- are
// accepted even with S3 wired. Real AWS Backup supports resource types
// gopherstack does not model; rejecting them would be a fabricated
// rejection.
func TestStartBackupJob_UnclassifiableARNPermissive(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	mustVault(t, b, "vault-permissive")
	b.SetS3Backend(&fakeBackupS3{})

	tests := []struct {
		name string
		arn  string
	}{
		{name: "ec2 instance", arn: "arn:aws:ec2:us-east-1:123456789012:instance/i-1"},
		{name: "dynamodb table", arn: "arn:aws:dynamodb:us-east-1:123456789012:table/t1"},
		{name: "s3 object key", arn: "arn:aws:s3:::bucket/key.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := b.StartBackupJob("vault-permissive", tt.arn, "arn:aws:iam::123456789012:role/r", "")
			require.NoError(t, err)
		})
	}
}

// TestStartBackupJob_UnwiredS3Permissive proves that with no SetS3Backend
// call, an S3 ResourceArn naming a nonexistent bucket is still accepted --
// an unwired cross-service hook must be a silent no-op, never a rejection.
func TestStartBackupJob_UnwiredS3Permissive(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	mustVault(t, b, "vault-unwired")

	_, err := b.StartBackupJob(
		"vault-unwired",
		"arn:aws:s3:::does-not-exist",
		"arn:aws:iam::123456789012:role/r",
		"S3",
	)
	require.NoError(t, err)
}
