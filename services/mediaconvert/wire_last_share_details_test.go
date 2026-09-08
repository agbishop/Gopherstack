package mediaconvert_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	mediaconvertsdk "github.com/aws/aws-sdk-go-v2/service/mediaconvert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/mediaconvert"
)

// TestGetJob_LastShareDetailsDecodesAsString drives GetJob through the real
// aws-sdk-go-v2 mediaconvert client. The real types.Job.LastShareDetails is
// a bare *string -- the deserializer's case "lastShareDetails" type-
// switches on value.(string), so gopherstack's previous
// {shareToken, sharedAt} object failed every real client's decode once a
// job had been shared.
func TestGetJob_LastShareDetailsDecodesAsString(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	h := mediaconvert.NewHandler(b)
	client := newTestMediaConvertClient(t, h)

	j, err := b.CreateJob("arn:aws:iam::123456789012:role/role", "", "", nil, nil, nil, "")
	require.NoError(t, err)

	_, err = b.CreateResourceShare(j.ID, "case-1234")
	require.NoError(t, err)

	out, err := client.GetJob(t.Context(), &mediaconvertsdk.GetJobInput{Id: aws.String(j.ID)})
	require.NoError(t, err, "real SDK client must decode GetJob without error")
	require.NotNil(t, out.Job)
	assert.NotEmpty(t, aws.ToString(out.Job.LastShareDetails))
}
