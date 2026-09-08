package sts

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// GetCallerIdentity returns the mock caller identity.
// When accessKeyID corresponds to an assumed-role session, returns the assumed-role ARN and user ID.
// When sessionToken is non-empty (ASIA-prefixed key), the stored token must match; a mismatch
// returns ErrUnknownAccessKeyID mapped to HTTP 400 InvalidClientTokenId (matching AWS).
func (b *InMemoryBackend) GetCallerIdentity(
	accessKeyID, sessionToken string,
) (*GetCallerIdentityResponse, error) {
	b.cntGetCallerIdentity.Add(1)

	if accessKeyID == "" {
		return b.rootCallerIdentity(), nil
	}

	var session *SessionInfo
	var ok bool
	wasExpired := false

	func() {
		b.mu.Lock("GetCallerIdentity")
		defer b.mu.Unlock()

		session, ok = b.sessions.Get(accessKeyID)

		if ok && isSessionExpired(session) {
			b.sessions.Delete(accessKeyID)
			ok = false
			wasExpired = true
		}
	}()

	if ok {
		// A session minted with a token requires that same token here too: an
		// absent/wrong X-Amz-Security-Token must not be treated as a match, or the
		// ASIA access key ID alone would impersonate the session. AWS rejects a
		// mismatched session token with HTTP 400 InvalidClientTokenId, not 403 AccessDenied.
		if session.SessionToken != "" && sessionToken != session.SessionToken {
			return nil, fmt.Errorf(
				"%w: the security token included in the request is invalid",
				ErrUnknownAccessKeyID,
			)
		}

		return &GetCallerIdentityResponse{
			Xmlns: STSNamespace,
			GetCallerIdentityResult: GetCallerIdentityResult{
				Account: session.AccountID,
				Arn:     session.AssumedRoleArn,
				UserID:  session.AssumedRoleID,
			},
			ResponseMetadata: ResponseMetadata{RequestID: uuid.NewString()},
		}, nil
	}

	// ASIA-prefixed keys are temporary session credentials. AWS returns
	// ExpiredTokenException when a known session has expired, and
	// InvalidClientTokenId when the key was never issued by this service.
	// Long-term AKIA keys that are untracked fall back to the root identity.
	if strings.HasPrefix(accessKeyID, accessKeyIDPrefix) {
		if wasExpired {
			return nil, fmt.Errorf(
				"%w: the security token included in the request has expired",
				ErrSessionExpired,
			)
		}

		return nil, fmt.Errorf(
			"%w: the security token included in the request is invalid",
			ErrUnknownAccessKeyID,
		)
	}

	return b.rootCallerIdentity(), nil
}

func (b *InMemoryBackend) rootCallerIdentity() *GetCallerIdentityResponse {
	callerArn := arn.Build(arnServiceIAM, "", b.accountID, "root")

	return &GetCallerIdentityResponse{
		Xmlns: STSNamespace,
		GetCallerIdentityResult: GetCallerIdentityResult{
			Account: b.accountID,
			Arn:     callerArn,
			UserID:  MockUserID,
		},
		ResponseMetadata: ResponseMetadata{RequestID: uuid.NewString()},
	}
}
