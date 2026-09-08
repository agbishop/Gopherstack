package ec2

import (
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/collections"
)

// CreateVpcEndpointConnectionNotification creates a notification for endpoint connection events.
func (b *InMemoryBackend) CreateVpcEndpointConnectionNotification(
	serviceID, endpointID, notifARN string,
	events []string,
) (*VpcEndpointConnectionNotification, error) {
	if notifARN == "" {
		return nil, fmt.Errorf("%w: ConnectionNotificationArn is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateVpcEndpointConnectionNotification")
	defer b.mu.Unlock()

	notif := &VpcEndpointConnectionNotification{
		ConnectionNotificationID:    "vpce-nfn-" + uuid.New().String()[:8],
		ServiceID:                   serviceID,
		VpcEndpointID:               endpointID,
		ConnectionNotificationARN:   notifARN,
		ConnectionEvents:            events,
		ConnectionNotificationType:  "Topic",
		ConnectionNotificationState: "Enabled",
	}
	b.endpointConnectionNotifs.Put(notif)

	return notif, nil
}

// DescribeVpcEndpointConnectionNotifications returns notifications, optionally filtered by IDs.
func (b *InMemoryBackend) DescribeVpcEndpointConnectionNotifications(
	ids []string,
) []*VpcEndpointConnectionNotification {
	b.mu.RLock("DescribeVpcEndpointConnectionNotifications")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(ids))
	for _, id := range ids {
		filter[id] = true
	}

	var out []*VpcEndpointConnectionNotification
	for _, n := range b.endpointConnectionNotifs.All() {
		if len(filter) > 0 && !filter[n.ConnectionNotificationID] {
			continue
		}
		cp := *n
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ConnectionNotificationID < out[j].ConnectionNotificationID
	})

	return out
}

// DeleteVpcEndpointConnectionNotifications removes notifications by ID.
func (b *InMemoryBackend) DeleteVpcEndpointConnectionNotifications(ids []string) error {
	b.mu.Lock("DeleteVpcEndpointConnectionNotifications")
	defer b.mu.Unlock()

	for _, id := range ids {
		if _, ok := b.endpointConnectionNotifs.Get(id); !ok {
			return fmt.Errorf("%w: %s", ErrEndpointConnectionNotificationNotFound, id)
		}
	}
	for _, id := range ids {
		b.endpointConnectionNotifs.Delete(id)
	}

	return nil
}

// ModifyVpcEndpointConnectionNotification updates events or state for a notification.
func (b *InMemoryBackend) ModifyVpcEndpointConnectionNotification(
	id, notifARN string,
	events []string,
) (*VpcEndpointConnectionNotification, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: ConnectionNotificationId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyVpcEndpointConnectionNotification")
	defer b.mu.Unlock()

	notif, ok := b.endpointConnectionNotifs.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrEndpointConnectionNotificationNotFound, id)
	}
	if notifARN != "" {
		notif.ConnectionNotificationARN = notifARN
	}
	if len(events) > 0 {
		notif.ConnectionEvents = events
	}
	cp := *notif

	return &cp, nil
}

// ---- VPC Endpoint Connections ----

// DescribeVpcEndpointConnections returns endpoint connections for owned services.
func (b *InMemoryBackend) DescribeVpcEndpointConnections(
	serviceIDs []string,
) []*VpcEndpointConnection {
	b.mu.RLock("DescribeVpcEndpointConnections")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(serviceIDs))
	for _, id := range serviceIDs {
		filter[id] = true
	}

	var out []*VpcEndpointConnection
	for _, conn := range b.vpcEndpointConnections.All() {
		if len(filter) > 0 && !filter[conn.ServiceID] {
			continue
		}
		cp := *conn
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].VpcEndpointID < out[j].VpcEndpointID
	})

	return out
}

// DescribeVpcEndpointAssociations returns VPC-endpoint associations.
func (b *InMemoryBackend) DescribeVpcEndpointAssociations(endpointIDs []string) []*VpcEndpoint {
	b.mu.RLock("DescribeVpcEndpointAssociations")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(endpointIDs))
	for _, id := range endpointIDs {
		filter[id] = true
	}

	var out []*VpcEndpoint
	for _, ep := range b.vpcEndpoints.All() {
		if len(filter) > 0 && !filter[ep.ID] {
			continue
		}
		cp := *ep
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	return out
}

// ---- VPC Endpoint Service Config modifications ----

// ModifyVpcEndpointServicePayerResponsibility updates who is billed for
// traffic through a VPC endpoint service: the endpoint owner (the default,
// left unset) or the service owner ("ServiceOwner"). Previously a disguised
// stub: the payerResponsibility argument was declared `_ string` and
// discarded entirely, so the call always succeeded without mutating any
// state and DescribeVpcEndpointServiceConfigurations never reflected it.
func (b *InMemoryBackend) ModifyVpcEndpointServicePayerResponsibility(
	serviceID, payerResponsibility string,
) error {
	if serviceID == "" {
		return fmt.Errorf("%w: ServiceId is required", ErrInvalidParameter)
	}

	if payerResponsibility == "" {
		return fmt.Errorf("%w: PayerResponsibility is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyVpcEndpointServicePayerResponsibility")
	defer b.mu.Unlock()

	cfg, ok := b.vpcEndpointServiceConfigs.Get(serviceID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrVpcEndpointServiceNotFound, serviceID)
	}

	cfg.PayerResponsibility = payerResponsibility

	return nil
}

// DescribeVpcEndpointServicePermissions returns allowed principals for a service.
func (b *InMemoryBackend) DescribeVpcEndpointServicePermissions(serviceID string) []string {
	b.mu.RLock("DescribeVpcEndpointServicePermissions")
	defer b.mu.RUnlock()

	return append([]string{}, b.vpcEndpointServicePermissions[serviceID]...)
}

// ModifyVpcEndpointServicePermissions adds/removes allowed principals for a service.
func (b *InMemoryBackend) ModifyVpcEndpointServicePermissions(
	serviceID string,
	add, remove []string,
) ([]string, error) {
	if serviceID == "" {
		return nil, fmt.Errorf("%w: ServiceId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyVpcEndpointServicePermissions")
	defer b.mu.Unlock()

	if _, ok := b.vpcEndpointServiceConfigs.Get(serviceID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrVpcEndpointServiceNotFound, serviceID)
	}

	existing := make(map[string]bool)
	for _, p := range b.vpcEndpointServicePermissions[serviceID] {
		existing[p] = true
	}

	var added []string
	for _, p := range add {
		if !existing[p] {
			added = append(added, p)
		}
		existing[p] = true
	}
	for _, p := range remove {
		delete(existing, p)
	}

	result := collections.SortedKeys(existing)
	b.vpcEndpointServicePermissions[serviceID] = result

	return added, nil
}

// ---- ModifyVpcEndpoint ----

// ModifyVpcEndpoint modifies a VPC endpoint (adds/removes subnets, SGs, route tables).
func (b *InMemoryBackend) ModifyVpcEndpoint(
	endpointID string,
	addSubnetIDs, removeSubnetIDs []string,
) error {
	if endpointID == "" {
		return fmt.Errorf("%w: VpcEndpointId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyVpcEndpoint")
	defer b.mu.Unlock()

	ep, ok := b.vpcEndpoints.Get(endpointID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrVpcEndpointNotFound, endpointID)
	}

	removeSet := make(map[string]bool, len(removeSubnetIDs))
	for _, id := range removeSubnetIDs {
		removeSet[id] = true
	}

	filtered := ep.SubnetIDs[:0]
	for _, id := range ep.SubnetIDs {
		if !removeSet[id] {
			filtered = append(filtered, id)
		}
	}
	ep.SubnetIDs = filtered
	ep.SubnetIDs = append(ep.SubnetIDs, addSubnetIDs...)

	return nil
}

// ModifyVpcEndpointPayerResponsibility sets who is billed for a VPC
// endpoint's usage within the given charge scope, upserting the entry for
// that scope. Unlike the pre-existing (and disguised-stub)
// ModifyVpcEndpointServicePayerResponsibility, this real op mutates and
// returns the endpoint's actual PayerResponsibilities state.
func (b *InMemoryBackend) ModifyVpcEndpointPayerResponsibility(
	endpointID, payerResponsibilityType, scope string,
) ([]PayerResponsibilityEntry, error) {
	if endpointID == "" {
		return nil, fmt.Errorf("%w: VpcEndpointId is required", ErrInvalidParameter)
	}

	if payerResponsibilityType == "" {
		return nil, fmt.Errorf("%w: PayerResponsibility is required", ErrInvalidParameter)
	}

	if scope == "" {
		return nil, fmt.Errorf("%w: Scope is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyVpcEndpointPayerResponsibility")
	defer b.mu.Unlock()

	ep, ok := b.vpcEndpoints.Get(endpointID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrVpcEndpointIDNotFound, endpointID)
	}

	replaced := false

	for i, entry := range ep.PayerResponsibilities {
		if entry.Scope == scope {
			ep.PayerResponsibilities[i].PayerResponsibilityType = payerResponsibilityType
			replaced = true

			break
		}
	}

	if !replaced {
		ep.PayerResponsibilities = append(ep.PayerResponsibilities, PayerResponsibilityEntry{
			PayerResponsibilityType: payerResponsibilityType,
			Scope:                   scope,
		})
	}

	out := append([]PayerResponsibilityEntry(nil), ep.PayerResponsibilities...)

	return out, nil
}

// ---- EBS encryption defaults ----

// DeleteVpcEndpoints deletes the named VPC endpoints. Real AWS: "You can
// only delete Gateway Load Balancer endpoints when the routes that are
// associated with the endpoint are deleted".
func (b *InMemoryBackend) DeleteVpcEndpoints(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: at least one VpcEndpointId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteVpcEndpoints")
	defer b.mu.Unlock()

	var unsuccessful []string

	var hasDependencyViolation bool

	for _, id := range ids {
		ep, ok := b.vpcEndpoints.Get(id)
		if !ok {
			unsuccessful = append(unsuccessful, id)

			continue
		}

		if ep.VpcEndpointType == vpcEndpointTypeGatewayLoadBalancer && len(ep.RouteTableIDs) > 0 {
			unsuccessful = append(unsuccessful, id)
			hasDependencyViolation = true

			continue
		}

		b.vpcEndpoints.Delete(id)
		delete(b.tags, id)
	}

	if hasDependencyViolation {
		return unsuccessful, fmt.Errorf(
			"%w: Gateway Load Balancer endpoints %v have routes; delete the routes first",
			ErrDependencyViolation,
			unsuccessful,
		)
	}

	if len(unsuccessful) > 0 {
		return unsuccessful, fmt.Errorf(
			"%w: endpoints not found: %v",
			ErrVpcEndpointNotFound,
			unsuccessful,
		)
	}

	return nil, nil
}

// ---- Sorted describe helpers ----

// DescribeVpcEndpointsByVPC returns VPC endpoints for a specific VPC.
func (b *InMemoryBackend) DescribeVpcEndpointsByVPC(vpcID string) []*VpcEndpoint {
	b.mu.RLock("DescribeVpcEndpointsByVPC")
	defer b.mu.RUnlock()

	var out []*VpcEndpoint

	for _, ep := range b.vpcEndpoints.All() {
		if ep.VPCID != vpcID {
			continue
		}

		cp := *ep
		cp.SubnetIDs = append([]string(nil), ep.SubnetIDs...)
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

// DescribeVpcEndpointServices returns the set of commonly available AWS
// endpoint service names for the backend region.
func (b *InMemoryBackend) DescribeVpcEndpointServices() []string {
	return []string{
		"com.amazonaws." + b.Region + ".s3",
		"com.amazonaws." + b.Region + ".dynamodb",
		"com.amazonaws." + b.Region + ".ec2",
		"com.amazonaws." + b.Region + ".ec2messages",
		"com.amazonaws." + b.Region + ".ssm",
		"com.amazonaws." + b.Region + ".ssmmessages",
		"com.amazonaws." + b.Region + ".kms",
		"com.amazonaws." + b.Region + ".secretsmanager",
		"com.amazonaws." + b.Region + ".sts",
		"com.amazonaws." + b.Region + ".logs",
		"com.amazonaws." + b.Region + ".monitoring",
		"com.amazonaws." + b.Region + ".elasticloadbalancing",
		"com.amazonaws." + b.Region + ".lambda",
		"com.amazonaws." + b.Region + ".sqs",
		"com.amazonaws." + b.Region + ".sns",
	}
}

// ---- Key pair: export ----

// RejectVpcEndpointConnections rejects existing pending VPC endpoint
// connections for a service. Endpoint IDs with no matching connection are
// reported back as unsuccessful, matching AcceptVpcEndpointConnections'
// sibling behaviour of never fabricating a connection that was never
// requested.
func (b *InMemoryBackend) RejectVpcEndpointConnections(serviceID string, vpcEndpointIDs []string) ([]string, error) {
	if serviceID == "" {
		return nil, fmt.Errorf("%w: ServiceId is required", ErrInvalidParameter)
	}

	if len(vpcEndpointIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one VpcEndpointId is required", ErrInvalidParameter)
	}

	b.mu.Lock("RejectVpcEndpointConnections")
	defer b.mu.Unlock()

	unsuccessful := make([]string, 0, len(vpcEndpointIDs))

	for _, epID := range vpcEndpointIDs {
		key := serviceID + ":" + epID

		conn, ok := b.vpcEndpointConnections.Get(key)
		if !ok {
			unsuccessful = append(unsuccessful, epID)

			continue
		}

		conn.State = vpcEndpointConnectionStateRejected
	}

	return unsuccessful, nil
}

const vpcEndpointConnectionStateRejected = "rejected"
