package sesv2

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

const (
	// identity / DKIM defaults.
	dkimStatusSuccess             = "SUCCESS"
	verificationStatusSuccess     = "SUCCESS"
	mailFromStatusPending         = "PENDING"
	behaviorOnMxFailureUseDefault = "USE_DEFAULT_VALUE"
)

// EmailIdentity represents a verified email address or domain identity.
type EmailIdentity struct {
	Tags                 map[string]string `json:"tags,omitempty"`
	Identity             string            `json:"identity"`
	IdentityType         string            `json:"identityType"`
	ConfigurationSetName string            `json:"configurationSetName,omitempty"`
	MailFromDomain       string            `json:"mailFromDomain,omitempty"`
	MailFromDomainStatus string            `json:"mailFromDomainStatus,omitempty"`
	BehaviorOnMxFailure  string            `json:"behaviorOnMxFailure,omitempty"`
	VerificationStatus   string            `json:"verificationStatus"`
	DkimStatus           string            `json:"dkimStatus"`
	DkimTokens           []string          `json:"dkimTokens,omitempty"`
	VerifiedForSending   bool              `json:"verifiedForSending"`
	FeedbackForwarding   bool              `json:"feedbackForwarding"`
	DkimSigningEnabled   bool              `json:"dkimSigningEnabled"`
}

// Clone returns a deep copy of EmailIdentity.
func (ei *EmailIdentity) Clone() *EmailIdentity {
	if ei == nil {
		return nil
	}

	cp := *ei
	if ei.Tags != nil {
		cp.Tags = maps.Clone(ei.Tags)
	}
	if ei.DkimTokens != nil {
		cp.DkimTokens = slices.Clone(ei.DkimTokens)
	}

	return &cp
}

// identityType determines the identity type from the identity string.
func identityType(identity string) string {
	if strings.Contains(identity, "@") {
		return "EMAIL_ADDRESS"
	}

	return "DOMAIN"
}

// CreateEmailIdentity creates a new email identity and marks it as verified.
func (b *InMemoryBackend) CreateEmailIdentity(
	identity, configurationSetName string,
	tags map[string]string,
) (*EmailIdentity, error) {
	if strings.TrimSpace(identity) == "" {
		return nil, fmt.Errorf("%w: EmailIdentity is required", ErrInvalidInput)
	}

	b.mu.Lock("CreateEmailIdentity")
	defer b.mu.Unlock()

	if b.identities.Has(identity) {
		return nil, fmt.Errorf("%w: identity %s already exists", ErrAlreadyExists, identity)
	}

	idType := identityType(identity)

	ei := &EmailIdentity{
		Identity:             identity,
		IdentityType:         idType,
		VerifiedForSending:   true,
		FeedbackForwarding:   true,
		DkimSigningEnabled:   true,
		DkimStatus:           dkimStatusSuccess,
		VerificationStatus:   verificationStatusSuccess,
		ConfigurationSetName: configurationSetName,
		Tags:                 tags,
	}

	if idType == "DOMAIN" {
		ei.DkimTokens = generateDkimTokens()
	}

	b.identities.Put(ei)
	b.putResourceTagsLocked(b.identityARN(identity), tags)

	return ei.Clone(), nil
}

const (
	dkimTokenCount  = 3
	dkimTokenLength = 24
)

// generateDkimTokens generates three DKIM CNAME selector tokens.
func generateDkimTokens() []string {
	tokens := make([]string, dkimTokenCount)
	for i := range tokens {
		id := uuid.New().String()
		tokens[i] = strings.ReplaceAll(id, "-", "")[:dkimTokenLength]
	}

	return tokens
}

// GetEmailIdentity returns the email identity with the given name.
func (b *InMemoryBackend) GetEmailIdentity(identity string) (*EmailIdentity, error) {
	b.mu.RLock("GetEmailIdentity")
	defer b.mu.RUnlock()

	ei, ok := b.identities.Get(identity)
	if !ok {
		return nil, fmt.Errorf("%w: identity %s not found", ErrNotFound, identity)
	}

	cp := ei.Clone()
	cp.Tags = b.liveTagsLocked(b.identityARN(identity))

	return cp, nil
}

// ListEmailIdentities returns a paginated, sorted list of email identities.
func (b *InMemoryBackend) ListEmailIdentities(
	nextToken string,
	pageSize int,
) page.Page[*EmailIdentity] {
	var out []*EmailIdentity

	func() {
		b.mu.RLock("ListEmailIdentities")
		defer b.mu.RUnlock()

		snap := b.identities.Snapshot()
		out = make([]*EmailIdentity, 0, len(snap))

		for _, ei := range snap {
			out = append(out, ei.Clone())
		}
	}()

	sort.Slice(out, func(i, j int) bool {
		return out[i].Identity < out[j].Identity
	})

	return page.New(out, nextToken, pageSize, sesv2DefaultMaxItems)
}

// DeleteEmailIdentity removes an email identity.
func (b *InMemoryBackend) DeleteEmailIdentity(identity string) error {
	b.mu.Lock("DeleteEmailIdentity")
	defer b.mu.Unlock()

	if !b.identities.Has(identity) {
		return fmt.Errorf("%w: identity %s not found", ErrNotFound, identity)
	}

	b.identities.Delete(identity)
	delete(b.resourceTags, b.identityARN(identity))
	delete(b.emailIdentityPolicies, identity)

	return nil
}

// CreateEmailIdentityPolicy creates a policy for an email identity.
func (b *InMemoryBackend) CreateEmailIdentityPolicy(identity, policyName, policy string) error {
	b.mu.Lock("CreateEmailIdentityPolicy")
	defer b.mu.Unlock()

	if !b.identities.Has(identity) {
		return fmt.Errorf("%w: identity %s not found", ErrNotFound, identity)
	}

	if _, ok := b.emailIdentityPolicies[identity]; !ok {
		b.emailIdentityPolicies[identity] = make(map[string]string)
	}

	if _, exists := b.emailIdentityPolicies[identity][policyName]; exists {
		return fmt.Errorf(
			"%w: policy %s already exists for identity %s",
			ErrAlreadyExists,
			policyName,
			identity,
		)
	}

	b.emailIdentityPolicies[identity][policyName] = policy

	return nil
}

// ---- email identity policies ----

// GetEmailIdentityPolicies returns policies for an identity.
func (b *InMemoryBackend) GetEmailIdentityPolicies(identity string) (map[string]string, error) {
	b.mu.RLock("GetEmailIdentityPolicies")
	defer b.mu.RUnlock()

	if !b.identities.Has(identity) {
		return nil, fmt.Errorf("%w: identity %s not found", ErrNotFound, identity)
	}

	policies := b.emailIdentityPolicies[identity]
	out := make(map[string]string, len(policies))
	maps.Copy(out, policies)

	return out, nil
}

// DeleteEmailIdentityPolicy deletes a policy from an identity.
func (b *InMemoryBackend) DeleteEmailIdentityPolicy(identity, policyName string) error {
	b.mu.Lock("DeleteEmailIdentityPolicy")
	defer b.mu.Unlock()

	policies, ok := b.emailIdentityPolicies[identity]
	if !ok {
		return fmt.Errorf("%w: identity %s not found", ErrNotFound, identity)
	}

	if _, exists := policies[policyName]; !exists {
		return fmt.Errorf(
			"%w: policy %s not found for identity %s",
			ErrNotFound,
			policyName,
			identity,
		)
	}

	delete(policies, policyName)

	return nil
}

// UpdateEmailIdentityPolicy updates a policy for an identity.
func (b *InMemoryBackend) UpdateEmailIdentityPolicy(identity, policyName, policy string) error {
	b.mu.Lock("UpdateEmailIdentityPolicy")
	defer b.mu.Unlock()

	if !b.identities.Has(identity) {
		return fmt.Errorf("%w: identity %s not found", ErrNotFound, identity)
	}

	if _, ok := b.emailIdentityPolicies[identity]; !ok {
		b.emailIdentityPolicies[identity] = make(map[string]string)
	}

	b.emailIdentityPolicies[identity][policyName] = policy

	return nil
}

// ---- email identity attributes ----

// PutEmailIdentityConfigurationSetAttributes associates a configuration set with the identity.
func (b *InMemoryBackend) PutEmailIdentityConfigurationSetAttributes(
	identity, configSetName string,
) error {
	b.mu.Lock("PutEmailIdentityConfigurationSetAttributes")
	defer b.mu.Unlock()

	ei, ok := b.identities.Get(identity)
	if !ok {
		return fmt.Errorf("%w: identity %s not found", ErrNotFound, identity)
	}

	ei.ConfigurationSetName = configSetName

	return nil
}

// PutEmailIdentityDkimAttributes enables or disables DKIM signing for the identity.
func (b *InMemoryBackend) PutEmailIdentityDkimAttributes(
	identity string,
	signingEnabled bool,
) error {
	b.mu.Lock("PutEmailIdentityDkimAttributes")
	defer b.mu.Unlock()

	ei, ok := b.identities.Get(identity)
	if !ok {
		return fmt.Errorf("%w: identity %s not found", ErrNotFound, identity)
	}

	ei.DkimSigningEnabled = signingEnabled

	return nil
}

// PutEmailIdentityDkimSigningAttributes validates the identity exists and returns its
// DKIM attributes (BYODKIM not modelled).
func (b *InMemoryBackend) PutEmailIdentityDkimSigningAttributes(identity string) (*EmailIdentity, error) {
	b.mu.RLock("PutEmailIdentityDkimSigningAttributes")
	defer b.mu.RUnlock()

	ei, ok := b.identities.Get(identity)
	if !ok {
		return nil, fmt.Errorf("%w: identity %s not found", ErrNotFound, identity)
	}

	return ei.Clone(), nil
}

// PutEmailIdentityFeedbackAttributes sets the feedback forwarding flag for the identity.
func (b *InMemoryBackend) PutEmailIdentityFeedbackAttributes(
	identity string,
	emailForwardingEnabled bool,
) error {
	b.mu.Lock("PutEmailIdentityFeedbackAttributes")
	defer b.mu.Unlock()

	ei, ok := b.identities.Get(identity)
	if !ok {
		return fmt.Errorf("%w: identity %s not found", ErrNotFound, identity)
	}

	ei.FeedbackForwarding = emailForwardingEnabled

	return nil
}

// PutEmailIdentityMailFromAttributes stores the custom MAIL FROM domain and failure behaviour.
func (b *InMemoryBackend) PutEmailIdentityMailFromAttributes(
	identity, mailFromDomain, behaviorOnMxFailure string,
) error {
	b.mu.Lock("PutEmailIdentityMailFromAttributes")
	defer b.mu.Unlock()

	ei, ok := b.identities.Get(identity)
	if !ok {
		return fmt.Errorf("%w: identity %s not found", ErrNotFound, identity)
	}

	ei.MailFromDomain = mailFromDomain

	if mailFromDomain == "" {
		ei.MailFromDomainStatus = ""
		ei.BehaviorOnMxFailure = ""
	} else {
		ei.MailFromDomainStatus = mailFromStatusPending
		if behaviorOnMxFailure != "" {
			ei.BehaviorOnMxFailure = behaviorOnMxFailure
		} else {
			ei.BehaviorOnMxFailure = behaviorOnMxFailureUseDefault
		}
	}

	return nil
}
