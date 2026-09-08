package ses

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/collections"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// VerifyEmailIdentity adds an identity (address or domain) and marks it as verified.
// If the identity already exists its verified flag is updated without clearing other attributes.
func (b *InMemoryBackend) VerifyEmailIdentity(identity string) error {
	if strings.TrimSpace(identity) == "" {
		return fmt.Errorf("%w: identity is required", ErrInvalidParameter)
	}

	b.mu.Lock("VerifyEmailIdentity")
	defer b.mu.Unlock()

	if rec, ok := b.identities.Get(identity); ok {
		rec.Verified = true
	} else {
		b.identities.Put(&IdentityRecord{Identity: identity, Verified: true, ForwardingEnabled: true})
	}

	return nil
}

// DeleteIdentity removes a verified identity.
// This is idempotent — deleting a non-existent identity returns success,
// matching real AWS SES behavior.
func (b *InMemoryBackend) DeleteIdentity(identity string) {
	b.mu.Lock("DeleteIdentity")
	defer b.mu.Unlock()

	b.identities.Delete(identity)
	delete(b.policies, identity)
}

// getOrCreateIdentityLocked returns the IdentityRecord for identity, creating one if absent.
// The caller MUST hold b.mu for writing.
func (b *InMemoryBackend) getOrCreateIdentityLocked(identity string) *IdentityRecord {
	if rec, ok := b.identities.Get(identity); ok {
		return rec
	}

	rec := &IdentityRecord{Identity: identity, ForwardingEnabled: true}
	b.identities.Put(rec)

	return rec
}

// identityTypeEmailAddress and identityTypeDomain are the two values of the AWS
// SES IdentityType enum, confirmed via aws-sdk-go-v2/service/ses/types/enums.go.
const (
	identityTypeEmailAddress = "EmailAddress"
	identityTypeDomain       = "Domain"
)

// ListIdentities returns a paginated list of registered identities sorted
// alphabetically, optionally filtered by identityType ("EmailAddress" or
// "Domain"; "" means no filter). The filter is applied before pagination,
// matching real AWS SES (NextToken continuation requires re-supplying the
// same IdentityType used in the original call).
func (b *InMemoryBackend) ListIdentities(nextToken string, maxItems int, identityType string) page.Page[string] {
	b.mu.RLock("ListIdentities")
	defer b.mu.RUnlock()

	snap := b.identities.Snapshot()
	out := make([]string, 0, len(snap))

	for _, rec := range snap {
		isEmail := strings.Contains(rec.Identity, "@")

		switch identityType {
		case identityTypeEmailAddress:
			if !isEmail {
				continue
			}
		case identityTypeDomain:
			if isEmail {
				continue
			}
		}

		out = append(out, rec.Identity)
	}

	return page.New(out, nextToken, maxItems, sesDefaultMaxItems)
}

// GetIdentityVerificationAttributes returns verification status for each requested identity.
// Verified identities return Success; unknown identities return NotStarted.
func (b *InMemoryBackend) GetIdentityVerificationAttributes(identities []string) map[string]string {
	b.mu.RLock("GetIdentityVerificationAttributes")
	defer b.mu.RUnlock()

	result := make(map[string]string, len(identities))

	for _, id := range identities {
		if rec, ok := b.identities.Get(id); ok && rec.Verified {
			result[id] = identityStatusSuccess
		} else {
			result[id] = identityStatusNotStarted
		}
	}

	return result
}

// isVerifiedLocked reports whether the sender address is authorised to send.
// It performs an exact-identity match first, then falls back to domain-level
// verification: if example.com is a verified identity, any address @example.com
// is also considered verified — matching real AWS SES behavior.
//
// The caller MUST hold b.mu for reading or writing.
func (b *InMemoryBackend) isVerifiedLocked(from string) bool {
	if rec, ok := b.identities.Get(from); ok && rec.Verified {
		return true
	}

	// Domain-level check: strip the local-part and check the domain.
	if at := strings.LastIndex(from, "@"); at >= 0 {
		domain := from[at+1:]
		rec, ok := b.identities.Get(domain)

		return ok && rec.Verified
	}

	return false
}

// PutIdentityPolicy stores a sending authorization policy for an identity.
// Real AWS SES validates that Policy is a well-formed JSON IAM-style policy
// document and rejects malformed input with InvalidPolicyException; this
// backend does not evaluate policy semantics (no sending-authorization
// enforcement exists anywhere in this emulator), but it does reject
// non-JSON input the same way, matching the wire error code.
func (b *InMemoryBackend) PutIdentityPolicy(identity, policyName, policy string) error {
	if strings.TrimSpace(identity) == "" {
		return fmt.Errorf("%w: Identity is required", ErrInvalidParameter)
	}

	if strings.TrimSpace(policyName) == "" {
		return fmt.Errorf("%w: PolicyName is required", ErrInvalidParameter)
	}

	if strings.TrimSpace(policy) == "" {
		return fmt.Errorf("%w: Policy is required", ErrInvalidParameter)
	}

	if !json.Valid([]byte(policy)) {
		return fmt.Errorf("%w: Policy must be a valid JSON policy document", ErrInvalidPolicy)
	}

	b.mu.Lock("PutIdentityPolicy")
	defer b.mu.Unlock()

	if b.policies[identity] == nil {
		b.policies[identity] = make(map[string]string)
	}

	b.policies[identity][policyName] = policy

	return nil
}

// DeleteIdentityPolicy removes a sending authorization policy from an identity.
// Deleting a non-existent policy is idempotent.
func (b *InMemoryBackend) DeleteIdentityPolicy(identity, policyName string) error {
	if strings.TrimSpace(identity) == "" {
		return fmt.Errorf("%w: Identity is required", ErrInvalidParameter)
	}

	if strings.TrimSpace(policyName) == "" {
		return fmt.Errorf("%w: PolicyName is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteIdentityPolicy")
	defer b.mu.Unlock()

	if m := b.policies[identity]; m != nil {
		delete(m, policyName)
	}

	return nil
}

// GetIdentityPolicies returns the policy documents for the given identity
// filtered by name. PolicyNames is a required member (api_op_GetIdentityPolicies.go:
// "This member is required") -- there is no real "return everything" mode; the
// doc directs callers who don't know the names to ListIdentityPolicies first.
func (b *InMemoryBackend) GetIdentityPolicies(identity string, policyNames []string) (map[string]string, error) {
	if strings.TrimSpace(identity) == "" {
		return nil, fmt.Errorf("%w: Identity is required", ErrInvalidParameter)
	}

	if len(policyNames) == 0 {
		return nil, fmt.Errorf("%w: PolicyNames is required", ErrInvalidParameter)
	}

	b.mu.RLock("GetIdentityPolicies")
	defer b.mu.RUnlock()

	all := b.policies[identity]
	out := make(map[string]string)

	for _, name := range policyNames {
		if v, ok := all[name]; ok {
			out[name] = v
		}
	}

	return out, nil
}

// ListIdentityPolicies returns the names of sending authorization policies for an identity.
func (b *InMemoryBackend) ListIdentityPolicies(identity string) ([]string, error) {
	if strings.TrimSpace(identity) == "" {
		return nil, fmt.Errorf("%w: Identity is required", ErrInvalidParameter)
	}

	b.mu.RLock("ListIdentityPolicies")
	defer b.mu.RUnlock()

	m := b.policies[identity]
	if len(m) == 0 {
		return []string{}, nil
	}

	names := collections.SortedKeys(m)

	return names, nil
}

// GetIdentityMailFromDomainAttributes returns MailFrom attributes for each identity.
// Identities with a configured MailFromDomain return Success; others return an empty status.
func (b *InMemoryBackend) GetIdentityMailFromDomainAttributes(identities []string) map[string]MailFromDomainAttributes {
	b.mu.RLock("GetIdentityMailFromDomainAttributes")
	defer b.mu.RUnlock()

	out := make(map[string]MailFromDomainAttributes, len(identities))

	for _, id := range identities {
		rec, ok := b.identities.Get(id)
		if !ok {
			out[id] = MailFromDomainAttributes{}

			continue
		}

		status := rec.MailFromStatus
		if status == "" && rec.MailFromDomain != "" {
			status = identityStatusSuccess
		}

		out[id] = MailFromDomainAttributes{
			MailFromDomain:       rec.MailFromDomain,
			MailFromDomainStatus: status,
			BehaviorOnMXFailure:  rec.BehaviorOnMXFail,
		}
	}

	return out
}

// SetIdentityMailFromDomain persists the custom MAIL FROM domain for an identity.
// An empty mailFromDomain clears the setting (and its BehaviorOnMXFailure).
// behaviorOnMXFailure must be "UseDefaultValue" or "RejectMessage"; an empty
// value defaults to "UseDefaultValue", matching real AWS SES.
func (b *InMemoryBackend) SetIdentityMailFromDomain(identity, mailFromDomain, behaviorOnMXFailure string) error {
	if strings.TrimSpace(identity) == "" {
		return fmt.Errorf("%w: Identity is required", ErrInvalidParameter)
	}

	if behaviorOnMXFailure == "" {
		behaviorOnMXFailure = behaviorOnMXFailureUseDefault
	}

	if behaviorOnMXFailure != behaviorOnMXFailureUseDefault && behaviorOnMXFailure != behaviorOnMXFailureReject {
		return fmt.Errorf(
			"%w: BehaviorOnMXFailure must be %s or %s",
			ErrInvalidParameter, behaviorOnMXFailureUseDefault, behaviorOnMXFailureReject,
		)
	}

	b.mu.Lock("SetIdentityMailFromDomain")
	defer b.mu.Unlock()

	rec := b.getOrCreateIdentityLocked(identity)
	rec.MailFromDomain = mailFromDomain

	if mailFromDomain == "" {
		rec.MailFromStatus = ""
		rec.BehaviorOnMXFail = ""
	} else {
		rec.MailFromStatus = identityStatusSuccess
		rec.BehaviorOnMXFail = behaviorOnMXFailure
	}

	return nil
}

// GetIdentityNotificationAttributes returns notification attributes for each identity.
func (b *InMemoryBackend) GetIdentityNotificationAttributes(identities []string) map[string]NotificationAttributes {
	b.mu.RLock("GetIdentityNotificationAttributes")
	defer b.mu.RUnlock()

	out := make(map[string]NotificationAttributes, len(identities))

	for _, id := range identities {
		rec, ok := b.identities.Get(id)
		if !ok {
			out[id] = NotificationAttributes{ForwardingEnabled: true}

			continue
		}

		out[id] = NotificationAttributes{
			BounceTopic:        rec.BounceTopic,
			ComplaintTopic:     rec.ComplaintTopic,
			DeliveryTopic:      rec.DeliveryTopic,
			ForwardingEnabled:  rec.ForwardingEnabled,
			HeadersInBounce:    rec.HeadersInBounce,
			HeadersInComplaint: rec.HeadersInComplaint,
			HeadersInDelivery:  rec.HeadersInDelivery,
		}
	}

	return out
}

// SetIdentityFeedbackForwardingEnabled persists the forwarding-enabled flag for an identity.
func (b *InMemoryBackend) SetIdentityFeedbackForwardingEnabled(identity string, enabled bool) error {
	if strings.TrimSpace(identity) == "" {
		return fmt.Errorf("%w: Identity is required", ErrInvalidParameter)
	}

	b.mu.Lock("SetIdentityFeedbackForwardingEnabled")
	defer b.mu.Unlock()

	b.getOrCreateIdentityLocked(identity).ForwardingEnabled = enabled

	return nil
}

// isValidNotificationType reports whether notificationType is one of the
// three values of the AWS SES NotificationType enum (Bounce, Complaint,
// Delivery); confirmed via aws-sdk-go-v2/service/ses/types/enums.go.
func isValidNotificationType(notificationType string) bool {
	switch notificationType {
	case notifTypeBounce, notifTypeComplaint, notifTypeDelivery:
		return true
	default:
		return false
	}
}

// SetIdentityHeadersInNotificationsEnabled persists the header-inclusion flag for an identity
// and notification type (Bounce, Complaint, or Delivery).
func (b *InMemoryBackend) SetIdentityHeadersInNotificationsEnabled(
	identity, notificationType string, enabled bool,
) error {
	if strings.TrimSpace(identity) == "" {
		return fmt.Errorf("%w: Identity is required", ErrInvalidParameter)
	}

	if strings.TrimSpace(notificationType) == "" {
		return fmt.Errorf("%w: NotificationType is required", ErrInvalidParameter)
	}

	if !isValidNotificationType(notificationType) {
		return fmt.Errorf("%w: NotificationType must be Bounce, Complaint, or Delivery", ErrValidation)
	}

	b.mu.Lock("SetIdentityHeadersInNotificationsEnabled")
	defer b.mu.Unlock()

	rec := b.getOrCreateIdentityLocked(identity)

	switch notificationType {
	case notifTypeBounce:
		rec.HeadersInBounce = enabled
	case notifTypeComplaint:
		rec.HeadersInComplaint = enabled
	case notifTypeDelivery:
		rec.HeadersInDelivery = enabled
	}

	return nil
}

// SetIdentityNotificationTopic persists the SNS notification topic for an identity
// and notification type (Bounce, Complaint, or Delivery).
func (b *InMemoryBackend) SetIdentityNotificationTopic(identity, notificationType, snsTopic string) error {
	if strings.TrimSpace(identity) == "" {
		return fmt.Errorf("%w: Identity is required", ErrInvalidParameter)
	}

	if strings.TrimSpace(notificationType) == "" {
		return fmt.Errorf("%w: NotificationType is required", ErrInvalidParameter)
	}

	if !isValidNotificationType(notificationType) {
		return fmt.Errorf("%w: NotificationType must be Bounce, Complaint, or Delivery", ErrValidation)
	}

	b.mu.Lock("SetIdentityNotificationTopic")
	defer b.mu.Unlock()

	rec := b.getOrCreateIdentityLocked(identity)

	switch notificationType {
	case notifTypeBounce:
		rec.BounceTopic = snsTopic
	case notifTypeComplaint:
		rec.ComplaintTopic = snsTopic
	case notifTypeDelivery:
		rec.DeliveryTopic = snsTopic
	}

	return nil
}
