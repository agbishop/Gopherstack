package acm

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// AcmeAccount is an ACME account registered with an endpoint. Real ACME
// accounts are created by an ACME client's own "newAccount" protocol call
// against the endpoint's EndpointUrl (RFC 8555 section 7.3) -- a real ACME
// protocol server is explicitly out of scope for this rollout (see
// CLAUDE.md's parity principles: no fabricated verified state). gopherstack
// therefore never populates this table itself; DescribeAcmeAccount/
// ListAcmeAccounts/RevokeAcmeAccount are wired against real (honestly empty)
// backend state and validate their AcmeEndpointArn foreign key for real, but
// will only ever operate over accounts that do not exist until a real ACME
// protocol front-end is added -- see PARITY.md's gaps entry for this
// family. Owned by exactly one AcmeEndpoint, cascade-deleted with it.
type AcmeAccount struct {
	CreatedAt                     time.Time `json:"createdAt"`
	AccountURL                    string    `json:"accountUrl"`
	Region                        string    `json:"region"`
	AcmeEndpointArn               string    `json:"acmeEndpointArn"`
	AcmeExternalAccountBindingArn string    `json:"acmeExternalAccountBindingArn,omitempty"`
	PublicKeyThumbprint           string    `json:"publicKeyThumbprint,omitempty"`
	Status                        string    `json:"status"`
	Contacts                      []string  `json:"contacts,omitempty"`
}

// key is AcmeAccount's store.Table primary key: accounts have no ARN of
// their own on the real wire (they are addressed by AccountUrl scoped to an
// endpoint), so the composite "endpointArn|accountUrl" string stands in for
// one, matching the pattern services/acm's own regionKey uses for
// Certificate.
func (a *AcmeAccount) key() string { return a.AcmeEndpointArn + "|" + a.AccountURL }

func acmeAccountKeyFn(v *AcmeAccount) string         { return v.key() }
func acmeAccountRegionIndexFn(v *AcmeAccount) string { return v.Region }
func acmeAccountEndpointIndexFn(v *AcmeAccount) string {
	return v.AcmeEndpointArn
}

func copyAcmeAccount(v *AcmeAccount) AcmeAccount {
	cp := *v
	if len(v.Contacts) > 0 {
		cp.Contacts = append([]string(nil), v.Contacts...)
	}

	return cp
}

// DescribeAcmeAccount looks up an ACME account by endpoint + AccountUrl.
func (b *InMemoryBackend) DescribeAcmeAccount(ctx context.Context, epARN, accountURL string) (*AcmeAccount, error) {
	if err := validateAcmeEndpointArn(epARN); err != nil {
		return nil, err
	}

	if accountURL == "" {
		return nil, fmt.Errorf("%w: AccountUrl is required", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeAcmeAccount")
	defer b.mu.RUnlock()

	ep, ok := b.endpoints.Get(epARN)
	if !ok || ep.Region != region {
		return nil, fmt.Errorf("%w: ACME endpoint %s not found", ErrAcmeResourceNotFound, epARN)
	}

	acct, ok := b.acmeAccounts.Get(epARN + "|" + accountURL)
	if !ok {
		return nil, fmt.Errorf("%w: ACME account %s not found", ErrAcmeResourceNotFound, accountURL)
	}

	cp := copyAcmeAccount(acct)

	return &cp, nil
}

// ListAcmeAccounts returns a paginated list of ACME accounts registered with
// epARN.
func (b *InMemoryBackend) ListAcmeAccounts(
	ctx context.Context, epARN, nextToken string, maxResults int,
) (page.Page[AcmeAccount], error) {
	if err := validateAcmeEndpointArn(epARN); err != nil {
		return page.Page[AcmeAccount]{}, err
	}

	if err := page.ValidateToken(nextToken); err != nil {
		return page.Page[AcmeAccount]{}, fmt.Errorf("%w: invalid NextToken", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("ListAcmeAccounts")
	defer b.mu.RUnlock()

	ep, ok := b.endpoints.Get(epARN)
	if !ok || ep.Region != region {
		return page.Page[AcmeAccount]{}, fmt.Errorf("%w: ACME endpoint %s not found", ErrAcmeResourceNotFound, epARN)
	}

	owned := b.acmeAccountsByEndpoint.Get(epARN)
	accts := make([]AcmeAccount, 0, len(owned))

	for _, a := range owned {
		accts = append(accts, copyAcmeAccount(a))
	}

	slices.SortFunc(accts, func(a, c AcmeAccount) int { return strings.Compare(a.AccountURL, c.AccountURL) })

	return page.New(accts, nextToken, maxResults, acmDefaultMaxItems), nil
}

// RevokeAcmeAccount revokes an ACME account by endpoint + AccountUrl.
// RevokeAcmeAccount's deserializer declares ConflictException, not
// InvalidStateException, for an already-revoked account -- gopherstack-ftkd.
// The AcmeAccount table is always empty (see the type's doc comment), so
// this branch is unreachable through any real client flow today; the
// sentinel is still corrected for when a real ACME front-end populates it.
func (b *InMemoryBackend) RevokeAcmeAccount(ctx context.Context, epARN, accountURL string) error {
	if err := validateAcmeEndpointArn(epARN); err != nil {
		return err
	}

	if accountURL == "" {
		return fmt.Errorf("%w: AccountUrl is required", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("RevokeAcmeAccount")
	defer b.mu.Unlock()

	ep, ok := b.endpoints.Get(epARN)
	if !ok || ep.Region != region {
		return fmt.Errorf("%w: ACME endpoint %s not found", ErrAcmeResourceNotFound, epARN)
	}

	acct, ok := b.acmeAccounts.Get(epARN + "|" + accountURL)
	if !ok {
		return fmt.Errorf("%w: ACME account %s not found", ErrAcmeResourceNotFound, accountURL)
	}

	if acct.Status == acmeAccountStatusRevoked {
		return fmt.Errorf("%w: ACME account %s is already revoked", ErrConflict, accountURL)
	}

	acct.Status = acmeAccountStatusRevoked

	return nil
}
