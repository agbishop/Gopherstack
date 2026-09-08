package ssm_test

import (
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	ssmsdk "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/ssm"
)

const ssmTagsRTRegion = "us-east-1"

// newTestSSMClient stands up the real aws-sdk-go-v2 ssm client against an
// httptest server running this package's Handler, wired through the same
// pkgs/service registry/router used in production.
func newTestSSMClient(t *testing.T, h *ssm.Handler) *ssmsdk.Client {
	t.Helper()

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(h))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion(ssmTagsRTRegion),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return ssmsdk.NewFromConfig(cfg, func(o *ssmsdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

// TestCreateOpsWithTags_RoundTrip drives every ssm Create op whose real
// Input struct accepts Tags (ssm@v1.73.4: api_op_CreateMaintenanceWindow.go,
// api_op_CreateCloudConnector.go, api_op_CreateDocument.go:116,
// api_op_CreateAssociation.go:225, api_op_CreateOpsItem.go,
// api_op_CreateActivation.go, api_op_CreatePatchBaseline.go,
// api_op_CreateOpsMetadata.go) through the real SDK client and asserts the
// real read path for each resource type sees what was supplied at creation
// (gopherstack-2mwl).
func TestCreateOpsWithTags_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("maintenance window", func(t *testing.T) {
		t.Parallel()

		backend := ssm.NewInMemoryBackend()
		client := newTestSSMClient(t, ssm.NewHandler(backend))

		out, err := client.CreateMaintenanceWindow(t.Context(), &ssmsdk.CreateMaintenanceWindowInput{
			Name:                     aws.String("tagged-mw"),
			Schedule:                 aws.String("cron(0 0 ? * * *)"),
			Duration:                 aws.Int32(2),
			Cutoff:                   1,
			AllowUnassociatedTargets: false,
			Tags:                     []ssmtypes.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
		})
		require.NoError(t, err)

		got, err := client.ListTagsForResource(t.Context(), &ssmsdk.ListTagsForResourceInput{
			ResourceType: ssmtypes.ResourceTypeForTaggingMaintenanceWindow,
			ResourceId:   out.WindowId,
		})
		require.NoError(t, err)
		require.Len(t, got.TagList, 1)
		assert.Equal(t, "env", aws.ToString(got.TagList[0].Key))
		assert.Equal(t, "test", aws.ToString(got.TagList[0].Value))
	})

	t.Run("cloud connector", func(t *testing.T) {
		t.Parallel()

		backend := ssm.NewInMemoryBackend()
		client := newTestSSMClient(t, ssm.NewHandler(backend))

		out, err := client.CreateCloudConnector(t.Context(), &ssmsdk.CreateCloudConnectorInput{
			ConfigConnectorArn: aws.String("arn:aws:someservice:us-east-1:000000000000:connector/abc"),
			DisplayName:        aws.String("tagged-cc"),
			RoleArn:            aws.String("arn:aws:iam::000000000000:role/access"),
			Configuration: &ssmtypes.CloudConnectorConfigurationMemberAzureConfiguration{
				Value: ssmtypes.AzureConfiguration{
					ApplicationId: aws.String("app-1"),
					TenantId:      aws.String("tenant-1"),
				},
			},
			Tags: []ssmtypes.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
		})
		require.NoError(t, err)

		got, err := client.ListTagsForResource(t.Context(), &ssmsdk.ListTagsForResourceInput{
			ResourceType: ssmtypes.ResourceTypeForTaggingCloudConnector,
			ResourceId:   out.CloudConnectorId,
		})
		require.NoError(t, err)
		require.Len(t, got.TagList, 1)
		assert.Equal(t, "env", aws.ToString(got.TagList[0].Key))
		assert.Equal(t, "test", aws.ToString(got.TagList[0].Value))
	})

	t.Run("document", func(t *testing.T) {
		t.Parallel()

		backend := ssm.NewInMemoryBackend()
		client := newTestSSMClient(t, ssm.NewHandler(backend))

		_, err := client.CreateDocument(t.Context(), &ssmsdk.CreateDocumentInput{
			Name:    aws.String("tagged-doc"),
			Content: aws.String(`{"schemaVersion":"2.2","mainSteps":[]}`),
			Tags:    []ssmtypes.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
		})
		require.NoError(t, err)

		got, err := client.ListTagsForResource(t.Context(), &ssmsdk.ListTagsForResourceInput{
			ResourceType: ssmtypes.ResourceTypeForTaggingDocument,
			ResourceId:   aws.String("tagged-doc"),
		})
		require.NoError(t, err)
		require.Len(t, got.TagList, 1)
		assert.Equal(t, "env", aws.ToString(got.TagList[0].Key))
		assert.Equal(t, "test", aws.ToString(got.TagList[0].Value))
	})

	t.Run("association", func(t *testing.T) {
		t.Parallel()

		backend := ssm.NewInMemoryBackend()
		client := newTestSSMClient(t, ssm.NewHandler(backend))

		_, err := client.CreateDocument(t.Context(), &ssmsdk.CreateDocumentInput{
			Name:    aws.String("assoc-doc"),
			Content: aws.String(`{"schemaVersion":"2.2","mainSteps":[]}`),
		})
		require.NoError(t, err)

		out, err := client.CreateAssociation(t.Context(), &ssmsdk.CreateAssociationInput{
			Name: aws.String("assoc-doc"),
			Tags: []ssmtypes.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
		})
		require.NoError(t, err)

		got, err := client.ListTagsForResource(t.Context(), &ssmsdk.ListTagsForResourceInput{
			ResourceType: ssmtypes.ResourceTypeForTaggingAssociation,
			ResourceId:   out.AssociationDescription.AssociationId,
		})
		require.NoError(t, err)
		require.Len(t, got.TagList, 1)
		assert.Equal(t, "env", aws.ToString(got.TagList[0].Key))
		assert.Equal(t, "test", aws.ToString(got.TagList[0].Value))
	})

	t.Run("ops item", func(t *testing.T) {
		t.Parallel()

		backend := ssm.NewInMemoryBackend()
		client := newTestSSMClient(t, ssm.NewHandler(backend))

		out, err := client.CreateOpsItem(t.Context(), &ssmsdk.CreateOpsItemInput{
			Title:       aws.String("tagged-item"),
			Source:      aws.String("EC2"),
			Description: aws.String("desc"),
			Tags:        []ssmtypes.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
		})
		require.NoError(t, err)

		got, err := client.ListTagsForResource(t.Context(), &ssmsdk.ListTagsForResourceInput{
			ResourceType: ssmtypes.ResourceTypeForTaggingOpsItem,
			ResourceId:   out.OpsItemId,
		})
		require.NoError(t, err)
		require.Len(t, got.TagList, 1)
		assert.Equal(t, "env", aws.ToString(got.TagList[0].Key))
		assert.Equal(t, "test", aws.ToString(got.TagList[0].Value))
	})

	t.Run("patch baseline", func(t *testing.T) {
		t.Parallel()

		backend := ssm.NewInMemoryBackend()
		client := newTestSSMClient(t, ssm.NewHandler(backend))

		out, err := client.CreatePatchBaseline(t.Context(), &ssmsdk.CreatePatchBaselineInput{
			Name: aws.String("tagged-baseline"),
			Tags: []ssmtypes.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
		})
		require.NoError(t, err)

		got, err := client.ListTagsForResource(t.Context(), &ssmsdk.ListTagsForResourceInput{
			ResourceType: ssmtypes.ResourceTypeForTaggingPatchBaseline,
			ResourceId:   out.BaselineId,
		})
		require.NoError(t, err)
		require.Len(t, got.TagList, 1)
		assert.Equal(t, "env", aws.ToString(got.TagList[0].Key))
		assert.Equal(t, "test", aws.ToString(got.TagList[0].Value))
	})

	t.Run("ops metadata", func(t *testing.T) {
		t.Parallel()

		backend := ssm.NewInMemoryBackend()
		client := newTestSSMClient(t, ssm.NewHandler(backend))

		out, err := client.CreateOpsMetadata(t.Context(), &ssmsdk.CreateOpsMetadataInput{
			ResourceId: aws.String("arn:aws:ssm:us-east-1:000000000000:document/tagged-doc"),
			Tags:       []ssmtypes.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
		})
		require.NoError(t, err)

		got, err := client.ListTagsForResource(t.Context(), &ssmsdk.ListTagsForResourceInput{
			ResourceType: ssmtypes.ResourceTypeForTaggingOpsmetadata,
			ResourceId:   out.OpsMetadataArn,
		})
		require.NoError(t, err)
		require.Len(t, got.TagList, 1)
		assert.Equal(t, "env", aws.ToString(got.TagList[0].Key))
		assert.Equal(t, "test", aws.ToString(got.TagList[0].Value))
	})

	// CreateActivation's Tags are applied to registered managed instances, not the
	// activation itself (ssm@v1.73.4 api_op_CreateActivation.go:83-95) -- but real
	// AWS's Activation struct (returned by DescribeActivations) also carries its own
	// Tags field (types/types.go:57), which is the read path this asserts.
	t.Run("activation", func(t *testing.T) {
		t.Parallel()

		backend := ssm.NewInMemoryBackend()
		client := newTestSSMClient(t, ssm.NewHandler(backend))

		out, err := client.CreateActivation(t.Context(), &ssmsdk.CreateActivationInput{
			IamRole: aws.String("arn:aws:iam::000000000000:role/access"),
			Tags:    []ssmtypes.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
		})
		require.NoError(t, err)

		got, err := client.DescribeActivations(t.Context(), &ssmsdk.DescribeActivationsInput{})
		require.NoError(t, err)

		var found *ssmtypes.Activation
		for i := range got.ActivationList {
			if aws.ToString(got.ActivationList[i].ActivationId) == aws.ToString(out.ActivationId) {
				found = &got.ActivationList[i]

				break
			}
		}
		require.NotNil(t, found, "created activation must appear in DescribeActivations")
		require.Len(t, found.Tags, 1)
		assert.Equal(t, "env", aws.ToString(found.Tags[0].Key))
		assert.Equal(t, "test", aws.ToString(found.Tags[0].Value))
	})

	// PutParameterInput.Tags (api_op_PutParameter.go:196-204) had no Go struct
	// member at all -- a real client's tags supplied on create were silently
	// dropped, only reachable afterward via a second AddTagsToResource call.
	t.Run("parameter", func(t *testing.T) {
		t.Parallel()

		backend := ssm.NewInMemoryBackend()
		client := newTestSSMClient(t, ssm.NewHandler(backend))

		_, err := client.PutParameter(t.Context(), &ssmsdk.PutParameterInput{
			Name:  aws.String("/tagged/param"),
			Type:  ssmtypes.ParameterTypeString,
			Value: aws.String("v"),
			Tags:  []ssmtypes.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
		})
		require.NoError(t, err)

		got, err := client.ListTagsForResource(t.Context(), &ssmsdk.ListTagsForResourceInput{
			ResourceType: ssmtypes.ResourceTypeForTaggingParameter,
			ResourceId:   aws.String("/tagged/param"),
		})
		require.NoError(t, err)
		require.Len(t, got.TagList, 1)
		assert.Equal(t, "env", aws.ToString(got.TagList[0].Key))
		assert.Equal(t, "test", aws.ToString(got.TagList[0].Value))

		// AWS's own doc comment: an Overwrite of an EXISTING parameter does not
		// apply Tags -- "To add tags to an existing Systems Manager parameter,
		// use the AddTagsToResource operation."
		_, err = client.PutParameter(t.Context(), &ssmsdk.PutParameterInput{
			Name:      aws.String("/tagged/param"),
			Type:      ssmtypes.ParameterTypeString,
			Value:     aws.String("v2"),
			Overwrite: aws.Bool(true),
			Tags:      []ssmtypes.Tag{{Key: aws.String("second"), Value: aws.String("ignored")}},
		})
		require.NoError(t, err)

		got, err = client.ListTagsForResource(t.Context(), &ssmsdk.ListTagsForResourceInput{
			ResourceType: ssmtypes.ResourceTypeForTaggingParameter,
			ResourceId:   aws.String("/tagged/param"),
		})
		require.NoError(t, err)
		require.Len(t, got.TagList, 1, "overwrite must not apply new Tags")
		assert.Equal(t, "env", aws.ToString(got.TagList[0].Key))
	})
}

// TestDeleteDocument_TagsDoNotSurviveRecreate: document names are user-chosen
// (unlike the generated IDs of most other taggable resources), so deleting
// and recreating a document under the same name is a realistic sequence. A
// fresh document under a reused name must not inherit tags from the deleted
// one.
func TestDeleteDocument_TagsDoNotSurviveRecreate(t *testing.T) {
	t.Parallel()

	backend := ssm.NewInMemoryBackend()
	client := newTestSSMClient(t, ssm.NewHandler(backend))

	_, err := client.CreateDocument(t.Context(), &ssmsdk.CreateDocumentInput{
		Name:    aws.String("reused-doc"),
		Content: aws.String(`{"schemaVersion":"2.2","mainSteps":[]}`),
		Tags:    []ssmtypes.Tag{{Key: aws.String("env"), Value: aws.String("old")}},
	})
	require.NoError(t, err)

	_, err = client.DeleteDocument(t.Context(), &ssmsdk.DeleteDocumentInput{
		Name: aws.String("reused-doc"),
	})
	require.NoError(t, err)

	_, err = client.CreateDocument(t.Context(), &ssmsdk.CreateDocumentInput{
		Name:    aws.String("reused-doc"),
		Content: aws.String(`{"schemaVersion":"2.2","mainSteps":[]}`),
	})
	require.NoError(t, err)

	got, err := client.ListTagsForResource(t.Context(), &ssmsdk.ListTagsForResourceInput{
		ResourceType: ssmtypes.ResourceTypeForTaggingDocument,
		ResourceId:   aws.String("reused-doc"),
	})
	require.NoError(t, err)
	assert.Empty(t, got.TagList, "recreated document must not inherit tags from the deleted one")
}
