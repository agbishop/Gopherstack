package glacier

import (
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// multipartUploadIDLength is the length of the random multipart upload ID.
	multipartUploadIDLength = 60
	// minMultipartPartSize is the minimum part size for multipart uploads (1 MiB).
	minMultipartPartSize = 1 << 20
	// maxMultipartPartSize is the maximum part size for multipart uploads (4 GiB).
	maxMultipartPartSize = 4 << 30
)

// uploadKey uniquely identifies a multipart upload, and remains the key type
// for the (still-raw) multipartParts map -- see store_setup.go's package doc
// for why multipartParts wasn't converted to a *store.Table.
type uploadKey struct {
	AccountID string `json:"accountID"`
	Region    string `json:"region"`
	VaultName string `json:"vaultName"`
	UploadID  string `json:"uploadID"`
}

// isPowerOfTwo reports whether n is a power of two (n > 0).
func isPowerOfTwo(n int64) bool {
	return n > 0 && (n&(n-1)) == 0
}

// parseMultipartRange parses a "bytes START-END/*" Content-Range header into its
// inclusive start/end byte offsets. Returns ok=false if the header is not in
// exactly that format -- mirrors isValidMultipartRange's own parsing so the two
// never disagree on what counts as valid.
func parseMultipartRange(rangeHeader string) (int64, int64, bool) {
	const prefix = "bytes "
	if !strings.HasPrefix(rangeHeader, prefix) {
		return 0, 0, false
	}

	rest := rangeHeader[len(prefix):]

	const suffix = "/*"
	if !strings.HasSuffix(rest, suffix) {
		return 0, 0, false
	}

	rangePart := rest[:len(rest)-len(suffix)]
	dashIdx := strings.IndexByte(rangePart, '-')

	if dashIdx <= 0 || dashIdx == len(rangePart)-1 {
		return 0, 0, false
	}

	s, err1 := strconv.ParseInt(rangePart[:dashIdx], 10, 64)
	e, err2 := strconv.ParseInt(rangePart[dashIdx+1:], 10, 64)

	if err1 != nil || err2 != nil || s < 0 || e < s {
		return 0, 0, false
	}

	return s, e, true
}

// InitiateMultipartUpload begins a multipart upload for a vault.
func (b *InMemoryBackend) InitiateMultipartUpload(
	accountID, region, vaultName, description string,
	partSize int64,
) (*MultipartUpload, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	v, ok := b.vaults.Get(vaultARN(accountID, region, vaultName))
	if !ok {
		return nil, ErrVaultNotFound
	}

	// Part size must be a power of 2 between 1 MiB and 4 GiB (inclusive).
	if partSize != 0 &&
		(!isPowerOfTwo(partSize) || partSize < minMultipartPartSize || partSize > maxMultipartPartSize) {
		return nil, ErrValidation
	}

	uploadID := generateID(multipartUploadIDLength)
	up := &MultipartUpload{
		MultipartUploadID:  uploadID,
		VaultARN:           v.VaultARN,
		ArchiveDescription: description,
		PartSizeInBytes:    partSize,
		CreationDate:       formatDate(time.Now()),
	}

	b.multipartUploads.Put(up)

	return up, nil
}

// UploadMultipartPart records a part for an in-progress multipart upload.
//
// Per api_op_UploadMultipartPart.go's doc comment, Amazon Glacier rejects the
// request if the supplied SHA256 tree hash doesn't match the uploaded data, if
// the part size exceeds the size declared at InitiateMultipartUpload, or if the
// Content-Range doesn't align to a part-size boundary. A part smaller than the
// declared size that turns out not to be the last part is accepted here (AWS
// can't know yet whether it's last) and is instead rejected by
// CompleteMultipartUpload. Re-uploading the same range overwrites the previous
// part, matching the operation's documented idempotency.
func (b *InMemoryBackend) UploadMultipartPart(
	accountID, region, vaultName, uploadID, rangeHeader, checksum string,
	data []byte,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	vArn := vaultARN(accountID, region, vaultName)

	if !b.vaults.Has(vArn) {
		return ErrVaultNotFound
	}

	up, ok := b.multipartUploads.Get(multipartUploadKey(vArn, uploadID))
	if !ok {
		return ErrUploadNotFound
	}

	start, end, ok := parseMultipartRange(rangeHeader)
	if !ok {
		return fmt.Errorf("%w: Content-Range must be in the form \"bytes START-END/*\"", ErrValidation)
	}

	length := end - start + 1

	if length != int64(len(data)) {
		return fmt.Errorf(
			"%w: uploaded data length (%d) does not match Content-Range length (%d)",
			ErrValidation, len(data), length,
		)
	}

	if up.PartSizeInBytes > 0 {
		if length > up.PartSizeInBytes {
			return fmt.Errorf(
				"%w: part size (%d) exceeds the part size declared at InitiateMultipartUpload (%d)",
				ErrValidation, length, up.PartSizeInBytes,
			)
		}

		if start%up.PartSizeInBytes != 0 {
			return fmt.Errorf(
				"%w: Content-Range start %d does not align with part size %d",
				ErrValidation, start, up.PartSizeInBytes,
			)
		}
	}

	computed := computeTreeHash(data)
	if checksum != "" && checksum != computed {
		return fmt.Errorf("%w: SHA256 tree hash does not match: computed %s", ErrValidation, computed)
	}

	treeHash := computed
	if checksum != "" {
		treeHash = checksum
	}

	uKey := uploadKey{AccountID: accountID, Region: region, VaultName: vaultName, UploadID: uploadID}
	part := MultipartPart{RangeInBytes: rangeHeader, SHA256TreeHash: treeHash}
	putMultipartPart(b.multipartParts, uKey, part)

	if b.multipartPartData[uKey] == nil {
		b.multipartPartData[uKey] = make(map[string][]byte)
	}

	b.multipartPartData[uKey][rangeHeader] = append([]byte(nil), data...)

	return nil
}

// putMultipartPart stores part into parts[uKey], overwriting any existing entry
// for the same RangeInBytes -- see UploadMultipartPart's idempotency doc note.
func putMultipartPart(parts map[uploadKey][]MultipartPart, uKey uploadKey, part MultipartPart) {
	for i, existing := range parts[uKey] {
		if existing.RangeInBytes == part.RangeInBytes {
			parts[uKey][i] = part

			return
		}
	}

	parts[uKey] = append(parts[uKey], part)
}

// CompleteMultipartUpload finalises a multipart upload and creates an archive.
//
// Per api_op_CompleteMultipartUpload.go's doc comment: the assembled archive's
// SHA256 tree hash (the tree hash of the individual parts' tree hashes) must
// match checksum, the sum of part sizes must match archiveSize, and Glacier
// checks for missing content ranges when assembling the archive -- any of
// these failing fails the request.
func (b *InMemoryBackend) CompleteMultipartUpload(
	accountID, region, vaultName, uploadID, checksum string,
	archiveSize int64,
) (*Archive, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	vArn := vaultARN(accountID, region, vaultName)

	v, ok := b.vaults.Get(vArn)
	if !ok {
		return nil, ErrVaultNotFound
	}

	upKey := multipartUploadKey(vArn, uploadID)

	up, ok := b.multipartUploads.Get(upKey)
	if !ok {
		return nil, ErrUploadNotFound
	}

	uKey := uploadKey{AccountID: accountID, Region: region, VaultName: vaultName, UploadID: uploadID}
	parts := slices.Clone(b.multipartParts[uKey])

	sort.Slice(parts, func(i, j int) bool {
		si, _, _ := parseMultipartRange(parts[i].RangeInBytes)
		sj, _, _ := parseMultipartRange(parts[j].RangeInBytes)

		return si < sj
	})

	assembled, totalSize, err := assembleMultipartParts(parts, b.multipartPartData[uKey], up.PartSizeInBytes)
	if err != nil {
		return nil, err
	}

	if totalSize != archiveSize {
		return nil, fmt.Errorf(
			"%w: ArchiveSize (%d) does not match the sum of uploaded part sizes (%d)",
			ErrValidation, archiveSize, totalSize,
		)
	}

	expectedHash, err := aggregatePartTreeHashes(parts)
	if err != nil {
		return nil, err
	}

	if checksum != expectedHash {
		return nil, fmt.Errorf("%w: checksum does not match the assembled archive: computed %s",
			ErrValidation, expectedHash)
	}

	archiveID := generateID(archiveIDLength)
	a := &Archive{
		ArchiveID:      archiveID,
		Description:    up.ArchiveDescription,
		CreationDate:   formatDate(time.Now()),
		Size:           archiveSize,
		SHA256TreeHash: checksum,
	}

	if v.Archives == nil {
		v.Archives = make(map[string]*Archive)
	}

	v.Archives[archiveID] = a
	v.NumberOfArchives++
	v.SizeInBytes += archiveSize
	v.WriteSinceLastInventory = new(true)
	b.archiveData[archiveID] = assembled

	b.multipartUploads.Delete(upKey)
	delete(b.multipartParts, uKey)
	delete(b.multipartPartData, uKey)

	return a, nil
}

// assembleMultipartParts validates that parts (already sorted by range start)
// cover [0, N) contiguously with no gaps, that every part except the last
// equals partSize, and concatenates their stored bytes into a single archive.
// It returns the assembled bytes and their total length.
func assembleMultipartParts(
	parts []MultipartPart, data map[string][]byte, partSize int64,
) ([]byte, int64, error) {
	var (
		assembled     []byte
		expectedStart int64
	)

	for i, p := range parts {
		start, end, ok := parseMultipartRange(p.RangeInBytes)
		if !ok {
			return nil, 0, fmt.Errorf("%w: stored part has a malformed Content-Range %q", ErrValidation, p.RangeInBytes)
		}

		if start != expectedStart {
			return nil, 0, fmt.Errorf("%w: missing content range at byte offset %d", ErrValidation, expectedStart)
		}

		length := end - start + 1
		if partSize > 0 && i != len(parts)-1 && length != partSize {
			return nil, 0, fmt.Errorf(
				"%w: part at range %q (%d bytes) is not the last part but does not match the declared part size (%d)",
				ErrValidation, p.RangeInBytes, length, partSize,
			)
		}

		assembled = append(assembled, data[p.RangeInBytes]...)
		expectedStart = end + 1
	}

	return assembled, expectedStart, nil
}

// aggregatePartTreeHashes computes the archive-level SHA256 tree hash from the
// (already range-sorted) per-part tree hashes, matching Glacier's documented
// algorithm: the tree hash of the individual parts' SHA256 tree hashes.
func aggregatePartTreeHashes(parts []MultipartPart) (string, error) {
	if len(parts) == 0 {
		return computeTreeHash(nil), nil
	}

	leaves := make([][]byte, 0, len(parts))

	for _, p := range parts {
		leaf, err := hex.DecodeString(p.SHA256TreeHash)
		if err != nil {
			return "", fmt.Errorf("%w: stored part tree hash is not valid hex: %w", ErrValidation, err)
		}

		leaves = append(leaves, leaf)
	}

	return hex.EncodeToString(reduceTreeHashes(leaves)), nil
}

// AbortMultipartUpload cancels an in-progress multipart upload.
func (b *InMemoryBackend) AbortMultipartUpload(accountID, region, vaultName, uploadID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	vArn := vaultARN(accountID, region, vaultName)

	if !b.vaults.Has(vArn) {
		return ErrVaultNotFound
	}

	upKey := multipartUploadKey(vArn, uploadID)

	if !b.multipartUploads.Has(upKey) {
		return ErrUploadNotFound
	}

	b.multipartUploads.Delete(upKey)

	uKey := uploadKey{AccountID: accountID, Region: region, VaultName: vaultName, UploadID: uploadID}
	delete(b.multipartParts, uKey)
	delete(b.multipartPartData, uKey)

	return nil
}

// ListMultipartUploads returns all in-progress multipart uploads for a vault.
func (b *InMemoryBackend) ListMultipartUploads(accountID, region, vaultName string) []*MultipartUpload {
	b.mu.RLock()
	defer b.mu.RUnlock()

	ups := b.multipartUploadsByVault.Get(vaultARN(accountID, region, vaultName))

	result := make([]*MultipartUpload, 0, len(ups))
	for _, up := range ups {
		cp := *up
		result = append(result, &cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].MultipartUploadID < result[j].MultipartUploadID
	})

	return result
}

// ListParts returns the parts for an in-progress multipart upload.
func (b *InMemoryBackend) ListParts(
	accountID, region, vaultName, uploadID string,
) (*ListPartsOutput, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	vArn := vaultARN(accountID, region, vaultName)

	if !b.vaults.Has(vArn) {
		return nil, ErrVaultNotFound
	}

	up, ok := b.multipartUploads.Get(multipartUploadKey(vArn, uploadID))
	if !ok {
		return nil, ErrUploadNotFound
	}

	uKey := uploadKey{AccountID: accountID, Region: region, VaultName: vaultName, UploadID: uploadID}
	stored := b.multipartParts[uKey]
	parts := make([]MultipartPart, len(stored))
	copy(parts, stored)

	// Sort parts by their byte-range start value for deterministic output.
	sort.Slice(parts, func(i, j int) bool {
		return rangeStart(parts[i].RangeInBytes) < rangeStart(parts[j].RangeInBytes)
	})

	return &ListPartsOutput{
		MultipartUploadID:  uploadID,
		VaultARN:           up.VaultARN,
		ArchiveDescription: up.ArchiveDescription,
		PartSizeInBytes:    up.PartSizeInBytes,
		CreationDate:       up.CreationDate,
		Parts:              parts,
	}, nil
}

// rangeStart parses the byte start from a Content-Range header value (e.g. "0-1048575/*").
// Returns 0 on parse failure to maintain stable sort behaviour.
func rangeStart(rangeHeader string) int64 {
	for i := range len(rangeHeader) {
		if rangeHeader[i] == '-' || rangeHeader[i] == '/' {
			n := int64(0)
			for j := range i {
				if rangeHeader[j] < '0' || rangeHeader[j] > '9' {
					return 0
				}

				n = n*10 + int64(rangeHeader[j]-'0') //nolint:mnd // decimal digit extraction
			}

			return n
		}
	}

	return 0
}

// AddMultipartUploadInternal adds an in-progress multipart upload directly to the backend for testing.
// VaultARN is always recomputed from the accountID/region/vaultName parameters -- see the
// AddVaultInternal doc comment for why.
func (b *InMemoryBackend) AddMultipartUploadInternal(accountID, region, vaultName string, up *MultipartUpload) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cp := *up
	cp.VaultARN = vaultARN(accountID, region, vaultName)
	b.multipartUploads.Put(&cp)
}

// AddMultipartPartInternal adds an uploaded part directly to the backend for testing,
// bypassing the real byte-range upload + tree-hash computation.
func (b *InMemoryBackend) AddMultipartPartInternal(accountID, region, vaultName, uploadID string, part MultipartPart) {
	b.mu.Lock()
	defer b.mu.Unlock()

	uKey := uploadKey{AccountID: accountID, Region: region, VaultName: vaultName, UploadID: uploadID}
	b.multipartParts[uKey] = append(b.multipartParts[uKey], part)
}
