package ec2_test

// ec2sweep44 (gopherstack-1a5): RunInstancesInput declares IamInstanceProfile
// (serializers.go:91938, awsEc2query_serializeDocumentIamInstanceProfileSpecification
// -- Arn/Name under the IamInstanceProfile object key) but handleRunInstances
// never read it at all, so launching an instance with an instance profile
// silently dropped it: no IamInstanceProfileAssociation was created, and
// DescribeIamInstanceProfileAssociations came back empty for that instance --
// a real client relying on RunInstances to attach a role (the common launch
// path, distinct from the separate AssociateIamInstanceProfile call) got no
// role at all with no error. Also, types.Instance.IamInstanceProfile
// (deserializers.go:110585, element "iamInstanceProfile" with sub-elements
// "arn"/"id") was never rendered on the instance item at all, on either
// RunInstances or DescribeInstances -- even an instance profile attached via
// the pre-existing AssociateIamInstanceProfile call never showed up on the
// instance's own Describe output, only via the separate
// DescribeIamInstanceProfileAssociations call.
//
// Fixed: handleRunInstances now reads IamInstanceProfile.Arn/.Name (falling
// back to .Name, matching the existing AssociateIamInstanceProfile/
// ReplaceIamInstanceProfileAssociation handlers' convention) and associates
// it with each launched instance via the existing
// Backend.AssociateIamInstanceProfile; both RunInstances and DescribeInstances
// now render the instance's active association as iamInstanceProfile.

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunInstances_IamInstanceProfile_RealClient(t *testing.T) {
	t.Parallel()

	_, client := newTestBackendAndClient(t)

	const profileARN = "arn:aws:iam::123456789012:instance-profile/ec2sweep44-role"

	runOut, err := client.RunInstances(t.Context(), &ec2sdk.RunInstancesInput{
		ImageId:      aws.String("ami-ec2sweep44"),
		InstanceType: types.InstanceTypeT3Micro,
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		IamInstanceProfile: &types.IamInstanceProfileSpecification{
			Arn: aws.String(profileARN),
		},
	})
	require.NoError(t, err)
	require.Len(t, runOut.Instances, 1)

	instanceID := aws.ToString(runOut.Instances[0].InstanceId)

	require.NotNil(t, runOut.Instances[0].IamInstanceProfile,
		"RunInstances response never rendered the launch-time instance profile")
	assert.Equal(t, profileARN, aws.ToString(runOut.Instances[0].IamInstanceProfile.Arn))

	descOut, err := client.DescribeInstances(t.Context(), &ec2sdk.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	require.NoError(t, err)
	require.Len(t, descOut.Reservations, 1)
	require.Len(t, descOut.Reservations[0].Instances, 1)

	got := descOut.Reservations[0].Instances[0]
	require.NotNil(t, got.IamInstanceProfile,
		"DescribeInstances never rendered the instance's associated IAM instance profile")
	assert.Equal(t, profileARN, aws.ToString(got.IamInstanceProfile.Arn))

	assocOut, err := client.DescribeIamInstanceProfileAssociations(
		t.Context(), &ec2sdk.DescribeIamInstanceProfileAssociationsInput{},
	)
	require.NoError(t, err)
	require.Len(t, assocOut.IamInstanceProfileAssociations, 1,
		"RunInstances with IamInstanceProfile must create a real association")
	assert.Equal(t, instanceID, aws.ToString(assocOut.IamInstanceProfileAssociations[0].InstanceId))
	assert.Equal(
		t,
		types.IamInstanceProfileAssociationStateAssociated,
		assocOut.IamInstanceProfileAssociations[0].State,
	)
}
