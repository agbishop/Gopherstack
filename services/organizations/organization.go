package organizations

import (
	"fmt"
	"time"
)

// featureSetAll is the CreateOrganization FeatureSet value under which SCPs
// are automatically enabled in the root (see seedFullAWSAccessPolicyLocked).
const featureSetAll = "ALL"

// CreateOrganization creates a new organization.
func (b *InMemoryBackend) CreateOrganization(featureSet string) (*Organization, *Root, error) {
	b.mu.Lock("CreateOrganization")
	defer b.mu.Unlock()

	if b.org != nil {
		return nil, nil, ErrOrgAlreadyExists
	}

	if featureSet == "" {
		featureSet = featureSetAll
	}

	if featureSet != featureSetAll && featureSet != "CONSOLIDATED_BILLING" {
		return nil, nil, ErrInvalidInput
	}

	orgID := newOrgID()
	rootID := newRootID()

	// The management account is the account making the CreateOrganization
	// call, matching real AWS: whichever account calls CreateOrganization
	// becomes the organization's management account, so its ID must equal
	// the caller identity other services (STS, IAM) report for this backend
	// -- not a synthetic counter-derived ID. accountCounter still starts at
	// managementAccountCounter so subsequently created member accounts get
	// sequential 12-digit IDs, independent of the management account's ID.
	b.accountCounter = managementAccountCounter
	mgmtAcctID := b.accountID

	org := &Organization{
		ID:                 orgID,
		ARN:                b.orgARN(orgID),
		FeatureSet:         featureSet,
		MasterAccountID:    mgmtAcctID,
		MasterAccountARN:   b.masterAccountARN(orgID, mgmtAcctID),
		MasterAccountEmail: fmt.Sprintf("master@%s.example.com", mgmtAcctID),
	}

	root := &Root{
		ID:          rootID,
		ARN:         b.rootARN(orgID, rootID),
		Name:        "Root",
		PolicyTypes: []PolicyTypeSummary{},
	}

	mgmtAcct := &Account{
		ID:           mgmtAcctID,
		ARN:          b.accountARN(orgID, mgmtAcctID),
		Name:         "master",
		Email:        org.MasterAccountEmail,
		Status:       accountStatusActive,
		JoinedMethod: joinedMethodInvited,
		JoinedAt:     time.Now(),
	}

	b.org = org
	b.root = root
	b.accounts.Put(mgmtAcct)
	b.accountParent[mgmtAcctID] = rootID
	b.addAccountChild(rootID, mgmtAcctID)
	b.seedFullAWSAccessPolicyLocked(featureSet, orgID, rootID)

	return org, root, nil
}

// DescribeOrganization returns the current organization.
func (b *InMemoryBackend) DescribeOrganization() (*Organization, error) {
	b.mu.RLock("DescribeOrganization")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	org := copyOrg(b.org)

	// Populate AvailablePolicyTypes: all valid types, ENABLED for those in root.PolicyTypes.
	enabledTypes := make(map[string]bool)
	if b.root != nil {
		for _, pt := range b.root.PolicyTypes {
			if pt.Status == policyStatusEnabled {
				enabledTypes[pt.Type] = true
			}
		}
	}

	pts := make([]PolicyTypeSummary, 0, len(validPolicyTypes()))
	for _, vpt := range validPolicyTypes() {
		status := "DISABLED"
		if enabledTypes[vpt] {
			status = policyStatusEnabled
		}
		pts = append(pts, PolicyTypeSummary{Type: vpt, Status: status})
	}
	org.AvailablePolicyTypes = pts

	return org, nil
}

// DeleteOrganization removes the organization.
func (b *InMemoryBackend) DeleteOrganization() error {
	b.mu.Lock("DeleteOrganization")
	defer b.mu.Unlock()

	if b.org == nil {
		return ErrOrgNotFound
	}

	// AWS rejects deletion when member accounts (other than the management account) still exist.
	for _, acct := range b.accounts.All() {
		if acct.ID != b.org.MasterAccountID {
			return ErrOrganizationNotEmpty
		}
	}

	b.resetStateLocked()

	return nil
}

// copyOrg returns a deep copy of org (including slice fields).
func copyOrg(org *Organization) *Organization {
	cp := *org

	if org.AvailablePolicyTypes != nil {
		cp.AvailablePolicyTypes = make([]PolicyTypeSummary, len(org.AvailablePolicyTypes))
		copy(cp.AvailablePolicyTypes, org.AvailablePolicyTypes)
	}

	return &cp
}
