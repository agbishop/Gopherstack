package s3control_test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	s3control "github.com/blackbirdworks/gopherstack/services/s3control"
)

// ---- Access Grants Instance ----

func TestAccessGrantsInstance(t *testing.T) {
	t.Parallel()

	t.Run("get after create", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateAccessGrantsInstance("000000000000", "")

		inst, err := b.GetAccessGrantsInstance("000000000000")
		require.NoError(t, err)
		assert.NotEmpty(t, inst.AccessGrantsInstanceArn)
	})

	t.Run("delete instance", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateAccessGrantsInstance("000000000000", "")
		require.NoError(t, b.DeleteAccessGrantsInstance("000000000000"))
		_, err := b.GetAccessGrantsInstance("000000000000")
		require.Error(t, err)
	})

	// "delete instance cascade cleans state" locks in the ghost-map-row
	// fix: DeleteAccessGrantsInstance previously only removed the instance
	// row itself, leaving its resource policy and generic resource tags
	// behind forever.
	t.Run("delete instance cascade cleans state", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		inst := b.CreateAccessGrantsInstance("000000000000", "")
		b.PutAccessGrantsInstanceResourcePolicy("000000000000", `{"p":1}`)
		b.TagResource(inst.AccessGrantsInstanceArn, map[string]string{"env": "test"})

		require.NoError(t, b.DeleteAccessGrantsInstance("000000000000"))

		policy, err := b.GetAccessGrantsInstanceResourcePolicy("000000000000")
		require.NoError(t, err)
		assert.Empty(t, policy, "resource policy must not survive delete")

		assert.Empty(t, b.ListTagsForResource(inst.AccessGrantsInstanceArn), "tags must not survive delete")
	})

	// "delete instance rejected while grants exist" and "delete instance
	// rejected while locations exist" lock in the gopherstack-tir4 fix:
	// DeleteAccessGrantsInstance's own doc comment ("You must first delete
	// the access grants and locations before S3 Access Grants can delete
	// the instance") was previously unenforced -- a real AWS account
	// rejects this, but gopherstack silently allowed it.
	t.Run("delete instance rejected while grants exist", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateAccessGrantsInstance("000000000000", "")
		loc := b.CreateAccessGrantsLocation("000000000000", "s3://bucket/", "arn:aws:iam::000000000000:role/r")
		_, err := b.CreateAccessGrant(
			"000000000000", loc.AccessGrantsLocationID, "IAMUser", "arn:test", "READ", "",
		)
		require.NoError(t, err)

		err = b.DeleteAccessGrantsInstance("000000000000")
		require.Error(t, err)
		require.ErrorIs(t, err, s3control.ErrValidation)

		_, getErr := b.GetAccessGrantsInstance("000000000000")
		require.NoError(t, getErr, "instance must survive a rejected delete")
	})

	t.Run("delete instance rejected while locations exist", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateAccessGrantsInstance("000000000000", "")
		b.CreateAccessGrantsLocation("000000000000", "s3://bucket/", "arn:aws:iam::000000000000:role/r")

		err := b.DeleteAccessGrantsInstance("000000000000")
		require.Error(t, err)
		require.ErrorIs(t, err, s3control.ErrValidation)
	})

	t.Run("delete instance succeeds once grants and locations are gone", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateAccessGrantsInstance("000000000000", "")
		loc := b.CreateAccessGrantsLocation("000000000000", "s3://bucket/", "arn:aws:iam::000000000000:role/r")
		grant, err := b.CreateAccessGrant(
			"000000000000", loc.AccessGrantsLocationID, "IAMUser", "arn:test", "READ", "",
		)
		require.NoError(t, err)

		require.Error(t, b.DeleteAccessGrantsInstance("000000000000"))

		require.NoError(t, b.DeleteAccessGrant("000000000000", grant.AccessGrantID))
		require.Error(t, b.DeleteAccessGrantsInstance("000000000000"), "location still attached")

		require.NoError(t, b.DeleteAccessGrantsLocation("000000000000", loc.AccessGrantsLocationID))
		require.NoError(t, b.DeleteAccessGrantsInstance("000000000000"))
	})

	// "delete instance rejected while identity center associated" and
	// "delete instance succeeds once identity center is dissociated" lock in
	// the third precondition from DeleteAccessGrantsInstance's own doc
	// comment: "If you have associated an IAM Identity Center instance with
	// your S3 Access Grants instance, you must first dissassociate the
	// Identity Center instance ... before you can delete the S3 Access
	// Grants instance."
	t.Run("delete instance rejected while identity center associated", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateAccessGrantsInstance("000000000000", "arn:aws:sso:::instance/ssoins-test")

		err := b.DeleteAccessGrantsInstance("000000000000")
		require.Error(t, err)
		require.ErrorIs(t, err, s3control.ErrValidation)

		_, getErr := b.GetAccessGrantsInstance("000000000000")
		require.NoError(t, getErr, "instance must survive a rejected delete")
	})

	t.Run("delete instance succeeds once identity center is dissociated", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateAccessGrantsInstance("000000000000", "arn:aws:sso:::instance/ssoins-test")

		require.Error(t, b.DeleteAccessGrantsInstance("000000000000"))

		b.DissociateAccessGrantsIdentityCenter("000000000000")
		require.NoError(t, b.DeleteAccessGrantsInstance("000000000000"))
	})

	t.Run("resource policy CRUD", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.PutAccessGrantsInstanceResourcePolicy("000000000000", `{"Version":"2012-10-17"}`)
		policy, err := b.GetAccessGrantsInstanceResourcePolicy("000000000000")
		require.NoError(t, err)
		assert.Contains(t, policy, "Version")
		b.DeleteAccessGrantsInstanceResourcePolicy("000000000000")
		policy2, _ := b.GetAccessGrantsInstanceResourcePolicy("000000000000")
		assert.Empty(t, policy2)
	})

	t.Run("dissociate identity center", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateAccessGrantsInstance("000000000000", "arn:aws:sso:::instance/ssoins-test")
		b.DissociateAccessGrantsIdentityCenter("000000000000")
		inst, _ := b.GetAccessGrantsInstance("000000000000")
		assert.Empty(t, inst.IdentityCenterArn)
	})

	t.Run("get for prefix after create", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateAccessGrantsInstance("000000000000", "")
		inst, err := b.GetAccessGrantsInstanceForPrefix("000000000000", "s3://my-bucket/prefix/")
		require.NoError(t, err)
		assert.NotEmpty(t, inst.AccessGrantsInstanceArn)
	})
}

// ---- Access Grants CRUD ----

func TestAccessGrantsCRUD(t *testing.T) {
	t.Parallel()
	b := s3control.NewInMemoryBackend()
	b.CreateAccessGrantsInstance("000000000000", "")
	loc := b.CreateAccessGrantsLocation(
		"000000000000",
		"s3://bucket/",
		"arn:aws:iam::000000000000:role/test",
	)
	grant, setupErr := b.CreateAccessGrant(
		"000000000000",
		loc.AccessGrantsLocationID,
		"IAMUser",
		"arn:aws:iam::000000000000:user/test",
		"READ",
		"",
	)
	require.NoError(t, setupErr)

	t.Run("get grant", func(t *testing.T) {
		t.Parallel()
		g, err := b.GetAccessGrant("000000000000", grant.AccessGrantID)
		require.NoError(t, err)
		assert.Equal(t, "READ", g.Permission)
	})

	t.Run("list grants", func(t *testing.T) {
		t.Parallel()
		grants := b.ListAccessGrants("000000000000", s3control.AccessGrantsFilter{})
		assert.NotEmpty(t, grants)
	})

	t.Run("list caller grants", func(t *testing.T) {
		t.Parallel()
		grants := b.ListCallerAccessGrants("000000000000", "")
		assert.NotEmpty(t, grants)
	})

	t.Run("delete grant", func(t *testing.T) {
		t.Parallel()
		b2 := s3control.NewInMemoryBackend()
		b2.CreateAccessGrantsInstance("000000000000", "")
		l := b2.CreateAccessGrantsLocation(
			"000000000000",
			"s3://bucket/",
			"arn:aws:iam::000000000000:role/r",
		)
		g, _ := b2.CreateAccessGrant(
			"000000000000",
			l.AccessGrantsLocationID,
			"IAMUser",
			"arn:test",
			"READ",
			"",
		)
		require.NoError(t, b2.DeleteAccessGrant("000000000000", g.AccessGrantID))
	})

	// "delete grant cascade cleans tags" locks in the ghost-map-row fix:
	// DeleteAccessGrant previously left generic resource tags behind
	// forever after the grant row itself was removed.
	t.Run("delete grant cascade cleans tags", func(t *testing.T) {
		t.Parallel()
		b2 := s3control.NewInMemoryBackend()
		b2.CreateAccessGrantsInstance("000000000000", "")
		l := b2.CreateAccessGrantsLocation(
			"000000000000",
			"s3://bucket/",
			"arn:aws:iam::000000000000:role/r",
		)
		g, err := b2.CreateAccessGrant(
			"000000000000",
			l.AccessGrantsLocationID,
			"IAMUser",
			"arn:test",
			"READ",
			"",
		)
		require.NoError(t, err)
		b2.TagResource(g.AccessGrantArn, map[string]string{"env": "test"})

		require.NoError(t, b2.DeleteAccessGrant("000000000000", g.AccessGrantID))

		assert.Empty(t, b2.ListTagsForResource(g.AccessGrantArn), "tags must not survive delete")
	})
}

func TestAccessGrantsLocation(t *testing.T) {
	t.Parallel()
	b := s3control.NewInMemoryBackend()
	loc := b.CreateAccessGrantsLocation(
		"000000000000",
		"s3://bucket/",
		"arn:aws:iam::000000000000:role/test",
	)

	t.Run("get location", func(t *testing.T) {
		t.Parallel()
		l, err := b.GetAccessGrantsLocation("000000000000", loc.AccessGrantsLocationID)
		require.NoError(t, err)
		assert.Equal(t, "s3://bucket/", l.LocationScope)
	})

	t.Run("update location", func(t *testing.T) {
		t.Parallel()
		b2 := s3control.NewInMemoryBackend()
		l := b2.CreateAccessGrantsLocation("000000000000", "s3://b/", "arn:old")
		updated, err := b2.UpdateAccessGrantsLocation(
			"000000000000",
			l.AccessGrantsLocationID,
			"arn:new",
		)
		require.NoError(t, err)
		assert.Equal(t, "arn:new", updated.IAMRoleArn)
	})

	t.Run("list locations", func(t *testing.T) {
		t.Parallel()
		locs := b.ListAccessGrantsLocations("000000000000")
		assert.NotEmpty(t, locs)
	})

	t.Run("delete location", func(t *testing.T) {
		t.Parallel()
		b2 := s3control.NewInMemoryBackend()
		l := b2.CreateAccessGrantsLocation("000000000000", "s3://b/", "arn:test")
		require.NoError(t, b2.DeleteAccessGrantsLocation("000000000000", l.AccessGrantsLocationID))
	})

	// "delete location rejected while grants exist" and "delete location
	// succeeds once grants are gone" lock in DeleteAccessGrantsLocation's
	// own doc comment: "You can only delete a location registration ... if
	// there are no grants associated with this location" -- previously
	// unenforced, so gopherstack silently deleted a location out from under
	// live grants that still pointed at it.
	t.Run("delete location rejected while grants exist", func(t *testing.T) {
		t.Parallel()
		b2 := s3control.NewInMemoryBackend()
		l := b2.CreateAccessGrantsLocation("000000000000", "s3://b/", "arn:test")
		_, err := b2.CreateAccessGrant("000000000000", l.AccessGrantsLocationID, "IAMUser", "arn:test", "READ", "")
		require.NoError(t, err)

		err = b2.DeleteAccessGrantsLocation("000000000000", l.AccessGrantsLocationID)
		require.Error(t, err)
		require.ErrorIs(t, err, s3control.ErrValidation)

		_, getErr := b2.GetAccessGrantsLocation("000000000000", l.AccessGrantsLocationID)
		require.NoError(t, getErr, "location must survive a rejected delete")
	})

	t.Run("delete location succeeds once grants are gone", func(t *testing.T) {
		t.Parallel()
		b2 := s3control.NewInMemoryBackend()
		l := b2.CreateAccessGrantsLocation("000000000000", "s3://b/", "arn:test")
		grant, err := b2.CreateAccessGrant(
			"000000000000", l.AccessGrantsLocationID, "IAMUser", "arn:test", "READ", "",
		)
		require.NoError(t, err)

		require.Error(t, b2.DeleteAccessGrantsLocation("000000000000", l.AccessGrantsLocationID))

		require.NoError(t, b2.DeleteAccessGrant("000000000000", grant.AccessGrantID))
		require.NoError(t, b2.DeleteAccessGrantsLocation("000000000000", l.AccessGrantsLocationID))
	})

	// "delete location cascade cleans tags" locks in the ghost-map-row fix:
	// DeleteAccessGrantsLocation previously left generic resource tags
	// behind forever after the location row itself was removed.
	t.Run("delete location cascade cleans tags", func(t *testing.T) {
		t.Parallel()
		b2 := s3control.NewInMemoryBackend()
		l := b2.CreateAccessGrantsLocation("000000000000", "s3://b/", "arn:test")
		b2.TagResource(l.AccessGrantsLocationArn, map[string]string{"env": "test"})

		require.NoError(t, b2.DeleteAccessGrantsLocation("000000000000", l.AccessGrantsLocationID))

		assert.Empty(t, b2.ListTagsForResource(l.AccessGrantsLocationArn), "tags must not survive delete")
	})
}

func TestGetDataAccess(t *testing.T) {
	t.Parallel()
	b := s3control.NewInMemoryBackend()
	b.CreateAccessGrantsInstance("000000000000", "")

	url, err := b.GetDataAccess("000000000000", "s3://bucket/prefix/", "READ")
	require.NoError(t, err)
	assert.NotEmpty(t, url)
}

func TestHTTP_GetAccessGrantsInstance(t *testing.T) {
	t.Parallel()
	b := s3control.NewInMemoryBackend()
	h := s3control.NewHandler(b)
	b.CreateAccessGrantsInstance("000000000000", "")

	resp := doS3ControlNewOpRequest(
		t,
		h,
		http.MethodGet,
		"/v20180820/accessgrantsinstance",
		"000000000000",
		"",
	)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestCreateAccessGrantsInstance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		accountID        string
		body             string
		wantBodyContains string
		wantStatus       int
	}{
		{
			name:      "creates_instance_with_identity_center_arn",
			accountID: "123456789012",
			body: `<CreateAccessGrantsInstanceRequest>
<IdentityCenterArn>arn:aws:sso:::instance/ssoins-abc</IdentityCenterArn>
</CreateAccessGrantsInstanceRequest>`,
			wantStatus:       http.StatusOK,
			wantBodyContains: "AccessGrantsInstanceArn",
		},
		{
			name:             "creates_instance_without_identity_center",
			accountID:        "000000000000",
			body:             `<CreateAccessGrantsInstanceRequest></CreateAccessGrantsInstanceRequest>`,
			wantStatus:       http.StatusOK,
			wantBodyContains: "AccessGrantsInstanceId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			rec := doS3ControlNewOpRequest(
				t,
				h,
				http.MethodPost,
				"/v20180820/accessgrantsinstance",
				tt.accountID,
				tt.body,
			)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantBodyContains != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContains)
			}
		})
	}
}

func TestAssociateAccessGrantsIdentityCenter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		accountID  string
		body       string
		wantStatus int
	}{
		{
			name:      "associates_identity_center",
			accountID: "123456789012",
			body: `<AssociateAccessGrantsIdentityCenterRequest>
<IdentityCenterArn>arn:aws:sso:::instance/ssoins-xyz</IdentityCenterArn>
</AssociateAccessGrantsIdentityCenterRequest>`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "associates_with_empty_body",
			accountID:  "000000000000",
			body:       "",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			rec := doS3ControlNewOpRequest(
				t,
				h,
				http.MethodPost,
				"/v20180820/accessgrantsinstance/identitycenter",
				tt.accountID,
				tt.body,
			)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestCreateAccessGrant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		accountID        string
		body             string
		wantBodyContains string
		wantStatus       int
	}{
		{
			name:      "creates_access_grant",
			accountID: "123456789012",
			body: `<CreateAccessGrantRequest>
<AccessGrantsLocationId>default</AccessGrantsLocationId>
<Permission>READ</Permission>
<Grantee>
<GranteeType>IAM</GranteeType>
<GranteeIdentifier>arn:aws:iam::123456789012:user/test-user</GranteeIdentifier>
</Grantee>
</CreateAccessGrantRequest>`,
			wantStatus:       http.StatusOK,
			wantBodyContains: "AccessGrantArn",
		},
		{
			name:      "creates_access_grant_with_application_arn",
			accountID: "000000000000",
			body: `<CreateAccessGrantRequest>
<AccessGrantsLocationId>location-1</AccessGrantsLocationId>
<Permission>READWRITE</Permission>
<Grantee>
<GranteeType>DIRECTORY_USER</GranteeType>
<GranteeIdentifier>user-id-123</GranteeIdentifier>
</Grantee>
<ApplicationArn>arn:aws:sso::000000000000:application/app-123</ApplicationArn>
</CreateAccessGrantRequest>`,
			wantStatus:       http.StatusOK,
			wantBodyContains: "AccessGrantId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			rec := doS3ControlNewOpRequest(
				t,
				h,
				http.MethodPost,
				"/v20180820/accessgrantsinstance/grant",
				tt.accountID,
				tt.body,
			)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantBodyContains != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContains)
			}
		})
	}
}

func TestCreateAccessGrantsLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		accountID        string
		body             string
		wantBodyContains string
		wantStatus       int
	}{
		{
			name:      "creates_location_with_scope_and_role",
			accountID: "123456789012",
			body: `<CreateAccessGrantsLocationRequest>
<LocationScope>s3://my-bucket/prefix/</LocationScope>
<IAMRoleArn>arn:aws:iam::123456789012:role/S3AccessGrantsRole</IAMRoleArn>
</CreateAccessGrantsLocationRequest>`,
			wantStatus:       http.StatusOK,
			wantBodyContains: "AccessGrantsLocationArn",
		},
		{
			name:       "empty_body_missing_role_rejected",
			accountID:  "000000000000",
			body:       `<CreateAccessGrantsLocationRequest></CreateAccessGrantsLocationRequest>`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			rec := doS3ControlNewOpRequest(
				t,
				h,
				http.MethodPost,
				"/v20180820/accessgrantsinstance/location",
				tt.accountID,
				tt.body,
			)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantBodyContains != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContains)
			}
		})
	}
}

const (
	agLocationPath = "/v20180820/accessgrantsinstance/location"
	agLocationXML  = `<CreateAccessGrantsLocationRequest>` +
		`<LocationScope>s3://</LocationScope>` +
		`<IAMRoleArn>arn:aws:iam::000000000000:role/MyRole</IAMRoleArn>` +
		`</CreateAccessGrantsLocationRequest>`
	agLocationNoRoleXML = `<CreateAccessGrantsLocationRequest>` +
		`<LocationScope>s3://</LocationScope>` +
		`</CreateAccessGrantsLocationRequest>`
	agLocationEmptyRoleXML = `<CreateAccessGrantsLocationRequest>` +
		`<LocationScope>s3://</LocationScope>` +
		`<IAMRoleArn></IAMRoleArn>` +
		`</CreateAccessGrantsLocationRequest>`
)

// TestCreateAccessGrantsLocation_RequiresIAMRoleArn verifies that
// CreateAccessGrantsLocation rejects requests with a missing or empty
// IAMRoleArn. Real AWS returns 400 for this case; the emulator previously
// silently stored the location with an empty role ARN.
func TestCreateAccessGrantsLocation_RequiresIAMRoleArn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "absent_iam_role_arn_rejected",
			body:     agLocationNoRoleXML,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty_iam_role_arn_rejected",
			body:     agLocationEmptyRoleXML,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "valid_iam_role_arn_accepted",
			body:     agLocationXML,
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			rec := doS3Request(t, h, http.MethodPost, agLocationPath, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code,
				"CreateAccessGrantsLocation status for case %q", tt.name)
		})
	}
}

func TestListAccessGrantsInstances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		createAGI bool
		wantCode  int
		wantCount int
	}{
		{
			name:      "no_instance_returns_empty",
			createAGI: false,
			wantCode:  http.StatusOK,
			wantCount: 0,
		},
		{
			name:      "created_instance_returned",
			createAGI: true,
			wantCode:  http.StatusOK,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			if tt.createAGI {
				h.Backend.AddAccessGrantsInstanceInternal("acct1", "")
			}

			rec := doS3Request(t, h, http.MethodGet, "/v20180820/accessgrantsinstances", "")
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var out struct {
					XMLName               xml.Name `xml:"ListAccessGrantsInstancesResult"`
					AccessGrantsInstances []struct {
						AccessGrantsInstanceID string `xml:"AccessGrantsInstanceId"`
					} `xml:"AccessGrantsInstancesList>AccessGrantsInstance"`
				}
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))
				assert.Len(t, out.AccessGrantsInstances, tt.wantCount)
			}
		})
	}
}

func TestListAccessGrants_Pagination(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	b.AddAccessGrantsInstanceInternal("acct1", "")
	loc := b.CreateAccessGrantsLocation("acct1", "s3://", "arn:aws:iam::123456789012:role/role")
	for range 4 {
		b.AddAccessGrantInternal("acct1", loc.AccessGrantsLocationID, "IAM", "arn:aws:iam::123456789012:user/u", "READ")
	}
	h := s3control.NewHandler(b)

	tests := []struct {
		path          string
		name          string
		wantLen       int
		wantNextToken bool
	}{
		{
			name:          "no_limit_returns_all",
			path:          "/v20180820/accessgrantsinstance/grants",
			wantLen:       4,
			wantNextToken: false,
		},
		{
			name:          "page1_two_items",
			path:          "/v20180820/accessgrantsinstance/grants?maxResults=2",
			wantLen:       2,
			wantNextToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doS3Request(t, h, http.MethodGet, tt.path, "")
			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				XMLName      xml.Name `xml:"ListAccessGrantsResult"`
				NextToken    string   `xml:"NextToken"`
				AccessGrants []struct {
					AccessGrantID string `xml:"AccessGrantId"`
				} `xml:"AccessGrantsList>AccessGrant"`
			}
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))
			assert.Len(t, out.AccessGrants, tt.wantLen)
			if tt.wantNextToken {
				assert.NotEmpty(t, out.NextToken)
			} else {
				assert.Empty(t, out.NextToken)
			}
		})
	}
}

func TestListAccessGrantsLocations_Pagination(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	b.AddAccessGrantsInstanceInternal("acct1", "")
	for i := range 4 {
		b.CreateAccessGrantsLocation(
			"acct1",
			fmt.Sprintf("s3://bucket-%d/", i),
			"arn:aws:iam::123456789012:role/role",
		)
	}
	h := s3control.NewHandler(b)

	tests := []struct {
		path          string
		name          string
		wantLen       int
		wantNextToken bool
	}{
		{
			name:          "no_limit_returns_all",
			path:          "/v20180820/accessgrantsinstance/locations",
			wantLen:       4,
			wantNextToken: false,
		},
		{
			name:          "page1_two_items",
			path:          "/v20180820/accessgrantsinstance/locations?maxResults=2",
			wantLen:       2,
			wantNextToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doS3Request(t, h, http.MethodGet, tt.path, "")
			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				XMLName   xml.Name `xml:"ListAccessGrantsLocationsResult"`
				NextToken string   `xml:"NextToken"`
				Locations []struct {
					AccessGrantsLocationID string `xml:"AccessGrantsLocationId"`
				} `xml:"AccessGrantsLocationsList>AccessGrantsLocation"`
			}
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))
			assert.Len(t, out.Locations, tt.wantLen)
			if tt.wantNextToken {
				assert.NotEmpty(t, out.NextToken)
			} else {
				assert.Empty(t, out.NextToken)
			}
		})
	}
}

func TestCreateAccessGrant_RequiresPermission(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		permission string
		wantErr    bool
	}{
		{name: "empty_permission", permission: "", wantErr: true},
		{name: "read_permission", permission: "READ", wantErr: false},
		{name: "write_permission", permission: "WRITE", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3control.NewInMemoryBackend()
			grant, err := b.CreateAccessGrant("acc1", "loc1", "DIRECTORY_USER", "user@example.com", tt.permission, "")

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, s3control.ErrValidation)
				assert.Nil(t, grant)
			} else {
				require.NoError(t, err)
				require.NotNil(t, grant)
				assert.Equal(t, tt.permission, grant.Permission)
			}
		})
	}
}

func TestHandler_CreateAccessGrant_EmptyPermission(t *testing.T) {
	t.Parallel()

	h := newTestS3ControlHandler(t)
	body := `<CreateAccessGrantRequest>
<AccessGrantsLocationId>loc-1</AccessGrantsLocationId>
<Grantee><GranteeType>DIRECTORY_USER</GranteeType><GranteeIdentifier>user@example.com</GranteeIdentifier></Grantee>
</CreateAccessGrantRequest>`

	rec := doS3ControlNewOpRequest(t, h, http.MethodPost, "/v20180820/accessgrantsinstance/grant", "123456789012", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestAccessGrantsResponseWireShape asserts the literal nested XML envelope
// (not substrings) for GetAccessGrant and ListCallerAccessGrants against
// aws-sdk-go-v2/service/s3control's deserializers.go: GetAccessGrantOutput
// carries AccessGrantsLocationId/GrantScope/ApplicationArn/CreatedAt
// alongside AccessGrantId/AccessGrantArn/Permission/Grantee; and
// ListCallerAccessGrantsOutput wraps its list under "CallerAccessGrantsList"
// (not "AccessGrantsList"), with entries (ListCallerAccessGrantsEntry)
// carrying no AccessGrantId field.
func TestAccessGrantsResponseWireShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T)
		name string
	}{
		{
			name: "get_access_grant_includes_location_scope_application_created",
			run: func(t *testing.T) {
				t.Helper()

				b := s3control.NewInMemoryBackend()
				b.CreateAccessGrantsInstance("000000000000", "")
				loc := b.CreateAccessGrantsLocation(
					"000000000000", "s3://bucket/", "arn:aws:iam::000000000000:role/test",
				)
				grant, err := b.CreateAccessGrant(
					"000000000000", loc.AccessGrantsLocationID,
					"IAMUser", "arn:aws:iam::000000000000:user/test", "READ",
					"arn:aws:sso::000000000000:application/app-1",
				)
				require.NoError(t, err)

				h := s3control.NewHandler(b)
				rec := doS3ControlNewOpRequest(
					t, h, http.MethodGet,
					"/v20180820/accessgrantsinstance/grant/"+grant.AccessGrantID,
					"000000000000", "",
				)
				require.Equal(t, http.StatusOK, rec.Code)

				var out struct {
					XMLName                xml.Name `xml:"GetAccessGrantResult"`
					AccessGrantID          string   `xml:"AccessGrantId"`
					AccessGrantArn         string   `xml:"AccessGrantArn"`
					AccessGrantsLocationID string   `xml:"AccessGrantsLocationId"`
					GrantScope             string   `xml:"GrantScope"`
					Permission             string   `xml:"Permission"`
					ApplicationArn         string   `xml:"ApplicationArn"`
					CreatedAt              string   `xml:"CreatedAt"`
					Grantee                struct {
						GranteeType       string `xml:"GranteeType"`
						GranteeIdentifier string `xml:"GranteeIdentifier"`
					} `xml:"Grantee"`
				}
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))

				assert.Equal(t, grant.AccessGrantID, out.AccessGrantID)
				assert.Equal(t, loc.AccessGrantsLocationID, out.AccessGrantsLocationID)
				assert.NotEmpty(t, out.GrantScope)
				assert.Equal(t, "READ", out.Permission)
				assert.Equal(t, "arn:aws:sso::000000000000:application/app-1", out.ApplicationArn)
				assert.NotEmpty(t, out.CreatedAt)
				assert.Equal(t, "IAMUser", out.Grantee.GranteeType)
				assert.Equal(t, "arn:aws:iam::000000000000:user/test", out.Grantee.GranteeIdentifier)
			},
		},
		{
			name: "list_caller_access_grants_uses_CallerAccessGrantsList_envelope_no_grant_id",
			run: func(t *testing.T) {
				t.Helper()

				b := s3control.NewInMemoryBackend()
				b.CreateAccessGrantsInstance("000000000000", "")
				loc := b.CreateAccessGrantsLocation(
					"000000000000", "s3://bucket/", "arn:aws:iam::000000000000:role/test",
				)
				_, err := b.CreateAccessGrant(
					"000000000000", loc.AccessGrantsLocationID,
					"IAMUser", "arn:aws:iam::000000000000:user/test", "READ",
					"arn:aws:sso::000000000000:application/app-1",
				)
				require.NoError(t, err)

				h := s3control.NewHandler(b)
				rec := doS3ControlNewOpRequest(
					t, h, http.MethodGet,
					"/v20180820/accessgrantsinstance/caller/grants",
					"000000000000", "",
				)
				require.Equal(t, http.StatusOK, rec.Code)

				// Assert the literal nested envelope: CallerAccessGrantsList,
				// not AccessGrantsList. Decoding into a struct scoped to the
				// wrong wrapper key would silently yield a zero-length slice,
				// so this also functions as a substring check via
				// assert.Contains below (belt and suspenders).
				assert.Contains(t, rec.Body.String(), "<CallerAccessGrantsList>")
				assert.NotContains(t, rec.Body.String(), "<AccessGrantsList>")
				assert.NotContains(t, rec.Body.String(), "<AccessGrantId>",
					"ListCallerAccessGrantsEntry has no AccessGrantId field in the real SDK")

				var out struct {
					XMLName      xml.Name `xml:"ListCallerAccessGrantsResult"`
					AccessGrants []struct {
						Permission     string `xml:"Permission"`
						GrantScope     string `xml:"GrantScope"`
						ApplicationArn string `xml:"ApplicationArn"`
					} `xml:"CallerAccessGrantsList>AccessGrant"`
				}
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))

				require.Len(t, out.AccessGrants, 1)
				assert.Equal(t, "READ", out.AccessGrants[0].Permission)
				assert.NotEmpty(t, out.AccessGrants[0].GrantScope)
				assert.Equal(t, "arn:aws:sso::000000000000:application/app-1", out.AccessGrants[0].ApplicationArn)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

// TestHandler_DeleteAccessGrantsInstance_Precondition locks in the
// gopherstack-tir4 fix for DeleteAccessGrantsInstance's real-API
// preconditions -- "You must first delete the access grants and locations
// before S3 Access Grants can delete the instance" and (separately) the
// Identity Center dissociation requirement -- at the HTTP layer: a real
// client attempting any of these now gets a 400 BadRequestException instead
// of a silent success.
func TestHandler_DeleteAccessGrantsInstance_Precondition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(b *s3control.InMemoryBackend)
		name     string
		wantCode int
	}{
		{
			name:     "empty_instance_deletes",
			setup:    func(*s3control.InMemoryBackend) {},
			wantCode: http.StatusNoContent,
		},
		{
			name: "instance_with_location_rejected",
			setup: func(b *s3control.InMemoryBackend) {
				b.CreateAccessGrantsLocation("000000000000", "s3://bucket/", "arn:aws:iam::000000000000:role/r")
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "instance_with_grant_rejected",
			setup: func(b *s3control.InMemoryBackend) {
				loc := b.CreateAccessGrantsLocation("000000000000", "s3://bucket/", "arn:aws:iam::000000000000:role/r")
				_, err := b.CreateAccessGrant(
					"000000000000", loc.AccessGrantsLocationID, "IAMUser", "arn:test", "READ", "",
				)
				require.NoError(t, err)
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "instance_with_identity_center_rejected",
			setup: func(b *s3control.InMemoryBackend) {
				b.AssociateAccessGrantsIdentityCenter("000000000000", "arn:aws:sso:::instance/ssoins-test")
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3control.NewInMemoryBackend()
			b.CreateAccessGrantsInstance("000000000000", "")
			tt.setup(b)
			h := s3control.NewHandler(b)

			rec := doS3ControlNewOpRequest(
				t, h, http.MethodDelete, "/v20180820/accessgrantsinstance", "000000000000", "",
			)
			assert.Equal(t, tt.wantCode, rec.Code)
			if tt.wantCode == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "BadRequestException")
			}
		})
	}
}

// TestListAccessGrants_Filters locks in ListAccessGrants's real query
// filters (s3control@v1.73.4 api_op_ListAccessGrants.go serializers.go:
// awsRestxml_serializeOpHttpBindingsListAccessGrantsInput -- wire keys
// "grantscope" and "granteeidentifier"). The handler previously read
// "locationscope" (ListAccessGrantsLocations's own filter key, not
// ListAccessGrants's) so a real client's grantscope filter was silently
// ignored, and granteeidentifier was never read at all.
func TestListAccessGrants_Filters(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	b.AddAccessGrantsInstanceInternal("acct1", "")
	locA := b.CreateAccessGrantsLocation("acct1", "s3://bucket-a", "arn:aws:iam::123456789012:role/role")
	locB := b.CreateAccessGrantsLocation("acct1", "s3://bucket-b", "arn:aws:iam::123456789012:role/role")

	grantA := b.AddAccessGrantInternal(
		"acct1", locA.AccessGrantsLocationID, "IAM", "arn:aws:iam::123456789012:user/ua", "READ",
	)
	b.AddAccessGrantInternal(
		"acct1", locB.AccessGrantsLocationID, "IAM", "arn:aws:iam::123456789012:user/ub", "WRITE",
	)
	h := s3control.NewHandler(b)

	type listAccessGrantsResult struct {
		XMLName      xml.Name `xml:"ListAccessGrantsResult"`
		AccessGrants []struct {
			AccessGrantID string `xml:"AccessGrantId"`
		} `xml:"AccessGrantsList>AccessGrant"`
	}

	tests := []struct {
		name    string
		path    string
		wantIDs []string
	}{
		{
			name:    "grantscope filter",
			path:    "/v20180820/accessgrantsinstance/grants?grantscope=" + grantA.GrantScope,
			wantIDs: []string{grantA.AccessGrantID},
		},
		{
			name: "granteeidentifier filter",
			path: "/v20180820/accessgrantsinstance/grants?granteeidentifier=" +
				"arn:aws:iam::123456789012:user/ua",
			wantIDs: []string{grantA.AccessGrantID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doS3Request(t, h, http.MethodGet, tt.path, "")
			require.Equal(t, http.StatusOK, rec.Code)

			var out listAccessGrantsResult
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))

			gotIDs := make([]string, 0, len(out.AccessGrants))
			for _, g := range out.AccessGrants {
				gotIDs = append(gotIDs, g.AccessGrantID)
			}
			assert.Equal(t, tt.wantIDs, gotIDs)
		})
	}
}

// TestListAccessGrantsLocations_LocationScopeFilter locks in the
// locationscope query filter (s3control@v1.73.4
// api_op_ListAccessGrantsLocations.go's LocationScope, wire query key
// "locationscope" per serializers.go:5564-5566) -- previously never read
// by handleListAccessGrantsLocations, so every caller got every location
// regardless of the filter they sent.
func TestListAccessGrantsLocations_LocationScopeFilter(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	b.AddAccessGrantsInstanceInternal("acct1", "")
	wantLoc := b.CreateAccessGrantsLocation("acct1", "s3://bucket-a", "arn:aws:iam::123456789012:role/role")
	b.CreateAccessGrantsLocation("acct1", "s3://bucket-b", "arn:aws:iam::123456789012:role/role")
	h := s3control.NewHandler(b)

	rec := doS3Request(
		t, h, http.MethodGet, "/v20180820/accessgrantsinstance/locations?locationscope=s3://bucket-a", "",
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		XMLName   xml.Name `xml:"ListAccessGrantsLocationsResult"`
		Locations []struct {
			AccessGrantsLocationID string `xml:"AccessGrantsLocationId"`
		} `xml:"AccessGrantsLocationsList>AccessGrantsLocation"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.Locations, 1)
	assert.Equal(t, wantLoc.AccessGrantsLocationID, out.Locations[0].AccessGrantsLocationID)
}
