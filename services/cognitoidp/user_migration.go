package cognitoidp

// user_migration.go implements the UserMigration Lambda trigger: when a sign-in
// attempt names a username that does not exist in the pool, and the pool has a
// UserMigration trigger configured, Cognito hands the plaintext password to the
// Lambda so it can validate the credential against an external identity store (e.g. a
// legacy user database) and, on success, supply the attributes for a brand-new
// Cognito user -- letting an application migrate users to Cognito one sign-in at a
// time instead of a bulk import.

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// userMigrationFinalStatusReset is the FinalUserStatus value a UserMigration Lambda
// can set to request that the migrated user's password be treated as untrusted for
// future sign-ins: per the AWS Cognito developer guide, this sign-in attempt still
// succeeds (Cognito already trusts the Lambda's out-of-band validation of the
// plaintext password for this one attempt), but the account is left in
// FORCE_CHANGE_PASSWORD so the *next* sign-in attempt requires a password reset.
const userMigrationFinalStatusReset = "RESET_REQUIRED"

// migrationApplicableAuthFlows are the AuthFlow values AWS invokes UserMigration for.
// USER_SRP_AUTH never exposes the plaintext password to Cognito (the whole point of
// SRP), so there is nothing to hand the Lambda, and CUSTOM_AUTH has its own
// independent DefineAuthChallenge-driven state machine (custom_auth.go) -- neither
// flow supports migration.
var migrationApplicableAuthFlows = map[string]bool{ //nolint:gochecknoglobals // static lookup set
	"USER_PASSWORD_AUTH":       true,
	"ADMIN_USER_PASSWORD_AUTH": true,
	"ADMIN_NO_SRP_AUTH":        true,
}

// tryUserMigration invokes the UserMigration Lambda trigger (if configured and
// applicable to authFlow) for a username that was not found in pool, and -- if the
// Lambda supplies userAttributes -- creates and returns a new, CONFIRMED Cognito user
// with those attributes and a bcrypt hash of password (so this sign-in attempt's own
// subsequent password check in authenticate() succeeds trivially, and so does any
// future sign-in that reuses the same password). Returns (nil, "", nil) -- not an
// error -- when migration is unavailable, not applicable to authFlow, or the Lambda
// declines, so the caller falls back to its normal "unknown user" handling exactly as
// before this feature existed. Caller must hold b.mu.
func (b *InMemoryBackend) tryUserMigration(
	pool *UserPool, clientID, authFlow, username, password string,
) (*User, string, error) {
	if !migrationApplicableAuthFlows[authFlow] {
		return nil, "", nil
	}

	resp, err := b.invokeUserMigrationTrigger(pool, clientID, username, password)
	if err != nil {
		return nil, "", err
	}

	if resp == nil {
		return nil, "", nil
	}

	hash, saltHex, verifierHex, err := hashAndSRP(pool.ID, username, password)
	if err != nil {
		return nil, "", fmt.Errorf("hashing migrated password: %w", err)
	}

	now := time.Now()
	user := &User{
		Sub:          uuid.New().String(),
		Username:     username,
		UserPoolID:   pool.ID,
		PasswordHash: hash,
		SRPSalt:      saltHex,
		SRPVerifier:  verifierHex,
		Status:       UserStatusConfirmed,
		Attributes:   resp.UserAttributes,
		CreatedAt:    now,
		UpdatedAt:    now,
		Enabled:      true,
	}

	b.users.Put(user)

	return user, resp.FinalUserStatus, nil
}

// tryUserMigrationForgotPassword invokes the UserMigration Lambda trigger (if
// configured) for a ForgotPassword call naming a username that does not exist in the
// pool, and -- if the Lambda supplies userAttributes -- creates a new user with those
// attributes. Unlike tryUserMigration, no password/SRP credentials are set: Cognito
// never sends a password into a forgot-password migration, so the user has none until
// ConfirmForgotPassword completes the flow the caller is already in the middle of.
// Returns (nil, nil) -- not an error -- when migration is unavailable or declined, so
// ForgotPassword falls back to its normal unknown-user handling. Caller must hold b.mu.
func (b *InMemoryBackend) tryUserMigrationForgotPassword(pool *UserPool, clientID, username string) (*User, error) {
	resp, err := b.invokeUserMigrationTriggerForgotPassword(pool, clientID, username)
	if err != nil {
		return nil, err
	}

	if resp == nil {
		return nil, nil //nolint:nilnil // sentinel "declined" pair, documented above
	}

	now := time.Now()
	user := &User{
		Sub:                  uuid.New().String(),
		Username:             username,
		UserPoolID:           pool.ID,
		Status:               UserStatusForceChangePassword,
		Attributes:           resp.UserAttributes,
		CreatedAt:            now,
		UpdatedAt:            now,
		TempPasswordIssuedAt: now,
		Enabled:              true,
	}

	b.users.Put(user)

	return user, nil
}

// applyPostMigrationFinalStatus, called after authenticate() has already used a
// freshly-migrated user's CONFIRMED status to complete (or challenge) this one
// sign-in attempt, applies FinalUserStatus=RESET_REQUIRED by flipping the user to
// FORCE_CHANGE_PASSWORD for every subsequent sign-in. Caller must hold b.mu.
func (b *InMemoryBackend) applyPostMigrationFinalStatus(poolID, username, finalUserStatus string) {
	if finalUserStatus != userMigrationFinalStatusReset {
		return
	}

	user, ok := b.users.Get(userKey(poolID, username))
	if !ok {
		return
	}

	user.Status = UserStatusForceChangePassword
	user.UpdatedAt = time.Now()
	user.TempPasswordIssuedAt = user.UpdatedAt
}
