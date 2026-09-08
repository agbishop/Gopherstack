package iam_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iam"
)

// gopherstack-q97k: SimulateCustomPolicy's enforcePermissionsBoundary downgraded
// an explicit boundary Deny to implicit deny, because it only checked
// `!allowed` (true for both explicit and implicit boundary denials) rather
// than distinguishing the two. Per the AWS IAM User Guide, "An explicit deny
// in any policy type results in a request being denied" and callers treat an
// explicit deny as unrecoverable, unlike an implicit one.

func TestSimulateCustomPolicy_ExplicitBoundaryDenyReportsExplicitDeny(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	boundaryDenyAll := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"*","Resource":"*"}]}`

	results, err := b.SimulateCustomPolicy(
		[]string{identityAllow}, []string{boundaryDenyAll},
		[]string{"s3:GetObject"},
		[]string{"arn:aws:s3:::my-bucket/key"},
		iam.ConditionContext{},
	)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "explicitDeny", results[0].Decision,
		"an explicit Deny in the permissions boundary must report as explicitDeny, not implicitDeny")
}

func TestSimulateCustomPolicy_ImplicitBoundaryDenyReportsImplicitDeny(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	// Boundary only covers ec2:* — a non-covering (implicit) deny for s3 actions.
	boundaryEC2Only := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ec2:*","Resource":"*"}]}`

	results, err := b.SimulateCustomPolicy(
		[]string{identityAllow}, []string{boundaryEC2Only},
		[]string{"s3:GetObject"},
		[]string{"arn:aws:s3:::my-bucket/key"},
		iam.ConditionContext{},
	)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "implicitDeny", results[0].Decision,
		"a non-covering (implicit) boundary deny must still report as implicitDeny")
}

func TestSimulateCustomPolicy_AllowWithCoveringBoundaryStillAllows(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	boundaryS3Covers := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`

	results, err := b.SimulateCustomPolicy(
		[]string{identityAllow}, []string{boundaryS3Covers},
		[]string{"s3:GetObject"},
		[]string{"arn:aws:s3:::my-bucket/key"},
		iam.ConditionContext{},
	)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "allowed", results[0].Decision,
		"an identity Allow covered by the boundary must still allow")
}

func TestSimulateCustomPolicy_NoBoundaryUnchanged(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`

	results, err := b.SimulateCustomPolicy(
		[]string{identityAllow}, nil,
		[]string{"s3:GetObject"},
		[]string{"arn:aws:s3:::my-bucket/key"},
		iam.ConditionContext{},
	)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "allowed", results[0].Decision,
		"no boundary supplied must leave the identity-policy decision unchanged")
}
