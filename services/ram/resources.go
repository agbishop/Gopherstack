package ram

import (
	"fmt"
	"sort"
	"strings"
)

// arnPartCount is the number of colon-separated components in an AWS ARN.
const arnPartCount = 6

// resourceTypeFromARN derives the AWS resource type string (e.g. "ec2:Subnet") from an ARN.
// Returns "UNKNOWN" if the ARN cannot be parsed or the service is unrecognised.
func resourceTypeFromARN(resourceARN string) string {
	// ARN format: arn:partition:service:region:account-id:resource-type/resource-id
	parts := strings.SplitN(resourceARN, ":", arnPartCount)
	if len(parts) < arnPartCount || parts[0] != arnPrefix {
		return "UNKNOWN"
	}

	service := parts[2]
	resourcePart := parts[5]

	resType := resourcePart
	if idx := strings.IndexAny(resourcePart, "/:"); idx >= 0 {
		resType = resourcePart[:idx]
	}

	// Map known resource types to their canonical RAM type strings.
	typeMap := map[string]string{
		"subnet":                resourceTypeEC2Subnet,
		"vpc":                   resourceTypeEC2VPC,
		"transit-gateway":       resourceTypeEC2TransitGateway,
		"prefix-list":           resourceTypeEC2PrefixList,
		"resolver-rule":         resourceTypeRoute53Resolver,
		"license-configuration": resourceTypeLicenseManager,
	}

	key := strings.ToLower(resType)
	if mapped, ok := typeMap[key]; ok {
		return mapped
	}

	if resType == "" {
		return "UNKNOWN"
	}

	upper := strings.ToUpper(resType[:1]) + resType[1:]

	return service + ":" + upper
}

// GetResourcePolicies returns resource-based policy documents for shared resources.
// Only ARNs that are actively associated with a resource share receive a policy entry;
// ARNs not in any share are omitted, matching real AWS behaviour.
func (b *InMemoryBackend) GetResourcePolicies(resourceARNs []string) []string {
	b.mu.RLock("GetResourcePolicies")
	defer b.mu.RUnlock()

	// Build a set of resource ARNs that are actively associated with shares.
	sharedARNs := make(map[string]struct{}, len(b.associations))
	for _, a := range b.associations {
		if a.AssociationType == associationTypeResource &&
			a.Status != associationStatusDisassociated {
			sharedARNs[a.AssociatedEntity] = struct{}{}
		}
	}

	result := make([]string, 0, len(resourceARNs))

	for _, resourceARN := range resourceARNs {
		if _, shared := sharedARNs[resourceARN]; !shared {
			continue
		}
		// Emit a minimal RAM-managed policy for this shared resource.
		result = append(result, `{"Version":"2012-10-17","Statement":[]}`)
	}

	return result
}

// ListResources returns resources (resource-type associations) for shares, filtered
// by resourceOwner ("SELF" or "OTHER-ACCOUNTS"), share ARNs (any-of; empty means no
// filter), and resource type.
func (b *InMemoryBackend) ListResources(
	resourceOwner string, shareARNs []string, resourceType string,
) []*ResourceShareAssociation {
	b.mu.RLock("ListResources")
	defer b.mu.RUnlock()

	shareARNSet := make(map[string]struct{}, len(shareARNs))
	for _, s := range shareARNs {
		shareARNSet[s] = struct{}{}
	}

	result := make([]*ResourceShareAssociation, 0, len(b.associations))

	for _, a := range b.associations {
		if a.AssociationType != associationTypeResource {
			continue
		}

		if a.Status == associationStatusDisassociated {
			continue
		}

		if len(shareARNSet) > 0 {
			if _, ok := shareARNSet[a.ResourceShareARN]; !ok {
				continue
			}
		}

		if resourceOwner != "" && !b.ownerMatchesFilter(a.ResourceShareARN, resourceOwner) {
			continue
		}

		if resourceType != "" && resourceTypeFromARN(a.AssociatedEntity) != resourceType {
			continue
		}

		result = append(result, cloneAssociation(a))
	}

	sort.Slice(
		result,
		func(i, j int) bool { return result[i].AssociatedEntity < result[j].AssociatedEntity },
	)

	return result
}

// ListPendingInvitationResources returns the resource associations for the resource share
// associated with the given invitation, filtered to resources that are in an active state.
func (b *InMemoryBackend) ListPendingInvitationResources(
	invitationARN string,
) ([]*ResourceShareAssociation, error) {
	b.mu.RLock("ListPendingInvitationResources")
	defer b.mu.RUnlock()

	inv, ok := b.invitations.Get(invitationARN)
	if !ok {
		return nil, fmt.Errorf("%w: invitation %s not found", ErrInvitationNotFound, invitationARN)
	}

	result := make([]*ResourceShareAssociation, 0, len(b.associations))

	for _, a := range b.associations {
		if a.ResourceShareARN != inv.ResourceShareARN {
			continue
		}

		if a.AssociationType != associationTypeResource {
			continue
		}

		result = append(result, cloneAssociation(a))
	}

	sort.Slice(
		result,
		func(i, j int) bool { return result[i].AssociatedEntity < result[j].AssociatedEntity },
	)

	return result, nil
}
