package ec2_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ec2"
)

// TestAssociateIamInstanceProfile_RejectsSecondAssociation covers
// gopherstack-847g: real AssociateIamInstanceProfile "cannot associate more
// than one IAM instance profile with an instance" (ec2@v1.319.1
// api_op_AssociateIamInstanceProfile.go doc comment).
func TestAssociateIamInstanceProfile_RejectsSecondAssociation(t *testing.T) {
	t.Parallel()

	bk := newTestBackend()
	insts, err := bk.RunInstances("ami-123", "t2.micro", "", 2)
	require.NoError(t, err)
	require.Len(t, insts, 2)

	first, err := bk.AssociateIamInstanceProfile(insts[0].ID, "arn:aws:iam::000000000000:instance-profile/First")
	require.NoError(t, err)
	assert.NotEmpty(t, first.AssociationID)

	_, err = bk.AssociateIamInstanceProfile(insts[0].ID, "arn:aws:iam::000000000000:instance-profile/Second")
	require.Error(t, err)
	require.ErrorIs(t, err, ec2.ErrIAMInstanceProfileAlreadyAssociated)

	// Negative: a valid single association on another instance still succeeds.
	second, err := bk.AssociateIamInstanceProfile(insts[1].ID, "arn:aws:iam::000000000000:instance-profile/Other")
	require.NoError(t, err)
	assert.NotEmpty(t, second.AssociationID)

	// Once disassociated, a fresh association on the same instance succeeds again.
	_, err = bk.DisassociateIamInstanceProfile(first.AssociationID)
	require.NoError(t, err)

	third, err := bk.AssociateIamInstanceProfile(insts[0].ID, "arn:aws:iam::000000000000:instance-profile/Third")
	require.NoError(t, err)
	assert.NotEmpty(t, third.AssociationID)
}

// TestTerminateInstances_DisassociatesIamInstanceProfile covers
// gopherstack-hmfm: terminating an instance must not leave a ghost
// "associated" row behind in DescribeIamInstanceProfileAssociations. Also
// exercises 847g/hmfm interaction: the association slot an instance held
// must free up on termination, and disassociating the now-gone association
// afterward must fail with a real EC2 error, not a 500.
func TestTerminateInstances_DisassociatesIamInstanceProfile(t *testing.T) {
	t.Parallel()

	bk := newTestBackend()
	insts, err := bk.RunInstances("ami-123", "t2.micro", "", 2)
	require.NoError(t, err)
	require.Len(t, insts, 2)
	terminated, survivor := insts[0].ID, insts[1].ID

	assoc1, err := bk.AssociateIamInstanceProfile(terminated, "arn:aws:iam::000000000000:instance-profile/A")
	require.NoError(t, err)
	assoc2, err := bk.AssociateIamInstanceProfile(survivor, "arn:aws:iam::000000000000:instance-profile/B")
	require.NoError(t, err)

	_, err = bk.TerminateInstances([]string{terminated})
	require.NoError(t, err)

	// Ghost row must be gone: no longer observable via Describe.
	assert.Empty(t, bk.DescribeIamInstanceProfileAssociations(nil, terminated))

	// Terminating one instance must not disturb another's association.
	survivorAssocs := bk.DescribeIamInstanceProfileAssociations(nil, survivor)
	require.Len(t, survivorAssocs, 1)
	assert.Equal(t, assoc2.AssociationID, survivorAssocs[0].AssociationID)

	// Disassociating the now-gone association behaves sensibly (real EC2
	// error, not an internal failure).
	_, err = bk.DisassociateIamInstanceProfile(assoc1.AssociationID)
	require.Error(t, err)
	require.ErrorIs(t, err, ec2.ErrIAMAssociationNotFound)
}
