package cloudtrail

import (
	"context"

	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Backend is the subset of S3 operations CloudTrail needs to validate a
// trail's configured bucket (HeadBucket, used by CreateTrail/UpdateTrail)
// and to deliver recorded management events as log files to it (PutObject),
// wired via SetS3Backend. When unset, CreateTrail/UpdateTrail skip bucket
// validation and no log files are ever written -- S3BucketName is only
// stored/echoed, matching this repo's unwired-hook-stays-permissive
// convention.
type S3Backend interface {
	HeadBucket(ctx context.Context, input *sdk_s3.HeadBucketInput) (*sdk_s3.HeadBucketOutput, error)
	PutObject(ctx context.Context, input *sdk_s3.PutObjectInput) (*sdk_s3.PutObjectOutput, error)
}
