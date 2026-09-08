package ecr_test

// layers_test.go — verifies layers.go: InitiateLayerUpload (abandoned-upload
// pruning), UploadLayerPart (part sequencing), CompleteLayerUpload (repo
// existence, LayerAlreadyExistsException), BatchCheckLayerAvailability, and
// GetDownloadUrlForLayer, at both the backend and HTTP-handler level.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecr"
)

func TestInitiateLayerUpload_PrunesAbandonedUploads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		abandoned int
	}{
		{name: "one abandoned", abandoned: 1},
		{name: "many abandoned", abandoned: 6},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "ecr.local")

			_, err := b.CreateRepository(ctx, "repo", "MUTABLE", false, "", "")
			require.NoError(t, err)

			for range tc.abandoned {
				_, err = b.InitiateLayerUpload(ctx, "repo")
				require.NoError(t, err)
			}
			require.Equal(t, tc.abandoned, b.LayerUploadCount())

			// Age the abandoned uploads past the TTL, then start a new one. The
			// stale uploads must be pruned, leaving only the fresh session.
			b.AgeAllLayerUploadsForTest(ecr.LayerUploadTTLForTest + 1)

			_, err = b.InitiateLayerUpload(ctx, "repo")
			require.NoError(t, err)

			require.Equal(t, 1, b.LayerUploadCount(),
				"abandoned layer uploads must be pruned on InitiateLayerUpload")
		})
	}
}

func TestUploadLayerPart_KeepsActiveUploadAlive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "ecr.local")

	_, err := b.CreateRepository(ctx, "repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	init, err := b.InitiateLayerUpload(ctx, "repo")
	require.NoError(t, err)

	// Age the upload, but then send a part — activity must refresh CreatedAt.
	b.AgeAllLayerUploadsForTest(ecr.LayerUploadTTLForTest + 1)
	_, err = b.UploadLayerPart(ctx, "repo", init.UploadID, 0, -1, []byte("data"))
	require.NoError(t, err)

	// A subsequent initiate must NOT prune the refreshed upload.
	_, err = b.InitiateLayerUpload(ctx, "repo")
	require.NoError(t, err)

	require.Equal(t, 2, b.LayerUploadCount(),
		"an actively-uploading session must survive pruning")
}

func TestLayerUploadFlow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantType error
		data     []byte
		wantErr  bool
	}{
		{
			name: "valid_layer_completes",
			data: []byte("fake-layer-data"),
		},
		{
			// AWS rejects CompleteLayerUpload against a live session that
			// never received any UploadLayerPart data with
			// EmptyUploadException ("The specified layer upload does not
			// contain any layer parts."). Older revisions of this test
			// asserted the opposite (a long-standing test-seeding
			// convenience); that shortcut has been removed -- see
			// EmptyUploadException enforcement in resolveCompletedLayerLocked.
			name:     "empty_upload_rejected",
			data:     []byte{},
			wantErr:  true,
			wantType: ecr.ErrEmptyUpload,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			makeRepo(t, b, "layer-repo")

			init, err := b.InitiateLayerUpload(context.Background(), "layer-repo")
			require.NoError(t, err)
			assert.NotEmpty(t, init.UploadID)

			if len(tt.data) > 0 {
				_, err = b.UploadLayerPart(context.Background(), "layer-repo", init.UploadID,
					0, int64(len(tt.data))-1, tt.data)
				require.NoError(t, err)
			}

			result, err := b.CompleteLayerUpload(context.Background(), "layer-repo", init.UploadID, nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantType)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, result.LayerDigest)
			}
		})
	}
}

func TestBatchCheckLayerAvailability_Backend(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	makeRepo(t, b, "bchk-repo")

	init, err := b.InitiateLayerUpload(context.Background(), "bchk-repo")
	require.NoError(t, err)
	_, err = b.UploadLayerPart(context.Background(), "bchk-repo", init.UploadID, 0, 9, []byte("layer-data"))
	require.NoError(t, err)
	result, err := b.CompleteLayerUpload(context.Background(), "bchk-repo", init.UploadID, nil)
	require.NoError(t, err)
	digest := result.LayerDigest

	avail, failures, err := b.BatchCheckLayerAvailability(context.Background(), "bchk-repo",
		[]string{digest, "sha256:nonexistent"})
	require.NoError(t, err)

	assert.Len(t, avail, 1, "one known layer must be AVAILABLE")
	assert.Equal(t, "AVAILABLE", avail[0].LayerAvailability)
	assert.Equal(t, digest, avail[0].LayerDigest)

	assert.Len(t, failures, 1, "unknown layer must produce a failure")
	assert.Equal(t, "sha256:nonexistent", failures[0].LayerDigest)
}

func Test_CompleteLayerUpload_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		setup      func(t *testing.T) (h *ecr.Handler, repoName, uploadID string)
		name       string
		wantType   string
		wantStatus int
	}{
		{
			name: "repository not found",
			setup: func(t *testing.T) (*ecr.Handler, string, string) {
				t.Helper()

				return newAccuracyHandler(), "does-not-exist", "upload-1"
			},
			wantStatus: http.StatusNotFound,
			wantType:   "RepositoryNotFoundException",
		},
		{
			name: "duplicate layer digest rejected",
			setup: func(t *testing.T) (*ecr.Handler, string, string) {
				t.Helper()

				h := newAccuracyHandler()
				mustCreateRepo(t, h, "dup-layer-repo")

				// Seed the digest as an already-registered layer via a real
				// Initiate -> UploadPart -> Complete flow (the old "direct
				// digest" shortcut no longer works: CompleteLayerUpload now
				// requires a live upload session, see UploadNotFoundException).
				mustUploadLayerHTTP(t, h, "dup-layer-repo", []byte("seed"), "sha256:aaaa")

				// A second live session, so the assertion call below hits
				// LayerAlreadyExistsException rather than UploadNotFoundException.
				secondUploadID := mustUploadLayerPartHTTP(t, h, "dup-layer-repo", []byte("seed2"))

				return h, "dup-layer-repo", secondUploadID
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "LayerAlreadyExistsException",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, repoName, uploadID := tc.setup(t)

			rec := doAccuracy(t, h, "CompleteLayerUpload", map[string]any{
				"repositoryName": repoName,
				"uploadId":       uploadID,
				"layerDigests":   []string{"sha256:aaaa"},
			})

			assert.Equal(t, tc.wantStatus, rec.Code)
			out := parseAccuracy(t, rec)
			assert.Equal(t, tc.wantType, out["__type"])
		})
	}
}

// Test_CompleteLayerUpload_UnknownUploadID_ReturnsUploadNotFoundException
// locks the removal of the old "direct digest" shortcut: completing an
// uploadId with no live InitiateLayerUpload session must now fail with
// UploadNotFoundException rather than silently succeeding.
func Test_CompleteLayerUpload_UnknownUploadID_ReturnsUploadNotFoundException(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "fresh-layer-repo")

	rec := doAccuracy(t, h, "CompleteLayerUpload", map[string]any{
		"repositoryName": "fresh-layer-repo",
		"uploadId":       "upload-never-initiated",
		"layerDigests":   []string{"sha256:bbbb"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "UploadNotFoundException", out["__type"])
}

func Test_UploadLayerPart_PartSequencing(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		wantType       string
		firstBytes     []int64
		blobs          []string
		wantLastStatus int
	}{
		{
			name:           "consecutive parts accepted",
			firstBytes:     []int64{0, 4},
			blobs:          []string{"AQIDBA==", "BQYHCA=="}, // 4 bytes each
			wantLastStatus: http.StatusOK,
		},
		{
			name:           "first part must start at zero",
			firstBytes:     []int64{4},
			blobs:          []string{"AQIDBA=="},
			wantLastStatus: http.StatusBadRequest,
			wantType:       "InvalidLayerPartException",
		},
		{
			name:           "gap between parts rejected",
			firstBytes:     []int64{0, 10},
			blobs:          []string{"AQIDBA==", "BQYHCA=="},
			wantLastStatus: http.StatusBadRequest,
			wantType:       "InvalidLayerPartException",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newAccuracyHandler()
			mustCreateRepo(t, h, "seq-repo")

			initRec := doAccuracy(t, h, "InitiateLayerUpload", map[string]any{
				"repositoryName": "seq-repo",
			})
			require.Equal(t, http.StatusOK, initRec.Code)
			uploadID, _ := parseAccuracy(t, initRec)["uploadId"].(string)
			require.NotEmpty(t, uploadID)

			rec := initRec
			for i, firstByte := range tc.firstBytes {
				rec = doAccuracy(t, h, "UploadLayerPart", map[string]any{
					"repositoryName": "seq-repo",
					"uploadId":       uploadID,
					"partFirstByte":  firstByte,
					"partLastByte":   firstByte + 3,
					"layerPartBlob":  tc.blobs[i],
				})
			}

			assert.Equal(t, tc.wantLastStatus, rec.Code)
			if tc.wantType != "" {
				out := parseAccuracy(t, rec)
				assert.Equal(t, tc.wantType, out["__type"])
			}
		})
	}
}

func TestECR_BatchCheckLayerAvailability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		repositoryName string
		layerDigests   []string
		wantStatus     int
		wantLayers     int
		wantFailures   int
		preUpload      bool
	}{
		{
			name:           "no layers uploaded returns failures",
			repositoryName: "my-repo",
			layerDigests:   []string{"sha256:abc123"},
			wantStatus:     http.StatusOK,
			wantLayers:     0,
			wantFailures:   1,
		},
		{
			name:           "uploaded layer is available",
			preUpload:      true,
			repositoryName: "my-repo",
			layerDigests:   []string{"sha256:abc123"},
			wantStatus:     http.StatusOK,
			wantLayers:     1,
			wantFailures:   0,
		},
		{
			name:           "empty digests returns empty results",
			repositoryName: "my-repo",
			layerDigests:   []string{},
			wantStatus:     http.StatusOK,
			wantLayers:     0,
			wantFailures:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Repository must exist for BatchCheckLayerAvailability.
			rec0 := doECRRequest(
				t,
				h,
				"CreateRepository",
				map[string]any{"repositoryName": tt.repositoryName},
			)
			require.Equal(t, http.StatusOK, rec0.Code)

			if tt.preUpload {
				mustUploadLayerHTTP(t, h, tt.repositoryName, []byte("seed"), "sha256:abc123")
			}

			rec := doECRRequest(t, h, "BatchCheckLayerAvailability", map[string]any{
				"repositoryName": tt.repositoryName,
				"layerDigests":   tt.layerDigests,
			})
			require.Equal(t, tt.wantStatus, rec.Code)

			out := parseAccuracy(t, rec)
			assert.Len(t, out["layers"], tt.wantLayers)
			assert.Len(t, out["failures"], tt.wantFailures)
		})
	}
}

func TestECR_CompleteLayerUpload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		repositoryName string
		wantDigest     string
		layerDigests   []string
		wantStatus     int
	}{
		{
			name:           "completes upload and returns digest",
			repositoryName: "my-repo",
			layerDigests:   []string{"sha256:abc123"},
			wantStatus:     http.StatusOK,
			wantDigest:     "sha256:abc123",
		},
		{
			name:           "empty digests still completes using the computed digest",
			repositoryName: "my-repo",
			layerDigests:   []string{},
			wantStatus:     http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// The repository must exist: real ECR returns RepositoryNotFoundException
			// for CompleteLayerUpload against an unknown repository.
			createRec := doECRRequest(
				t, h, "CreateRepository", map[string]any{"repositoryName": tt.repositoryName},
			)
			require.Equal(t, http.StatusOK, createRec.Code)

			// A real Initiate -> UploadPart session is required: CompleteLayerUpload
			// now returns UploadNotFoundException for any uploadId that was never
			// Initiated (the old "direct digest" shortcut has been removed) and
			// EmptyUploadException for a session with no uploaded parts.
			uploadID := mustUploadLayerPartHTTP(t, h, tt.repositoryName, []byte("layer-bytes"))

			rec := doECRRequest(t, h, "CompleteLayerUpload", map[string]any{
				"repositoryName": tt.repositoryName,
				"uploadId":       uploadID,
				"layerDigests":   tt.layerDigests,
			})
			require.Equal(t, tt.wantStatus, rec.Code)

			out := parseAccuracy(t, rec)
			if tt.wantDigest != "" {
				assert.Equal(t, tt.wantDigest, out["layerDigest"])
			} else {
				assert.NotEmpty(t, out["layerDigest"],
					"empty layerDigests must fall back to the computed digest")
			}
			assert.Equal(t, tt.repositoryName, out["repositoryName"])
			assert.Equal(t, uploadID, out["uploadId"])
		})
	}
}

func TestECR_RestoreClearsInFlightLayerUploads(t *testing.T) {
	t.Parallel()

	backend := ecr.NewInMemoryBackend(testAccountID, testRegion, testEndpoint)
	h := ecr.NewHandler(backend, nil)

	rec := doECRRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "restore-repo"})
	require.Equal(t, http.StatusOK, rec.Code)
	rec = doECRRequest(
		t,
		h,
		"InitiateLayerUpload",
		map[string]any{"repositoryName": "restore-repo"},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	upload := parseAccuracy(t, rec)
	snapshot := h.Snapshot(t.Context())
	require.NotEmpty(t, snapshot)
	require.NoError(t, h.Restore(t.Context(), snapshot))

	rec = doECRRequest(t, h, "UploadLayerPart", map[string]any{
		"repositoryName": "restore-repo",
		"uploadId":       upload["uploadId"],
		"partFirstByte":  0,
		"partLastByte":   0,
		"layerPartBlob":  "AQ==",
	})
	// In-flight layer uploads are intentionally NOT persisted across
	// Snapshot/Restore, so the uploadId from before the restore is now
	// unknown. Real AWS returns UploadNotFoundException (400) for an unknown
	// uploadId, not RepositoryNotFoundException (404) -- see the round 3
	// wire-shape fix in UploadLayerPart (layers.go): this test previously
	// asserted 404, which was itself a bug this fix corrected.
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "UploadNotFoundException", out["__type"])
}

func TestBatchCheckLayerAvailability_NonExistentRepo_Returns404(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "BatchCheckLayerAvailability", map[string]any{
		"repositoryName": "does-not-exist",
		"layerDigests":   []string{"sha256:aabbcc"},
	})
	assert.Equal(t, http.StatusNotFound, rec.Code,
		"BatchCheckLayerAvailability must return 404 for non-existent repository")
}

func TestBatchCheckLayerAvailability_ExistingRepo_AvailableLayer(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "layer-repo-accuracy")

	// Upload a layer then check its availability.
	mustUploadLayerHTTP(t, h, "layer-repo-accuracy", []byte("seed"), "sha256:deadbeef")

	rec := doAccuracy(t, h, "BatchCheckLayerAvailability", map[string]any{
		"repositoryName": "layer-repo-accuracy",
		"layerDigests":   []string{"sha256:deadbeef"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	layers, _ := out["layers"].([]any)
	require.Len(t, layers, 1)
	layer := layers[0].(map[string]any)
	assert.Equal(t, "sha256:deadbeef", layer["layerDigest"])
	assert.Equal(t, "AVAILABLE", layer["layerAvailability"])
}

func TestBatchCheckLayerAvailability_ExistingRepo_MissingLayer(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "empty-layer-repo")

	rec := doAccuracy(t, h, "BatchCheckLayerAvailability", map[string]any{
		"repositoryName": "empty-layer-repo",
		"layerDigests":   []string{"sha256:nothere"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	layers, _ := out["layers"].([]any)
	failures, _ := out["failures"].([]any)
	assert.Empty(t, layers, "no available layers expected")
	require.Len(t, failures, 1)
	failure := failures[0].(map[string]any)
	assert.Equal(t, "sha256:nothere", failure["layerDigest"])
}

func TestLayerUpload_PartSize_Tracked(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "upload-size-repo")

	initRec := doAccuracy(t, h, "InitiateLayerUpload", map[string]any{
		"repositoryName": "upload-size-repo",
	})
	require.Equal(t, http.StatusOK, initRec.Code)
	initOut := parseAccuracy(t, initRec)
	uploadID, _ := initOut["uploadId"].(string)
	require.NotEmpty(t, uploadID)

	// Upload returns lastByteReceived.
	blob := make([]byte, 100)
	uploadRec := doAccuracy(t, h, "UploadLayerPart", map[string]any{
		"repositoryName": "upload-size-repo",
		"uploadId":       uploadID,
		"partFirstByte":  0,
		"partLastByte":   99,
		"layerPartBlob":  blob,
	})
	require.Equal(t, http.StatusOK, uploadRec.Code)
	uploadOut := parseAccuracy(t, uploadRec)
	assert.InDelta(t, float64(99), uploadOut["lastByteReceived"], 0,
		"lastByteReceived must equal partLastByte")
}

func TestInitiateLayerUpload_NonExistentRepo_Returns404(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "InitiateLayerUpload", map[string]any{
		"repositoryName": "ghost",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestInitiateLayerUpload_ReturnsUploadIdAndPartSize(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "upload-meta")

	rec := doAccuracy(t, h, "InitiateLayerUpload", map[string]any{
		"repositoryName": "upload-meta",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	assert.NotEmpty(t, out["uploadId"], "uploadId must be returned")
	partSize, _ := out["partSize"].(float64)
	assert.Greater(t, partSize, float64(0), "partSize must be positive")
}

func TestCompleteLayerUpload_Makes_Layer_Available(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "complete-upload")

	// A live upload session must actually receive part data before it can be
	// completed (EmptyUploadException otherwise).
	digest := mustUploadLayerHTTP(t, h, "complete-upload", []byte("layer-bytes"), "sha256:cafebabe")

	checkRec := doAccuracy(t, h, "BatchCheckLayerAvailability", map[string]any{
		"repositoryName": "complete-upload",
		"layerDigests":   []string{digest},
	})
	require.Equal(t, http.StatusOK, checkRec.Code)
	out := parseAccuracy(t, checkRec)
	layers, _ := out["layers"].([]any)
	require.Len(t, layers, 1)
	assert.Equal(t, "AVAILABLE", layers[0].(map[string]any)["layerAvailability"])
}

func TestBatchCheckLayerAvailability_AllAvailable(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "layer-repo-2", "MUTABLE", false, "", "")
	require.NoError(t, err)

	// A full-length sha256-looking digest is now digest-verified against the
	// real uploaded bytes (see verifiedUploadDigestLocked), so this must use
	// the digest ECR actually computes rather than an arbitrary fixed
	// literal -- the old "direct digest" shortcut that accepted any literal
	// unconditionally no longer exists.
	digest := mustUploadLayer(t, b, "layer-repo-2", []byte("layer-repo-2-bytes"))

	h := ecr.NewHandler(b, nil)
	rec := doAccuracy(t, h, "BatchCheckLayerAvailability", map[string]any{
		"repositoryName": "layer-repo-2",
		"layerDigests":   []string{digest},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	layers, _ := out["layers"].([]any)
	failures, _ := out["failures"].([]any)

	require.Len(t, layers, 1)
	require.Empty(t, failures)
	layer := layers[0].(map[string]any)
	assert.Equal(t, "AVAILABLE", layer["layerAvailability"])
	assert.Equal(t, digest, layer["layerDigest"])
}

func TestBatchCheckLayerAvailability_Mixed(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "mixed-layer-repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	// presentDigest must be the digest ECR actually computes for the
	// uploaded bytes (full-length sha256-looking digests are now verified);
	// missingDigest is never uploaded, so it stays an arbitrary literal.
	presentDigest := mustUploadLayer(t, b, "mixed-layer-repo", []byte("mixed-layer-repo-bytes"))
	missingDigest := "sha256:9999999999999999999999999999999999999999999999999999999999999999"

	h := ecr.NewHandler(b, nil)
	rec := doAccuracy(t, h, "BatchCheckLayerAvailability", map[string]any{
		"repositoryName": "mixed-layer-repo",
		"layerDigests":   []string{presentDigest, missingDigest},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	layers, _ := out["layers"].([]any)
	failures, _ := out["failures"].([]any)

	assert.Len(t, layers, 1, "one present layer must appear in layers")
	assert.Len(t, failures, 1, "one missing layer must appear in failures")
	failure := failures[0].(map[string]any)
	assert.Equal(t, missingDigest, failure["layerDigest"])
	assert.Equal(t, "MissingLayerDigest", failure["failureCode"])
}

func TestBatchCheckLayerAvailability_RepoNotFound_Errors(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()

	rec := doAccuracy(t, h, "BatchCheckLayerAvailability", map[string]any{
		"repositoryName": "does-not-exist",
		"layerDigests":   []string{"sha256:abcdef"},
	})
	assert.NotEqual(t, http.StatusOK, rec.Code)
}

func TestGetDownloadUrlForLayer_URLFormat(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "download-repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	digest := mustUploadLayer(t, b, "download-repo", []byte("download-repo-bytes"))

	h := ecr.NewHandler(b, nil)
	rec := doAccuracy(t, h, "GetDownloadUrlForLayer", map[string]any{
		"repositoryName": "download-repo",
		"layerDigest":    digest,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	downloadURL, _ := out["downloadUrl"].(string)
	assert.NotEmpty(t, downloadURL, "downloadUrl must be present")
	assert.Contains(t, downloadURL, "download-repo",
		"downloadUrl must contain the repository name")
	assert.Contains(t, downloadURL, digest,
		"downloadUrl must contain the layer digest")
	assert.Equal(t, digest, out["layerDigest"],
		"layerDigest in response must match the requested digest")
}

func TestGetDownloadUrlForLayer_MissingLayer_Errors(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "missing-layer-repo")

	rec := doAccuracy(t, h, "GetDownloadUrlForLayer", map[string]any{
		"repositoryName": "missing-layer-repo",
		"layerDigest":    "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	})
	assert.NotEqual(t, http.StatusOK, rec.Code,
		"GetDownloadUrlForLayer for non-existent layer must error")
}

func TestGetDownloadURLForLayer_LayerErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantType   string
		wantCode   int
		repoExists bool
	}{
		{
			name:       "repo_not_found",
			repoExists: false,
			wantCode:   http.StatusNotFound,
			wantType:   "RepositoryNotFoundException",
		},
		{
			name:       "layer_not_in_repo_returns_LayerInaccessibleException",
			repoExists: true,
			wantCode:   http.StatusBadRequest,
			wantType:   "LayerInaccessibleException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newHandlerWithBackend()
			ctx := context.Background()

			if tt.repoExists {
				_, err := b.CreateRepository(ctx, "myrepo", "MUTABLE", false, "", "")
				require.NoError(t, err)
			}

			rec := doAccuracy(t, h, "GetDownloadUrlForLayer", map[string]any{
				"repositoryName": "myrepo",
				"layerDigest":    "sha256:" + strings.Repeat("a", 64),
			})
			assert.Equal(t, tt.wantCode, rec.Code)

			out := parseAccuracy(t, rec)
			assert.Equal(t, tt.wantType, out["__type"])
		})
	}
}

// TestCompleteLayerUpload_UnknownUploadID_DifferentRepo_ReturnsUploadNotFoundException
// locks that an uploadId belonging to a live session in a DIFFERENT
// repository is treated the same as an unknown uploadId: AWS scopes upload
// sessions to the repository they were Initiated against.
func TestCompleteLayerUpload_UnknownUploadID_DifferentRepo_ReturnsUploadNotFoundException(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "repo-a")
	mustCreateRepo(t, h, "repo-b")

	uploadID := mustUploadLayerPartHTTP(t, h, "repo-a", []byte("data"))

	rec := doAccuracy(t, h, "CompleteLayerUpload", map[string]any{
		"repositoryName": "repo-b",
		"uploadId":       uploadID,
		"layerDigests":   []string{"sha256:xyz"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "UploadNotFoundException", out["__type"])
}

// TestCompleteLayerUpload_LayerPartTooSmall locks the 5MiB minimum
// non-last-part-size rule (LayerPartTooSmallException): AWS requires every
// UploadLayerPart except the last in a session to be at least 5MiB. A
// single-part upload is never rejected (its one part is always "last"),
// which is why every other test in this package that uploads a tiny single
// part continues to work unchanged.
func TestCompleteLayerUpload_LayerPartTooSmall(t *testing.T) {
	t.Parallel()

	// Mirrors the unexported minLayerPartSize constant in models.go (5MiB);
	// duplicated here rather than exported via export_test.go.
	const minLayerPartSizeForTest = 5 * 1024 * 1024

	small := make([]byte, 10)
	big := make([]byte, minLayerPartSizeForTest)

	tests := []struct {
		name       string
		wantType   string
		parts      [][]byte
		wantStatus int
	}{
		{
			name:       "single_tiny_part_is_always_last_and_succeeds",
			parts:      [][]byte{small},
			wantStatus: http.StatusOK,
		},
		{
			name:       "two_parts_first_too_small_rejected",
			parts:      [][]byte{small, small},
			wantStatus: http.StatusBadRequest,
			wantType:   "LayerPartTooSmallException",
		},
		{
			name:       "two_parts_first_full_size_last_tiny_succeeds",
			parts:      [][]byte{big, small},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAccuracyHandler()
			mustCreateRepo(t, h, "part-size-repo")

			initRec := doAccuracy(t, h, "InitiateLayerUpload", map[string]any{
				"repositoryName": "part-size-repo",
			})
			require.Equal(t, http.StatusOK, initRec.Code)
			uploadID, _ := parseAccuracy(t, initRec)["uploadId"].(string)
			require.NotEmpty(t, uploadID)

			var firstByte int64
			for _, part := range tt.parts {
				rec := doAccuracy(t, h, "UploadLayerPart", map[string]any{
					"repositoryName": "part-size-repo",
					"uploadId":       uploadID,
					"partFirstByte":  firstByte,
					"partLastByte":   firstByte + int64(len(part)) - 1,
					"layerPartBlob":  part,
				})
				require.Equal(t, http.StatusOK, rec.Code, "UploadLayerPart failed: %s", rec.Body.String())
				firstByte += int64(len(part))
			}

			rec := doAccuracy(t, h, "CompleteLayerUpload", map[string]any{
				"repositoryName": "part-size-repo",
				"uploadId":       uploadID,
				"layerDigests":   []string{},
			})
			assert.Equal(t, tt.wantStatus, rec.Code, "CompleteLayerUpload body: %s", rec.Body.String())

			if tt.wantType != "" {
				out := parseAccuracy(t, rec)
				assert.Equal(t, tt.wantType, out["__type"])
			}
		})
	}
}

// TestInitiateLayerUpload_Reset_CounterRestarts verifies that Reset() zeroes
// layerUploadSeq, so the next upload ID's sequence suffix restarts at 1
// (matching ec2's fix establishing that this codebase resets ID sequence
// counters on Reset -- nextPrivateIPIndex, nextElasticIPIndex), not a suffix
// that keeps climbing from the pre-Reset run.
func TestInitiateLayerUpload_Reset_CounterRestarts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "ecr.local")

	_, err := b.CreateRepository(ctx, "repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	init1, err := b.InitiateLayerUpload(ctx, "repo")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(init1.UploadID, "-1"),
		"sanity: first upload ID should end in -1, got %s", init1.UploadID)

	b.Reset()

	_, err = b.CreateRepository(ctx, "repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	init2, err := b.InitiateLayerUpload(ctx, "repo")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(init2.UploadID, "-1"),
		"layerUploadSeq must restart at 1 after Reset, got upload ID %s", init2.UploadID)
}
