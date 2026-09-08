package glacier

import (
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// idChars are the characters used for generating random IDs.
const idChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// InMemoryBackend is the in-memory backend for Glacier.
//
// vaults, jobs, multipartUploads, and vaultLocks are *store.Table[T],
// registered once on registry -- see store_setup.go for the Phase 3.3
// pkgs/store conversion this follows. Archives stay nested inline on Vault
// rather than their own table (see the Vault doc comment in models.go).
// multipartParts, provisionedCapacity, dataRetrievalPolicies, and
// archiveData remain plain maps because their values are slice/string-typed
// (not *T) with no identity field of their own to key a Table by.
type InMemoryBackend struct {
	// s3 is the (optional) wired S3 backend a completed Select job's real
	// OutputLocation output is written to -- see select_output.go. Nil until
	// SetS3Backend is called (cli.go's wireGlacierS3); nil is a valid, silently
	// degraded state (no S3 write-back, matching pre-wiring behavior).
	s3                      S3Accessor
	multipartUploadsByVault *store.Index[MultipartUpload]
	multipartParts          map[uploadKey][]MultipartPart
	// multipartPartData holds the raw uploaded bytes for each in-progress part,
	// keyed by the same uploadKey as multipartParts and then by RangeInBytes. Kept
	// separate from MultipartPart (the wire DTO) so raw bytes never leak into
	// ListParts JSON. Like archiveData, it is never persisted (see persistence.go).
	multipartPartData     map[uploadKey]map[string][]byte
	jobs                  *store.Table[Job]
	jobsByVault           *store.Index[Job]
	multipartUploads      *store.Table[MultipartUpload]
	registry              *store.Registry
	vaultLocks            *store.Table[VaultLock]
	vaultsByAccountRegion *store.Index[Vault]
	provisionedCapacity   map[string][]*ProvisionedCapacity
	dataRetrievalPolicies map[string]string
	archiveData           map[string][]byte
	vaults                *store.Table[Vault]
	// retrievalDelay is the simulated asynchronous retrieval window applied to newly
	// initiated jobs. Jobs stay InProgress until CreationDate+retrievalDelay, matching
	// AWS, which does not make archive/inventory output available immediately.
	retrievalDelay time.Duration
	mu             sync.RWMutex
}

// NewInMemoryBackend creates a new in-memory Glacier backend.
func NewInMemoryBackend() *InMemoryBackend {
	b := &InMemoryBackend{
		registry:              store.NewRegistry(),
		multipartParts:        make(map[uploadKey][]MultipartPart),
		multipartPartData:     make(map[uploadKey]map[string][]byte),
		provisionedCapacity:   make(map[string][]*ProvisionedCapacity),
		dataRetrievalPolicies: make(map[string]string),
		archiveData:           make(map[string][]byte),
		retrievalDelay:        defaultRetrievalDelay,
	}

	registerAllTables(b)

	return b
}

// defaultRetrievalDelay is the simulated asynchronous retrieval window applied to
// newly initiated jobs. Kept short so callers and tests are not forced to wait a
// realistic multi-hour window, while still exercising the InProgress -> Succeeded
// lifecycle (real AWS Standard retrievals take 3-5 hours).
const defaultRetrievalDelay = 100 * time.Millisecond

// generateID creates a random ID of the given length using a single batch read
// from crypto/rand rather than one syscall per character.
func generateID(length int) string {
	const nChars = len(idChars)
	const byteRange = 256 // number of distinct byte values
	// Bytes in [0, nChars*(byteRange/nChars)) have no modulo bias.
	const maxByte = byte(nChars * (byteRange / nChars))
	const bufHeadroom = 8 // extra headroom for rejected bytes

	result := make([]byte, 0, length)
	buf := make([]byte, length+length/2+bufHeadroom) // extra headroom for rejections

	for len(result) < length {
		if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			// Unreachable in practice; degrade to index 0 for remaining chars.
			for len(result) < length {
				result = append(result, idChars[0])
			}

			break
		}

		for _, b := range buf {
			if b < maxByte {
				result = append(result, idChars[int(b)%nChars])
				if len(result) == length {
					break
				}
			}
		}
	}

	return string(result)
}

// vaultARN returns the ARN for a Glacier vault.
func vaultARN(accountID, region, vaultName string) string {
	return arn.Build("glacier", region, accountID, fmt.Sprintf("vaults/%s", vaultName))
}

// Reset clears all backend state, resetting to an empty store.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.multipartParts = make(map[uploadKey][]MultipartPart)
	b.multipartPartData = make(map[uploadKey]map[string][]byte)
	b.provisionedCapacity = make(map[string][]*ProvisionedCapacity)
	b.dataRetrievalPolicies = make(map[string]string)
	b.archiveData = make(map[string][]byte)
}
