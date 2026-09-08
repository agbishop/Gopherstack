package sesv2

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wantCreationTimeTagValue is the tag value newTaggableTenant/
// newTaggableEmailIdentity set at creation and
// TestCreateOpsWithTags_CreationTimeTagsRoundTrip asserts round-trips back
// -- a named const rather than a repeated literal so it doesn't tip
// goconst's threshold for the package.
const wantCreationTimeTagValue = "gopherstack-82lk"

func newTaggableTenant(t *testing.T, b *InMemoryBackend) string {
	t.Helper()

	out, err := b.CreateTenant("tag-tenant", map[string]string{"Env": wantCreationTimeTagValue})
	require.NoError(t, err)

	return out.TenantARN
}

func newTaggableEmailIdentity(t *testing.T, b *InMemoryBackend) string {
	t.Helper()

	const identity = "tag-identity@example.com"

	_, err := b.CreateEmailIdentity(identity, "", map[string]string{"Env": wantCreationTimeTagValue})
	require.NoError(t, err)

	return b.identityARN(identity)
}

// TestCreateOpsWithTags_CreationTimeTagsRoundTrip tags a resource of every
// kind gopherstack currently routes creation-time tags into b.resourceTags
// for, and reads the tags back through ListTagsForResource -- the read path
// TagResource, UntagResource and cross-service GetResources all share. This
// is the regression guard for gopherstack-82lk: CreateTenant accepted tags
// and wrote them only to the tenant's own local map, so ListTagsForResource
// (and, since sesv2 was wired into cross-service tag queries, GetResources)
// never saw them. CreateEmailIdentity had the identical bug.
func TestCreateOpsWithTags_CreationTimeTagsRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(t *testing.T, b *InMemoryBackend) string
		name  string
	}{
		{name: "tenant", setup: newTaggableTenant},
		{name: "email_identity", setup: newTaggableEmailIdentity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewInMemoryBackend()
			resourceARN := tt.setup(t, b)

			got, err := b.ListTagsForResource(resourceARN)
			require.NoError(t, err)
			assert.Equal(t, wantCreationTimeTagValue, got["Env"])
		})
	}
}

// doWhiteboxRequest drives a real JSON request through h's HTTP handler --
// unlike calling an InMemoryBackend method directly, this is the only way to
// catch a handler that never decodes a Tags field from the request body in
// the first place (the bug TestCreateOpsWithTags_HandlerDecodesTags guards
// against).
func doWhiteboxRequest(t *testing.T, h *Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	b, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	require.NoError(t, h.Handler()(c))

	return rec
}

// TestCreateOpsWithTags_HandlerDecodesTags drives a Tags-bearing create
// request through the real HTTP handler (not a direct backend call) for
// every op gopherstack-uljk fixed: configuration sets, contact lists,
// dedicated IP pools and email templates all take Tags in the pinned
// aws-sdk-go-v2/service/sesv2 v1.66.4 Input struct, but their
// create*Input decode structs never had a Tags field, so the wire layer
// silently dropped the field before it ever reached the backend --
// TestCreateOpsWithTags_CreationTimeTagsRoundTrip's direct backend calls
// can't see that class of bug.
func TestCreateOpsWithTags_HandlerDecodesTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		arnFor func(b *InMemoryBackend) string
		body   map[string]any
		name   string
		path   string
	}{
		{
			name: "configuration_set",
			path: "/v2/email/configuration-sets",
			body: map[string]any{
				"ConfigurationSetName": "tag-config-set",
				"Tags":                 []tagEntry{{Key: "Env", Value: wantCreationTimeTagValue}},
			},
			arnFor: func(b *InMemoryBackend) string { return b.configurationSetARN("tag-config-set") },
		},
		{
			name: "contact_list",
			path: "/v2/email/contact-lists",
			body: map[string]any{
				"ContactListName": "tag-contact-list",
				"Tags":            []tagEntry{{Key: "Env", Value: wantCreationTimeTagValue}},
			},
			arnFor: func(b *InMemoryBackend) string { return b.contactListARN("tag-contact-list") },
		},
		{
			name: "dedicated_ip_pool",
			path: "/v2/email/dedicated-ip-pools",
			body: map[string]any{
				"PoolName": "tag-ip-pool",
				"Tags":     []tagEntry{{Key: "Env", Value: wantCreationTimeTagValue}},
			},
			arnFor: func(b *InMemoryBackend) string { return b.dedicatedIPPoolARN("tag-ip-pool") },
		},
		{
			name: "email_template",
			path: "/v2/email/templates",
			body: map[string]any{
				"TemplateName":    "tag-email-template",
				"TemplateContent": map[string]any{"Subject": "s", "Text": "t", "Html": "<p>h</p>"},
				"Tags":            []tagEntry{{Key: "Env", Value: wantCreationTimeTagValue}},
			},
			arnFor: func(b *InMemoryBackend) string { return b.emailTemplateARN("tag-email-template") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewInMemoryBackend()
			h := NewHandler(b)

			rec := doWhiteboxRequest(t, h, http.MethodPost, tt.path, tt.body)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			got, err := b.ListTagsForResource(tt.arnFor(b))
			require.NoError(t, err)
			assert.Equal(t, wantCreationTimeTagValue, got["Env"])
		})
	}
}

// TestCreateOps_AllCategorized walks every exported Create* method on
// InMemoryBackend and requires it to appear in exactly one of the three
// buckets below. A newly added Create* method that isn't sorted into one of
// them fails here instead of silently shipping with the same
// accepted-then-dropped tags bug as gopherstack-82lk.
func TestCreateOps_AllCategorized(t *testing.T) {
	t.Parallel()

	// fixedWithTags accept tags at creation and are proven to route them
	// into b.resourceTags -- CreateTenant/CreateEmailIdentity by
	// TestCreateOpsWithTags_CreationTimeTagsRoundTrip, the other four
	// (gopherstack-uljk) by TestCreateOpsWithTags_HandlerDecodesTags, whose
	// ARN builders are cited at their definitions (configuration_sets.go,
	// contact_lists.go, dedicated_ip_pools.go, email_templates.go).
	fixedWithTags := map[string]bool{
		"CreateTenant":           true,
		"CreateEmailIdentity":    true,
		"CreateConfigurationSet": true,
		"CreateContactList":      true,
		"CreateDedicatedIPPool":  true,
		"CreateEmailTemplate":    true,
	}

	// knownGapWithTags accept Tags in the pinned aws-sdk-go-v2/service/sesv2
	// v1.66.4 Input struct (cited per entry) but gopherstack doesn't yet
	// route creation-time tags for them into b.resourceTags because no ARN
	// format for the resource can be established from evidence (see each
	// entry). Tracked for follow-up; move an entry to fixedWithTags and give
	// it a row in TestCreateOpsWithTags_HandlerDecodesTags once fixed.
	knownGapWithTags := map[string]bool{
		// api_op_CreateMultiRegionEndpoint.go: Tags []types.Tag.
		// CreateMultiRegionEndpoint (multi_region_endpoints.go) stores tags on
		// the endpoint's own map only. GetMultiRegionEndpointOutput has no Arn
		// field in the real SDK (api_op_GetMultiRegionEndpoint.go:45-78,
		// v1.66.4) and the AWS::SES::MultiRegionEndpoint CloudFormation
		// resource's Ref returns only the endpoint name with no Arn GetAtt
		// attribute either -- no ARN is derivable for this resource from any
		// source checked.
		"CreateMultiRegionEndpoint": true,
		// api_op_CreateCustomVerificationEmailTemplate.go: Tags []types.Tag;
		// handler_custom_verification_email_templates.go's input struct has no
		// Tags field either. GetCustomVerificationEmailTemplateOutput has no
		// Arn field (api_op_GetCustomVerificationEmailTemplate.go:47-70,
		// v1.66.4); terraform-provider-aws has no resource for this template
		// kind at all; the AWS::SES::CustomVerificationEmailTemplate
		// CloudFormation resource's Ref returns only the template name with no
		// Arn GetAtt attribute -- no ARN is derivable from any source checked.
		"CreateCustomVerificationEmailTemplate": true,
		// api_op_CreateDeliverabilityTestReport.go: Tags []types.Tag;
		// CreateDeliverabilityTestReport (deliverability.go) takes no tags
		// param at all. GetDeliverabilityTestReportOutput has no Arn field
		// (api_op_GetDeliverabilityTestReport.go:40-72, v1.66.4); no
		// terraform-provider-aws resource or CloudFormation resource models
		// this ephemeral report kind -- no ARN is derivable from any source
		// checked.
		"CreateDeliverabilityTestReport": true,
	}

	// untaggable have no Tags field on their Input struct in the pinned
	// aws-sdk-go-v2/service/sesv2 v1.66.4, confirmed per entry below --
	// listed explicitly (not omitted) so a real gap can't hide as
	// "presumably untaggable".
	untaggable := map[string]bool{
		"CreateContact":                          true, // api_op_CreateContact.go: no Tags field
		"CreateConfigurationSetEventDestination": true, // api_op_CreateConfigurationSetEventDestination.go: no Tags field
		"CreateExportJob":                        true, // api_op_CreateExportJob.go: no Tags field
		"CreateImportJob":                        true, // api_op_CreateImportJob.go: no Tags field
		"CreateTenantResourceAssociation":        true, // api_op_CreateTenantResourceAssociation.go: no Tags field
		"CreateEmailIdentityPolicy":              true, // gopherstack-internal helper; no corresponding SDK Create op
	}

	typ := reflect.TypeFor[*InMemoryBackend]()

	for m := range typ.Methods() {
		if !strings.HasPrefix(m.Name, "Create") {
			continue
		}

		buckets := 0
		for _, set := range []map[string]bool{fixedWithTags, knownGapWithTags, untaggable} {
			if set[m.Name] {
				buckets++
			}
		}

		assert.Equalf(t, 1, buckets, "%s must appear in exactly one of fixedWithTags/"+
			"knownGapWithTags/untaggable in whitebox_test.go's TestCreateOps_AllCategorized", m.Name)
	}
}

// TestDeleteOps_ResourceTagsCleanedUp proves that deleting a resource
// removes its entry from the shared b.resourceTags store, for every
// resource kind fixedWithTags (above) routes creation-time tags into it.
// DeleteConfigurationSet and DeleteEmailIdentity already did this;
// DeleteContactList, DeleteDedicatedIPPool, DeleteEmailTemplate and
// DeleteTenant did not, leaving a ghost resourceTags[arn] entry behind --
// visible via ListTagsForResource on the deleted resource's ARN, and liable
// to leak stale tags onto a same-named resource created afterward.
func TestDeleteOps_ResourceTagsCleanedUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		create func(b *InMemoryBackend) string
		delete func(b *InMemoryBackend) error
		name   string
	}{
		{
			name: "contact_list",
			create: func(b *InMemoryBackend) string {
				_, err := b.CreateContactList("cl", "", map[string]string{"Env": wantCreationTimeTagValue})
				require.NoError(t, err)

				return b.contactListARN("cl")
			},
			delete: func(b *InMemoryBackend) error { return b.DeleteContactList("cl") },
		},
		{
			name: "dedicated_ip_pool",
			create: func(b *InMemoryBackend) string {
				_, err := b.CreateDedicatedIPPool("pool", "", map[string]string{"Env": wantCreationTimeTagValue})
				require.NoError(t, err)

				return b.dedicatedIPPoolARN("pool")
			},
			delete: func(b *InMemoryBackend) error { return b.DeleteDedicatedIPPool("pool") },
		},
		{
			name: "email_template",
			create: func(b *InMemoryBackend) string {
				_, err := b.CreateEmailTemplate("tmpl", &EmailTemplateContent{Subject: "s"},
					map[string]string{"Env": wantCreationTimeTagValue})
				require.NoError(t, err)

				return b.emailTemplateARN("tmpl")
			},
			delete: func(b *InMemoryBackend) error { return b.DeleteEmailTemplate("tmpl") },
		},
		{
			name: "tenant_resource",
			create: func(b *InMemoryBackend) string {
				out, err := b.CreateTenant("ghost-tag-tenant", map[string]string{"Env": wantCreationTimeTagValue})
				require.NoError(t, err)

				return out.TenantARN
			},
			delete: func(b *InMemoryBackend) error { return b.DeleteTenant("ghost-tag-tenant") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewInMemoryBackend()
			resourceARN := tt.create(b)

			before, err := b.ListTagsForResource(resourceARN)
			require.NoError(t, err)
			require.Equal(t, wantCreationTimeTagValue, before["Env"], "tags must be set before delete")

			require.NoError(t, tt.delete(b))

			after, err := b.ListTagsForResource(resourceARN)
			require.NoError(t, err)
			assert.Empty(t, after, "resourceTags must not retain a ghost entry for a deleted resource's ARN")
		})
	}
}
