package fsx_test

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	fsxsdk "github.com/aws/aws-sdk-go-v2/service/fsx"
	"github.com/aws/aws-sdk-go-v2/service/fsx/types"
	"github.com/stretchr/testify/require"
)

// manyTags returns n Tag values, each with a unique key (a repeated key
// would fail Create*'s own per-tag validation rather than the length check
// under test).
func manyTags(n int) []types.Tag {
	tags := make([]types.Tag, n)
	for i := range tags {
		tags[i] = types.Tag{Key: aws.String(fmt.Sprintf("k%d", i)), Value: aws.String("v")}
	}

	return tags
}

// TestCreateBackup_TagLimitExceeded: CreateBackupInput.Tags reuses the
// shared "Tags" list shape (fsx@v1.68.4 botocore service-2.json.gz),
// documented "A list of Tag values, with a maximum of 50 elements" (max:
// 50). The real SDK's own client-side validator
// (validateOpCreateBackupInput -> validateTags, validators.go) only checks
// each tag's own key/value constraints, never the list length -- so a real
// client CAN put 51 tags on the wire, and gopherstack must reject it
// itself. ServiceLimitExceeded is CreateBackup's own declared error for
// this (deserializers.go awsAwsjson11_deserializeOpErrorCreateBackup's
// switch; matches ErrTagLimitExceeded already used for TagResource's
// separate cumulative-tag-count check).
func TestCreateBackup_TagLimitExceeded(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestFSxClient(t, h)
	ctx := t.Context()

	fsID := createFS(t, h, "LUSTRE")

	_, err := client.CreateBackup(ctx, &fsxsdk.CreateBackupInput{
		FileSystemId: aws.String(fsID),
		Tags:         manyTags(51),
	})
	require.Error(t, err)

	var limitErr *types.ServiceLimitExceeded
	require.ErrorAs(t, err, &limitErr,
		"expected *types.ServiceLimitExceeded (CreateBackup's own declared type for the 50-tag max), got: %v", err)
}

// TestCopyBackup_TagLimitExceeded: same shared "Tags" shape and the same
// gap on CopyBackupInput.Tags; CopyBackup also legitimately declares
// ServiceLimitExceeded (deserializers.go
// awsAwsjson11_deserializeOpErrorCopyBackup).
func TestCopyBackup_TagLimitExceeded(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestFSxClient(t, h)
	ctx := t.Context()

	bkID := createFSandBackup(t, h, "LUSTRE")

	_, err := client.CopyBackup(ctx, &fsxsdk.CopyBackupInput{
		SourceBackupId: aws.String(bkID),
		Tags:           manyTags(51),
	})
	require.Error(t, err)

	var limitErr *types.ServiceLimitExceeded
	require.ErrorAs(t, err, &limitErr,
		"expected *types.ServiceLimitExceeded, got: %v", err)
}
