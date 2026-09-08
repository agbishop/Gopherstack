package glacier_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glacier"
)

// TestSortedListMultipartUploads verifies ListMultipartUploads returns sorted by MultipartUploadID.
func TestSortedListMultipartUploads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		uploadCount int
	}{
		{name: "uploads_sorted_by_id", uploadCount: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := glacier.NewInMemoryBackend()
			_, err := b.CreateVault(testAccountID, testRegion, "vault")
			require.NoError(t, err)

			for range tt.uploadCount {
				_, err = b.InitiateMultipartUpload(testAccountID, testRegion, "vault", "desc", 1024*1024)
				require.NoError(t, err)
			}

			uploads := b.ListMultipartUploads(testAccountID, testRegion, "vault")
			require.Len(t, uploads, tt.uploadCount)

			for i := 1; i < len(uploads); i++ {
				assert.LessOrEqual(t, uploads[i-1].MultipartUploadID, uploads[i].MultipartUploadID)
			}
		})
	}
}

// TestNonNilListMultipartUploads verifies ListMultipartUploads returns non-nil empty slice.
func TestNonNilListMultipartUploads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		vaultName string
	}{
		{name: "empty_vault_non_nil_uploads", vaultName: "empty-vault"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := glacier.NewInMemoryBackend()
			_, err := b.CreateVault(testAccountID, testRegion, tt.vaultName)
			require.NoError(t, err)

			uploads := b.ListMultipartUploads(testAccountID, testRegion, tt.vaultName)

			assert.NotNil(t, uploads)
			assert.Empty(t, uploads)
		})
	}
}

// TestUploadMultipartPart_Validation covers the rejection conditions
// documented on api_op_UploadMultipartPart.go: a SHA256 tree hash that
// doesn't match the uploaded data, a part larger than the size declared at
// InitiateMultipartUpload, and a Content-Range that doesn't align to a
// part-size boundary.
func TestUploadMultipartPart_Validation(t *testing.T) {
	t.Parallel()

	const partSize = 1 << 20 // 1 MiB

	body := []byte(strings.Repeat("x", partSize))
	realChecksum := glacier.ComputeTreeHash(body)

	tests := []struct {
		name     string
		rangeHdr string
		checksum string
		body     []byte
		wantErr  bool
	}{
		{
			name:     "valid_part_accepted",
			rangeHdr: "bytes 0-1048575/*",
			checksum: realChecksum,
			body:     body,
		},
		{
			name:     "no_checksum_supplied_accepted",
			rangeHdr: "bytes 0-1048575/*",
			body:     body,
		},
		{
			name:     "checksum_mismatch_rejected",
			rangeHdr: "bytes 0-1048575/*",
			checksum: strings.Repeat("0", 64),
			body:     body,
			wantErr:  true,
		},
		{
			name:     "oversized_part_rejected",
			rangeHdr: fmt.Sprintf("bytes 0-%d/*", 2*partSize-1),
			body:     []byte(strings.Repeat("x", 2*partSize)),
			wantErr:  true,
		},
		{
			name:     "misaligned_start_rejected",
			rangeHdr: fmt.Sprintf("bytes 100-%d/*", 100+partSize-1),
			body:     []byte(strings.Repeat("x", partSize)),
			wantErr:  true,
		},
		{
			name:     "body_length_mismatch_rejected",
			rangeHdr: "bytes 0-1048575/*",
			body:     []byte("too short"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := glacier.NewInMemoryBackend()
			_, err := b.CreateVault(testAccountID, testRegion, "vault")
			require.NoError(t, err)

			up, err := b.InitiateMultipartUpload(testAccountID, testRegion, "vault", "desc", partSize)
			require.NoError(t, err)

			err = b.UploadMultipartPart(
				testAccountID, testRegion, "vault", up.MultipartUploadID, tt.rangeHdr, tt.checksum, tt.body,
			)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestUploadMultipartPart_ReuploadOverwrites verifies that re-uploading the
// same Content-Range overwrites the previous part rather than duplicating it,
// per api_op_UploadMultipartPart.go: "If you upload the same part multiple
// times, the data included in the most recent request overwrites the
// previously uploaded data.".
func TestUploadMultipartPart_ReuploadOverwrites(t *testing.T) {
	t.Parallel()

	const partSize = 1 << 20

	b := glacier.NewInMemoryBackend()
	_, err := b.CreateVault(testAccountID, testRegion, "vault")
	require.NoError(t, err)

	up, err := b.InitiateMultipartUpload(testAccountID, testRegion, "vault", "desc", partSize)
	require.NoError(t, err)

	body1 := []byte(strings.Repeat("a", partSize))
	require.NoError(t, b.UploadMultipartPart(
		testAccountID, testRegion, "vault", up.MultipartUploadID, "bytes 0-1048575/*", "", body1,
	))

	body2 := []byte(strings.Repeat("b", partSize))
	require.NoError(t, b.UploadMultipartPart(
		testAccountID, testRegion, "vault", up.MultipartUploadID, "bytes 0-1048575/*", "", body2,
	))

	parts, err := b.ListParts(testAccountID, testRegion, "vault", up.MultipartUploadID)
	require.NoError(t, err)
	require.Len(t, parts.Parts, 1, "re-uploading the same range must overwrite, not duplicate")
	assert.Equal(t, glacier.ComputeTreeHash(body2), parts.Parts[0].SHA256TreeHash,
		"the most recent request's data must win, per AWS's documented idempotency")
}

// TestCompleteMultipartUpload_Validation covers the rejection conditions
// documented on api_op_CompleteMultipartUpload.go: an ArchiveSize that
// doesn't match the sum of uploaded part sizes, a Checksum that doesn't match
// the assembled archive's tree hash, a missing content range, and a non-last
// part smaller than the declared part size.
func TestCompleteMultipartUpload_Validation(t *testing.T) {
	t.Parallel()

	const partSize = 1 << 20

	t.Run("archive_size_mismatch_rejected", func(t *testing.T) {
		t.Parallel()

		b := glacier.NewInMemoryBackend()
		_, err := b.CreateVault(testAccountID, testRegion, "vault")
		require.NoError(t, err)

		up, err := b.InitiateMultipartUpload(testAccountID, testRegion, "vault", "desc", partSize)
		require.NoError(t, err)

		part := []byte(strings.Repeat("a", partSize))
		require.NoError(t, b.UploadMultipartPart(
			testAccountID, testRegion, "vault", up.MultipartUploadID, "bytes 0-1048575/*",
			glacier.ComputeTreeHash(part), part,
		))

		_, err = b.CompleteMultipartUpload(
			testAccountID, testRegion, "vault", up.MultipartUploadID,
			glacier.ComputeTreeHash(part), partSize*2,
		)
		require.Error(t, err)
	})

	t.Run("checksum_mismatch_rejected", func(t *testing.T) {
		t.Parallel()

		b := glacier.NewInMemoryBackend()
		_, err := b.CreateVault(testAccountID, testRegion, "vault")
		require.NoError(t, err)

		up, err := b.InitiateMultipartUpload(testAccountID, testRegion, "vault", "desc", partSize)
		require.NoError(t, err)

		part := []byte(strings.Repeat("a", partSize))
		require.NoError(t, b.UploadMultipartPart(
			testAccountID, testRegion, "vault", up.MultipartUploadID, "bytes 0-1048575/*",
			glacier.ComputeTreeHash(part), part,
		))

		_, err = b.CompleteMultipartUpload(
			testAccountID, testRegion, "vault", up.MultipartUploadID, strings.Repeat("0", 64), partSize,
		)
		require.Error(t, err)
	})

	t.Run("missing_content_range_rejected", func(t *testing.T) {
		t.Parallel()

		b := glacier.NewInMemoryBackend()
		_, err := b.CreateVault(testAccountID, testRegion, "vault")
		require.NoError(t, err)

		up, err := b.InitiateMultipartUpload(testAccountID, testRegion, "vault", "desc", partSize)
		require.NoError(t, err)

		// Only the SECOND part is uploaded -- [0, partSize) is missing. archiveSize
		// is set to what a gap-blind assembly would (wrongly) compute as the total
		// (2*partSize, i.e. the last part's end+1) so that only the dedicated gap
		// check -- not the independent ArchiveSize check -- can catch this.
		part2 := []byte(strings.Repeat("b", partSize))
		require.NoError(t, b.UploadMultipartPart(
			testAccountID, testRegion, "vault", up.MultipartUploadID,
			fmt.Sprintf("bytes %d-%d/*", partSize, 2*partSize-1),
			glacier.ComputeTreeHash(part2), part2,
		))

		_, err = b.CompleteMultipartUpload(
			testAccountID, testRegion, "vault", up.MultipartUploadID,
			glacier.ComputeTreeHash(part2), 2*partSize,
		)
		require.Error(t, err)
	})

	t.Run("non_last_part_smaller_than_declared_rejected", func(t *testing.T) {
		t.Parallel()

		b := glacier.NewInMemoryBackend()
		_, err := b.CreateVault(testAccountID, testRegion, "vault")
		require.NoError(t, err)

		up, err := b.InitiateMultipartUpload(testAccountID, testRegion, "vault", "desc", partSize)
		require.NoError(t, err)

		// UploadMultipartPart itself refuses any part whose start doesn't align to
		// a partSize boundary, which makes a short-and-not-last part impossible to
		// construct through it without also leaving a gap (a short part's end is
		// never partSize-aligned, so a contiguous next part could never pass the
		// alignment check either). Seed directly via AddMultipartPartInternal --
		// bypassing that enforcement, the way a different write path could -- to
		// build two genuinely contiguous parts where the first is short, isolating
		// CompleteMultipartUpload's own defence-in-depth check. The parts' tree
		// hashes are real (not garbage) and correctly combined into checksum below,
		// so only the part-size check -- not a checksum mismatch -- can reject this.
		h1 := glacier.ComputeTreeHash(make([]byte, partSize/2))
		h2 := glacier.ComputeTreeHash(make([]byte, partSize))

		b.AddMultipartPartInternal(testAccountID, testRegion, "vault", up.MultipartUploadID, glacier.MultipartPart{
			RangeInBytes:   fmt.Sprintf("bytes 0-%d/*", partSize/2-1),
			SHA256TreeHash: h1,
		})
		b.AddMultipartPartInternal(testAccountID, testRegion, "vault", up.MultipartUploadID, glacier.MultipartPart{
			RangeInBytes:   fmt.Sprintf("bytes %d-%d/*", partSize/2, partSize/2+partSize-1),
			SHA256TreeHash: h2,
		})

		h1Bytes, err := hex.DecodeString(h1)
		require.NoError(t, err)
		h2Bytes, err := hex.DecodeString(h2)
		require.NoError(t, err)
		combined := sha256.Sum256(append(append([]byte{}, h1Bytes...), h2Bytes...))
		checksum := hex.EncodeToString(combined[:])

		_, err = b.CompleteMultipartUpload(
			testAccountID, testRegion, "vault", up.MultipartUploadID,
			checksum, int64(partSize/2+partSize),
		)
		require.Error(t, err)
	})
}

// TestCompleteMultipartUpload_AssemblesRealData is the regression test for the
// most severe finding: uploaded part bytes were discarded entirely, so a
// multipart-completed archive had no retrievable data (GetArchiveData always
// returned not-found). This verifies the assembled archive's bytes equal the
// concatenation of the uploaded parts in range order, and that the resulting
// tree hash matches computing it directly over those assembled bytes.
func TestCompleteMultipartUpload_AssemblesRealData(t *testing.T) {
	t.Parallel()

	const partSize = 1 << 20

	b := glacier.NewInMemoryBackend()
	_, err := b.CreateVault(testAccountID, testRegion, "vault")
	require.NoError(t, err)

	up, err := b.InitiateMultipartUpload(testAccountID, testRegion, "vault", "desc", partSize)
	require.NoError(t, err)

	part1 := []byte(strings.Repeat("a", partSize))
	part2 := []byte(strings.Repeat("b", partSize/2))

	require.NoError(t, b.UploadMultipartPart(
		testAccountID, testRegion, "vault", up.MultipartUploadID,
		"bytes 0-1048575/*", glacier.ComputeTreeHash(part1), part1,
	))
	require.NoError(t, b.UploadMultipartPart(
		testAccountID, testRegion, "vault", up.MultipartUploadID,
		fmt.Sprintf("bytes %d-%d/*", partSize, partSize+partSize/2-1), glacier.ComputeTreeHash(part2), part2,
	))

	expected := append(append([]byte{}, part1...), part2...)
	expectedHash := glacier.ComputeTreeHash(expected)

	archive, err := b.CompleteMultipartUpload(
		testAccountID, testRegion, "vault", up.MultipartUploadID, expectedHash, int64(len(expected)),
	)
	require.NoError(t, err)
	assert.Equal(t, expectedHash, archive.SHA256TreeHash)

	data, ok := b.GetArchiveData(archive.ArchiveID)
	require.True(t, ok, "multipart-assembled archive data must be retrievable")
	assert.Equal(t, expected, data,
		"assembled archive bytes must equal the concatenation of uploaded parts, in range order")
}
