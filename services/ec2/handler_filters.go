package ec2

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// This file adds EC2 filter matching for resource types that previously
// supported only ID-based lookup. Each applyXxxFilters function follows the
// standard EC2 convention: AND across filter names, OR within each filter's
// values. Unknown filter names pass through (lenient mock behaviour).
//
// tag:<key> filters are supported on all types that store tags. They delegate
// to Backend.TagsForResource which is already used by applyInstanceFilters.

// Common EC2 filter key name constants — shared across filter match functions.
const (
	filterKeyVPCID            = "vpc-id"
	filterKeySubnetID         = "subnet-id"
	filterKeyState            = "state"
	filterKeyStatus           = "status"
	filterKeyDescription      = "description"
	filterKeyInstanceID       = "instance-id"
	filterKeyAvailabilityZone = "availability-zone"
	filterKeyVolumeID         = "volume-id"
	filterKeyDhcpConfigKey    = "key"
	filterKeyDhcpConfigValue  = "value"
	filterKeyResourceID       = "resource-id"
	filterKeyInstanceType     = "instance-type"
	filterKeyType             = "type"
	filterKeyOwnerID          = "owner-id"
	filterKeySecondaryNetID   = "secondary-network-id"
	filterKeyResourceType     = "resource-type"
	filterKeyAttachInstanceID = "attachment.instance-id"
)

// tagMatch returns true when the resource's tag at tagKey equals any of values.
func tagMatch(resourceID string, tagKey string, values []string, b Backend) bool {
	tags := b.TagsForResource(resourceID)
	tagVal, exists := tags[tagKey]
	if !exists {
		return false
	}

	return anyEqual(tagVal, values)
}

// ---- VPC filters ----

func applyVPCFilters(vpcs []*VPC, filters map[string][]string, b Backend) []*VPC {
	if len(filters) == 0 {
		return vpcs
	}

	out := vpcs[:0:0]
vpcLoop:
	for _, v := range vpcs {
		for name, values := range filters {
			if !vpcMatchesFilter(v, name, values, b) {
				continue vpcLoop
			}
		}

		out = append(out, v)
	}

	return out
}

func vpcMatchesFilter(v *VPC, filterName string, values []string, b Backend) bool {
	switch filterName {
	case filterKeyVPCID:
		return anyEqual(v.ID, values)
	case "cidr", "cidr-block", "cidrBlock":
		return anyEqual(v.CIDRBlock, values)
	case "isDefault", "is-default":
		want := anyEqual("true", values)

		return v.IsDefault == want
	case filterKeyState:
		return anyEqual("available", values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(v.ID, tagKey, values, b)
		}
	}

	return true
}

// ---- Subnet filters ----

func applySubnetFilters(subnets []*Subnet, filters map[string][]string, b Backend) []*Subnet {
	if len(filters) == 0 {
		return subnets
	}

	out := subnets[:0:0]
subnetLoop:
	for _, s := range subnets {
		for name, values := range filters {
			if !subnetMatchesFilter(s, name, values, b) {
				continue subnetLoop
			}
		}

		out = append(out, s)
	}

	return out
}

func subnetMatchesFilter(s *Subnet, filterName string, values []string, b Backend) bool {
	switch filterName {
	case filterKeySubnetID:
		return anyEqual(s.ID, values)
	case filterKeyVPCID:
		return anyEqual(s.VPCID, values)
	case "cidr", "cidr-block", "cidrBlock":
		return anyEqual(s.CIDRBlock, values)
	case "availabilityZone", filterKeyAvailabilityZone:
		return anyEqual(s.AvailabilityZone, values)
	case filterKeyState:
		return anyEqual("available", values)
	case "defaultForAz", "default-for-az":
		want := anyEqual("true", values)

		return s.IsDefault == want
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(s.ID, tagKey, values, b)
		}
	}

	return true
}

// ---- Volume filters ----

func applyVolumeFilters(vols []*Volume, filters map[string][]string, b Backend) []*Volume {
	if len(filters) == 0 {
		return vols
	}

	out := vols[:0:0]
volLoop:
	for _, vol := range vols {
		for name, values := range filters {
			if !volumeMatchesFilter(vol, name, values, b) {
				continue volLoop
			}
		}

		out = append(out, vol)
	}

	return out
}

func volumeMatchesFilter(vol *Volume, filterName string, values []string, b Backend) bool {
	switch filterName {
	case filterKeyVolumeID:
		return anyEqual(vol.ID, values)
	case filterKeyStatus:
		return anyEqual(vol.State, values)
	case filterKeyAvailabilityZone:
		return anyEqual(vol.AZ, values)
	case "volume-type":
		return anyEqual(vol.VolumeType, values)
	case "encrypted":
		want := anyEqual("true", values)

		return vol.Encrypted == want
	case filterKeyAttachInstanceID:
		if vol.Attachment == nil {
			return false
		}

		return anyEqual(vol.Attachment.InstanceID, values)
	case "attachment.status":
		if vol.Attachment == nil {
			return false
		}

		return anyEqual(vol.Attachment.State, values)
	case "attachment.device":
		if vol.Attachment == nil {
			return false
		}

		return anyEqual(vol.Attachment.Device, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(vol.ID, tagKey, values, b)
		}
	}

	return true
}

// ---- KeyPair filters ----

func applyKeyPairFilters(kps []*KeyPair, filters map[string][]string, b Backend) []*KeyPair {
	if len(filters) == 0 {
		return kps
	}

	out := kps[:0:0]
kpLoop:
	for _, kp := range kps {
		for name, values := range filters {
			if !keyPairMatchesFilter(kp, name, values, b) {
				continue kpLoop
			}
		}

		out = append(out, kp)
	}

	return out
}

func keyPairMatchesFilter(kp *KeyPair, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "key-name":
		return anyEqual(kp.Name, values)
	case "key-pair-id":
		return anyEqual(kp.KeyPairID, values)
	case "fingerprint":
		return anyEqual(kp.Fingerprint, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			// Tags are stored under the key pair's Name (its only real,
			// stable identifier in this backend — see resourceExistsCoreLocked);
			// this previously looked up "keypair-"+Name, a key nothing ever
			// wrote to, so the filter silently never matched.
			return tagMatch(kp.Name, tagKey, values, b)
		}
	}

	return true
}

// ---- Snapshot filters ----

func applySnapshotFilters(snaps []*Snapshot, filters map[string][]string, b Backend) []*Snapshot {
	if len(filters) == 0 {
		return snaps
	}

	out := snaps[:0:0]
snapLoop:
	for _, s := range snaps {
		for name, values := range filters {
			if !snapshotMatchesFilter(s, name, values, b) {
				continue snapLoop
			}
		}

		out = append(out, s)
	}

	return out
}

func snapshotMatchesFilter(s *Snapshot, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "snapshot-id":
		return anyEqual(s.SnapshotID, values)
	case filterKeyVolumeID:
		return anyEqual(s.VolumeID, values)
	case filterKeyStatus:
		return anyEqual(s.State, values)
	case "encrypted":
		want := anyEqual("true", values)

		return s.Encrypted == want
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(s.SnapshotID, tagKey, values, b)
		}
	}

	return true
}

// ---- InternetGateway filters ----

func applyIGWFilters(igws []*InternetGateway, filters map[string][]string, b Backend) []*InternetGateway {
	if len(filters) == 0 {
		return igws
	}

	out := igws[:0:0]
igwLoop:
	for _, igw := range igws {
		for name, values := range filters {
			if !igwMatchesFilter(igw, name, values, b) {
				continue igwLoop
			}
		}

		out = append(out, igw)
	}

	return out
}

func igwMatchesFilter(igw *InternetGateway, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "internet-gateway-id":
		return anyEqual(igw.ID, values)
	case "attachment.vpc-id":
		for _, att := range igw.Attachments {
			if anyEqual(att.VPCID, values) {
				return true
			}
		}

		return false
	case "attachment.state":
		for _, att := range igw.Attachments {
			if anyEqual(att.State, values) {
				return true
			}
		}

		return false
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(igw.ID, tagKey, values, b)
		}
	}

	return true
}

// ---- NatGateway filters ----

func applyNatGWFilters(ngws []*NatGateway, filters map[string][]string, b Backend) []*NatGateway {
	if len(filters) == 0 {
		return ngws
	}

	out := ngws[:0:0]
natLoop:
	for _, ngw := range ngws {
		for name, values := range filters {
			if !natGWMatchesFilter(ngw, name, values, b) {
				continue natLoop
			}
		}

		out = append(out, ngw)
	}

	return out
}

func natGWMatchesFilter(ngw *NatGateway, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "nat-gateway-id":
		return anyEqual(ngw.ID, values)
	case filterKeySubnetID:
		return anyEqual(ngw.SubnetID, values)
	case filterKeyState:
		return anyEqual(ngw.State, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(ngw.ID, tagKey, values, b)
		}
	}

	return true
}

// ---- NetworkInterface filters ----

func applyENIFilters(enis []*NetworkInterface, filters map[string][]string, b Backend) []*NetworkInterface {
	if len(filters) == 0 {
		return enis
	}

	out := enis[:0:0]
eniLoop:
	for _, eni := range enis {
		for name, values := range filters {
			if !eniMatchesFilter(eni, name, values, b) {
				continue eniLoop
			}
		}

		out = append(out, eni)
	}

	return out
}

func eniMatchesFilter(eni *NetworkInterface, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "network-interface-id":
		return anyEqual(eni.ID, values)
	case filterKeyVPCID:
		return anyEqual(eni.VPCID, values)
	case filterKeySubnetID:
		return anyEqual(eni.SubnetID, values)
	case filterKeyStatus:
		return anyEqual(eni.Status, values)
	case filterKeyDescription:
		return anyEqual(eni.Description, values)
	case "private-ip-address":
		return anyEqual(eni.PrivateIP, values)
	case filterKeyAttachInstanceID:
		return anyEqual(eni.InstanceID, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(eni.ID, tagKey, values, b)
		}
	}

	return true
}

// ---- Address (EIP) filters ----

func applyAddressFilters(addrs []*Address, filters map[string][]string, b Backend) []*Address {
	if len(filters) == 0 {
		return addrs
	}

	out := addrs[:0:0]
addrLoop:
	for _, addr := range addrs {
		for name, values := range filters {
			if !addressMatchesFilter(addr, name, values, b) {
				continue addrLoop
			}
		}

		out = append(out, addr)
	}

	return out
}

func addressMatchesFilter(addr *Address, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "allocation-id":
		return anyEqual(addr.AllocationID, values)
	case "public-ip":
		return anyEqual(addr.PublicIP, values)
	case "association-id":
		return anyEqual(addr.AssociationID, values)
	case filterKeyInstanceID:
		return anyEqual(addr.InstanceID, values)
	case "domain":
		return anyEqual(resourceTypeVPC, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(addr.AllocationID, tagKey, values, b)
		}
	}

	return true
}

// ---- RouteTable filters ----

func applyRouteTableFilters(rts []*RouteTable, filters map[string][]string, b Backend) []*RouteTable {
	if len(filters) == 0 {
		return rts
	}

	out := rts[:0:0]
rtLoop:
	for _, rt := range rts {
		for name, values := range filters {
			if !routeTableMatchesFilter(rt, name, values, b) {
				continue rtLoop
			}
		}

		out = append(out, rt)
	}

	return out
}

func routeTableMatchesFilter(rt *RouteTable, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "route-table-id":
		return anyEqual(rt.ID, values)
	case filterKeyVPCID:
		return anyEqual(rt.VPCID, values)
	case "association.subnet-id":
		return routeTableHasAssocSubnet(rt, values)
	case "association.route-table-association-id":
		return routeTableHasAssocID(rt, values)
	case "route.destination-cidr-block":
		return routeTableHasRoute(rt, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(rt.ID, tagKey, values, b)
		}
	}

	return true
}

func routeTableHasAssocSubnet(rt *RouteTable, values []string) bool {
	for _, assoc := range rt.Associations {
		if anyEqual(assoc.SubnetID, values) {
			return true
		}
	}

	return false
}

func routeTableHasAssocID(rt *RouteTable, values []string) bool {
	for _, assoc := range rt.Associations {
		if anyEqual(assoc.ID, values) {
			return true
		}
	}

	return false
}

func routeTableHasRoute(rt *RouteTable, values []string) bool {
	for _, r := range rt.Routes {
		if anyEqual(r.DestinationCIDR, values) {
			return true
		}
	}

	return false
}

// ---- AMI / Image filters ----

func applyImageFilters(amis []*AMIStub, filters map[string][]string, b Backend) []*AMIStub {
	if len(filters) == 0 {
		return amis
	}

	out := amis[:0:0]
amiLoop:
	for _, a := range amis {
		for name, values := range filters {
			if !imageMatchesFilter(a, name, values, b) {
				continue amiLoop
			}
		}

		out = append(out, a)
	}

	return out
}

func imageMatchesFilter(a *AMIStub, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "image-id":
		return anyEqual(a.ImageID, values)
	case "name":
		return anyEqual(a.Name, values)
	case "architecture":
		return anyEqual(a.Architecture, values)
	case "platform":
		return anyEqual(a.Platform, values)
	case filterKeyState:
		st := a.State
		if st == "" {
			st = stateAvailable
		}

		return anyEqual(st, values)
	case "root-device-name":
		return anyEqual(a.RootDeviceName, values)
	case filterKeyDescription:
		return anyEqual(a.Description, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(a.ImageID, tagKey, values, b)
		}
	}

	return true
}

// ---- SpotInstanceRequest filters ----

func applySpotRequestFilters(
	reqs []*SpotInstanceRequest,
	filters map[string][]string,
	b Backend,
) []*SpotInstanceRequest {
	if len(filters) == 0 {
		return reqs
	}

	out := reqs[:0:0]
spotLoop:
	for _, req := range reqs {
		for name, values := range filters {
			if !spotRequestMatchesFilter(req, name, values, b) {
				continue spotLoop
			}
		}

		out = append(out, req)
	}

	return out
}

func spotRequestMatchesFilter(req *SpotInstanceRequest, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "spot-instance-request-id":
		return anyEqual(req.ID, values)
	case filterKeyState:
		return anyEqual(req.State, values)
	case filterKeyInstanceID:
		return anyEqual(req.InstanceID, values)
	case "launch-specification.image-id":
		return anyEqual(req.LaunchSpec.ImageID, values)
	case "launch-specification.instance-type":
		return anyEqual(req.LaunchSpec.InstanceType, values)
	case "launch-specification.subnet-id":
		return anyEqual(req.LaunchSpec.SubnetID, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(req.ID, tagKey, values, b)
		}
	}

	return true
}

// itoa converts an int to decimal string.
func itoa(i int) string {
	return strconv.Itoa(i)
}

// parseIntValue parses s into *v. Ignores parse errors (best-effort).

// parseIntValue parses s into *v. Ignores parse errors (best-effort).
func parseIntValue(s string, v *int) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return
	}
	if v != nil {
		*v = n
	}
}

// parseInt32Value parses s directly into an int32 (via ParseInt with a 32-bit
// size, so there is no separate overflow-prone truncation step), returning 0
// for empty/unparseable/out-of-range input (best-effort, mirrors parseIntValue).

// parseInt32Value parses s directly into an int32 (via ParseInt with a 32-bit
// size, so there is no separate overflow-prone truncation step), returning 0
// for empty/unparseable/out-of-range input (best-effort, mirrors parseIntValue).
func parseInt32Value(s string) int32 {
	if s == "" {
		return 0
	}

	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}

	return int32(n)
}

// maxInstancesPerRunInstancesRequest bounds MinCount/MaxCount so a
// client-supplied value can never drive an unbounded slice allocation in
// RunInstances (CodeQL go/uncontrolled-allocation-size, alert #253). This is
// gopherstack's own allocation-safety cap, not a modeled AWS quota -- real
// EC2 has no flat per-request instance-count limit, only per-account/
// instance-type quotas (see gopherstack-x6r7).
const maxInstancesPerRunInstancesRequest = 1000

// parseRunInstancesCounts validates and returns MinCount and MaxCount from RunInstances params.
// MinCount defaults to 1 when absent. MaxCount defaults to MinCount when absent.
func parseRunInstancesCounts(vals url.Values) (int, int, error) {
	minCnt := 1
	if v := vals.Get("MinCount"); v != "" {
		if _, scanErr := fmt.Sscan(v, &minCnt); scanErr != nil || minCnt < 1 {
			return 0, 0, fmt.Errorf("%w: MinCount must be a positive integer", ErrInvalidParameter)
		}
	}

	if minCnt > maxInstancesPerRunInstancesRequest {
		return 0, 0, fmt.Errorf(
			"%w: MinCount must not exceed %d",
			ErrResourceCountExceeded, maxInstancesPerRunInstancesRequest,
		)
	}

	maxCnt := minCnt
	if v := vals.Get("MaxCount"); v != "" {
		if _, scanErr := fmt.Sscan(v, &maxCnt); scanErr != nil || maxCnt < 1 {
			return 0, 0, fmt.Errorf("%w: MaxCount must be a positive integer", ErrInvalidParameter)
		}
	}

	if maxCnt < minCnt {
		return 0, 0, fmt.Errorf("%w: MaxCount must be greater than or equal to MinCount", ErrInvalidParameter)
	}

	if maxCnt > maxInstancesPerRunInstancesRequest {
		return 0, 0, fmt.Errorf(
			"%w: MaxCount must not exceed %d",
			ErrResourceCountExceeded, maxInstancesPerRunInstancesRequest,
		)
	}

	return minCnt, maxCnt, nil
}

// validateSecurityGroupIDs parses RunInstances' SecurityGroupId.N (IDs, any
// VPC) and SecurityGroup.N (names) from vals and resolves both to security
// group IDs. Real RunInstancesInput.SecurityGroups is documented "[Default
// VPC] The names of the security groups" (ec2@v1.319.1 api_op_RunInstances.go)
// -- a name only resolves for the default VPC; supplying one for a subnet in
// a non-default VPC is rejected, matching AWS's InvalidParameterCombination
// ("The parameter groupName cannot be used with the parameter subnet").
func (h *Handler) validateSecurityGroupIDs(vals url.Values) ([]string, error) {
	sgIDs := parseMemberList(vals, "SecurityGroupId")
	if len(sgIDs) > 0 {
		existing := h.Backend.DescribeSecurityGroups(sgIDs)
		if len(existing) != len(sgIDs) {
			return nil, fmt.Errorf("%w: one or more SecurityGroupId values not found", ErrSecurityGroupNotFound)
		}
	}

	names := parseMemberList(vals, "SecurityGroup")
	if len(names) == 0 {
		return sgIDs, nil
	}

	resolvedIDs, err := h.resolveSecurityGroupNames(names, vals.Get("SubnetId"))
	if err != nil {
		return nil, err
	}

	return append(sgIDs, resolvedIDs...), nil
}

// resolveSecurityGroupNames resolves RunInstances SecurityGroup.N names to
// group IDs within the launch target's VPC (the subnet's VPC, or the
// account's default VPC when subnetID is empty).
func (h *Handler) resolveSecurityGroupNames(names []string, subnetID string) ([]string, error) {
	vpcID := vpcDefaultName
	if subnetID != "" {
		if subs := h.Backend.DescribeSubnets([]string{subnetID}); len(subs) == 1 {
			vpcID = subs[0].VPCID
		}
	}

	if vpcs := h.Backend.DescribeVpcs([]string{vpcID}); len(vpcs) == 1 && !vpcs[0].IsDefault {
		return nil, fmt.Errorf(
			"%w: The parameter groupName cannot be used with the parameter subnet",
			ErrInvalidParameterCombination,
		)
	}

	all := h.Backend.DescribeSecurityGroups(nil)
	resolvedIDs := make([]string, 0, len(names))

	for _, name := range names {
		id := ""

		for _, sg := range all {
			if sg.Name == name && sg.VPCID == vpcID {
				id = sg.ID

				break
			}
		}

		if id == "" {
			return nil, fmt.Errorf("%w: security group %q not found in VPC %s", ErrSecurityGroupNotFound, name, vpcID)
		}

		resolvedIDs = append(resolvedIDs, id)
	}

	return resolvedIDs, nil
}

// parseEC2Filters parses Filter.N.Name / Filter.N.Value.M from EC2 form values.
// Returns a map of filter name → list of accepted values (OR semantics per AWS).

// parseEC2Filters parses Filter.N.Name / Filter.N.Value.M from EC2 form values.
// Returns a map of filter name → list of accepted values (OR semantics per AWS).
func parseEC2Filters(vals url.Values) map[string][]string {
	filters := make(map[string][]string)

	for i := 1; ; i++ {
		name := vals.Get(fmt.Sprintf("Filter.%d.Name", i))
		if name == "" {
			break
		}

		var values []string
		for j := 1; ; j++ {
			v := vals.Get(fmt.Sprintf("Filter.%d.Value.%d", i, j))
			if v == "" {
				break
			}

			values = append(values, v)
		}

		if len(values) > 0 {
			filters[name] = values
		}
	}

	return filters
}

// applyInstanceFilters ANDs across filter names, ORs within each filter's values.
// Supports instance-state-name, image-id, vpc-id, subnet-id, instance-type, key-name,
// private-ip-address, ip-address, and tag:<key>.
func applyInstanceFilters(instances []*Instance, filters map[string][]string, b Backend) []*Instance {
	if len(filters) == 0 {
		return instances
	}

	out := instances[:0:0]

instanceLoop:
	for _, inst := range instances {
		for name, values := range filters {
			if !instanceMatchesFilter(inst, name, values, b) {
				continue instanceLoop
			}
		}

		out = append(out, inst)
	}

	return out
}

// instanceMatchesFilter returns true if the instance matches any value in the filter.

// instanceMatchesFilter returns true if the instance matches any value in the filter.
func instanceMatchesFilter(inst *Instance, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "instance-state-name":
		return anyEqual(inst.State.Name, values)
	case "image-id":
		return anyEqual(inst.ImageID, values)
	case filterKeyVPCID:
		return anyEqual(inst.VPCID, values)
	case filterKeySubnetID:
		return anyEqual(inst.SubnetID, values)
	case filterKeyInstanceType:
		return anyEqual(inst.InstanceType, values)
	case "key-name":
		return anyEqual(inst.KeyName, values)
	case "private-ip-address":
		return anyEqual(inst.PrivateIP, values)
	case "ip-address":
		return anyEqual(inst.PublicIPAddress, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			tags := b.TagsForResource(inst.ID)
			tagVal, exists := tags[tagKey]

			if !exists {
				return false
			}

			return slices.Contains(values, tagVal)
		}
	}

	// Unknown filters: pass through (lenient, per common mock behaviour).
	return true
}

// anyEqual returns true if target equals any element in vals.

// anyEqual returns true if target equals any element in vals.
func anyEqual(target string, vals []string) bool {
	return slices.Contains(vals, target)
}

// applySecurityGroupFilters filters security groups by named EC2 filter values.
// Supported filter names: vpc-id, group-name, group-id, tag:<key>.
func applySecurityGroupFilters(
	groups []*SecurityGroup,
	filters map[string][]string,
	b Backend,
) []*SecurityGroup {
	if len(filters) == 0 {
		return groups
	}

	out := groups[:0:0]

groupLoop:
	for _, sg := range groups {
		for name, values := range filters {
			if !sgMatchesFilter(sg, name, values, b) {
				continue groupLoop
			}
		}

		out = append(out, sg)
	}

	return out
}

// sgMatchesFilter returns true if the security group matches any value in the filter.
func sgMatchesFilter(sg *SecurityGroup, filterName string, values []string, b Backend) bool {
	switch filterName {
	case filterKeyVPCID:
		return anyEqual(sg.VPCID, values)
	case "group-name":
		return anyEqual(sg.Name, values)
	case "group-id":
		return anyEqual(sg.ID, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(sg.ID, tagKey, values, b)
		}
	}

	// Unknown filters: pass through (lenient).
	return true
}

// gopherstack-j2v5: the apply*Filters functions below wire up Filters for
// Describe operations that previously declared the parameter but never read
// it, so a real client's filter was silently ignored and every item came
// back. Each implements only the filter names its own SDK doc comment
// (api_op_Describe*.go) lists AND that this backend's struct actually
// stores; a documented name naming untracked data is left unimplemented and
// noted in PARITY.md rather than fabricated.

// ---- DhcpOptions filters ----

// applyDhcpOptionsFilters supports dhcp-options-id, key, value, tag,
// tag-key (api_op_DescribeDhcpOptions.go). owner-id is documented but left:
// this backend does not store a per-resource owner distinct from the single
// account, matching how the rest of this file omits owner-id elsewhere
// (e.g. imageMatchesFilter).
func applyDhcpOptionsFilters(opts []*DhcpOptions, filters map[string][]string, b Backend) []*DhcpOptions {
	if len(filters) == 0 {
		return opts
	}

	out := opts[:0:0]
dhcpLoop:
	for _, o := range opts {
		for name, values := range filters {
			if !dhcpOptionsMatchesFilter(o, name, values, b) {
				continue dhcpLoop
			}
		}

		out = append(out, o)
	}

	return out
}

func dhcpOptionsMatchesFilter(o *DhcpOptions, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "dhcp-options-id":
		return anyEqual(o.DhcpOptionsID, values)
	case filterKeyDhcpConfigKey:
		for _, cfg := range o.Configurations {
			if anyEqual(cfg.Key, values) {
				return true
			}
		}

		return false
	case filterKeyDhcpConfigValue:
		for _, cfg := range o.Configurations {
			for _, v := range cfg.Values {
				if anyEqual(v, values) {
					return true
				}
			}
		}

		return false
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(o.DhcpOptionsID, tagKey, values, b)
		}
	}

	return true
}

// ---- EgressOnlyInternetGateway filters ----

// applyEOIGWFilters supports only tag/tag-key
// (api_op_DescribeEgressOnlyInternetGateways.go documents no other filter names).
func applyEOIGWFilters(
	igws []*EgressOnlyInternetGateway,
	filters map[string][]string,
	b Backend,
) []*EgressOnlyInternetGateway {
	if len(filters) == 0 {
		return igws
	}

	out := igws[:0:0]
eoigwLoop:
	for _, igw := range igws {
		for name, values := range filters {
			if tagKey, ok := strings.CutPrefix(name, "tag:"); ok {
				if !tagMatch(igw.ID, tagKey, values, b) {
					continue eoigwLoop
				}

				continue
			}
			// Unknown/unsupported filter names pass through (lenient).
		}

		out = append(out, igw)
	}

	return out
}

// ---- Static PrefixList filters ----

// applyPrefixListFilters supports prefix-list-id, prefix-list-name
// (api_op_DescribePrefixLists.go).
func applyPrefixListFilters(lists []PrefixList, filters map[string][]string) []PrefixList {
	if len(filters) == 0 {
		return lists
	}

	out := lists[:0:0]
plLoop:
	for _, pl := range lists {
		for name, values := range filters {
			switch name {
			case "prefix-list-id":
				if !anyEqual(pl.PrefixListID, values) {
					continue plLoop
				}
			case "prefix-list-name":
				if !anyEqual(pl.PrefixListName, values) {
					continue plLoop
				}
			}
		}

		out = append(out, pl)
	}

	return out
}

// ---- ManagedPrefixList filters ----

// applyManagedPrefixListFilters supports owner-id, prefix-list-id,
// prefix-list-name (api_op_DescribeManagedPrefixLists.go).
func applyManagedPrefixListFilters(
	lists []*ManagedPrefixList,
	filters map[string][]string,
) []*ManagedPrefixList {
	if len(filters) == 0 {
		return lists
	}

	out := lists[:0:0]
mplLoop:
	for _, pl := range lists {
		for name, values := range filters {
			if !managedPrefixListMatchesFilter(pl, name, values) {
				continue mplLoop
			}
		}

		out = append(out, pl)
	}

	return out
}

func managedPrefixListMatchesFilter(pl *ManagedPrefixList, filterName string, values []string) bool {
	switch filterName {
	case filterKeyOwnerID:
		return anyEqual(pl.OwnerID, values)
	case "prefix-list-id":
		return anyEqual(pl.PrefixListID, values)
	case "prefix-list-name":
		return anyEqual(pl.PrefixListName, values)
	}

	return true
}

// ---- Ipv4Pool (DescribePublicIpv4Pools) filters ----

// applyIpv4PoolFilters supports only tag/tag-key
// (api_op_DescribePublicIpv4Pools.go documents no other filter names).
func applyIpv4PoolFilters(pools []*Ipv4Pool, filters map[string][]string, b Backend) []*Ipv4Pool {
	if len(filters) == 0 {
		return pools
	}

	out := pools[:0:0]
poolLoop:
	for _, p := range pools {
		for name, values := range filters {
			if tagKey, ok := strings.CutPrefix(name, "tag:"); ok {
				if !tagMatch(p.PoolID, tagKey, values, b) {
					continue poolLoop
				}

				continue
			}
			// Unknown/unsupported filter names pass through (lenient).
		}

		out = append(out, p)
	}

	return out
}

// ---- BundleTask filters ----

// applyBundleTaskFilters supports bundle-id, error-code, error-message,
// instance-id, progress, s3-bucket, s3-prefix, state
// (api_op_DescribeBundleTasks.go). start-time/update-time are documented but
// left: matching a Filter value against a timestamp requires the SDK's
// exact wire format, which BundleTask's Go time.Time doesn't preserve
// losslessly for string equality, and no other filter in this file matches
// on a timestamp field either.
func applyBundleTaskFilters(tasks []*BundleTask, filters map[string][]string) []*BundleTask {
	if len(filters) == 0 {
		return tasks
	}

	out := tasks[:0:0]
bundleLoop:
	for _, t := range tasks {
		for name, values := range filters {
			if !bundleTaskMatchesFilter(t, name, values) {
				continue bundleLoop
			}
		}

		out = append(out, t)
	}

	return out
}

func bundleTaskMatchesFilter(t *BundleTask, filterName string, values []string) bool {
	switch filterName {
	case "bundle-id":
		return anyEqual(t.BundleID, values)
	case "error-code":
		return anyEqual(t.ErrorCode, values)
	case "error-message":
		return anyEqual(t.ErrorMessage, values)
	case filterKeyInstanceID:
		return anyEqual(t.InstanceID, values)
	case "progress":
		return anyEqual(t.Progress, values)
	case "s3-bucket":
		return anyEqual(t.S3Bucket, values)
	case "s3-prefix":
		return anyEqual(t.S3Prefix, values)
	case filterKeyState:
		return anyEqual(t.State, values)
	}

	return true
}

// ---- CarrierGateway filters ----

// applyCarrierGatewayFilters supports carrier-gateway-id, state, owner-id,
// tag, tag-key, vpc-id (api_op_DescribeCarrierGateways.go).
func applyCarrierGatewayFilters(
	gws []*CarrierGateway,
	filters map[string][]string,
	b Backend,
) []*CarrierGateway {
	if len(filters) == 0 {
		return gws
	}

	out := gws[:0:0]
cgwLoop:
	for _, gw := range gws {
		for name, values := range filters {
			if !carrierGatewayMatchesFilter(gw, name, values, b) {
				continue cgwLoop
			}
		}

		out = append(out, gw)
	}

	return out
}

func carrierGatewayMatchesFilter(gw *CarrierGateway, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "carrier-gateway-id":
		return anyEqual(gw.CarrierGatewayID, values)
	case filterKeyState:
		return anyEqual(gw.State, values)
	case filterKeyOwnerID:
		return anyEqual(gw.OwnerID, values)
	case filterKeyVPCID:
		return anyEqual(gw.VpcID, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(gw.CarrierGatewayID, tagKey, values, b)
		}
	}

	return true
}

// ---- FlowLog filters ----

// applyFlowLogFilters supports deliver-log-status, log-destination-type,
// flow-log-id, log-group-name, resource-id, traffic-type, tag, tag-key
// (api_op_DescribeFlowLogs.go). log-group-name is documented but left: this
// backend does not model CloudWatch Logs log-group destinations separately
// from LogDestination, so there is nothing distinct to match.
func applyFlowLogFilters(logs []*FlowLog, filters map[string][]string, b Backend) []*FlowLog {
	if len(filters) == 0 {
		return logs
	}

	out := logs[:0:0]
flowLogLoop:
	for _, fl := range logs {
		for name, values := range filters {
			if !flowLogMatchesFilter(fl, name, values, b) {
				continue flowLogLoop
			}
		}

		out = append(out, fl)
	}

	return out
}

func flowLogMatchesFilter(fl *FlowLog, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "deliver-log-status":
		return anyEqual(fl.FlowLogStatus, values)
	case "log-destination-type":
		return anyEqual(fl.LogDestinationType, values)
	case "flow-log-id":
		return anyEqual(fl.FlowLogID, values)
	case filterKeyResourceID:
		return anyEqual(fl.ResourceID, values)
	case "traffic-type":
		return anyEqual(fl.TrafficType, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(fl.FlowLogID, tagKey, values, b)
		}
	}

	return true
}

// ---- NetworkACL filters ----

// applyNetworkACLFilters supports network-acl-id, vpc-id, default,
// association.association-id, association.network-acl-id,
// association.subnet-id, entry.cidr, entry.protocol, entry.rule-action,
// entry.rule-number, entry.egress, entry.port-range.from,
// entry.port-range.to, tag, tag-key (api_op_DescribeNetworkAcls.go).
// entry.icmp.code/entry.icmp.type/entry.ipv6-cidr and owner-id are
// documented but left: NACLEntry has no ICMP or IPv6 fields, and NetworkACL
// has no per-resource owner (see applyDhcpOptionsFilters' owner-id note).
//
// association.association-id and association.subnet-id both key off
// AssociationIDs: AddSubnetAssociation (network_acls.go) appends the raw
// subnetID there, so that list already IS the set of associated subnet IDs
// this backend tracks; there is no separately-modeled association ID.
func applyNetworkACLFilters(acls []*NetworkACL, filters map[string][]string, b Backend) []*NetworkACL {
	if len(filters) == 0 {
		return acls
	}

	out := acls[:0:0]
naclLoop:
	for _, acl := range acls {
		for name, values := range filters {
			if !naclMatchesFilter(acl, name, values, b) {
				continue naclLoop
			}
		}

		out = append(out, acl)
	}

	return out
}

func naclMatchesFilter(acl *NetworkACL, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "network-acl-id":
		return anyEqual(acl.ID, values)
	case filterKeyVPCID:
		return anyEqual(acl.VPCID, values)
	case "default":
		want := anyEqual("true", values)

		return acl.IsDefault == want
	}

	if strings.HasPrefix(filterName, "association.") {
		return naclMatchesAssociationFilter(acl, filterName, values)
	}

	if strings.HasPrefix(filterName, "entry.") {
		return naclMatchesEntryFilter(acl, filterName, values)
	}

	if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
		return tagMatch(acl.ID, tagKey, values, b)
	}

	return true
}

func naclMatchesAssociationFilter(acl *NetworkACL, filterName string, values []string) bool {
	switch filterName {
	case "association.association-id", "association.subnet-id":
		for _, aid := range acl.AssociationIDs {
			if anyEqual(aid, values) {
				return true
			}
		}

		return false
	case "association.network-acl-id":
		return len(acl.AssociationIDs) > 0 && anyEqual(acl.ID, values)
	}

	return true
}

func naclMatchesEntryFilter(acl *NetworkACL, filterName string, values []string) bool {
	switch filterName {
	case "entry.cidr":
		return naclEntryAny(acl, values, func(e NACLEntry) string { return e.CIDRBlock })
	case "entry.protocol":
		return naclEntryAny(acl, values, func(e NACLEntry) string { return e.Protocol })
	case "entry.rule-action":
		return naclEntryAny(acl, values, func(e NACLEntry) string { return e.RuleAction })
	case "entry.rule-number":
		return naclEntryAny(acl, values, func(e NACLEntry) string { return itoa(e.RuleNumber) })
	case "entry.port-range.from":
		return naclEntryAny(acl, values, func(e NACLEntry) string { return itoa(e.FromPort) })
	case "entry.port-range.to":
		return naclEntryAny(acl, values, func(e NACLEntry) string { return itoa(e.ToPort) })
	case "entry.egress":
		want := anyEqual("true", values)
		for _, e := range acl.Entries {
			if e.Egress == want {
				return true
			}
		}

		return false
	}

	return true
}

// naclEntryAny returns true if field(e) matches any value for any entry.
func naclEntryAny(acl *NetworkACL, values []string, field func(NACLEntry) string) bool {
	for _, e := range acl.Entries {
		if anyEqual(field(e), values) {
			return true
		}
	}

	return false
}

// ---- DescribeInstanceStatus filters ----

// applyInstanceStatusFilters supports availability-zone, instance-state-code,
// instance-state-name, instance-status.reachability, instance-status.status,
// system-status.reachability, system-status.status
// (api_op_DescribeInstanceStatus.go). availability-zone-id, event.*,
// operator.*, attached-ebs-status.status, and application-status.status are
// documented but left: this backend models neither scheduled events,
// managed-instance operators, nor per-resource-type health independent of
// the single computed instance/system status below.
func applyInstanceStatusFilters(instances []*Instance, filters map[string][]string) []*Instance {
	if len(filters) == 0 {
		return instances
	}

	out := instances[:0:0]
statusLoop:
	for _, inst := range instances {
		health := instanceHealthForState(inst.State.Name)
		for name, values := range filters {
			if !instanceStatusMatchesFilter(inst, health, name, values) {
				continue statusLoop
			}
		}

		out = append(out, inst)
	}

	return out
}

func instanceStatusMatchesFilter(
	inst *Instance,
	health instanceStatusDetails,
	filterName string,
	values []string,
) bool {
	switch filterName {
	case filterKeyAvailabilityZone:
		return anyEqual(inst.Placement.AvailabilityZone, values)
	case "instance-state-code":
		return anyEqual(itoa(inst.State.Code), values)
	case "instance-state-name":
		return anyEqual(inst.State.Name, values)
	case "instance-status.status", "system-status.status":
		return anyEqual(health.Status, values)
	case "instance-status.reachability", "system-status.reachability":
		for _, d := range health.Details {
			if d.Name == "reachability" && anyEqual(d.Status, values) {
				return true
			}
		}

		return false
	}

	return true
}

// applyActiveFleetInstanceFilters filters DescribeFleetInstances' results.
// Supports "instance-type", the only filter DescribeFleetInstancesInput
// documents (ec2@v1.319.1 api_op_DescribeFleetInstances.go).
func applyActiveFleetInstanceFilters(
	instances []ActiveFleetInstance, filters map[string][]string,
) []ActiveFleetInstance {
	if len(filters) == 0 {
		return instances
	}

	out := instances[:0:0]

instanceLoop:
	for _, inst := range instances {
		for name, values := range filters {
			if name == filterKeyInstanceType && !anyEqual(inst.InstanceType, values) {
				continue instanceLoop
			}
		}

		out = append(out, inst)
	}

	return out
}

// applyCustomerGatewayFilters supports bgp-asn, customer-gateway-id,
// ip-address, state, type, and tag: (api_op_DescribeCustomerGateways.go).
// amazon-side-asn/tag-key are documented but not implemented here.
func applyCustomerGatewayFilters(
	gws []*CustomerGateway, filters map[string][]string, b Backend,
) []*CustomerGateway {
	if len(filters) == 0 {
		return gws
	}

	out := gws[:0:0]

cgwLoop:
	for _, gw := range gws {
		for name, values := range filters {
			if !customerGatewayMatchesFilter(gw, name, values, b) {
				continue cgwLoop
			}
		}

		out = append(out, gw)
	}

	return out
}

func customerGatewayMatchesFilter(gw *CustomerGateway, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "bgp-asn":
		return anyEqual(gw.BgpAsn, values)
	case "customer-gateway-id":
		return anyEqual(gw.CustomerGatewayID, values)
	case "ip-address":
		return anyEqual(gw.IPAddress, values)
	case filterKeyState:
		return anyEqual(gw.State, values)
	case filterKeyType:
		return anyEqual(gw.Type, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(gw.CustomerGatewayID, tagKey, values, b)
		}
	}

	return true
}

// applyVpnGatewayFilters supports attachment.state, attachment.vpc-id,
// state, type, vpn-gateway-id, and tag: (api_op_DescribeVpnGateways.go).
// amazon-side-asn/availability-zone/tag-key are documented but not tracked
// by this backend's VpnGateway struct, so are left unimplemented.
func applyVpnGatewayFilters(
	gws []*VpnGateway, filters map[string][]string, b Backend,
) []*VpnGateway {
	if len(filters) == 0 {
		return gws
	}

	out := gws[:0:0]

vgwLoop:
	for _, gw := range gws {
		for name, values := range filters {
			if !vpnGatewayMatchesFilter(gw, name, values, b) {
				continue vgwLoop
			}
		}

		out = append(out, gw)
	}

	return out
}

func vpnGatewayMatchesFilter(gw *VpnGateway, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "attachment.state":
		return anyEqual(gw.AttachmentState, values)
	case "attachment.vpc-id":
		return anyEqual(gw.AttachedVPCID, values)
	case filterKeyState:
		return anyEqual(gw.State, values)
	case filterKeyType:
		return anyEqual(gw.Type, values)
	case "vpn-gateway-id":
		return anyEqual(gw.VpnGatewayID, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(gw.VpnGatewayID, tagKey, values, b)
		}
	}

	return true
}

// anyContains returns true when any element of list equals any of values.
func anyContains(list []string, values []string) bool {
	for _, item := range list {
		if anyEqual(item, values) {
			return true
		}
	}

	return false
}

// applyClassicLinkInstanceFilters supports group-id, vpc-id, and tag:
// (api_op_DescribeClassicLinkInstances.go). tag-key is documented but not
// implemented, matching this file's existing convention.
func applyClassicLinkInstanceFilters(
	links []*ClassicLinkInstance, filters map[string][]string, b Backend,
) []*ClassicLinkInstance {
	if len(filters) == 0 {
		return links
	}

	out := links[:0:0]

clLoop:
	for _, link := range links {
		for name, values := range filters {
			if !classicLinkInstanceMatchesFilter(link, name, values, b) {
				continue clLoop
			}
		}

		out = append(out, link)
	}

	return out
}

func classicLinkInstanceMatchesFilter(link *ClassicLinkInstance, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "group-id":
		return anyContains(link.Groups, values)
	case filterKeyVPCID:
		return anyEqual(link.VpcID, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(link.InstanceID, tagKey, values, b)
		}
	}

	return true
}

// applySecondaryInterfaceFilters supports owner-id, status,
// secondary-interface-id, secondary-interface-arn, secondary-interface-type,
// secondary-network-id, secondary-network-type, secondary-subnet-id,
// attachment.instance-id, private-ipv4-addresses.private-ip-address, and tag:
// (api_op_DescribeSecondaryInterfaces.go). attachment.attachment-id,
// attachment.instance-owner-id, attachment.status, and tag-key are
// documented but not tracked by this backend's SecondaryInterface struct.
func applySecondaryInterfaceFilters(
	sis []*SecondaryInterface, filters map[string][]string, b Backend,
) []*SecondaryInterface {
	if len(filters) == 0 {
		return sis
	}

	out := sis[:0:0]

siLoop:
	for _, si := range sis {
		for name, values := range filters {
			if !secondaryInterfaceMatchesFilter(si, name, values, b) {
				continue siLoop
			}
		}

		out = append(out, si)
	}

	return out
}

func secondaryInterfaceMatchesFilter(si *SecondaryInterface, filterName string, values []string, b Backend) bool {
	switch filterName {
	case filterKeyOwnerID:
		return anyEqual(si.OwnerID, values)
	case filterKeyStatus:
		return anyEqual(si.Status, values)
	case "secondary-interface-id":
		return anyEqual(si.SecondaryInterfaceID, values)
	case "secondary-interface-arn":
		return anyEqual(si.SecondaryInterfaceArn, values)
	case "secondary-interface-type":
		return anyEqual(si.SecondaryInterfaceType, values)
	case filterKeySecondaryNetID:
		return anyEqual(si.SecondaryNetworkID, values)
	case "secondary-network-type":
		return anyEqual(si.SecondaryNetworkType, values)
	case "secondary-subnet-id":
		return anyEqual(si.SecondarySubnetID, values)
	case filterKeyAttachInstanceID:
		return anyEqual(si.InstanceID, values)
	case "private-ipv4-addresses.private-ip-address":
		return anyContains(si.PrivateIpv4Addresses, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(si.SecondaryInterfaceID, tagKey, values, b)
		}
	}

	return true
}

// applySecondaryNetworkFilters supports owner-id, secondary-network-id,
// secondary-network-arn, state, type, ipv4-cidr-block-association.*, and tag:
// (api_op_DescribeSecondaryNetworks.go). tag-key is documented but not
// implemented, matching this file's existing convention.
func applySecondaryNetworkFilters(
	nets []*SecondaryNetwork, filters map[string][]string, b Backend,
) []*SecondaryNetwork {
	if len(filters) == 0 {
		return nets
	}

	out := nets[:0:0]

netLoop:
	for _, n := range nets {
		for name, values := range filters {
			if !secondaryNetworkMatchesFilter(n, name, values, b) {
				continue netLoop
			}
		}

		out = append(out, n)
	}

	return out
}

// secondaryNetworkCidrAssocField returns the association field matching
// filterName's "ipv4-cidr-block-association.*" suffix, and whether
// filterName was recognized as one of that family.
func secondaryNetworkCidrAssocField(assoc SecondaryNetworkCidrAssoc, filterName string) (string, bool) {
	switch filterName {
	case "ipv4-cidr-block-association.association-id":
		return assoc.AssociationID, true
	case "ipv4-cidr-block-association.cidr-block":
		return assoc.CidrBlock, true
	case "ipv4-cidr-block-association.state":
		return assoc.State, true
	default:
		return "", false
	}
}

func secondaryNetworkMatchesFilter(n *SecondaryNetwork, filterName string, values []string, b Backend) bool {
	switch filterName {
	case filterKeyOwnerID:
		return anyEqual(n.OwnerID, values)
	case filterKeySecondaryNetID:
		return anyEqual(n.SecondaryNetworkID, values)
	case "secondary-network-arn":
		return anyEqual(n.SecondaryNetworkArn, values)
	case filterKeyState:
		return anyEqual(n.State, values)
	case filterKeyType:
		return anyEqual(n.Type, values)
	default:
		if _, recognized := secondaryNetworkCidrAssocField(SecondaryNetworkCidrAssoc{}, filterName); recognized {
			for _, assoc := range n.Ipv4CidrBlockAssociations {
				field, _ := secondaryNetworkCidrAssocField(assoc, filterName)
				if anyEqual(field, values) {
					return true
				}
			}

			return false
		}

		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(n.SecondaryNetworkID, tagKey, values, b)
		}
	}

	return true
}

// applySecondarySubnetFilters supports owner-id, secondary-network-id,
// secondary-network-type, secondary-subnet-id, secondary-subnet-arn, state,
// ipv4-cidr-block-association.*, and tag: (api_op_DescribeSecondarySubnets.go).
// tag-key is documented but not implemented, matching this file's existing
// convention.
func applySecondarySubnetFilters(
	subs []*SecondarySubnet, filters map[string][]string, b Backend,
) []*SecondarySubnet {
	if len(filters) == 0 {
		return subs
	}

	out := subs[:0:0]

subLoop:
	for _, s := range subs {
		for name, values := range filters {
			if !secondarySubnetMatchesFilter(s, name, values, b) {
				continue subLoop
			}
		}

		out = append(out, s)
	}

	return out
}

// secondarySubnetCidrAssocField returns the association field matching
// filterName's "ipv4-cidr-block-association.*" suffix, and whether
// filterName was recognized as one of that family.
func secondarySubnetCidrAssocField(assoc SecondarySubnetCidrAssoc, filterName string) (string, bool) {
	switch filterName {
	case "ipv4-cidr-block-association.association-id":
		return assoc.AssociationID, true
	case "ipv4-cidr-block-association.cidr-block":
		return assoc.CidrBlock, true
	case "ipv4-cidr-block-association.state":
		return assoc.State, true
	default:
		return "", false
	}
}

func secondarySubnetMatchesFilter(s *SecondarySubnet, filterName string, values []string, b Backend) bool {
	switch filterName {
	case filterKeyOwnerID:
		return anyEqual(s.OwnerID, values)
	case filterKeySecondaryNetID:
		return anyEqual(s.SecondaryNetworkID, values)
	case "secondary-network-type":
		return anyEqual(s.SecondaryNetworkType, values)
	case "secondary-subnet-id":
		return anyEqual(s.SecondarySubnetID, values)
	case "secondary-subnet-arn":
		return anyEqual(s.SecondarySubnetArn, values)
	case filterKeyState:
		return anyEqual(s.State, values)
	default:
		if _, recognized := secondarySubnetCidrAssocField(SecondarySubnetCidrAssoc{}, filterName); recognized {
			for _, assoc := range s.Ipv4CidrBlockAssociations {
				field, _ := secondarySubnetCidrAssocField(assoc, filterName)
				if anyEqual(field, values) {
					return true
				}
			}

			return false
		}

		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(s.SecondarySubnetID, tagKey, values, b)
		}
	}

	return true
}

// applyServiceLinkVirtualInterfaceFilters supports owner-id, outpost-lag-id,
// outpost-arn, state, vlan, service-link-virtual-interface-id, and tag:
// (api_op_DescribeServiceLinkVirtualInterfaces.go). local-gateway-virtual-
// interface-id and tag-key are documented but not tracked by this backend's
// ServiceLinkVirtualInterface struct.
func applyServiceLinkVirtualInterfaceFilters(
	vifs []*ServiceLinkVirtualInterface, filters map[string][]string, b Backend,
) []*ServiceLinkVirtualInterface {
	if len(filters) == 0 {
		return vifs
	}

	out := vifs[:0:0]

vifLoop:
	for _, v := range vifs {
		for name, values := range filters {
			if !serviceLinkVirtualInterfaceMatchesFilter(v, name, values, b) {
				continue vifLoop
			}
		}

		out = append(out, v)
	}

	return out
}

func serviceLinkVirtualInterfaceMatchesFilter(
	v *ServiceLinkVirtualInterface, filterName string, values []string, b Backend,
) bool {
	switch filterName {
	case filterKeyOwnerID:
		return anyEqual(v.OwnerID, values)
	case "outpost-lag-id":
		return anyEqual(v.OutpostLagID, values)
	case "outpost-arn":
		return anyEqual(v.OutpostArn, values)
	case filterKeyState:
		return anyEqual(v.ConfigurationState, values)
	case "vlan":
		return anyEqual(strconv.Itoa(int(v.Vlan)), values)
	case "service-link-virtual-interface-id":
		return anyEqual(v.ServiceLinkVirtualInterfaceID, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(v.ServiceLinkVirtualInterfaceID, tagKey, values, b)
		}
	}

	return true
}

// applySQLHaHistoryFilters supports haStatus, sqlServerLicenseUsage, and
// tag: (api_op_DescribeInstanceSqlHaHistoryStates.go). tag-key is
// documented but not implemented, matching this file's existing convention.
func applySQLHaHistoryFilters(
	regs []*RegisteredSQLHaInstance, filters map[string][]string, b Backend,
) []*RegisteredSQLHaInstance {
	if len(filters) == 0 {
		return regs
	}

	out := regs[:0:0]

haLoop:
	for _, r := range regs {
		for name, values := range filters {
			if !sqlHaHistoryMatchesFilter(r, name, values, b) {
				continue haLoop
			}
		}

		out = append(out, r)
	}

	return out
}

func sqlHaHistoryMatchesFilter(r *RegisteredSQLHaInstance, filterName string, values []string, b Backend) bool {
	switch filterName {
	case "haStatus":
		return anyEqual(r.HaStatus, values)
	case "sqlServerLicenseUsage":
		return anyEqual(r.SQLServerLicenseUsage, values)
	default:
		if tagKey, ok := strings.CutPrefix(filterName, "tag:"); ok {
			return tagMatch(r.InstanceID, tagKey, values, b)
		}
	}

	return true
}

// applyImageUsageReportEntryFilters supports account-id, resource-type, and
// creation-time (api_op_DescribeImageUsageReportEntries.go). creation-time
// supports the documented "*" wildcard suffix (e.g. "2025-11-29*") to match
// an entire day/prefix, plus an exact RFC3339 match.
func applyImageUsageReportEntryFilters(
	entries []*UsageReportEntry, filters map[string][]string,
) []*UsageReportEntry {
	if len(filters) == 0 {
		return entries
	}

	out := entries[:0:0]

entryLoop:
	for _, e := range entries {
		for name, values := range filters {
			if !usageReportEntryMatchesFilter(e, name, values) {
				continue entryLoop
			}
		}

		out = append(out, e)
	}

	return out
}

func usageReportEntryMatchesFilter(e *UsageReportEntry, filterName string, values []string) bool {
	switch filterName {
	case "account-id":
		return anyEqual(e.AccountID, values)
	case filterKeyResourceType:
		return anyEqual(e.ResourceType, values)
	case "creation-time":
		// Must match toImageUsageReportEntryItem's wire format
		// (handler_image_ops.go) exactly, or an exact-match filter built
		// from the timestamp this API just returned never matches its own
		// record.
		creationTime := e.ReportCreationTime.UTC().Format(time.RFC3339)
		for _, v := range values {
			if prefix, ok := strings.CutSuffix(v, "*"); ok {
				if strings.HasPrefix(creationTime, prefix) {
					return true
				}

				continue
			}

			if creationTime == v {
				return true
			}
		}

		return false
	}

	return true
}
