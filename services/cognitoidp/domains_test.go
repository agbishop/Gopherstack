package cognitoidp_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cognitoidp"
)

func TestDomain_CreateDescribeDelete(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolID, _ := setupHandlerPoolAndClient(t, h, "domain-pool")

	createRec := doCognitoRequest(t, h, "CreateUserPoolDomain", map[string]any{
		"UserPoolId": poolID,
		"Domain":     "my-test-domain",
	})
	require.Equal(t, http.StatusOK, createRec.Code, createRec.Body.String())

	descRec := doCognitoRequest(t, h, "DescribeUserPoolDomain", map[string]any{
		"Domain": "my-test-domain",
	})
	require.Equal(t, http.StatusOK, descRec.Code, descRec.Body.String())

	delRec := doCognitoRequest(t, h, "DeleteUserPoolDomain", map[string]any{
		"UserPoolId": poolID,
		"Domain":     "my-test-domain",
	})
	require.Equal(t, http.StatusOK, delRec.Code, delRec.Body.String())
}

func TestCreateUserPoolDomain_ManagedDomain_EmptyCloudFrontDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		certArn string
		wantCF  bool
	}{
		{
			name:    "managed_domain_no_cert",
			certArn: "",
			wantCF:  false,
		},
		{
			name:    "custom_domain_with_cert",
			certArn: "arn:aws:acm:us-east-1:123456789012:certificate/abc123",
			wantCF:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			poolID, _ := setupHandlerPoolAndClient(t, h, "domain-audit-"+tt.name+"-pool")

			body := map[string]any{
				"UserPoolId": poolID,
				"Domain":     "audit-domain-" + tt.name,
			}
			if tt.certArn != "" {
				body["CustomDomainConfig"] = map[string]any{
					"CertificateArn": tt.certArn,
				}
			}

			rec := doCognitoRequest(t, h, "CreateUserPoolDomain", body)
			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				CloudFrontDomain string `json:"CloudFrontDomain,omitempty"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

			if tt.wantCF {
				assert.Contains(
					t,
					out.CloudFrontDomain,
					"cloudfront.net",
					"custom domain must return CloudFront domain",
				)
			} else {
				assert.Empty(
					t,
					out.CloudFrontDomain,
					"managed domain must return empty CloudFrontDomain in create response",
				)
			}
		})
	}
}

func TestUserPoolDomain_Managed(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolID, _ := setupHandlerPoolAndClient(t, h, "domain-managed-pool")

	rec := doCognitoRequest(t, h, "CreateUserPoolDomain", map[string]any{
		"UserPoolId": poolID,
		"Domain":     "myapp-managed",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		CloudFrontDomain string `json:"CloudFrontDomain,omitempty"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	// Managed domains: AWS returns empty CloudFrontDomain (no CloudFront distribution).
	assert.Empty(t, out.CloudFrontDomain)
}

func TestUserPoolDomain_Custom_WithCertArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolID, _ := setupHandlerPoolAndClient(t, h, "domain-custom-pool")

	certArn := "arn:aws:acm:us-east-1:123456789012:certificate/abc123"

	rec := doCognitoRequest(t, h, "CreateUserPoolDomain", map[string]any{
		"UserPoolId": poolID,
		"Domain":     "auth.mycompany.com",
		"CustomDomainConfig": map[string]any{
			"CertificateArn": certArn,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createOut struct {
		CloudFrontDomain string `json:"CloudFrontDomain,omitempty"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createOut))
	assert.Contains(t, createOut.CloudFrontDomain, "cloudfront.net")

	// Describe should return the domain with status.
	rec = doCognitoRequest(t, h, "DescribeUserPoolDomain", map[string]any{
		"Domain": "auth.mycompany.com",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var descOut struct {
		DomainDescription *struct {
			CustomDomainConfig *struct {
				CertificateArn string `json:"CertificateArn,omitempty"`
			} `json:"CustomDomainConfig,omitempty"`
			Domain                 string `json:"Domain,omitempty"`
			UserPoolID             string `json:"UserPoolId,omitempty"`
			Status                 string `json:"Status,omitempty"`
			CloudFrontDistribution string `json:"CloudFrontDistribution,omitempty"`
		} `json:"DomainDescription"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descOut))
	require.NotNil(t, descOut.DomainDescription)
	assert.Equal(t, "auth.mycompany.com", descOut.DomainDescription.Domain)
	assert.Equal(t, "ACTIVE", descOut.DomainDescription.Status)

	// AWS echoes CustomDomainConfig.CertificateArn back for a custom domain -- e.g. the
	// Terraform AWS provider reads this to detect drift on the configured certificate.
	require.NotNil(t, descOut.DomainDescription.CustomDomainConfig, "custom domain must echo CustomDomainConfig")
	assert.Equal(t, certArn, descOut.DomainDescription.CustomDomainConfig.CertificateArn)
}

// TestUserPoolDomain_Managed_NoCustomDomainConfig proves a managed (non-custom, no ACM
// certificate) domain's DescribeUserPoolDomain omits CustomDomainConfig entirely,
// matching AWS -- only domains with a certificate get one.
func TestUserPoolDomain_Managed_NoCustomDomainConfig(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolID, _ := setupHandlerPoolAndClient(t, h, "domain-managed-no-cdc-pool")

	rec := doCognitoRequest(t, h, "CreateUserPoolDomain", map[string]any{
		"UserPoolId": poolID,
		"Domain":     "myapp-managed-no-cdc",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doCognitoRequest(t, h, "DescribeUserPoolDomain", map[string]any{
		"Domain": "myapp-managed-no-cdc",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var descOut struct {
		DomainDescription *struct {
			CustomDomainConfig *struct {
				CertificateArn string `json:"CertificateArn,omitempty"`
			} `json:"CustomDomainConfig,omitempty"`
		} `json:"DomainDescription"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descOut))
	require.NotNil(t, descOut.DomainDescription)
	assert.Nil(t, descOut.DomainDescription.CustomDomainConfig)
}

func TestUserPoolDomain_Update_WithCertArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolID, _ := setupHandlerPoolAndClient(t, h, "domain-update-pool")

	// Create managed domain first.
	rec := doCognitoRequest(t, h, "CreateUserPoolDomain", map[string]any{
		"UserPoolId": poolID,
		"Domain":     "auth-update.mycompany.com",
		"CustomDomainConfig": map[string]any{
			"CertificateArn": "arn:aws:acm:us-east-1:123:certificate/old",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Update with new cert.
	rec = doCognitoRequest(t, h, "UpdateUserPoolDomain", map[string]any{
		"UserPoolId": poolID,
		"Domain":     "auth-update.mycompany.com",
		"CustomDomainConfig": map[string]any{
			"CertificateArn": "arn:aws:acm:us-east-1:123:certificate/new",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		CloudFrontDomain string `json:"CloudFrontDomain,omitempty"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Contains(t, out.CloudFrontDomain, "cloudfront.net")
}

type domainDescriptionWire struct {
	DomainDescription *struct {
		ManagedLoginVersion    *int32 `json:"ManagedLoginVersion,omitempty"`
		Domain                 string `json:"Domain,omitempty"`
		AWSAccountID           string `json:"AWSAccountId,omitempty"`
		S3Bucket               string `json:"S3Bucket,omitempty"`
		CloudFrontDistribution string `json:"CloudFrontDistribution,omitempty"`
	} `json:"DomainDescription"`
}

func TestUserPoolDomain_Describe_AWSAccountIdManagedLoginVersionS3Bucket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		requestBody map[string]any
		name        string
		wantMLV     int32
	}{
		{
			name:        "unset_defaults_to_hosted_ui_classic",
			requestBody: map[string]any{},
			wantMLV:     1,
		},
		{
			name:        "explicit_managed_login",
			requestBody: map[string]any{"ManagedLoginVersion": 2},
			wantMLV:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			poolID, _ := setupHandlerPoolAndClient(t, h, "domain-mlv-"+tt.name+"-pool")
			domain := "domain-mlv-" + tt.name

			body := map[string]any{"UserPoolId": poolID, "Domain": domain}
			maps.Copy(body, tt.requestBody)

			createRec := doCognitoRequest(t, h, "CreateUserPoolDomain", body)
			require.Equal(t, http.StatusOK, createRec.Code, createRec.Body.String())

			var createOut struct {
				ManagedLoginVersion *int32 `json:"ManagedLoginVersion,omitempty"`
			}
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
			require.NotNil(t, createOut.ManagedLoginVersion, "CreateUserPoolDomain must echo ManagedLoginVersion")
			assert.Equal(t, tt.wantMLV, *createOut.ManagedLoginVersion)

			descRec := doCognitoRequest(t, h, "DescribeUserPoolDomain", map[string]any{"Domain": domain})
			require.Equal(t, http.StatusOK, descRec.Code, descRec.Body.String())

			var out domainDescriptionWire
			require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &out))
			require.NotNil(t, out.DomainDescription)

			assert.NotEmpty(t, out.DomainDescription.AWSAccountID)
			assert.NotEmpty(t, out.DomainDescription.S3Bucket)
			require.NotNil(t, out.DomainDescription.ManagedLoginVersion)
			assert.Equal(t, tt.wantMLV, *out.DomainDescription.ManagedLoginVersion)
		})
	}
}

func TestUserPoolDomain_UpdateManagedLoginVersion_OmittedRetainsPrior(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolID, _ := setupHandlerPoolAndClient(t, h, "domain-mlv-update-pool")

	createRec := doCognitoRequest(t, h, "CreateUserPoolDomain", map[string]any{
		"UserPoolId":          poolID,
		"Domain":              "domain-mlv-update",
		"ManagedLoginVersion": 2,
	})
	require.Equal(t, http.StatusOK, createRec.Code, createRec.Body.String())

	updateRec := doCognitoRequest(t, h, "UpdateUserPoolDomain", map[string]any{
		"UserPoolId": poolID,
		"Domain":     "domain-mlv-update",
	})
	require.Equal(t, http.StatusOK, updateRec.Code, updateRec.Body.String())

	var updateOut struct {
		ManagedLoginVersion *int32 `json:"ManagedLoginVersion,omitempty"`
	}
	require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updateOut))
	require.NotNil(t, updateOut.ManagedLoginVersion, "UpdateUserPoolDomain must echo ManagedLoginVersion")
	assert.EqualValues(t, 2, *updateOut.ManagedLoginVersion, "omitting the field on update must not reset it")
}

func TestUserPoolDomain_Backend_Direct(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("domain-direct-pool")
	require.NoError(t, err)

	// Managed domain (no cert).
	d, err := b.CreateUserPoolDomainFull(pool.ID, "my-managed-domain", "", 0)
	require.NoError(t, err)
	assert.Contains(t, d.CloudFrontDistribution, "amazoncognito.com")
	assert.Empty(t, d.CertificateArn)
	assert.EqualValues(t, 1, d.ManagedLoginVersion)
	assert.NotEmpty(t, d.S3Bucket)

	// Custom domain with cert.
	certArn := "arn:aws:acm:us-east-1:123:certificate/xyz"
	d2, err := b.CreateUserPoolDomainFull(pool.ID, "auth.example.com", certArn, 2)
	require.NoError(t, err)
	assert.Contains(t, d2.CloudFrontDistribution, "cloudfront.net")
	assert.Equal(t, certArn, d2.CertificateArn)
	assert.EqualValues(t, 2, d2.ManagedLoginVersion)

	// Update cert.
	newCert := "arn:aws:acm:us-east-1:123:certificate/new"
	updated, err := b.UpdateUserPoolDomainFull(pool.ID, "auth.example.com", newCert, 0)
	require.NoError(t, err)
	assert.Contains(t, updated.CloudFrontDistribution, "cloudfront.net")
	assert.EqualValues(t, 2, updated.ManagedLoginVersion, "omitted ManagedLoginVersion on update leaves it unchanged")

	// Delete.
	require.NoError(t, b.DeleteUserPoolDomain(pool.ID, "auth.example.com"))
	d3 := b.FindUserPoolDomain("auth.example.com")
	assert.Nil(t, d3)
}

// TestDeleteUserPoolDomain_RecoversOrphanedDomain covers gopherstack-tq5q's
// recovery path: a domain whose recorded UserPoolID no longer resolves to a
// live pool (data from before DeleteUserPool started refusing to delete pools
// with an attached domain) must still be cleanable, keyed off the domain's
// own recorded UserPoolID -- otherwise the domain name is blocked forever.
func TestDeleteUserPoolDomain_RecoversOrphanedDomain(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	const orphanPoolID = "us-east-1_gone"

	b.AddUserPoolDomainInternal(&cognitoidp.UserPoolDomain{
		Domain:     "orphaned-domain",
		UserPoolID: orphanPoolID,
		Status:     "ACTIVE",
	})
	require.NotNil(t, b.FindUserPoolDomain("orphaned-domain"))

	require.NoError(t, b.DeleteUserPoolDomain(orphanPoolID, "orphaned-domain"))
	assert.Nil(t, b.FindUserPoolDomain("orphaned-domain"))

	// Recovery: the domain name is immediately usable again by a brand new pool.
	newPool, err := b.CreateUserPool("recovery-pool")
	require.NoError(t, err)
	_, err = b.CreateUserPoolDomain(newPool.ID, "orphaned-domain")
	require.NoError(t, err)
}

// TestDeleteUserPoolDomain_MismatchedOrphanClaimStillRejected proves the
// relaxed pool-existence guard only recovers a domain for the UserPoolID it
// was actually created under -- an arbitrary nonexistent pool ID cannot claim
// (and delete) a domain that belongs to a different orphaned pool.
func TestDeleteUserPoolDomain_MismatchedOrphanClaimStillRejected(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	b.AddUserPoolDomainInternal(&cognitoidp.UserPoolDomain{
		Domain:     "orphaned-domain-2",
		UserPoolID: "us-east-1_real-owner",
		Status:     "ACTIVE",
	})

	err := b.DeleteUserPoolDomain("us-east-1_someone-else", "orphaned-domain-2")
	require.ErrorIs(t, err, cognitoidp.ErrUserPoolNotFound)
	assert.NotNil(t, b.FindUserPoolDomain("orphaned-domain-2"))
}

func TestDomain_FullCRUDLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolID, _ := setupHandlerPoolAndClient(t, h, "domain-crud-pool")

	// Create
	rec := doCognitoRequest(t, h, "CreateUserPoolDomain", map[string]any{
		"UserPoolId": poolID,
		"Domain":     "myapp",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp struct {
		CloudFrontDomain string `json:"CloudFrontDomain,omitempty"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	// Managed domains: AWS returns empty CloudFrontDomain in the create response.
	assert.Empty(t, createResp.CloudFrontDomain)

	// Describe
	rec = doCognitoRequest(t, h, "DescribeUserPoolDomain", map[string]any{
		"Domain": "myapp",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp struct {
		DomainDescription struct {
			Domain string `json:"Domain,omitempty"`
			Status string `json:"Status,omitempty"`
		} `json:"DomainDescription"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "myapp", descResp.DomainDescription.Domain)
	assert.Equal(t, "ACTIVE", descResp.DomainDescription.Status)

	// Update
	rec = doCognitoRequest(t, h, "UpdateUserPoolDomain", map[string]any{
		"UserPoolId": poolID,
		"Domain":     "myapp",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Delete
	rec = doCognitoRequest(t, h, "DeleteUserPoolDomain", map[string]any{
		"UserPoolId": poolID,
		"Domain":     "myapp",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Describe after delete — returns empty description (not an error per AWS behaviour)
	rec = doCognitoRequest(t, h, "DescribeUserPoolDomain", map[string]any{
		"Domain": "myapp",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var afterDeleteResp struct {
		DomainDescription struct {
			Domain string `json:"Domain,omitempty"`
			Status string `json:"Status,omitempty"`
		} `json:"DomainDescription"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &afterDeleteResp))
	assert.Empty(t, afterDeleteResp.DomainDescription.Domain)
}
