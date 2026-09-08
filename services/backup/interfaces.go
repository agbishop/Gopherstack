package backup

import (
	"context"

	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Backend is the subset of S3 operations Backup needs to check whether a
// StartBackupJob ResourceArn naming an S3 bucket resolves to a bucket that
// actually exists, wired via SetS3Backend. When unset, ResourceArn is
// validated for non-emptiness only, matching this repo's
// unwired-hook-stays-permissive convention.
type S3Backend interface {
	HeadBucket(ctx context.Context, input *sdk_s3.HeadBucketInput) (*sdk_s3.HeadBucketOutput, error)
}
