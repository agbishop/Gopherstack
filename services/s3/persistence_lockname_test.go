package s3_test

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/s3"
)

// histSampleCount returns the cumulative sample count for the metric family
// with the given lock/operation/type label tuple. type may be "" to skip
// matching that label (write-lock families don't have one).
func histSampleCount(t *testing.T, family, lock, operation, typ string) uint64 {
	t.Helper()

	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() != family {
			continue
		}

		for _, m := range mf.GetMetric() {
			if !matchesLabels(m, lock, operation, typ) {
				continue
			}

			return m.GetHistogram().GetSampleCount()
		}
	}

	return 0
}

func matchesLabels(m *dto.Metric, lock, operation, typ string) bool {
	labels := make(map[string]string, len(m.GetLabel()))
	for _, lp := range m.GetLabel() {
		labels[lp.GetName()] = lp.GetValue()
	}

	if labels["lock"] != lock || labels["operation"] != operation {
		return false
	}

	return typ == "" || labels["type"] == typ
}

// TestInMemoryBackend_RestoredLockNamesMatchFreshlyCreated is the regression
// test for gopherstack-lf8p's real defect: reinitSingleBucket
// (services/s3/persistence.go) reinitialises restored bucket/object mutexes
// with the hyphenated names "s3-bucket"/"s3-object", while every live
// creation path (buckets.go, objects.go, multipart.go) uses "s3.bucket.<name>"
// and "s3.object". A bucket or object rehydrated from a snapshot therefore
// reports lock metrics under a different Prometheus series than one created
// fresh in the same process, and restored buckets additionally lose the
// per-bucket name resolution buckets.go deliberately provides.
//
//nolint:paralleltest // reads the global gatherer; contributing tests are all t.Parallel too.
func TestInMemoryBackend_RestoredLockNamesMatchFreshlyCreated(t *testing.T) {
	b := newTestBackend(t)
	ctx := t.Context()

	const bucketName = "restore-lockname-bucket"
	const key = "restore-lockname-key"

	_, err := b.CreateBucket(ctx, &sdk_s3.CreateBucketInput{Bucket: aws.String(bucketName)})
	require.NoError(t, err)

	_, err = b.PutObject(ctx, &sdk_s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
		Body:   strings.NewReader("hello"),
	})
	require.NoError(t, err)

	snap := b.Snapshot(ctx)
	require.NotNil(t, snap)

	restored := newTestBackend(t)
	require.NoError(t, restored.Restore(ctx, snap))

	// Baselines, taken before touching the restored resources, so the
	// pre/post delta below is attributable to this test's own call.
	bucketLockName := "s3.bucket." + bucketName

	beforeObj := histSampleCount(t, "gopherstack_lock_hold_seconds", "s3.object", "BackdateObjectForTest", "")
	beforeBucket := histSampleCount(t, "gopherstack_lock_wait_seconds", bucketLockName, "PeekStoredBytes", "read")

	s3.BackdateObjectForTest(restored, bucketName, key, time.Now())
	s3.PeekStoredBytes(restored, bucketName, key)

	afterObj := histSampleCount(t, "gopherstack_lock_hold_seconds", "s3.object", "BackdateObjectForTest", "")
	afterBucket := histSampleCount(t, "gopherstack_lock_wait_seconds", bucketLockName, "PeekStoredBytes", "read")

	assert.Equal(t, beforeObj+1, afterObj,
		"restored object's lock must emit under lock=%q, not a drifted name", "s3.object")
	assert.Equal(t, beforeBucket+1, afterBucket,
		"restored bucket's lock must emit under lock=%q, not a drifted name", bucketLockName)
}
