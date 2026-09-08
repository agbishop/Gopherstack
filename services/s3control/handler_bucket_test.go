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

// ---- Outposts Bucket ----

func TestOutpostsBucket(t *testing.T) {
	t.Parallel()

	t.Run("get bucket", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateBucket("000000000000", "my-bucket")
		bucket, err := b.GetBucket("my-bucket")
		require.NoError(t, err)
		assert.Equal(t, "my-bucket", bucket.Name)
	})

	t.Run("delete bucket", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateBucket("000000000000", "to-delete")
		require.NoError(t, b.DeleteBucket("to-delete"))
		_, err := b.GetBucket("to-delete")
		require.Error(t, err)
	})

	t.Run("bucket policy CRUD", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateBucket("000000000000", "policy-bucket")
		require.NoError(
			t,
			b.PutBucketPolicy("policy-bucket", `{"Version":"2012-10-17"}`),
		)
		policy, err := b.GetBucketPolicy("policy-bucket")
		require.NoError(t, err)
		assert.Contains(t, policy, "Version")
		require.NoError(t, b.DeleteBucketPolicy("policy-bucket"))
		policy2, _ := b.GetBucketPolicy("policy-bucket")
		assert.Empty(t, policy2)
	})

	t.Run("bucket tagging CRUD", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateBucket("000000000000", "tag-bucket")
		require.NoError(
			t,
			b.PutBucketTagging("tag-bucket", s3control.TagSet{"env": "prod"}),
		)
		tags, err := b.GetBucketTagging("tag-bucket")
		require.NoError(t, err)
		assert.Equal(t, "prod", tags["env"])
		require.NoError(t, b.DeleteBucketTagging("tag-bucket"))
	})

	t.Run("bucket versioning", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateBucket("000000000000", "ver-bucket")
		status, _ := b.GetBucketVersioning("ver-bucket")
		assert.Equal(t, "Suspended", status)
		require.NoError(t, b.PutBucketVersioning("ver-bucket", "Enabled"))
		status2, _ := b.GetBucketVersioning("ver-bucket")
		assert.Equal(t, "Enabled", status2)
	})

	t.Run("bucket lifecycle CRUD", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateBucket("000000000000", "lc-bucket")
		require.NoError(
			t,
			b.PutBucketLifecycleConfiguration("lc-bucket",
				"<LifecycleConfiguration/>",
			),
		)
		config, err := b.GetBucketLifecycleConfiguration("lc-bucket")
		require.NoError(t, err)
		assert.Contains(t, config, "Lifecycle")
		require.NoError(t, b.DeleteBucketLifecycleConfiguration("lc-bucket"))
	})

	t.Run("list regional buckets", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		b.CreateBucket("000000000000", "b1")
		b.CreateBucket("000000000000", "b2")
		buckets := b.ListRegionalBuckets()
		require.Len(t, buckets, 2)
	})

	// "delete bucket cascade cleans state" locks in the ghost-map-row fix:
	// DeleteBucket previously only removed the bucket row itself, leaving
	// its lifecycle, policy, tagging, versioning, replication, and generic
	// resource tags behind forever -- resurfacing on a delete/recreate
	// cycle under the same name.
	t.Run("delete bucket cascade cleans state", func(t *testing.T) {
		t.Parallel()
		b := s3control.NewInMemoryBackend()
		bkt := b.CreateBucket("000000000000", "cascade-bucket")
		require.NoError(t, b.PutBucketPolicy("cascade-bucket", `{"p":1}`))
		require.NoError(t, b.PutBucketTagging("cascade-bucket", s3control.TagSet{"env": "prod"}))
		require.NoError(t, b.PutBucketLifecycleConfiguration("cascade-bucket", "<Lifecycle/>"))
		require.NoError(t, b.PutBucketVersioning("cascade-bucket", "Enabled"))
		require.NoError(t, b.PutBucketReplication("cascade-bucket", "<Replication/>"))
		b.TagResource(bkt.BucketArn, map[string]string{"team": "infra"})

		require.NoError(t, b.DeleteBucket("cascade-bucket"))

		b.CreateBucket("000000000000", "cascade-bucket")

		policy, err := b.GetBucketPolicy("cascade-bucket")
		require.NoError(t, err)
		assert.Empty(t, policy, "policy must not survive delete")

		tags, err := b.GetBucketTagging("cascade-bucket")
		require.NoError(t, err)
		assert.Empty(t, tags, "tagging must not survive delete")

		lc, err := b.GetBucketLifecycleConfiguration("cascade-bucket")
		require.NoError(t, err)
		assert.Empty(t, lc, "lifecycle must not survive delete")

		v, err := b.GetBucketVersioning("cascade-bucket")
		require.NoError(t, err)
		assert.Equal(t, "Suspended", v, "versioning must reset, not survive delete")

		_, err = b.GetBucketReplication("cascade-bucket")
		require.Error(t, err, "replication must not survive delete")

		assert.Empty(t, b.ListTagsForResource(bkt.BucketArn), "generic tags must not survive delete")
	})
}

// TestHTTP_GetBucket locks in a gopherstack-tir4 finding: GetBucketOutput
// has no BucketArn or OutpostId field in the real SDK (confirmed against
// aws-sdk-go-v2/service/s3control's GetBucketOutput, whose only members are
// Bucket/CreationDate/PublicAccessBlockEnabled). A previous version of this
// handler fabricated a BucketArn element and mislabeled an internal HTTP
// Location-header path fragment as OutpostId.
func TestHTTP_GetBucket(t *testing.T) {
	t.Parallel()
	b := s3control.NewInMemoryBackend()
	h := s3control.NewHandler(b)
	b.CreateBucket("000000000000", "test-bucket")

	resp := doS3ControlNewOpRequest(
		t,
		h,
		http.MethodGet,
		"/v20180820/bucket/test-bucket",
		"000000000000",
		"",
	)
	assert.Equal(t, http.StatusOK, resp.Code)
	assert.NotContains(t, resp.Body.String(), "BucketArn")
	assert.NotContains(t, resp.Body.String(), "OutpostId")

	var out struct {
		XMLName xml.Name `xml:"GetBucketResult"`
		Bucket  string   `xml:"Bucket"`
	}
	require.NoError(t, xml.Unmarshal(resp.Body.Bytes(), &out))
	assert.Equal(t, "test-bucket", out.Bucket)
}

func TestHTTP_ListRegionalBuckets(t *testing.T) {
	t.Parallel()
	b := s3control.NewInMemoryBackend()
	h := s3control.NewHandler(b)
	b.CreateBucket("000000000000", "b1")

	resp := doS3ControlNewOpRequest(t, h, http.MethodGet, "/v20180820/bucket", "000000000000", "")
	assert.Equal(t, http.StatusOK, resp.Code)
}

// ---- Bucket Replication ----

func TestBucketReplication_PutGetDelete(t *testing.T) {
	t.Parallel()

	const accountID = "acct1"
	const bucketName = "mybucket"
	const replicationPath = "/v20180820/bucket/" + bucketName + "/replication"

	t.Run("put and get replication", func(t *testing.T) {
		t.Parallel()

		h := s3control.NewHandler(s3control.NewInMemoryBackend())
		h.Backend.CreateBucket(accountID, bucketName)

		putRec := doS3Request(t, h, http.MethodPut, replicationPath,
			`<ReplicationConfiguration><Rules>my-rule</Rules></ReplicationConfiguration>`)
		require.Equal(t, http.StatusOK, putRec.Code)

		getRec := doS3Request(t, h, http.MethodGet, replicationPath, "")
		require.Equal(t, http.StatusOK, getRec.Code)
		assert.Contains(t, getRec.Body.String(), "my-rule")
	})

	t.Run("get missing returns 404", func(t *testing.T) {
		t.Parallel()

		h := s3control.NewHandler(s3control.NewInMemoryBackend())
		h.Backend.CreateBucket(accountID, bucketName)

		rec := doS3Request(t, h, http.MethodGet, replicationPath, "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("delete replication", func(t *testing.T) {
		t.Parallel()

		h := s3control.NewHandler(s3control.NewInMemoryBackend())
		h.Backend.CreateBucket(accountID, bucketName)
		_ = doS3Request(t, h, http.MethodPut, replicationPath,
			`<ReplicationConfiguration><Rules>r</Rules></ReplicationConfiguration>`)

		delRec := doS3Request(t, h, http.MethodDelete, replicationPath, "")
		require.Equal(t, http.StatusNoContent, delRec.Code)

		getRec := doS3Request(t, h, http.MethodGet, replicationPath, "")
		assert.Equal(t, http.StatusNotFound, getRec.Code)
	})
}

func TestBackendBucketReplication(t *testing.T) {
	t.Parallel()

	t.Run("get missing returns error", func(t *testing.T) {
		t.Parallel()

		b := s3control.NewInMemoryBackend()
		b.CreateBucket("acct1", "bkt")

		_, err := b.GetBucketReplication("bkt")
		require.Error(t, err)
	})

	t.Run("delete missing is idempotent", func(t *testing.T) {
		t.Parallel()

		b := s3control.NewInMemoryBackend()
		b.CreateBucket("acct1", "bkt")
		err := b.DeleteBucketReplication("bkt")
		require.NoError(t, err)
	})

	// "delete requires bucket to exist" locks in the fix: PutBucketReplication/
	// GetBucketReplication/DeleteBucketReplication previously skipped the
	// bucket-existence check every sibling bucket sub-resource op enforces
	// (PutBucketPolicy, PutBucketTagging, PutBucketLifecycleConfiguration,
	// PutBucketVersioning), silently accepting replication config for a
	// bucket that was never created.
	t.Run("delete requires bucket to exist", func(t *testing.T) {
		t.Parallel()

		b := s3control.NewInMemoryBackend()
		require.Error(t, b.DeleteBucketReplication("no-such-bucket"))
	})

	t.Run("put requires bucket to exist", func(t *testing.T) {
		t.Parallel()

		b := s3control.NewInMemoryBackend()
		require.Error(t, b.PutBucketReplication("no-such-bucket", "<Rule/>"))
	})

	t.Run("get requires bucket to exist", func(t *testing.T) {
		t.Parallel()

		b := s3control.NewInMemoryBackend()
		_, err := b.GetBucketReplication("no-such-bucket")
		require.Error(t, err)
	})
}

func TestBucketReplication_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		bucket   string
		rules    string
		wantGet  string
		wantCode int
	}{
		{
			name:     "put_and_get_rules",
			bucket:   "my-bucket",
			rules:    "<Rule><ID>rule1</ID><Status>Enabled</Status></Rule>",
			wantGet:  "rule1",
			wantCode: http.StatusOK,
		},
		{
			name:     "overwrite_rules",
			bucket:   "bucket2",
			rules:    "<Rule><ID>rule2</ID></Rule>",
			wantGet:  "rule2",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3control.NewInMemoryBackend()
			h := s3control.NewHandler(b)
			b.CreateBucket("000000000000", tt.bucket)

			body := `<ReplicationConfiguration>` + tt.rules + `</ReplicationConfiguration>`
			rec := doS3ControlNewOpRequest(t, h, http.MethodPut,
				"/v20180820/bucket/"+tt.bucket+"/replication", "000000000000", body)
			assert.Equal(t, http.StatusOK, rec.Code)

			rec = doS3ControlNewOpRequest(t, h, http.MethodGet,
				"/v20180820/bucket/"+tt.bucket+"/replication", "000000000000", "")
			require.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantGet)
		})
	}
}

func TestBucketReplication_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		bucket      string
		createFirst bool
		preload     bool
		wantCode    int
	}{
		{
			name:        "delete_existing",
			bucket:      "my-bucket",
			createFirst: true,
			preload:     true,
			wantCode:    http.StatusNoContent,
		},
		{
			// Bucket never created: PutBucketReplication/
			// DeleteBucketReplication now require the bucket to exist, like
			// every other bucket sub-resource op, so this 404s instead of
			// silently succeeding.
			name:        "delete_nonexistent_bucket_404s",
			bucket:      "missing-bucket",
			createFirst: false,
			preload:     false,
			wantCode:    http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3control.NewInMemoryBackend()
			h := s3control.NewHandler(b)

			if tt.createFirst {
				b.CreateBucket("000000000000", tt.bucket)
			}
			if tt.preload {
				require.NoError(t, b.PutBucketReplication(tt.bucket, "<Rule/>"))
			}

			rec := doS3ControlNewOpRequest(t, h, http.MethodDelete,
				"/v20180820/bucket/"+tt.bucket+"/replication", "000000000000", "")
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestBucketReplication_GetMissing(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	h := s3control.NewHandler(b)

	rec := doS3ControlNewOpRequest(t, h, http.MethodGet,
		"/v20180820/bucket/no-such-bucket/replication", "000000000000", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestCreateBucket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		accountID        string
		bucketName       string
		wantBodyContains string
		wantStatus       int
		wantLocationHdr  bool
	}{
		{
			name:             "creates_outposts_bucket",
			accountID:        "123456789012",
			bucketName:       "my-outposts-bucket",
			wantStatus:       http.StatusOK,
			wantBodyContains: "BucketArn",
			wantLocationHdr:  true,
		},
		{
			name:             "creates_outposts_bucket_default_account",
			accountID:        "",
			bucketName:       "test-bucket",
			wantStatus:       http.StatusOK,
			wantBodyContains: "BucketArn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			path := "/v20180820/bucket/" + tt.bucketName
			rec := doS3ControlNewOpRequest(t, h, http.MethodPut, path, tt.accountID, "")

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantBodyContains != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContains)
			}

			if tt.wantLocationHdr {
				assert.NotEmpty(t, rec.Header().Get("Location"))
			}
		})
	}
}

// TestHTTP_CreateBucket_RealSDKShape_RoundTrip: real CreateBucketInput has
// no AccountId member at all, unlike GetBucket/DeleteBucket/ListRegionalBuckets
// which bind an optional AccountId to X-Amz-Account-Id. This exercises the
// real, headerless CreateBucket shape and reads it back using an explicit,
// different AccountId on every read op, exactly as a real SDK client would.
// Deliberately NOT split into t.Run subtests: List/Get must observe the
// bucket before Delete removes it, so the three reads share one strict
// sequence rather than running in parallel.
func TestHTTP_CreateBucket_RealSDKShape_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestS3ControlHandler(t)

	// Real CreateBucket: no X-Amz-Account-Id header at all.
	createRec := doS3ControlNewOpRequest(
		t, h, http.MethodPut, "/v20180820/bucket/real-bucket", "", "",
	)
	require.Equal(t, http.StatusOK, createRec.Code)

	// Real ListRegionalBuckets: the caller's actual AWS account ID header.
	listRec := doS3ControlNewOpRequest(t, h, http.MethodGet, "/v20180820/bucket", "123456789012", "")
	require.Equal(t, http.StatusOK, listRec.Code)
	assert.Contains(t, listRec.Body.String(), "real-bucket")

	// Real GetBucket: same real account ID header.
	getRec := doS3ControlNewOpRequest(
		t, h, http.MethodGet, "/v20180820/bucket/real-bucket", "123456789012", "",
	)
	require.Equal(t, http.StatusOK, getRec.Code)

	var out struct {
		XMLName xml.Name `xml:"GetBucketResult"`
		Bucket  string   `xml:"Bucket"`
	}
	require.NoError(t, xml.Unmarshal(getRec.Body.Bytes(), &out))
	assert.Equal(t, "real-bucket", out.Bucket)

	// Real DeleteBucket: same real account ID header.
	deleteRec := doS3ControlNewOpRequest(
		t, h, http.MethodDelete, "/v20180820/bucket/real-bucket", "123456789012", "",
	)
	assert.Equal(t, http.StatusNoContent, deleteRec.Code)
}

func TestListRegionalBuckets_Pagination(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	for i := range 4 {
		b.CreateBucket("acct1", fmt.Sprintf("bucket-%d", i))
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
			path:          "/v20180820/bucket",
			wantLen:       4,
			wantNextToken: false,
		},
		{
			name:          "page1_two_items",
			path:          "/v20180820/bucket?maxResults=2",
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
				XMLName   xml.Name `xml:"ListRegionalBucketsResult"`
				NextToken string   `xml:"NextToken"`
				Buckets   []struct {
					Bucket string `xml:"Bucket"`
				} `xml:"RegionalBucketList>RegionalBucket"`
			}
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))
			assert.Len(t, out.Buckets, tt.wantLen)
			if tt.wantNextToken {
				assert.NotEmpty(t, out.NextToken)
			} else {
				assert.Empty(t, out.NextToken)
			}
		})
	}
}

// TestBucketTagging_WireShape covers PutBucketTagging/GetBucketTagging:
// Tagging is "payload"-bound, so the entire request body root is
// "<Tagging>", no "<PutBucketTaggingRequest>" wrapper
// (awsRestxml_serializeOpPutBucketTaggingRequest); and TagSet serializes
// entries as "<member>", not "<Tag>" (awsRestxml_serializeDocumentS3TagSet
// — same type job tagging uses, handler_jobs.go).
func TestBucketTagging_WireShape(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	h := s3control.NewHandler(b)
	b.CreateBucket("acct1", "tag-bucket")
	path := "/v20180820/bucket/tag-bucket/tagging"

	putBody := `<Tagging><TagSet><member><Key>env</Key><Value>prod</Value></member></TagSet></Tagging>`
	putRec := doS3Request(t, h, http.MethodPut, path, putBody)
	require.Equal(t, http.StatusOK, putRec.Code)

	getRec := doS3Request(t, h, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, getRec.Code)

	body := getRec.Body.String()
	assert.Contains(t, body, "<TagSet><member>")
	assert.NotContains(t, body, "<Tag>")

	var out struct {
		XMLName xml.Name `xml:"GetBucketTaggingResult"`
		Tags    []struct {
			Key   string `xml:"Key"`
			Value string `xml:"Value"`
		} `xml:"TagSet>member"`
	}
	require.NoError(t, xml.Unmarshal(getRec.Body.Bytes(), &out))
	require.Len(t, out.Tags, 1)
	assert.Equal(t, "env", out.Tags[0].Key)
	assert.Equal(t, "prod", out.Tags[0].Value)
}

// TestBucketVersioning_WireShape covers: VersioningConfiguration is
// "payload"-bound, so the entire request body root is
// "<VersioningConfiguration>" with Status as its direct child, no
// "<PutBucketVersioningRequest>" wrapper
// (awsRestxml_serializeOpPutBucketVersioningRequest).
func TestBucketVersioning_WireShape(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	h := s3control.NewHandler(b)
	b.CreateBucket("acct1", "ver-bucket")
	path := "/v20180820/bucket/ver-bucket/versioning"

	putBody := `<VersioningConfiguration><Status>Enabled</Status></VersioningConfiguration>`
	putRec := doS3Request(t, h, http.MethodPut, path, putBody)
	require.Equal(t, http.StatusOK, putRec.Code)

	getRec := doS3Request(t, h, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, getRec.Code)

	var out struct {
		XMLName xml.Name `xml:"GetBucketVersioningResult"`
		Status  string   `xml:"Status"`
	}
	require.NoError(t, xml.Unmarshal(getRec.Body.Bytes(), &out))
	assert.Equal(t, "Enabled", out.Status)
}

// TestBucketPolicy_WireShape covers: unlike PutBucketLifecycleConfiguration/
// PutBucketTagging/PutBucketVersioning, PutBucketPolicyInput.Policy is not
// payload-bound — the real request body root is "<PutBucketPolicyRequest>"
// with the policy JSON document as the text of a nested "<Policy>" element
// (awsRestxml_serializeOpDocumentPutBucketPolicyInput), not the whole raw
// body treated as "the policy".
func TestBucketPolicy_WireShape(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	h := s3control.NewHandler(b)
	b.CreateBucket("acct1", "policy-bucket")
	path := "/v20180820/bucket/policy-bucket/policy"

	policyJSON := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow"}]}`
	putBody := `<PutBucketPolicyRequest><Policy>` + policyJSON + `</Policy></PutBucketPolicyRequest>`
	putRec := doS3Request(t, h, http.MethodPut, path, putBody)
	require.Equal(t, http.StatusOK, putRec.Code)

	getRec := doS3Request(t, h, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, getRec.Code)

	assert.Equal(t,
		`<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
			`<GetBucketPolicyResult><Policy>{&#34;Version&#34;:&#34;2012-10-17&#34;,`+
			`&#34;Statement&#34;:[{&#34;Effect&#34;:&#34;Allow&#34;}]}</Policy></GetBucketPolicyResult>`,
		getRec.Body.String(),
	)

	var out struct {
		XMLName xml.Name `xml:"GetBucketPolicyResult"`
		Policy  string   `xml:"Policy"`
	}
	require.NoError(t, xml.Unmarshal(getRec.Body.Bytes(), &out))
	assert.JSONEq(t, policyJSON, out.Policy,
		"GetBucketPolicy must return the plain policy JSON, not the PutBucketPolicyRequest envelope")
}
