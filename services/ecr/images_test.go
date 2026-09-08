package ecr_test

// images_test.go — mostly backend-level tests for images.go: PutImage digest
// computation, multi-tag/retag/untagged semantics, tag-vs-digest deletion,
// ListImages/DescribeImages filtering, IMMUTABLE tag-mutability enforcement
// (including exclusion filters), state-isolation (deep copy) guarantees, and
// (to balance file size) a handful of HTTP-handler-level tests for
// BatchGetImage/BatchDeleteImage/DescribeImages/ListImageReferrers/
// UpdateImageStorageClass error precedence. The bulk of the HTTP-handler-level
// coverage for images.go lives in handler_images_test.go.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecr"
)

func TestPutImage_DigestFromManifestOnly(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "digest-repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	img1, err := b.PutImage(context.Background(), "digest-repo", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"content":"same"}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
	})
	require.NoError(t, err)

	img2, err := b.PutImage(context.Background(), "digest-repo", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"content":"same"}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v2"},
	})
	require.NoError(t, err)

	assert.Equal(t, img1.ImageDigest, img2.ImageDigest,
		"same manifest content must produce the same digest regardless of tag")
}

func TestPutImage_DifferentManifest_DifferentDigest(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "diff-repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	img1, err := b.PutImage(context.Background(), "diff-repo", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"v":1}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
	})
	require.NoError(t, err)

	img2, err := b.PutImage(context.Background(), "diff-repo", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"v":2}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
	})
	require.NoError(t, err)

	assert.NotEqual(t, img1.ImageDigest, img2.ImageDigest,
		"different manifest content must produce different digests")
}

func TestMultiTag_Backend_TagIndexPopulated(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "idx-repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	manifest := `{"schemaVersion":2}`
	_, err = b.PutImage(context.Background(), "idx-repo", ecr.Image{
		ImageManifest: manifest,
		ImageID:       ecr.ImageIdentifier{ImageTag: "t1"},
	})
	require.NoError(t, err)

	_, err = b.PutImage(context.Background(), "idx-repo", ecr.Image{
		ImageManifest: manifest,
		ImageID:       ecr.ImageIdentifier{ImageTag: "t2"},
	})
	require.NoError(t, err)

	assert.Equal(t, 2, b.RepoTagCount("idx-repo"),
		"tagIndex must contain both tags after two pushes with same manifest")

	d1 := b.TagDigest("idx-repo", "t1")
	d2 := b.TagDigest("idx-repo", "t2")
	assert.Equal(t, d1, d2, "both tags must resolve to the same digest")
}

func TestMultiTag_ThreeTags_OneDigest(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "three-tag", "MUTABLE", false, "", "")
	require.NoError(t, err)

	manifest := `{"schemaVersion":2,"layers":[]}`
	for _, tag := range []string{"v1", "v1.0", "v1.0.0"} {
		_, err = b.PutImage(context.Background(), "three-tag", ecr.Image{
			ImageManifest: manifest,
			ImageID:       ecr.ImageIdentifier{ImageTag: tag},
		})
		require.NoError(t, err)
	}

	assert.Equal(t, 3, b.RepoTagCount("three-tag"), "three tags for one digest")
	assert.Equal(t, 1, b.ImageCount(), "one image entry regardless of tag count")
}

func TestRetag_MUTABLE_OldImageBecomesUntagged(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "retag-repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	_, err = b.PutImage(context.Background(), "retag-repo", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"v":1}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "latest"},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, b.RepoTagCount("retag-repo"), "one tag before retag")

	// Push different content with same tag — tag should move.
	_, err = b.PutImage(context.Background(), "retag-repo", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"v":2}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "latest"},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, b.RepoTagCount("retag-repo"),
		"after retag, still one tag entry (moved to new digest)")
	assert.Equal(t, 2, b.ImageCount(),
		"old image stays in storage as untagged after retag")
}

func TestRetag_MUTABLE_NewTag_PointsToNewDigest(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "newtag-repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	img1, err := b.PutImage(context.Background(), "newtag-repo", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"v":1}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "prod"},
	})
	require.NoError(t, err)

	img2, err := b.PutImage(context.Background(), "newtag-repo", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"v":2}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "prod"},
	})
	require.NoError(t, err)

	assert.NotEqual(t, img1.ImageDigest, img2.ImageDigest)
	assert.Equal(t, img2.ImageDigest, b.TagDigest("newtag-repo", "prod"),
		"tag must resolve to the newest pushed digest")
}

func TestUntagged_Image_PushWithoutTag(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "untag-repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	_, err = b.PutImage(context.Background(), "untag-repo", ecr.Image{
		ImageManifest: `{"schemaVersion":2}`,
		ImageID:       ecr.ImageIdentifier{},
	})
	require.NoError(t, err)

	assert.Equal(t, 0, b.RepoTagCount("untag-repo"), "untagged image has no tag entries")
	assert.Equal(t, 1, b.ImageCount(), "untagged image still stored")
}

func TestListImages_Filter_Backend_TAGGED(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "be-tagged", "MUTABLE", false, "", "")
	require.NoError(t, err)

	_, err = b.PutImage(context.Background(), "be-tagged", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"u":true}`,
	})
	require.NoError(t, err)

	_, err = b.PutImage(context.Background(), "be-tagged", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"t":true}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
	})
	require.NoError(t, err)

	ids, err := b.ListImages(context.Background(), "be-tagged", "TAGGED", "")
	require.NoError(t, err)
	assert.Len(t, ids, 1)
	assert.Equal(t, "v1", ids[0].ImageTag)
}

func TestListImages_Filter_Backend_UNTAGGED(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "be-untagged", "MUTABLE", false, "", "")
	require.NoError(t, err)

	_, err = b.PutImage(context.Background(), "be-untagged", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"u":true}`,
	})
	require.NoError(t, err)

	_, err = b.PutImage(context.Background(), "be-untagged", ecr.Image{
		ImageManifest: `{"schemaVersion":2,"t":true}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
	})
	require.NoError(t, err)

	ids, err := b.ListImages(context.Background(), "be-untagged", "UNTAGGED", "")
	require.NoError(t, err)
	assert.Len(t, ids, 1)
	assert.Empty(t, ids[0].ImageTag)
}

func TestBatchDeleteImage_ByTag_RemovesTagOnly(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "del-by-tag", "MUTABLE", false, "", "")
	require.NoError(t, err)

	manifest := `{"schemaVersion":2,"shared":true}`
	_, err = b.PutImage(context.Background(), "del-by-tag", ecr.Image{
		ImageManifest: manifest,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
	})
	require.NoError(t, err)

	_, err = b.PutImage(context.Background(), "del-by-tag", ecr.Image{
		ImageManifest: manifest,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v2"},
	})
	require.NoError(t, err)

	// Delete by tag v1 only.
	deleted, failures, err := b.BatchDeleteImage(context.Background(), "del-by-tag", []ecr.ImageIdentifier{
		{ImageTag: "v1"},
	})
	require.NoError(t, err)
	assert.Empty(t, failures)
	assert.Len(t, deleted, 1)

	// Image must still be accessible by digest and by v2 tag.
	assert.Equal(t, 1, b.RepoTagCount("del-by-tag"),
		"only v1 tag removed; v2 tag remains")
	assert.Equal(t, 1, b.ImageCount(), "image itself stays after tag-only delete")
}

func TestBatchDeleteImage_ByDigest_RemovesAllTags(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "del-by-digest", "MUTABLE", false, "", "")
	require.NoError(t, err)

	manifest := `{"schemaVersion":2,"multi":true}`
	img1, err := b.PutImage(context.Background(), "del-by-digest", ecr.Image{
		ImageManifest: manifest,
		ImageID:       ecr.ImageIdentifier{ImageTag: "alpha"},
	})
	require.NoError(t, err)

	_, err = b.PutImage(context.Background(), "del-by-digest", ecr.Image{
		ImageManifest: manifest,
		ImageID:       ecr.ImageIdentifier{ImageTag: "beta"},
	})
	require.NoError(t, err)

	assert.Equal(t, 2, b.RepoTagCount("del-by-digest"))

	// Delete by digest — must remove both tag bindings.
	deleted, failures, err := b.BatchDeleteImage(context.Background(), "del-by-digest", []ecr.ImageIdentifier{
		{ImageDigest: img1.ImageDigest},
	})
	require.NoError(t, err)
	assert.Empty(t, failures)
	assert.Len(t, deleted, 1)

	assert.Equal(t, 0, b.RepoTagCount("del-by-digest"),
		"digest delete must remove all associated tags from index")
	assert.Equal(t, 0, b.ImageCount(), "image itself deleted by digest")
}

func TestBackend_PutImage_ReturnedValue_Isolated(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "iso-img", "MUTABLE", false, "", "")
	require.NoError(t, err)

	img, err := b.PutImage(context.Background(), "iso-img", ecr.Image{
		ImageManifest: `{"schemaVersion":2}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
	})
	require.NoError(t, err)

	originalDigest := img.ImageDigest
	img.ImageDigest = "mutated"

	// Stored state must be unaffected.
	imgs, err := b.DescribeImages(context.Background(), "iso-img", nil)
	require.NoError(t, err)
	require.Len(t, imgs, 1)
	assert.Equal(t, originalDigest, imgs[0].ImageDigest,
		"mutating PutImage return value must not affect stored image")
}

func TestBackend_DescribeImages_ReturnedSlice_Isolated(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "iso-desc", "MUTABLE", false, "", "")
	require.NoError(t, err)

	_, err = b.PutImage(context.Background(), "iso-desc", ecr.Image{
		ImageManifest: `{"schemaVersion":2}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
	})
	require.NoError(t, err)

	imgs, err := b.DescribeImages(context.Background(), "iso-desc", nil)
	require.NoError(t, err)
	require.Len(t, imgs, 1)

	// Mutate returned copy.
	imgs[0].RepositoryName = "mutated"

	imgs2, err := b.DescribeImages(context.Background(), "iso-desc", nil)
	require.NoError(t, err)
	assert.Equal(t, "iso-desc", imgs2[0].RepositoryName,
		"mutating DescribeImages return value must not affect stored state")
}

func TestPutImage_ReturnedImage_DeepCopy(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "img-copy", "MUTABLE", false, "", "")
	require.NoError(t, err)

	img1, err := b.PutImage(context.Background(), "img-copy", ecr.Image{
		ImageManifest: `{"schemaVersion":2}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
	})
	require.NoError(t, err)

	// Mutate returned image.
	img1.RepositoryName = "mutated"

	// Retrieve via DescribeImages — must still see original.
	imgs, err := b.DescribeImages(context.Background(), "img-copy", nil)
	require.NoError(t, err)
	require.Len(t, imgs, 1)
	assert.Equal(t, "img-copy", imgs[0].RepositoryName,
		"mutation of PutImage return value must not affect stored image")
}

func TestImmutableExclusionFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		tag            string
		filterPattern  string
		filterType     string
		wantErrOnRetag bool
	}{
		{
			name:           "excluded_tag_can_be_retagged",
			tag:            "latest",
			filterPattern:  "latest",
			filterType:     "WILDCARD",
			wantErrOnRetag: false,
		},
		{
			name:           "excluded_wildcard_prefix_matches",
			tag:            "dev-build",
			filterPattern:  "dev-*",
			filterType:     "WILDCARD",
			wantErrOnRetag: false,
		},
		{
			name:           "non_excluded_tag_rejected",
			tag:            "v1.0.0",
			filterPattern:  "latest",
			filterType:     "WILDCARD",
			wantErrOnRetag: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			b.CreateRepoInternal("immutable-repo")

			_, err := b.PutImageTagMutability(context.Background(), "immutable-repo",
				"IMMUTABLE",
				[]ecr.ImageTagMutabilityExclusionFilter{
					{Filter: tt.filterPattern, FilterType: tt.filterType},
				},
			)
			require.NoError(t, err)

			img1 := ecr.Image{
				ImageDigest:   "sha256:digest1",
				ImageManifest: `{"v":1}`,
				ImageID:       ecr.ImageIdentifier{ImageDigest: "sha256:digest1", ImageTag: tt.tag},
			}
			_, err = b.PutImage(context.Background(), "immutable-repo", img1)
			require.NoError(t, err)

			img2 := ecr.Image{
				ImageDigest:   "sha256:digest2",
				ImageManifest: `{"v":2}`,
				ImageID:       ecr.ImageIdentifier{ImageDigest: "sha256:digest2", ImageTag: tt.tag},
			}
			_, err = b.PutImage(context.Background(), "immutable-repo", img2)

			if tt.wantErrOnRetag {
				assert.ErrorIs(t, err, ecr.ErrImageTagAlreadyExists,
					"non-excluded tag must be rejected in IMMUTABLE repo")
			} else {
				assert.NoError(t, err, "excluded tag must bypass IMMUTABLE check")
			}
		})
	}
}

func TestPutImage_ManifestRoundTrip(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	makeRepo(t, b, "manifest-repo")

	manifest := `{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"mediaType":"application/vnd.docker.container.image.v1+json","size":7023,"digest":"sha256:config"},"layers":[]}` //nolint:lll // JSON policy exceeds 120 chars; splitting worsens readability

	img := ecr.Image{
		ImageManifest:          manifest,
		ImageManifestMediaType: "application/vnd.docker.distribution.manifest.v2+json",
		ImageID:                ecr.ImageIdentifier{ImageTag: "v1.0"},
	}

	pushed, err := b.PutImage(context.Background(), "manifest-repo", img)
	require.NoError(t, err)
	assert.NotEmpty(t, pushed.ImageDigest)

	results, failures, err := b.BatchGetImage(context.Background(), "manifest-repo",
		[]ecr.ImageIdentifier{{ImageTag: "v1.0"}})
	require.NoError(t, err)
	assert.Empty(t, failures)
	require.Len(t, results, 1)
	assert.Equal(t, manifest, results[0].ImageManifest)
	assert.Equal(t, pushed.ImageDigest, results[0].ImageDigest)
}

func TestTagMutability_Enforcement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr    error
		name       string
		mutability string
		digest1    string
		digest2    string
		tag        string
	}{
		{
			name:       "mutable_allows_retag",
			mutability: "MUTABLE",
			digest1:    "sha256:aaa",
			digest2:    "sha256:bbb",
			tag:        "latest",
		},
		{
			name:       "immutable_rejects_retag",
			mutability: "IMMUTABLE",
			digest1:    "sha256:ccc",
			digest2:    "sha256:ddd",
			tag:        "v1",
			wantErr:    ecr.ErrImageTagAlreadyExists,
		},
		{
			// Re-pushing the exact same digest under the exact same tag is a
			// complete no-op push and is rejected with
			// ImageAlreadyExistsException, independent of repository tag
			// mutability -- see the PutImage ImageAlreadyExistsException doc
			// comment in images.go. This case used to assert success (the
			// gap this test now locks the closure of).
			name:       "immutable_same_digest_rejected_as_no_op_push",
			mutability: "IMMUTABLE",
			digest1:    "sha256:eee",
			digest2:    "sha256:eee",
			tag:        "stable",
			wantErr:    ecr.ErrImageAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			b.CreateRepoInternal("mut-repo")
			_, err := b.PutImageTagMutability(context.Background(), "mut-repo", tt.mutability, nil)
			require.NoError(t, err)

			img1 := ecr.Image{
				ImageDigest:   tt.digest1,
				ImageManifest: `{"v":1}`,
				ImageID:       ecr.ImageIdentifier{ImageDigest: tt.digest1, ImageTag: tt.tag},
			}
			_, err = b.PutImage(context.Background(), "mut-repo", img1)
			require.NoError(t, err)

			img2 := ecr.Image{
				ImageDigest:   tt.digest2,
				ImageManifest: `{"v":2}`,
				ImageID:       ecr.ImageIdentifier{ImageDigest: tt.digest2, ImageTag: tt.tag},
			}
			_, err = b.PutImage(context.Background(), "mut-repo", img2)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBatchDeleteImage_TagVsDigest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		deleteByTag  bool
		wantImgCount int
	}{
		{
			name:         "delete_by_tag_leaves_untagged_image",
			deleteByTag:  true,
			wantImgCount: 1, // image still exists, just untagged
		},
		{
			name:         "delete_by_digest_removes_image",
			deleteByTag:  false,
			wantImgCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			b.CreateRepoInternal("del-repo")

			digest := "sha256:deldel"
			b.AddImageInternal("del-repo", makeImage(digest, "del-tag"))

			var ids []ecr.ImageIdentifier
			if tt.deleteByTag {
				ids = []ecr.ImageIdentifier{{ImageTag: "del-tag"}}
			} else {
				ids = []ecr.ImageIdentifier{{ImageDigest: digest}}
			}

			deleted, failed, err := b.BatchDeleteImage(context.Background(), "del-repo", ids)
			require.NoError(t, err)
			assert.Empty(t, failed)
			assert.Len(t, deleted, 1)

			assert.Equal(t, tt.wantImgCount, b.ImageCount())
		})
	}
}

func TestListImages_Filter_Backend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tagStatus string
		wantLen   int
	}{
		{name: "tagged_only", tagStatus: "TAGGED", wantLen: 1},
		{name: "untagged_only", tagStatus: "UNTAGGED", wantLen: 1},
		{name: "all", tagStatus: "", wantLen: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			b.CreateRepoInternal("list-repo")
			b.AddImageInternal("list-repo", makeImage("sha256:tagged", "v1"))
			b.AddImageInternal("list-repo", ecr.Image{
				ImageDigest:   "sha256:untagged",
				ImageManifest: `{"schemaVersion":2}`,
				ImageID:       ecr.ImageIdentifier{ImageDigest: "sha256:untagged"},
			})

			ids, err := b.ListImages(context.Background(), "list-repo", tt.tagStatus, "")
			require.NoError(t, err)
			assert.Len(t, ids, tt.wantLen)
		})
	}
}

func TestDescribeImages_ReturnsAll(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	b.CreateRepoInternal("desc-repo")
	b.AddImageInternal("desc-repo", makeImage("sha256:desc1", "v1"))
	b.AddImageInternal("desc-repo", ecr.Image{
		ImageDigest:   "sha256:desc2",
		ImageManifest: `{"schemaVersion":2}`,
		ImageID:       ecr.ImageIdentifier{ImageDigest: "sha256:desc2"},
	})

	imgs, err := b.DescribeImages(context.Background(), "desc-repo", nil)
	require.NoError(t, err)
	assert.Len(t, imgs, 2, "DescribeImages with no filter must return all images")

	// Lookup by specific ID.
	imgs2, err := b.DescribeImages(context.Background(), "desc-repo",
		[]ecr.ImageIdentifier{{ImageDigest: "sha256:desc1"}})
	require.NoError(t, err)
	assert.Len(t, imgs2, 1)
	assert.Equal(t, "sha256:desc1", imgs2[0].ImageDigest)
}

func TestPutImage_PushedAtSet(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	makeRepo(t, b, "pushedat-repo")

	before := time.Now().Add(-time.Second)
	_, err := b.PutImage(context.Background(), "pushedat-repo", ecr.Image{
		ImageManifest: `{"schemaVersion":2}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
	})
	require.NoError(t, err)

	imgs, err := b.DescribeImages(context.Background(), "pushedat-repo", nil)
	require.NoError(t, err)
	require.Len(t, imgs, 1)
	assert.True(t, imgs[0].ImagePushedAt.After(before),
		"ImagePushedAt must be set to current time on push")
}
func TestOps2_BatchGetImage_RepoNotFound_TopLevelError(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()

	rec := doAccuracy(t, h, "BatchGetImage", map[string]any{
		"repositoryName": "ghost-repo",
		"imageIds":       []map[string]any{{"imageDigest": "sha256:abc"}},
	})
	require.Equal(t, http.StatusNotFound, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "RepositoryNotFoundException", out["__type"],
		"BatchGetImage on non-existent repo must return RepositoryNotFoundException")
}

func TestOps2_BatchGetImage_RepoExists_ImageNotFound_ReturnsFailure(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "bgi-repo")

	rec := doAccuracy(t, h, "BatchGetImage", map[string]any{
		"repositoryName": "bgi-repo",
		"imageIds":       []map[string]any{{"imageDigest": "sha256:missing"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	images, _ := out["images"].([]any)
	failures, _ := out["failures"].([]any)
	assert.Empty(t, images)
	assert.Len(t, failures, 1, "missing image must produce a per-entry failure, not a top-level error")
}

func TestOps2_BatchGetImage_RepoExists_ImageFound_ReturnsImage(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "bgi-found-repo")
	mustPutImage(t, h, "bgi-found-repo", "v1", `{"schemaVersion":2,"layers":[]}`)

	rec := doAccuracy(t, h, "BatchGetImage", map[string]any{
		"repositoryName": "bgi-found-repo",
		"imageIds":       []map[string]any{{"imageTag": "v1"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	images, _ := out["images"].([]any)
	assert.Len(t, images, 1, "existing image must be returned")
}

func TestOps2_BatchGetImage_MixedFound_And_Missing(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "bgi-mix-repo")
	mustPutImage(t, h, "bgi-mix-repo", "present", `{"schemaVersion":2,"layers":[]}`)

	rec := doAccuracy(t, h, "BatchGetImage", map[string]any{
		"repositoryName": "bgi-mix-repo",
		"imageIds": []map[string]any{
			{"imageTag": "present"},
			{"imageTag": "absent"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	images, _ := out["images"].([]any)
	failures, _ := out["failures"].([]any)
	assert.Len(t, images, 1)
	assert.Len(t, failures, 1)
}

func TestOps2_BatchDeleteImage_RepoNotFound_TopLevelError(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()

	rec := doAccuracy(t, h, "BatchDeleteImage", map[string]any{
		"repositoryName": "ghost-repo",
		"imageIds":       []map[string]any{{"imageDigest": "sha256:abc"}},
	})
	require.Equal(t, http.StatusNotFound, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "RepositoryNotFoundException", out["__type"],
		"BatchDeleteImage on non-existent repo must return RepositoryNotFoundException")
}

func TestOps2_BatchDeleteImage_RepoExists_ImageNotFound_ReturnsFailure(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "bdi-repo")

	rec := doAccuracy(t, h, "BatchDeleteImage", map[string]any{
		"repositoryName": "bdi-repo",
		"imageIds":       []map[string]any{{"imageDigest": "sha256:missing"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	deleted, _ := out["imageIds"].([]any)
	failures, _ := out["failures"].([]any)
	assert.Empty(t, deleted)
	assert.Len(t, failures, 1, "missing image must produce a per-entry failure, not a top-level error")
}

func TestOps2_BatchDeleteImage_RepoExists_ImageFound_Deletes(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "bdi-found-repo")
	mustPutImage(t, h, "bdi-found-repo", "del-tag", `{"schemaVersion":2,"layers":[]}`)

	rec := doAccuracy(t, h, "BatchDeleteImage", map[string]any{
		"repositoryName": "bdi-found-repo",
		"imageIds":       []map[string]any{{"imageTag": "del-tag"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	deleted, _ := out["imageIds"].([]any)
	failures, _ := out["failures"].([]any)
	assert.Len(t, deleted, 1)
	assert.Empty(t, failures)
}

func TestOps2_DescribeImages_ByImageID_NotFound_ImageNotFoundException(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "di-not-found-repo")

	rec := doAccuracy(t, h, "DescribeImages", map[string]any{
		"repositoryName": "di-not-found-repo",
		"imageIds": []map[string]any{
			{"imageDigest": "sha256:9999999999999999999999999999999999999999999999999999999999999999"},
		},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "ImageNotFoundException", out["__type"],
		"DescribeImages with missing imageId must return ImageNotFoundException, not RepositoryNotFoundException")
}

func TestOps2_DescribeImages_ByTag_NotFound_ImageNotFoundException(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "di-tag-not-found-repo")

	rec := doAccuracy(t, h, "DescribeImages", map[string]any{
		"repositoryName": "di-tag-not-found-repo",
		"imageIds": []map[string]any{
			{"imageTag": "ghost"},
		},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "ImageNotFoundException", out["__type"])
}

func TestOps2_DescribeImages_ByImageID_Found_ReturnsDetail(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "di-found-repo")
	digest := mustPutImage(t, h, "di-found-repo", "v1", `{"schemaVersion":2,"layers":[]}`)

	rec := doAccuracy(t, h, "DescribeImages", map[string]any{
		"repositoryName": "di-found-repo",
		"imageIds":       []map[string]any{{"imageDigest": digest}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	details, _ := out["imageDetails"].([]any)
	assert.Len(t, details, 1, "existing image must be returned")
}

func TestOps2_ListImageReferrers_SubjectNotFound_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "lir-not-found-repo")

	rec := doAccuracy(t, h, "ListImageReferrers", map[string]any{
		"repositoryName": "lir-not-found-repo",
		"subjectId": map[string]any{
			"imageDigest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
	})
	// ListImageReferrers declares no not-found error for the subject (only
	// RepositoryNotFoundException et al., per deserializeOpErrorListImageReferrers) --
	// an unknown subject digest returns 200 with an empty list.
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	assert.NotEqual(t, "ImageNotFoundException", out["__type"])
	referrers, _ := out["referrers"].([]any)
	assert.Empty(t, referrers, "non-existent subject must return an empty referrer list, not an error")
}

func TestOps2_ListImageReferrers_RepositoryNotFound_Errors(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()

	rec := doAccuracy(t, h, "ListImageReferrers", map[string]any{
		"repositoryName": "lir-no-such-repo",
		"subjectId": map[string]any{
			"imageDigest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
	})
	require.Equal(t, http.StatusNotFound, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "RepositoryNotFoundException", out["__type"],
		"ListImageReferrers on a non-existent repository must still return RepositoryNotFoundException")
}

func TestOps2_ListImageReferrers_SubjectFound_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "lir-found-repo")
	digest := mustPutImage(t, h, "lir-found-repo", "subj", `{"schemaVersion":2,"layers":[]}`)

	rec := doAccuracy(t, h, "ListImageReferrers", map[string]any{
		"repositoryName": "lir-found-repo",
		"subjectId":      map[string]any{"imageDigest": digest},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	referrers, _ := out["referrers"].([]any)
	assert.Empty(t, referrers, "no referrers expected for a fresh image")
}

func TestListImageReferrers_NoReferrers_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "referrer-repo")
	digest := mustPutImage(t, h, "referrer-repo", "v1", `{"schemaVersion":2,"referrer":"test"}`)

	rec := doAccuracy(t, h, "ListImageReferrers", map[string]any{
		"repositoryName": "referrer-repo",
		"subjectId": map[string]any{
			"imageDigest": digest,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	referrers, _ := out["referrers"].([]any)
	assert.Empty(t, referrers, "image with no referrers must return an empty list")
}

func TestListImageReferrers_NonExistentImage_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "referrer-repo-err")

	rec := doAccuracy(t, h, "ListImageReferrers", map[string]any{
		"repositoryName": "referrer-repo-err",
		"subjectId": map[string]any{
			"imageDigest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code,
		"non-existent image digest must return 200 with an empty list, not an error")

	out := parseAccuracy(t, rec)
	referrers, _ := out["referrers"].([]any)
	assert.Empty(t, referrers)
}

func TestUpdateImageStorageClass_Archive(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "storage-repo")
	digest := mustPutImage(t, h, "storage-repo", "latest", `{"schemaVersion":2,"storage":"test"}`)

	rec := doAccuracy(t, h, "UpdateImageStorageClass", map[string]any{
		"repositoryName": "storage-repo",
		"imageId": map[string]any{
			"imageDigest": digest,
		},
		"targetStorageClass": "ARCHIVE",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "ARCHIVED", out["imageStatus"],
		"ARCHIVE storage class must set imageStatus to ARCHIVED")
	assert.Equal(t, "storage-repo", out["repositoryName"])
	assert.Equal(t, "123456789012", out["registryId"])
}

func TestUpdateImageStorageClass_Standard_RestoresActive(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "restore-storage-repo")
	digest := mustPutImage(t, h, "restore-storage-repo", "v1", `{"schemaVersion":2,"restore":"test"}`)

	// Archive then restore.
	doAccuracy(t, h, "UpdateImageStorageClass", map[string]any{
		"repositoryName":     "restore-storage-repo",
		"imageId":            map[string]any{"imageDigest": digest},
		"targetStorageClass": "ARCHIVE",
	})

	rec := doAccuracy(t, h, "UpdateImageStorageClass", map[string]any{
		"repositoryName":     "restore-storage-repo",
		"imageId":            map[string]any{"imageDigest": digest},
		"targetStorageClass": "STANDARD",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "ACTIVE", out["imageStatus"],
		"STANDARD storage class must restore imageStatus to ACTIVE")
}

// Test_PutImage_DigestValidation covers the ImageDigestDoesNotMatchException
// gap found in the parity-sweep-3 audit: real ECR validates a caller-supplied
// imageDigest against the digest it computes from the manifest and rejects a
// mismatch, but the emulator previously trusted whatever digest the client sent.
func Test_PutImage_DigestValidation(t *testing.T) {
	t.Parallel()

	const manifest = `{"schemaVersion":2,"digest-check":true}`

	cases := []struct {
		name        string
		imageDigest string
		wantType    string
		wantStatus  int
	}{
		{
			name:        "no digest supplied - server computes it",
			imageDigest: "",
			wantStatus:  http.StatusOK,
		},
		{
			name: "mismatched digest rejected",
			imageDigest: "sha256:" +
				"0000000000000000000000000000000000000000000000000000000000000000",
			wantStatus: http.StatusBadRequest,
			wantType:   "ImageDigestDoesNotMatchException",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newAccuracyHandler()
			mustCreateRepo(t, h, "digest-check-repo")

			body := map[string]any{
				"repositoryName": "digest-check-repo",
				"imageManifest":  manifest,
				"imageTag":       "v1",
			}
			if tc.imageDigest != "" {
				body["imageDigest"] = tc.imageDigest
			}

			rec := doAccuracy(t, h, "PutImage", body)
			assert.Equal(t, tc.wantStatus, rec.Code)

			if tc.wantType != "" {
				out := parseAccuracy(t, rec)
				assert.Equal(t, tc.wantType, out["__type"])
			}
		})
	}
}

// Test_PutImage_DigestValidation_MatchingDigestAccepted verifies that a
// caller-supplied imageDigest which genuinely matches the manifest's computed
// sha256 is accepted (the validation only rejects a real mismatch).
func Test_PutImage_DigestValidation_MatchingDigestAccepted(t *testing.T) {
	t.Parallel()

	const manifest = `{"schemaVersion":2,"exact":"match"}`

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "digest-match-repo")

	// First push without an explicit digest so the server tells us the real one.
	digest := mustPutImage(t, h, "digest-match-repo", "v1", manifest)

	// Re-push the same manifest, this time supplying the (correct) digest
	// explicitly, as a real client that already knows it would.
	rec := doAccuracy(t, h, "PutImage", map[string]any{
		"repositoryName": "digest-match-repo",
		"imageManifest":  manifest,
		"imageTag":       "v2",
		"imageDigest":    digest,
	})
	assert.Equal(
		t, http.StatusOK, rec.Code, "matching imageDigest must be accepted: %s", rec.Body.String(),
	)
}

func TestECR_BatchDeleteImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup          func(*ecr.InMemoryBackend)
		name           string
		repositoryName string
		imageIDs       []map[string]any
		wantStatus     int
		wantDeleted    int
		wantFailures   int
	}{
		{
			name: "image not found returns failure",
			setup: func(b *ecr.InMemoryBackend) {
				b.CreateRepoInternal("my-repo")
			},
			repositoryName: "my-repo",
			imageIDs:       []map[string]any{{"imageDigest": "sha256:notfound"}},
			wantStatus:     http.StatusOK,
			wantDeleted:    0,
			wantFailures:   1,
		},
		{
			name: "empty image list returns empty results",
			setup: func(b *ecr.InMemoryBackend) {
				b.CreateRepoInternal("my-repo")
			},
			repositoryName: "my-repo",
			imageIDs:       []map[string]any{},
			wantStatus:     http.StatusOK,
			wantDeleted:    0,
			wantFailures:   0,
		},
		{
			name: "delete by digest succeeds",
			setup: func(b *ecr.InMemoryBackend) {
				b.CreateRepoInternal("my-repo")
				b.AddImageInternal("my-repo", ecr.Image{
					ImageDigest:    "sha256:abc111",
					ImageID:        ecr.ImageIdentifier{ImageDigest: "sha256:abc111"},
					RepositoryName: "my-repo",
					RegistryID:     testAccountID,
				})
			},
			repositoryName: "my-repo",
			imageIDs:       []map[string]any{{"imageDigest": "sha256:abc111"}},
			wantStatus:     http.StatusOK,
			wantDeleted:    1,
			wantFailures:   0,
		},
		{
			name: "delete by tag succeeds",
			setup: func(b *ecr.InMemoryBackend) {
				b.CreateRepoInternal("my-repo")
				b.AddImageInternal("my-repo", ecr.Image{
					ImageDigest: "sha256:tag111",
					ImageID: ecr.ImageIdentifier{
						ImageDigest: "sha256:tag111",
						ImageTag:    "latest",
					},
					RepositoryName: "my-repo",
					RegistryID:     testAccountID,
				})
			},
			repositoryName: "my-repo",
			imageIDs:       []map[string]any{{"imageTag": "latest"}},
			wantStatus:     http.StatusOK,
			wantDeleted:    1,
			wantFailures:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := ecr.NewInMemoryBackend(testAccountID, testRegion, testEndpoint)
			if tt.setup != nil {
				tt.setup(backend)
			}

			h := ecr.NewHandler(backend, nil)

			rec := doECRRequest(t, h, "BatchDeleteImage", map[string]any{
				"repositoryName": tt.repositoryName,
				"imageIds":       tt.imageIDs,
			})
			require.Equal(t, tt.wantStatus, rec.Code)

			out := parseAccuracy(t, rec)
			imageIDsOut, _ := out["imageIds"].([]any)
			failures, _ := out["failures"].([]any)
			assert.Len(t, imageIDsOut, tt.wantDeleted)
			assert.Len(t, failures, tt.wantFailures)
		})
	}
}

func TestECR_BatchGetImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup          func(*ecr.InMemoryBackend)
		name           string
		repositoryName string
		imageIDs       []map[string]any
		wantStatus     int
		wantImages     int
		wantFailures   int
	}{
		{
			name: "image not found returns failure",
			setup: func(b *ecr.InMemoryBackend) {
				b.CreateRepoInternal("my-repo")
			},
			repositoryName: "my-repo",
			imageIDs:       []map[string]any{{"imageDigest": "sha256:notfound"}},
			wantStatus:     http.StatusOK,
			wantImages:     0,
			wantFailures:   1,
		},
		{
			name: "empty image list returns empty results",
			setup: func(b *ecr.InMemoryBackend) {
				b.CreateRepoInternal("my-repo")
			},
			repositoryName: "my-repo",
			imageIDs:       []map[string]any{},
			wantStatus:     http.StatusOK,
			wantImages:     0,
			wantFailures:   0,
		},
		{
			name: "get image by digest succeeds",
			setup: func(b *ecr.InMemoryBackend) {
				b.CreateRepoInternal("my-repo")
				b.AddImageInternal("my-repo", ecr.Image{
					ImageDigest:    "sha256:getdig",
					ImageManifest:  `{"schemaVersion":2}`,
					ImageID:        ecr.ImageIdentifier{ImageDigest: "sha256:getdig"},
					RepositoryName: "my-repo",
					RegistryID:     testAccountID,
				})
			},
			repositoryName: "my-repo",
			imageIDs:       []map[string]any{{"imageDigest": "sha256:getdig"}},
			wantStatus:     http.StatusOK,
			wantImages:     1,
			wantFailures:   0,
		},
		{
			name: "get image by tag succeeds",
			setup: func(b *ecr.InMemoryBackend) {
				b.CreateRepoInternal("my-repo")
				b.AddImageInternal("my-repo", ecr.Image{
					ImageDigest:   "sha256:gettag",
					ImageManifest: `{"schemaVersion":2}`,
					ImageID: ecr.ImageIdentifier{
						ImageDigest: "sha256:gettag",
						ImageTag:    "stable",
					},
					RepositoryName: "my-repo",
					RegistryID:     testAccountID,
				})
			},
			repositoryName: "my-repo",
			imageIDs:       []map[string]any{{"imageTag": "stable"}},
			wantStatus:     http.StatusOK,
			wantImages:     1,
			wantFailures:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := ecr.NewInMemoryBackend(testAccountID, testRegion, testEndpoint)
			if tt.setup != nil {
				tt.setup(backend)
			}

			h := ecr.NewHandler(backend, nil)

			rec := doECRRequest(t, h, "BatchGetImage", map[string]any{
				"repositoryName": tt.repositoryName,
				"imageIds":       tt.imageIDs,
			})
			require.Equal(t, tt.wantStatus, rec.Code)

			out := parseAccuracy(t, rec)
			images, _ := out["images"].([]any)
			failures, _ := out["failures"].([]any)
			assert.Len(t, images, tt.wantImages)
			assert.Len(t, failures, tt.wantFailures)
		})
	}
}
