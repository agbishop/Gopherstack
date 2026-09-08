package appconfig

import (
	"fmt"
	"time"

	appconfigdatabackend "github.com/blackbirdworks/gopherstack/services/appconfigdata"
)

const (
	// deletionProtectionCheckBypass/Apply mirror two of
	// types.DeletionProtectionCheck's three enum values (appconfig@v1.48.4
	// types/enums.go:93-95) -- kept as local string constants, like
	// validDeletionProtectionChecks in handler.go, rather than importing the
	// SDK types package. The third value, ACCOUNT_DEFAULT, and an absent
	// header both fall through to checkDeletionProtectionLocked's default
	// case below.
	deletionProtectionCheckBypass = "BYPASS"
	deletionProtectionCheckApply  = "APPLY"

	// defaultProtectionPeriodMinutes is the fallback interval when the
	// account has not set DeletionProtection.ProtectionPeriodInMinutes
	// (DeletionProtectionSettings doc, appconfig@v1.48.4 types/types.go:229:
	// "The default interval specified by ProtectionPeriodInMinutes is 60").
	defaultProtectionPeriodMinutes = 60

	// deletionProtectionGraceInterval excludes resources created within the
	// past hour from the check under ACCOUNT_DEFAULT (types/types.go:230-231:
	// "DeletionProtectionCheck skips configuration profiles and environments
	// that were created in the past hour"). APPLY forces the check to run
	// anyway (api_op_DeleteEnvironment.go's DeletionProtectionCheck doc:
	// "APPLY also forces the deletion protection check to run against
	// resources created in the past hour").
	deletionProtectionGraceInterval = time.Hour
)

// deletionProtectionShouldRun reports whether checkDeletionProtectionLocked
// needs to consult AppConfigData at all, per check's semantics: BYPASS never
// runs it; APPLY always does (ignoring the account Enabled setting and the
// past-hour grace period); ACCOUNT_DEFAULT (or an absent header) runs it only
// when the account has deletion protection Enabled and createdAt is outside
// the past-hour grace period (types/types.go:230-231, api_op_DeleteEnvironment.go's
// DeletionProtectionCheck doc).
func (b *InMemoryBackend) deletionProtectionShouldRun(check string, createdAt time.Time) bool {
	switch check {
	case deletionProtectionCheckBypass:
		return false
	case deletionProtectionCheckApply:
		return true
	default:
		dp := b.accountSettings.DeletionProtection
		if dp == nil || dp.Enabled == nil || !*dp.Enabled {
			return false
		}

		return time.Since(createdAt) >= deletionProtectionGraceInterval
	}
}

// recentlyAccessedSession reports whether reader has a session matching
// applicationID (and, when non-empty, environmentID/profileID) whose
// LastAccessedAt falls within periodMinutes of now.
func recentlyAccessedSession(
	reader appconfigdatabackend.StorageBackend,
	applicationID, environmentID, profileID string,
	periodMinutes int,
) bool {
	cutoff := time.Now().Add(-time.Duration(periodMinutes) * time.Minute)

	for _, sess := range reader.ListSessions() {
		if sess.ApplicationIdentifier != applicationID {
			continue
		}

		if environmentID != "" && sess.EnvironmentIdentifier != environmentID {
			continue
		}

		if profileID != "" && sess.ConfigurationProfileIdentifier != profileID {
			continue
		}

		if sess.LastAccessedAt.After(cutoff) {
			return true
		}
	}

	return false
}

// checkDeletionProtectionLocked enforces DeletionProtectionCheck for
// DeleteEnvironment/DeleteConfigurationProfile, per DeletionProtectionSettings'
// doc comment (aws-sdk-go-v2 appconfig@v1.48.4 types/types.go:222-247) and
// DeleteEnvironmentInput/DeleteConfigurationProfileInput.DeletionProtectionCheck's
// own doc (api_op_DeleteEnvironment.go:47-65, api_op_DeleteConfigurationProfile.go:47-65).
// environmentID == "" checks the configuration profile across every
// environment it may have been read from (DeleteConfigurationProfile, whose
// real precondition is not scoped to one environment); profileID == "" checks
// the whole environment, blocked by ANY profile within it having been read
// recently (DeleteEnvironment). createdAt is the resource's own creation
// time. Returns nil (allow) when no AppConfigData backend is wired, matching
// every other sibling-pattern user's behavior for an unwired sibling
// (services/mgn, services/guardduty, ...): unknown means allowed, never
// blocked. Callers must hold b.mu (Delete* holds the write lock).
func (b *InMemoryBackend) checkDeletionProtectionLocked(
	check string,
	applicationID, environmentID, profileID string,
	createdAt time.Time,
) error {
	if !b.deletionProtectionShouldRun(check, createdAt) {
		return nil
	}

	periodMinutes := defaultProtectionPeriodMinutes
	if dp := b.accountSettings.DeletionProtection; dp != nil && dp.ProtectionPeriodInMinutes != nil {
		periodMinutes = int(*dp.ProtectionPeriodInMinutes)
	}

	reader, ok := b.appConfigDataBackend()
	if !ok {
		return nil
	}

	if recentlyAccessedSession(reader, applicationID, environmentID, profileID, periodMinutes) {
		return fmt.Errorf("%w: GetLatestConfiguration was called within the last %d minutes",
			ErrDeletionProtected, periodMinutes)
	}

	return nil
}
