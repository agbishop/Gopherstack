package waf

import "github.com/google/uuid"

const (
	changeTokenStatusINSYNC      = "INSYNC"
	changeTokenStatusPROVISIONED = "PROVISIONED"

	maxChangeTokens = 10_000
)

// GetChangeToken returns the outstanding PROVISIONED token if one exists
// (real AWS: "If your application submits a GetChangeToken request and then
// submits a second GetChangeToken request before submitting a create,
// update, or delete request, the second GetChangeToken request returns the
// same value as the first" -- waf@v1.33.4 api_op_GetChangeToken.go:23-27),
// otherwise it mints a new one. A token stops being outstanding once
// MarkChangeTokenUsed consumes it.
func (b *InMemoryBackend) GetChangeToken() string {
	b.mu.Lock("GetChangeToken")
	defer b.mu.Unlock()

	if b.outstandingChangeToken != "" {
		return b.outstandingChangeToken
	}

	token := uuid.New().String()
	b.changeTokens[token] = changeTokenStatusPROVISIONED
	b.outstandingChangeToken = token

	if len(b.changeTokens) > maxChangeTokens {
		for k, v := range b.changeTokens {
			if v == changeTokenStatusINSYNC {
				delete(b.changeTokens, k)
			}
		}
	}

	return token
}

// GetChangeTokenStatus returns the status of a change token.
// Unknown tokens return INSYNC, matching real AWS WAF Classic behavior.
func (b *InMemoryBackend) GetChangeTokenStatus(token string) string {
	b.mu.RLock("GetChangeTokenStatus")
	defer b.mu.RUnlock()

	if status, ok := b.changeTokens[token]; ok {
		return status
	}

	return changeTokenStatusINSYNC
}

// MarkChangeTokenUsed transitions a change token from PROVISIONED to INSYNC
// and, if it was the outstanding token, clears that so the next
// GetChangeToken mints a fresh one.
func (b *InMemoryBackend) MarkChangeTokenUsed(token string) {
	b.mu.Lock("MarkChangeTokenUsed")
	defer b.mu.Unlock()

	if _, ok := b.changeTokens[token]; ok {
		b.changeTokens[token] = changeTokenStatusINSYNC
	}

	if b.outstandingChangeToken == token {
		b.outstandingChangeToken = ""
	}
}

// validateChangeToken returns ErrStaleToken unless token was returned by an
// earlier GetChangeToken call and has not yet been consumed by a mutation
// (real AWS WAFStaleDataException: "you tried to create, update, or delete
// an object by using a change token that has already been used"). It does
// not itself consume the token -- the caller (Handler.dispatch) marks it
// INSYNC once the mutation it guards has actually succeeded, via
// MarkChangeTokenUsed. Callers must hold b.mu.
func (b *InMemoryBackend) validateChangeToken(token string) error {
	if status, ok := b.changeTokens[token]; !ok || status != changeTokenStatusPROVISIONED {
		return ErrStaleToken
	}

	return nil
}
