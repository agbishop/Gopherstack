package organizations

import (
	"fmt"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// orgARN builds an ARN for the organization.
func (b *InMemoryBackend) orgARN(orgID string) string {
	return arn.Build("organizations", "", b.accountID, fmt.Sprintf("organization/%s", orgID))
}

// masterAccountARN builds an ARN for the management account.
func (b *InMemoryBackend) masterAccountARN(orgID, accountID string) string {
	return arn.Build("organizations", "", b.accountID, fmt.Sprintf("account/%s/%s", orgID, accountID))
}

// accountARN builds an ARN for an account.
func (b *InMemoryBackend) accountARN(orgID, accountID string) string {
	return arn.Build("organizations", "", b.accountID, fmt.Sprintf("account/%s/%s", orgID, accountID))
}

// rootARN builds an ARN for the root.
func (b *InMemoryBackend) rootARN(orgID, rootID string) string {
	return arn.Build("organizations", "", b.accountID, fmt.Sprintf("root/%s/%s", orgID, rootID))
}

// ouARN builds an ARN for an OU.
func (b *InMemoryBackend) ouARN(orgID, ouID string) string {
	return arn.Build("organizations", "", b.accountID, fmt.Sprintf("ou/%s/%s", orgID, ouID))
}

// policyARN builds an ARN for a customer-owned policy.
func (b *InMemoryBackend) policyARN(orgID, policyType, policyID string) string {
	return fmt.Sprintf(
		"arn:aws:organizations::%s:policy/%s/%s/%s",
		b.accountID,
		orgID,
		policyType,
		policyID,
	)
}

// awsManagedPolicyARN builds an ARN for an AWS-owned policy (e.g. the default
// FullAWSAccess SCP): "aws" authority, no account or org segment. Verified
// against botocore's PolicyArn pattern (organizations api-2.json), which
// offers exactly two alternatives -- the customer-owned shape policyARN
// builds, and this one.
func (b *InMemoryBackend) awsManagedPolicyARN(policyType, policyID string) string {
	return fmt.Sprintf("arn:aws:organizations::aws:policy/%s/%s", policyType, policyID)
}

// resourcePolicyARN builds an ARN for the organization resource policy.
func (b *InMemoryBackend) resourcePolicyARN(orgID string) string {
	return fmt.Sprintf(
		"arn:aws:organizations::%s:resourcepolicy/%s/p-rp-default",
		b.accountID,
		orgID,
	)
}

// handshakeARN builds an ARN for a handshake.
// action should be the lowercase action string (e.g. "invite", "enable_all_features").
func (b *InMemoryBackend) handshakeARN(orgID, action, handshakeID string) string {
	return fmt.Sprintf(
		"arn:aws:organizations::%s:handshake/%s/%s/%s",
		b.accountID,
		orgID,
		strings.ToLower(action),
		handshakeID,
	)
}

// responsibilityTransferARN builds an ARN for a responsibility transfer.
// Pattern verified against ResponsibilityTransfer.Arn's documented regex
// (docs.aws.amazon.com/organizations/latest/APIReference/API_ResponsibilityTransfer.html):
// arn:...:organizations::<account>:transfer/o-.../(billing)/(inbound|outbound)/rt-....
// direction is caller-relative -- the same transfer has a different ARN (and
// therefore shows under a different List op) for each of its two accounts.
func (b *InMemoryBackend) responsibilityTransferARN(orgID, transferType, direction, transferID string) string {
	return fmt.Sprintf(
		"arn:aws:organizations::%s:transfer/%s/%s/%s/%s",
		b.accountID,
		orgID,
		strings.ToLower(transferType),
		direction,
		transferID,
	)
}
