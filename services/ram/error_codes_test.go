package ram_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ramsdk "github.com/aws/aws-sdk-go-v2/service/ram"
	ramtypes "github.com/aws/aws-sdk-go-v2/service/ram/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ram"
)

// TestCreatePermission_AlreadyExists drives a real ram client's
// CreatePermission twice with the same name. RAM's own CreatePermission
// error model (ram@v1.39.4 deserializers.go
// awsRestjson1_deserializeOpErrorCreatePermission) defines
// PermissionAlreadyExistsException for this; the handler previously emitted
// the shared "ResourceShareAlreadyExistsException" (real only for
// CreateResourceShare, which models no AlreadyExists error at all), which
// names no type CreatePermission's client can match via errors.As.
func TestCreatePermission_AlreadyExists(t *testing.T) {
	t.Parallel()

	client := newRoundTripClient(t, ram.NewHandler(ram.NewInMemoryBackend("000000000000", "us-east-1")))

	in := &ramsdk.CreatePermissionInput{
		Name:           aws.String("dup-permission"),
		ResourceType:   aws.String("ec2:Subnet"),
		PolicyTemplate: aws.String(`{"Effect":"Allow","Action":["ec2:DescribeSubnets"]}`),
	}

	_, err := client.CreatePermission(t.Context(), in)
	require.NoError(t, err)

	_, err = client.CreatePermission(t.Context(), in)
	require.Error(t, err)

	var apiErr *ramtypes.PermissionAlreadyExistsException
	require.ErrorAs(t, err, &apiErr, "expected a real PermissionAlreadyExistsException from the SDK deserializer")
}

// TestDeletePermission_InUse drives a real ram client's DeletePermission on
// a permission still associated with a resource share. DeletePermission's
// own error model has no "PermissionInUseException" -- it isn't a type RAM
// defines anywhere -- but does define OperationNotPermittedException, which
// its own doc comment (api_op_DeletePermission.go: "You can delete a
// customer managed permission only if it isn't attached to any resource
// share") and the exception's own doc ("the requested operation isn't
// permitted") both match.
func TestDeletePermission_InUse(t *testing.T) {
	t.Parallel()

	client := newRoundTripClient(t, ram.NewHandler(ram.NewInMemoryBackend("000000000000", "us-east-1")))

	share, err := client.CreateResourceShare(t.Context(), &ramsdk.CreateResourceShareInput{
		Name: aws.String("perm-in-use-share"),
	})
	require.NoError(t, err)

	perm, err := client.CreatePermission(t.Context(), &ramsdk.CreatePermissionInput{
		Name:           aws.String("in-use-permission"),
		ResourceType:   aws.String("ec2:Subnet"),
		PolicyTemplate: aws.String(`{"Effect":"Allow","Action":["ec2:DescribeSubnets"]}`),
	})
	require.NoError(t, err)

	_, err = client.AssociateResourceSharePermission(t.Context(), &ramsdk.AssociateResourceSharePermissionInput{
		ResourceShareArn: share.ResourceShare.ResourceShareArn,
		PermissionArn:    perm.Permission.Arn,
	})
	require.NoError(t, err)

	_, err = client.DeletePermission(t.Context(), &ramsdk.DeletePermissionInput{
		PermissionArn: perm.Permission.Arn,
	})
	require.Error(t, err)

	var apiErr *ramtypes.OperationNotPermittedException
	require.ErrorAs(t, err, &apiErr, "expected a real OperationNotPermittedException from the SDK deserializer")
}

// TestAssociateResourceShare_ExternalPrincipalNotAllowed drives a real ram
// client's AssociateResourceShare against a share created with
// AllowExternalPrincipals=false. AssociateResourceShare's own error model
// has no "MalformedQueryStringException" (an EC2-query-style code that
// names no type in this REST-JSON service at all) but does define
// InvalidParameterException.
func TestAssociateResourceShare_ExternalPrincipalNotAllowed(t *testing.T) {
	t.Parallel()

	client := newRoundTripClient(t, ram.NewHandler(ram.NewInMemoryBackend("000000000000", "us-east-1")))

	share, err := client.CreateResourceShare(t.Context(), &ramsdk.CreateResourceShareInput{
		Name:                    aws.String("no-external-share"),
		AllowExternalPrincipals: aws.Bool(false),
	})
	require.NoError(t, err)

	_, err = client.AssociateResourceShare(t.Context(), &ramsdk.AssociateResourceShareInput{
		ResourceShareArn: share.ResourceShare.ResourceShareArn,
		Principals:       []string{"999999999999"},
	})
	require.Error(t, err)

	var apiErr *ramtypes.InvalidParameterException
	require.ErrorAs(t, err, &apiErr, "expected a real InvalidParameterException from the SDK deserializer")
}

// TestAssociateResourceShare_MalformedResourceArn drives a real ram client's
// AssociateResourceShare with a resourceArns entry that isn't ARN-shaped.
// AssociateResourceShare's own error model (ram@v1.39.4 deserializers.go
// awsRestjson1_deserializeOpErrorAssociateResourceShare) defines
// MalformedArnException for exactly this; gopherstack previously accepted
// any string as a resource ARN and associated it unconditionally.
func TestAssociateResourceShare_MalformedResourceArn(t *testing.T) {
	t.Parallel()

	client := newRoundTripClient(t, ram.NewHandler(ram.NewInMemoryBackend("000000000000", "us-east-1")))

	share, err := client.CreateResourceShare(t.Context(), &ramsdk.CreateResourceShareInput{
		Name: aws.String("malformed-arn-associate-share"),
	})
	require.NoError(t, err)

	_, err = client.AssociateResourceShare(t.Context(), &ramsdk.AssociateResourceShareInput{
		ResourceShareArn: share.ResourceShare.ResourceShareArn,
		ResourceArns:     []string{"not-an-arn"},
	})
	require.Error(t, err)

	var apiErr *ramtypes.MalformedArnException
	require.ErrorAs(t, err, &apiErr, "expected a real MalformedArnException from the SDK deserializer")
}

// TestCreateResourceShare_MalformedResourceArn drives a real ram client's
// CreateResourceShare with a resourceArns entry that isn't ARN-shaped.
// CreateResourceShare's own error model (ram@v1.39.4 deserializers.go
// awsRestjson1_deserializeOpErrorCreateResourceShare) defines
// MalformedArnException for exactly this.
func TestCreateResourceShare_MalformedResourceArn(t *testing.T) {
	t.Parallel()

	client := newRoundTripClient(t, ram.NewHandler(ram.NewInMemoryBackend("000000000000", "us-east-1")))

	_, err := client.CreateResourceShare(t.Context(), &ramsdk.CreateResourceShareInput{
		Name:         aws.String("malformed-arn-create-share"),
		ResourceArns: []string{"not-an-arn"},
	})
	require.Error(t, err)

	var apiErr *ramtypes.MalformedArnException
	require.ErrorAs(t, err, &apiErr, "expected a real MalformedArnException from the SDK deserializer")
}
