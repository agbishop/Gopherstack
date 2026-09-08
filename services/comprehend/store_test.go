package comprehend_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/comprehend"
)

// TestInMemoryBackend_Reset_ResourcePolicyTimestamps verifies that Reset()
// clears policyCreatedAt/policyModifiedAt alongside the policies/
// policyRevisions maps they're tracked next to. Neither map is observable
// through GetResourcePolicy once its backing policy entry is deleted (it
// errors before reaching them), and PutResourcePolicy always overwrites a
// stale entry for the same ARN on next use -- so a leaked entry under a
// never-reused ARN is a pure memory leak, checked directly via the map size.
func TestInMemoryBackend_Reset_ResourcePolicyTimestamps(t *testing.T) {
	t.Parallel()

	b := comprehend.NewInMemoryBackend("123456789012", "us-east-1")

	const resourceArn = "arn:aws:comprehend:us-east-1:123456789012:document-classifier/test"

	_, err := b.PutResourcePolicy(resourceArn, `{"Version":"2012-10-17"}`, "")
	require.NoError(t, err)

	createdBefore, modifiedBefore := b.PolicyTimestampCountsForTest()
	require.Equal(t, 1, createdBefore, "sanity: PutResourcePolicy should record a created timestamp")
	require.Equal(t, 1, modifiedBefore, "sanity: PutResourcePolicy should record a modified timestamp")

	b.Reset()

	createdAfter, modifiedAfter := b.PolicyTimestampCountsForTest()
	assert.Equal(t, 0, createdAfter, "policyCreatedAt must be cleared by Reset")
	assert.Equal(t, 0, modifiedAfter, "policyModifiedAt must be cleared by Reset")
}

// TestInMemoryBackend_StartJob_TagsSynced verifies StartJob registers the
// job's ARN in the same tag store CreateResource uses, and that tags passed
// at start time are stored against it. Start*DetectionJob requests all
// accept an optional Tags field in the real API; before this fix StartJob's
// signature had no tags parameter at all, so any Tags on the request were
// silently discarded and the job's ARN was never added to the tag store --
// TagResource/ListTagsForResource/UntagResource all failed with
// ResourceNotFoundException against an existing job's ARN.
func TestInMemoryBackend_StartJob_TagsSynced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []comprehend.Tag
	}{
		{name: "with_tags", tags: []comprehend.Tag{{Key: "team", Value: "nlp"}}},
		{name: "no_tags", tags: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			b := comprehend.NewInMemoryBackend("123456789012", "us-east-1")
			job, err := b.StartJob(
				"entities-detection-job", "job-"+test.name, map[string]any{"LanguageCode": "en"}, test.tags,
			)
			require.NoError(t, err)

			tags, err := b.ListTags(job.JobArn)
			require.NoError(t, err, "job ARN must be registered in the tag store even with zero tags")
			assert.Len(t, tags, len(test.tags))

			require.NoError(t, b.TagResource(job.JobArn, []comprehend.Tag{{Key: "env", Value: "prod"}}))
			tags, err = b.ListTags(job.JobArn)
			require.NoError(t, err)
			assert.Len(t, tags, len(test.tags)+1)
		})
	}
}
