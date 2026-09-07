package kms

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
)

// GenerateRandom returns the requested number of cryptographically secure random bytes.
// NumberOfBytes defaults to 32 when not specified; maximum is 1024.
func (b *InMemoryBackend) GenerateRandom(
	_ context.Context,
	input *GenerateRandomInput,
) (*GenerateRandomOutput, error) {
	n := int32(aes256Bytes)

	if input.NumberOfBytes != nil {
		n = *input.NumberOfBytes
	}

	if n <= 0 || n > maxDataKeyBytes {
		return nil, fmt.Errorf(
			"%w: NumberOfBytes must be between 1 and %d, got %d",
			ErrValidation, maxDataKeyBytes, n,
		)
	}

	buf := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return nil, fmt.Errorf("generating random bytes: %w", err)
	}

	return &GenerateRandomOutput{Plaintext: buf}, nil
}
