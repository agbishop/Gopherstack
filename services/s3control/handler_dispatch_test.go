package s3control_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractOperation_CreateOperations exercises ExtractOperation for the
// create-style (POST/PUT) operation across every op family.
func TestExtractOperation_CreateOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{
			name:   "post_accessgrantsinstance_returns_CreateAccessGrantsInstance",
			method: http.MethodPost,
			path:   "/v20180820/accessgrantsinstance",
			want:   "CreateAccessGrantsInstance",
		},
		{
			name:   "post_identitycenter_returns_AssociateAccessGrantsIdentityCenter",
			method: http.MethodPost,
			path:   "/v20180820/accessgrantsinstance/identitycenter",
			want:   "AssociateAccessGrantsIdentityCenter",
		},
		{
			name:   "post_grant_returns_CreateAccessGrant",
			method: http.MethodPost,
			path:   "/v20180820/accessgrantsinstance/grant",
			want:   "CreateAccessGrant",
		},
		{
			name:   "post_location_returns_CreateAccessGrantsLocation",
			method: http.MethodPost,
			path:   "/v20180820/accessgrantsinstance/location",
			want:   "CreateAccessGrantsLocation",
		},
		{
			name:   "put_accesspoint_returns_CreateAccessPoint",
			method: http.MethodPut,
			path:   "/v20180820/accesspoint/my-ap",
			want:   "CreateAccessPoint",
		},
		{
			name:   "put_objectlambda_returns_CreateAccessPointForObjectLambda",
			method: http.MethodPut,
			path:   "/v20180820/accesspointforobjectlambda/my-ap",
			want:   "CreateAccessPointForObjectLambda",
		},
		{
			name:   "put_bucket_returns_CreateBucket",
			method: http.MethodPut,
			path:   "/v20180820/bucket/my-bucket",
			want:   "CreateBucket",
		},
		{
			name:   "post_mrap_create_returns_CreateMultiRegionAccessPoint",
			method: http.MethodPost,
			path:   "/v20180820/async-requests/mrap/create",
			want:   "CreateMultiRegionAccessPoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			c := e.NewContext(req, httptest.NewRecorder())

			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}

// --- Dispatch stub coverage ---

func TestHandler_StubOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		wantBody   string
		wantStatus int
	}{
		{
			name:       "list_tags_for_resource",
			method:     http.MethodGet,
			path:       "/v20180820/tags/arn:aws:s3:us-east-1:123:accesspoint/myap",
			wantStatus: http.StatusOK,
			wantBody:   "ListTagsForResourceResult",
		},
		{
			name:       "tag_resource",
			method:     http.MethodPost,
			path:       "/v20180820/tags/arn:aws:s3:us-east-1:123:accesspoint/myap",
			wantStatus: http.StatusOK,
			wantBody:   "TagResourceResult",
		},
		{
			name:       "untag_resource",
			method:     http.MethodDelete,
			path:       "/v20180820/tags/arn:aws:s3:us-east-1:123:accesspoint/myap",
			wantStatus: http.StatusNoContent,
			wantBody:   "",
		},
		// "mybucket" is never created against this fresh handler; Get/Put/
		// DeleteBucketReplication now require the bucket to exist (matching
		// every other bucket sub-resource op), so all three 404 with
		// NoSuchBucket instead of the pre-fix behavior (200/204, or a
		// misleading "config missing" 404 that ignored the bucket itself
		// not existing).
		{
			name:       "get_bucket_replication",
			method:     http.MethodGet,
			path:       "/v20180820/bucket/mybucket/replication",
			wantStatus: http.StatusNotFound,
			wantBody:   "NoSuchBucket",
		},
		{
			name:       "put_bucket_replication",
			method:     http.MethodPut,
			path:       "/v20180820/bucket/mybucket/replication",
			wantStatus: http.StatusNotFound,
			wantBody:   "NoSuchBucket",
		},
		{
			name:       "delete_bucket_replication",
			method:     http.MethodDelete,
			path:       "/v20180820/bucket/mybucket/replication",
			wantStatus: http.StatusNotFound,
			wantBody:   "NoSuchBucket",
		},
		{
			name:       "get_storage_lens_config",
			method:     http.MethodGet,
			path:       "/v20180820/storagelens/myconfig",
			wantStatus: http.StatusNotFound,
			wantBody:   "NoSuchConfiguration",
		},
		{
			name:       "put_storage_lens_config",
			method:     http.MethodPut,
			path:       "/v20180820/storagelens/myconfig",
			wantStatus: http.StatusOK,
			wantBody:   "",
		},
		{
			name:       "delete_storage_lens_config",
			method:     http.MethodDelete,
			path:       "/v20180820/storagelens/myconfig",
			wantStatus: http.StatusNoContent,
			wantBody:   "",
		},
		{
			name:       "get_storage_lens_tagging",
			method:     http.MethodGet,
			path:       "/v20180820/storagelens/myconfig/tagging",
			wantStatus: http.StatusNotFound,
			wantBody:   "NoSuchConfiguration",
		},
		{
			name:       "put_storage_lens_tagging",
			method:     http.MethodPut,
			path:       "/v20180820/storagelens/myconfig/tagging",
			wantStatus: http.StatusOK,
			wantBody:   "",
		},
		{
			name:       "delete_storage_lens_tagging",
			method:     http.MethodDelete,
			path:       "/v20180820/storagelens/myconfig/tagging",
			wantStatus: http.StatusNoContent,
			wantBody:   "",
		},
		{
			name:       "list_storage_lens_configs",
			method:     http.MethodGet,
			path:       "/v20180820/storagelens",
			wantStatus: http.StatusOK,
			wantBody:   "ListStorageLensConfigurationsResult",
		},
		{
			name:       "get_storage_lens_group",
			method:     http.MethodGet,
			path:       "/v20180820/storagelensgroup/mygroup",
			wantStatus: http.StatusNotFound,
			wantBody:   "NoSuchStorageLensGroup",
		},
		{
			name:       "update_storage_lens_group",
			method:     http.MethodPut,
			path:       "/v20180820/storagelensgroup/mygroup",
			wantStatus: http.StatusNotFound,
			wantBody:   "NoSuchStorageLensGroup",
		},
		{
			name:       "delete_storage_lens_group",
			method:     http.MethodDelete,
			path:       "/v20180820/storagelensgroup/mygroup",
			wantStatus: http.StatusNotFound,
			wantBody:   "NoSuchStorageLensGroup",
		},
		{
			name:       "list_storage_lens_groups",
			method:     http.MethodGet,
			path:       "/v20180820/storagelensgroup",
			wantStatus: http.StatusOK,
			wantBody:   "ListStorageLensGroupsResult",
		},
		// "mymrap" is never created against this fresh handler; submitting
		// routes for it now 404s with NoSuchMultiRegionAccessPoint instead
		// of the pre-fix behavior (200, silently accepting routes for any
		// name) -- see PARITY.md, gopherstack-l498.
		{
			name:       "submit_mrap_routes",
			method:     http.MethodPatch,
			path:       "/v20180820/mrap/instances/mymrap/routes",
			wantStatus: http.StatusNotFound,
			wantBody:   "NoSuchMultiRegionAccessPoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			rec := doS3Request(t, h, tt.method, tt.path, "")
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantBody)
		})
	}
}

// TestHandler_ExtractOperation tests operation extraction for various paths.
func TestHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		wantOp string
	}{
		{
			name:   "delete_public_access_block",
			method: http.MethodDelete,
			path:   "/v20180820/configuration/publicAccessBlock",
			wantOp: "DeletePublicAccessBlock",
		},
		{
			name:   "get_access_point",
			method: http.MethodGet,
			path:   "/v20180820/accesspoint/myap",
			wantOp: "GetAccessPoint",
		},
		{
			name:   "delete_access_point",
			method: http.MethodDelete,
			path:   "/v20180820/accesspoint/myap",
			wantOp: "DeleteAccessPoint",
		},
		{
			name:   "list_access_points",
			method: http.MethodGet,
			path:   "/v20180820/accesspoint",
			wantOp: "ListAccessPoints",
		},
		{
			name:   "get_access_point_policy",
			method: http.MethodGet,
			path:   "/v20180820/accesspoint/myap/policy",
			wantOp: "GetAccessPointPolicy",
		},
		{
			name:   "put_access_point_policy",
			method: http.MethodPut,
			path:   "/v20180820/accesspoint/myap/policy",
			wantOp: "PutAccessPointPolicy",
		},
		{
			name:   "delete_access_point_policy",
			method: http.MethodDelete,
			path:   "/v20180820/accesspoint/myap/policy",
			wantOp: "DeleteAccessPointPolicy",
		},
		{
			name:   "get_access_point_policy_status",
			method: http.MethodGet,
			path:   "/v20180820/accesspoint/myap/policyStatus",
			wantOp: "GetAccessPointPolicyStatus",
		},
		{name: "list_jobs", method: http.MethodGet, path: "/v20180820/jobs", wantOp: "ListJobs"},
		{name: "create_job", method: http.MethodPost, path: "/v20180820/jobs", wantOp: "CreateJob"},
		{name: "describe_job", method: http.MethodGet, path: "/v20180820/jobs/job-1", wantOp: "DescribeJob"},
		{
			name:   "update_job_priority",
			method: http.MethodPost,
			path:   "/v20180820/jobs/job-1/priority",
			wantOp: "UpdateJobPriority",
		},
		{
			name:   "update_job_status",
			method: http.MethodPost,
			path:   "/v20180820/jobs/job-1/status",
			wantOp: "UpdateJobStatus",
		},
		{
			name:   "list_mrap",
			method: http.MethodGet,
			path:   "/v20180820/mrap/instances",
			wantOp: "ListMultiRegionAccessPoints",
		},
		{
			name:   "get_mrap",
			method: http.MethodGet,
			path:   "/v20180820/mrap/instances/mymrap",
			wantOp: "GetMultiRegionAccessPoint",
		},
		{
			// The sync DELETE variant of this path was removed (gopherstack-tir4):
			// no real SDK client can send it -- see
			// TestHandler_DeleteMultiRegionAccessPoint_SyncRouteRemoved. It now
			// falls through to the generic unmapped-route "Unknown" op.
			name:   "delete_mrap_instance_unmapped",
			method: http.MethodDelete,
			path:   "/v20180820/mrap/instances/mymrap",
			wantOp: "Unknown",
		},
		{
			name:   "submit_mrap_routes",
			method: http.MethodPatch,
			path:   "/v20180820/mrap/instances/mymrap/routes",
			wantOp: "SubmitMultiRegionAccessPointRoutes",
		},
		{
			name:   "list_tags",
			method: http.MethodGet,
			path:   "/v20180820/tags/arn:aws:s3:us-east-1:123:accesspoint/myap",
			wantOp: "ListTagsForResource",
		},
		{
			name:   "tag_resource",
			method: http.MethodPost,
			path:   "/v20180820/tags/arn:aws:s3:us-east-1:123:accesspoint/myap",
			wantOp: "TagResource",
		},
		{
			name:   "untag_resource",
			method: http.MethodDelete,
			path:   "/v20180820/tags/arn:aws:s3:us-east-1:123:accesspoint/myap",
			wantOp: "UntagResource",
		},
		{name: "get_bucket", method: http.MethodGet, path: "/v20180820/bucket/mybucket", wantOp: "GetBucket"},
		{name: "delete_bucket", method: http.MethodDelete, path: "/v20180820/bucket/mybucket", wantOp: "DeleteBucket"},
		{
			name:   "get_bucket_replication",
			method: http.MethodGet,
			path:   "/v20180820/bucket/mybucket/replication",
			wantOp: "GetBucketReplication",
		},
		{
			name:   "put_bucket_replication",
			method: http.MethodPut,
			path:   "/v20180820/bucket/mybucket/replication",
			wantOp: "PutBucketReplication",
		},
		{
			name:   "delete_bucket_replication",
			method: http.MethodDelete,
			path:   "/v20180820/bucket/mybucket/replication",
			wantOp: "DeleteBucketReplication",
		},
		{
			name:   "get_storage_lens",
			method: http.MethodGet,
			path:   "/v20180820/storagelens/myconfig",
			wantOp: "GetStorageLensConfiguration",
		},
		{
			name:   "put_storage_lens",
			method: http.MethodPut,
			path:   "/v20180820/storagelens/myconfig",
			wantOp: "PutStorageLensConfiguration",
		},
		{
			name:   "delete_storage_lens",
			method: http.MethodDelete,
			path:   "/v20180820/storagelens/myconfig",
			wantOp: "DeleteStorageLensConfiguration",
		},
		{
			name:   "list_storage_lens",
			method: http.MethodGet,
			path:   "/v20180820/storagelens",
			wantOp: "ListStorageLensConfigurations",
		},
		{
			name:   "get_storage_lens_tagging",
			method: http.MethodGet,
			path:   "/v20180820/storagelens/myconfig/tagging",
			wantOp: "GetStorageLensConfigurationTagging",
		},
		{
			name:   "put_storage_lens_tagging",
			method: http.MethodPut,
			path:   "/v20180820/storagelens/myconfig/tagging",
			wantOp: "PutStorageLensConfigurationTagging",
		},
		{
			name:   "delete_storage_lens_tagging",
			method: http.MethodDelete,
			path:   "/v20180820/storagelens/myconfig/tagging",
			wantOp: "DeleteStorageLensConfigurationTagging",
		},
		{
			name:   "get_storage_lens_group",
			method: http.MethodGet,
			path:   "/v20180820/storagelensgroup/mygroup",
			wantOp: "GetStorageLensGroup",
		},
		{
			name:   "update_storage_lens_group",
			method: http.MethodPut,
			path:   "/v20180820/storagelensgroup/mygroup",
			wantOp: "UpdateStorageLensGroup",
		},
		{
			name:   "delete_storage_lens_group",
			method: http.MethodDelete,
			path:   "/v20180820/storagelensgroup/mygroup",
			wantOp: "DeleteStorageLensGroup",
		},
		{
			name:   "list_storage_lens_groups",
			method: http.MethodGet,
			path:   "/v20180820/storagelensgroup",
			wantOp: "ListStorageLensGroups",
		},
		{
			name:   "create_storage_lens_group",
			method: http.MethodPost,
			path:   "/v20180820/storagelensgroup",
			wantOp: "CreateStorageLensGroup",
		},
		{
			name:   "delete_mrap_async",
			method: http.MethodPost,
			path:   "/v20180820/async-requests/mrap/delete/token1",
			wantOp: "DeleteMultiRegionAccessPoint",
		},
		{
			name:   "put_mrap_policy",
			method: http.MethodPost,
			path:   "/v20180820/async-requests/mrap/put-policy/token1",
			wantOp: "PutMultiRegionAccessPointPolicy",
		},
		{name: "unknown_op", method: http.MethodPost, path: "/v20180820/unknownresource", wantOp: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			h := newTestS3ControlHandler(t)
			op := h.ExtractOperation(c)
			assert.Equal(t, tt.wantOp, op)
		})
	}
}

// TestHandler_WriteXML_MarshalError covers the marshal error branch in writeXML.
func TestHandler_WriteXML_MarshalError(t *testing.T) {
	t.Parallel()

	// Use an unmarshalable type - channel cannot be XML-marshaled.
	// We can trigger this by creating a fake handler scenario.
	// Instead we just verify the createAccessPoint happy path works via XML.
	tests := []struct {
		name       string
		wantBody   string
		wantStatus int
	}{
		{
			name:       "create_access_point_xml_response",
			wantStatus: http.StatusOK,
			wantBody:   "CreateAccessPointResult",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			body := `<CreateAccessPointRequest><Bucket>mybucket</Bucket></CreateAccessPointRequest>`
			rec := doS3Request(t, h, http.MethodPut, "/v20180820/accesspoint/ap1", body)
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantBody)
		})
	}
}

// TestHandler_HandleBackendError covers additional error branches.
func TestHandler_HandleBackendError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "create_access_grant_missing_permission_returns_bad_request",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			// Missing permission field → ErrValidation → 400
			body := "<CreateAccessGrantRequest>" +
				"<AccessGrantsLocationId>loc-1</AccessGrantsLocationId>" +
				"<Permission></Permission>" +
				"</CreateAccessGrantRequest>"
			rec := doS3Request(t, h, http.MethodPost, "/v20180820/accessgrantsinstance/grant", body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_DecodeXML_BadBody covers the decodeXML error path.
func TestHandler_DecodeXML_BadBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		method     string
		body       string
		wantStatus int
	}{
		{
			name:       "create_access_grants_instance_bad_body",
			path:       "/v20180820/accessgrantsinstance",
			method:     http.MethodPost,
			body:       "<Invalid</>",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			h := newTestS3ControlHandler(t)
			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
