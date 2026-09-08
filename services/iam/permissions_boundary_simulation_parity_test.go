package iam_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iam"
)

// gopherstack-uywm: SimulatePrincipalPolicy must apply a permissions boundary
// the same way the live enforcement path (middleware.go's
// applyPermissionsBoundary, gopherstack-7gnj) does — to the identity-policy
// result only, before combining with any resource-based policy. Per the AWS
// IAM User Guide ("Permissions boundaries for IAM entities"): "resource-based
// policies that grant permissions to an IAM user ARN ... are not limited by
// an implicit deny in an identity-based policy or permissions boundary."

func TestSimulatePrincipalPolicy_ResourcePolicyAllowSurvivesBoundaryImplicitDeny(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	// Boundary covers only ec2:* — an implicit (non-explicit) deny for s3 actions.
	boundaryPol, err := b.CreatePolicy("EC2OnlyBoundary", "/",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ec2:*","Resource":"*"}]}`)
	require.NoError(t, err)

	_, err = b.CreateUser("resource-survivor", "/", boundaryPol.Arn)
	require.NoError(t, err)
	// No identity policy attached: the identity-policy result is implicit deny.

	bucketPolicy := `{"Version":"2012-10-17","Statement":[{
		"Effect":"Allow","Principal":"*","Action":"s3:GetObject",
		"Resource":"arn:aws:s3:::survivor-bucket/*"
	}]}`

	results, err := b.SimulatePrincipalPolicy(
		"arn:aws:iam::000000000000:user/resource-survivor", "", "",
		[]string{bucketPolicy},
		[]string{"s3:GetObject"},
		[]string{"arn:aws:s3:::survivor-bucket/obj"},
		iam.ConditionContext{},
	)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "allowed", results[0].Decision,
		"a resource-policy Allow must survive an implicit (non-covering) boundary deny")
}

func TestSimulatePrincipalPolicy_ExplicitBoundaryDenyBeatsResourcePolicyAllow(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	boundaryPol, err := b.CreatePolicy("DenyAllBoundary", "/",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"*","Resource":"*"}]}`)
	require.NoError(t, err)

	_, err = b.CreateUser("explicit-boundary-deny", "/", boundaryPol.Arn)
	require.NoError(t, err)

	bucketPolicy := `{"Version":"2012-10-17","Statement":[{
		"Effect":"Allow","Principal":"*","Action":"s3:GetObject",
		"Resource":"arn:aws:s3:::deny-bucket/*"
	}]}`

	results, err := b.SimulatePrincipalPolicy(
		"arn:aws:iam::000000000000:user/explicit-boundary-deny", "", "",
		[]string{bucketPolicy},
		[]string{"s3:GetObject"},
		[]string{"arn:aws:s3:::deny-bucket/obj"},
		iam.ConditionContext{},
	)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "explicitDeny", results[0].Decision,
		"an explicit boundary Deny beats a resource-policy Allow, like any other explicit deny")
}

func TestSimulatePrincipalPolicy_BoundaryCoveringActionStillAllowsWithResourcePolicy(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	boundaryPol, err := b.CreatePolicy("S3CoveredBoundary", "/",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`)
	require.NoError(t, err)

	_, err = b.CreateUser("boundary-covers", "/", boundaryPol.Arn)
	require.NoError(t, err)

	bucketPolicy := `{"Version":"2012-10-17","Statement":[{
		"Effect":"Allow","Principal":"*","Action":"s3:GetObject",
		"Resource":"arn:aws:s3:::covered-bucket/*"
	}]}`

	results, err := b.SimulatePrincipalPolicy(
		"arn:aws:iam::000000000000:user/boundary-covers", "", "",
		[]string{bucketPolicy},
		[]string{"s3:GetObject"},
		[]string{"arn:aws:s3:::covered-bucket/obj"},
		iam.ConditionContext{},
	)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "allowed", results[0].Decision)
}

// TestPermissionsBoundary_SimulateAndEnforceAgree drives the identical
// scenario through SimulatePrincipalPolicy and through the live
// EnforcementMiddleware on the same InMemoryBackend, and asserts they reach
// the same decision. That agreement is the property gopherstack-uywm is
// about: a simulation is supposed to predict what enforcement does.
func TestPermissionsBoundary_SimulateAndEnforceAgree(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	boundaryPol, err := b.CreatePolicy("EC2OnlyBoundaryParity", "/",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ec2:*","Resource":"*"}]}`)
	require.NoError(t, err)

	_, err = b.CreateUser("parity-user", "/", boundaryPol.Arn)
	require.NoError(t, err)

	ak, err := b.CreateAccessKey("parity-user")
	require.NoError(t, err)

	bucketPolicy := `{"Version":"2012-10-17","Statement":[{
		"Effect":"Allow","Principal":"*","Action":"s3:GetObject",
		"Resource":"arn:aws:s3:::parity-bucket/*"
	}]}`

	results, err := b.SimulatePrincipalPolicy(
		"arn:aws:iam::000000000000:user/parity-user", "", "",
		[]string{bucketPolicy},
		[]string{"s3:GetObject"},
		[]string{"arn:aws:s3:::parity-bucket/obj"},
		iam.ConditionContext{},
	)
	require.NoError(t, err)
	require.Len(t, results, 1)
	simulateAllowed := results[0].Decision == "allowed"

	provider := &mockResourceProvider{
		policies: map[string]string{
			"arn:aws:s3:::parity-bucket/obj": bucketPolicy,
		},
	}

	e := echo.New()
	e.Use(iam.EnforcementMiddleware(b, iam.EnforcementConfig{
		ResourceProviders: []iam.ResourcePolicyProvider{provider},
	}))
	e.Any("/*", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/parity-bucket/obj", strings.NewReader(""))
	req.Header.Set("Authorization",
		"AWS4-HMAC-SHA256 Credential="+ak.AccessKeyID+"/20230101/us-east-1/s3/aws4_request")

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	enforceAllowed := rec.Code == http.StatusOK

	assert.Equal(t, simulateAllowed, enforceAllowed,
		"SimulatePrincipalPolicy and the live enforcement middleware must agree on the same scenario")
	assert.True(t, simulateAllowed,
		"resource-policy Allow should survive the non-covering boundary in both paths")
}
