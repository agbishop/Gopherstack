package organizations

import (
	"cmp"
	"fmt"
	"slices"
	"time"
)

const (
	accountStatusActive         = "ACTIVE"
	accountStatusSuspended      = "SUSPENDED"
	accountStatusPendingClosure = "PENDING_CLOSURE"
	joinedMethodInvited         = "INVITED"
	joinedMethodCreated         = "CREATED"

	createAccountStateSucceeded = "SUCCEEDED"
)

// createAccountLocked creates an account and status record.
// Must be called with the write lock held.
// Returns nil if the email already exists (duplicate email).
func (b *InMemoryBackend) createAccountLocked(
	name, email, roleName, iamUserAccessToBilling string,
	acctIDFn func(counter int) string,
	govCloudID string,
	tags []Tag,
) *CreateAccountStatus {
	// Check for duplicate email.
	if b.emailToAccountID != nil {
		if _, exists := b.emailToAccountID[email]; exists {
			return nil
		}
	}

	b.accountCounter++
	acctID := acctIDFn(b.accountCounter)

	now := time.Now()
	acct := &Account{
		ID:                     acctID,
		ARN:                    b.accountARN(b.org.ID, acctID),
		Name:                   name,
		Email:                  email,
		Status:                 accountStatusActive,
		JoinedMethod:           joinedMethodCreated,
		JoinedAt:               now,
		RoleName:               roleName,
		IamUserAccessToBilling: iamUserAccessToBilling,
	}

	b.accounts.Put(acct)
	b.accountParent[acctID] = b.root.ID
	b.addAccountChild(b.root.ID, acctID)
	b.setTagsLocked(acctID, tags)

	if b.emailToAccountID == nil {
		b.emailToAccountID = make(map[string]string)
	}
	b.emailToAccountID[email] = acctID

	b.statusCounter++
	statusID := fmt.Sprintf("car-%012d", b.statusCounter)

	status := &CreateAccountStatus{
		ID:                 statusID,
		AccountID:          acctID,
		GovCloudAccountID:  govCloudID,
		AccountName:        name,
		State:              createAccountStateSucceeded,
		RequestedTimestamp: epochSeconds(now),
		CompletedTimestamp: epochSeconds(now),
	}

	b.createStatuses.Put(status)

	return status
}

// CreateAccount creates a new account and returns its status.
func (b *InMemoryBackend) CreateAccount(
	name, email, roleName, iamUserAccessToBilling string,
	tags []Tag,
) (*CreateAccountStatus, error) {
	b.mu.Lock("CreateAccount")
	defer b.mu.Unlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if err := validateNewTags(nil, tags); err != nil {
		return nil, err
	}

	status := b.createAccountLocked(name, email, roleName, iamUserAccessToBilling, newAccountID, "", tags)
	if status == nil {
		return nil, ErrInvalidInput
	}

	return status, nil
}

// DescribeCreateAccountStatus returns the status of a CreateAccount request.
func (b *InMemoryBackend) DescribeCreateAccountStatus(
	requestID string,
) (*CreateAccountStatus, error) {
	b.mu.RLock("DescribeCreateAccountStatus")
	defer b.mu.RUnlock()

	s, ok := b.createStatuses.Get(requestID)
	if !ok {
		return nil, ErrCreateAccountStatusNotFound
	}

	return s, nil
}

// DescribeAccount returns an account by ID.
func (b *InMemoryBackend) DescribeAccount(accountID string) (*Account, error) {
	b.mu.RLock("DescribeAccount")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	a, ok := b.accounts.Get(accountID)
	if !ok {
		return nil, ErrAccountNotFound
	}

	cp := copyAccount(a)
	cp.Paths = b.accountPathsLocked(accountID)

	return cp, nil
}

func (b *InMemoryBackend) ListAccounts() ([]*Account, error) {
	b.mu.RLock("ListAccounts")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	out := make([]*Account, 0, b.accounts.Len())
	for _, a := range b.accounts.All() {
		cp := copyAccount(a)
		cp.Paths = b.accountPathsLocked(a.ID)
		out = append(out, cp)
	}

	slices.SortFunc(out, func(a, b *Account) int { return cmp.Compare(a.ID, b.ID) })

	return out, nil
}

// RemoveAccountFromOrganization removes an account from the organization.
func (b *InMemoryBackend) RemoveAccountFromOrganization(accountID string) error {
	b.mu.Lock("RemoveAccountFromOrganization")
	defer b.mu.Unlock()

	if b.org == nil {
		return ErrOrgNotFound
	}

	if accountID == b.org.MasterAccountID {
		return ErrMasterCannotLeaveOrganization
	}

	acct, ok := b.accounts.Get(accountID)
	if !ok {
		return ErrAccountNotFound
	}

	// AWS rejects removal while the account is still a delegated
	// administrator for any service; the caller must deregister it first.
	if len(b.delegatedAdminsByAccount.Get(accountID)) > 0 {
		return ErrCannotRemoveDelegatedAdministratorFromOrg
	}

	// Clean policyTargets reverse mapping: remove accountID from each policy's target list.
	for _, policyID := range b.targetPolicies[accountID] {
		b.policyTargets[policyID] = removeString(b.policyTargets[policyID], accountID)
	}

	b.removeAccountChild(accountID)
	b.accounts.Delete(accountID)
	delete(b.accountParent, accountID)
	delete(b.tags, accountID)
	delete(b.targetPolicies, accountID)

	if acct != nil {
		delete(b.emailToAccountID, acct.Email)
	}

	// For INVITED accounts, generate a terminal LEAVE_ORGANIZATION handshake record.
	if b.org != nil && acct != nil && acct.JoinedMethod == joinedMethodInvited {
		now := time.Now()
		hID := newHandshakeID()
		h := &Handshake{
			ID:                  hID,
			ARN:                 b.handshakeARN(b.org.ID, handshakeActionLeave, hID),
			Action:              handshakeActionLeave,
			State:               handshakeStateAccepted,
			RequestedTimestamp:  now,
			ExpirationTimestamp: now.Add(handshakeExpirationDuration),
			Parties: []HandshakeParty{
				{ID: b.org.ID, Type: "ORGANIZATION"},
				{ID: accountID, Type: "ACCOUNT"},
			},
		}
		b.handshakes.Put(h)
	}

	return nil
}

// MoveAccount moves an account from one parent to another.
func (b *InMemoryBackend) MoveAccount(accountID, sourceParentID, destParentID string) error {
	b.mu.Lock("MoveAccount")
	defer b.mu.Unlock()

	if b.org == nil {
		return ErrOrgNotFound
	}

	if !b.accounts.Has(accountID) {
		return ErrAccountNotFound
	}

	// AWS models SourceParentId/DestinationParentId validation failures as
	// their own error codes, not InvalidInputException.
	current := b.accountParent[accountID]
	if current != sourceParentID {
		return ErrSourceParentNotFound
	}

	if !b.parentExists(destParentID) {
		return ErrDestinationParentNotFound
	}

	b.removeAccountChild(accountID)
	b.accountParent[accountID] = destParentID
	b.addAccountChild(destParentID, accountID)

	return nil
}

// CloseAccount marks an account as PENDING_CLOSURE.
func (b *InMemoryBackend) CloseAccount(accountID string) error {
	b.mu.Lock("CloseAccount")
	defer b.mu.Unlock()

	if b.org == nil {
		return ErrOrgNotFound
	}

	acct, ok := b.accounts.Get(accountID)
	if !ok {
		return ErrAccountNotFound
	}

	if accountID == b.org.MasterAccountID {
		return ErrInvalidInput
	}

	if acct.Status == accountStatusPendingClosure || acct.Status == accountStatusSuspended {
		return ErrAccountAlreadyClosed
	}

	acct.Status = accountStatusPendingClosure

	return nil
}

// CreateGovCloudAccount creates a commercial account paired with a GovCloud account.
func (b *InMemoryBackend) CreateGovCloudAccount(
	name, email, roleName, iamUserAccessToBilling string,
	tags []Tag,
) (*CreateAccountStatus, error) {
	b.mu.Lock("CreateGovCloudAccount")
	defer b.mu.Unlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if err := validateNewTags(nil, tags); err != nil {
		return nil, err
	}

	// Pre-calculate the GovCloud account ID using the next counter value.
	govCloudID := newGovCloudAccountID(b.accountCounter + 1)

	status := b.createAccountLocked(name, email, roleName, iamUserAccessToBilling, newAccountID, govCloudID, tags)
	if status == nil {
		return nil, ErrInvalidInput
	}

	return status, nil
}

// ListCreateAccountStatus returns all CreateAccount status records, optionally filtered by state.
func (b *InMemoryBackend) ListCreateAccountStatus(states []string) ([]*CreateAccountStatus, error) {
	b.mu.RLock("ListCreateAccountStatus")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	out := make([]*CreateAccountStatus, 0, b.createStatuses.Len())
	for _, s := range b.createStatuses.All() {
		if len(states) == 0 || slices.Contains(states, s.State) {
			cp := *s
			out = append(out, &cp)
		}
	}

	slices.SortFunc(out, func(a, b *CreateAccountStatus) int { return cmp.Compare(a.ID, b.ID) })

	return out, nil
}

// ListAccountsWithInvalidEffectivePolicy returns accounts with invalid effective policies (always empty for stub).
func (b *InMemoryBackend) ListAccountsWithInvalidEffectivePolicy(
	policyType string,
) ([]*Account, error) {
	b.mu.RLock("ListAccountsWithInvalidEffectivePolicy")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if !slices.Contains(validPolicyTypes(), policyType) {
		return nil, ErrInvalidInput
	}

	return []*Account{}, nil
}

// AddAccountInternal seeds an account directly under the root for testing.
// Requires an organization to have been created first.
func (b *InMemoryBackend) AddAccountInternal(a *Account) {
	b.mu.Lock("AddAccountInternal")
	defer b.mu.Unlock()

	b.accounts.Put(a)

	if b.root != nil {
		b.accountParent[a.ID] = b.root.ID
		b.addAccountChild(b.root.ID, a.ID)
	}
}

// copyAccount returns a value copy of account (all fields are scalars).
func copyAccount(a *Account) *Account {
	cp := *a

	return &cp
}
