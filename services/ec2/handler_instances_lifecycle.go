package ec2

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/url"
	"sort"
	"strconv"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// ---- action handlers ----

// maxUserDataBytes is the AWS limit on decoded EC2 user data (16 KiB).
const maxUserDataBytes = 16384

// validateUserData enforces the AWS EC2 contract for the UserData parameter:
// it must be valid standard base64 and, once decoded, must not exceed the
// 16 KiB limit. An empty value is accepted (user data is optional). Malformed
// base64 yields InvalidUserData.Malformed; an over-limit payload yields
// InvalidParameterValue, matching AWS error codes.
func validateUserData(userData string) error {
	if userData == "" {
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(userData)
	if err != nil {
		return fmt.Errorf(
			"%w: user data must be a valid base64-encoded string",
			ErrInvalidUserData,
		)
	}

	if len(decoded) > maxUserDataBytes {
		return fmt.Errorf(
			"%w: User data is limited to %d bytes",
			ErrInvalidParameter,
			maxUserDataBytes,
		)
	}

	return nil
}

// applyInstanceLaunchSettings persists the base64 user data and applies the key
// name and security groups to each freshly launched instance.
func (h *Handler) applyInstanceLaunchSettings(
	instances []*Instance,
	userData, keyName string,
	sgIDs []string,
) error {
	for _, inst := range instances {
		if userData != "" {
			// Store as-is; DescribeInstanceAttribute returns the raw (base64) form.
			if err := h.Backend.SetInstanceAttribute(inst.ID, attrUserData, userData); err != nil {
				return err
			}
		}

		if keyName != "" {
			inst.KeyName = keyName
		}

		if len(sgIDs) > 0 {
			inst.SecurityGroups = sgIDs
		}
	}

	return nil
}

// applyInstanceLaunchAttributes wires the RunInstances top-level
// DisableApiTermination / InstanceInitiatedShutdownBehavior / EbsOptimized
// parameters (distinct from the post-launch ModifyInstanceAttribute path)
// onto newly-created instances.
func (h *Handler) applyInstanceLaunchAttributes(
	instances []*Instance,
	disableAPITermination, shutdownBehavior, ebsOptimized string,
) error {
	type launchAttr struct {
		name, value string
	}

	// maxLaunchAttrs is the number of RunInstances attribute params handled
	// below (DisableApiTermination, InstanceInitiatedShutdownBehavior, EbsOptimized).
	const maxLaunchAttrs = 3

	attrs := make([]launchAttr, 0, maxLaunchAttrs)
	if disableAPITermination != "" {
		attrs = append(attrs, launchAttr{attrDisableAPITermination, disableAPITermination})
	}

	if shutdownBehavior != "" {
		attrs = append(attrs, launchAttr{attrInstanceInitiatedShutdownBehavior, shutdownBehavior})
	}

	if ebsOptimized != "" {
		attrs = append(attrs, launchAttr{attrEBSOptimized, ebsOptimized})
	}

	if len(attrs) == 0 {
		return nil
	}

	for _, inst := range instances {
		for _, a := range attrs {
			if err := h.Backend.SetInstanceAttribute(inst.ID, a.name, a.value); err != nil {
				return err
			}
		}
	}

	return nil
}

// iamInstanceProfileArg reads the real RunInstances/AssociateIamInstanceProfile
// IamInstanceProfile.Arn/IamInstanceProfile.Name wire keys (serializers.go:91938,
// awsEc2query_serializeDocumentIamInstanceProfileSpecification), preferring Arn.
func iamInstanceProfileArg(vals url.Values) string {
	if arn := vals.Get("IamInstanceProfile.Arn"); arn != "" {
		return arn
	}

	return vals.Get("IamInstanceProfile.Name")
}

// activeIamInstanceProfile returns the wire-shaped IAM instance profile for
// instanceID's current "associated" association, or nil if it has none --
// mirrors real DescribeInstances/RunInstances rendering types.Instance.
// IamInstanceProfile (deserializers.go:110585).
func (h *Handler) activeIamInstanceProfile(instanceID string) *iamProfileSpec {
	for _, assoc := range h.Backend.DescribeIamInstanceProfileAssociations(nil, instanceID) {
		if assoc.State == stateAssociated {
			return &iamProfileSpec{ARN: assoc.IamInstanceProfile, ID: iamProfileName(assoc.IamInstanceProfile)}
		}
	}

	return nil
}

func (h *Handler) handleRunInstances(vals url.Values, reqID string) (any, error) {
	imageID := vals.Get("ImageId")
	instanceType := vals.Get("InstanceType")
	subnetID := vals.Get("SubnetId")
	userData := vals.Get("UserData")
	keyName := vals.Get("KeyName")
	disableAPITermination := vals.Get("DisableApiTermination")
	shutdownBehavior := vals.Get("InstanceInitiatedShutdownBehavior")
	ebsOptimized := vals.Get("EbsOptimized")

	if err := validateUserData(userData); err != nil {
		return nil, err
	}

	minCount, maxCount, err := parseRunInstancesCounts(vals)
	if err != nil {
		return nil, err
	}

	_ = maxCount // AWS uses MaxCount for capacity planning; mock always launches minCount

	sgIDs, err := h.validateSecurityGroupIDs(vals)
	if err != nil {
		return nil, err
	}

	instances, err := h.Backend.RunInstances(imageID, instanceType, subnetID, minCount)
	if err != nil {
		return nil, err
	}

	if err = h.applyInstanceLaunchSettings(instances, userData, keyName, sgIDs); err != nil {
		return nil, err
	}

	if err = h.applyInstanceLaunchAttributes(
		instances, disableAPITermination, shutdownBehavior, ebsOptimized,
	); err != nil {
		return nil, err
	}

	if profileARN := iamInstanceProfileArg(vals); profileARN != "" {
		for _, inst := range instances {
			if _, err = h.Backend.AssociateIamInstanceProfile(inst.ID, profileARN); err != nil {
				return nil, err
			}
		}
	}

	if cb, c := h.computeBackend(); c != nil {
		h.launchOnCompute(h.svcCtx, cb, c, instances, keyName, userData)
	}

	if tags := parseTagSpecification(vals, "instance"); len(tags) > 0 {
		ids := make([]string, 0, len(instances))
		for _, inst := range instances {
			ids = append(ids, inst.ID)
		}

		if err = h.Backend.CreateTags(ids, tags); err != nil {
			return nil, err
		}
	}

	items := make([]instanceItem, 0, len(instances))
	for _, inst := range instances {
		items = append(
			items,
			toInstanceItem(inst, h.Backend.TagsForResource(inst.ID), h.activeIamInstanceProfile(inst.ID)),
		)
	}

	return &runInstancesResponse{
		Xmlns:         ec2XMLNS,
		RequestID:     reqID,
		ReservationID: newReservationID(),
		OwnerID:       h.AccountID,
		InstancesSet:  instanceItemSet{Items: items},
	}, nil
}

// describeInstancesMaxResults is the maximum MaxResults for DescribeInstances.
const describeInstancesMaxResults = 1000

// describeInstancesMinResults is the minimum MaxResults for DescribeInstances.
const describeInstancesMinResults = 5

func (h *Handler) handleDescribeInstances(vals url.Values, reqID string) (any, error) {
	ids := parseMemberList(vals, "InstanceId")

	// Parse named EC2 filters: Filter.N.Name / Filter.N.Value.M
	filters := parseEC2Filters(vals)

	// Fetch all instances matching the IDs (state filter applied post-fetch so
	// that multi-value OR semantics work: e.g. state=running OR state=stopped).
	instances := h.Backend.DescribeInstances(ids, "")

	// Apply all filters post-fetch (AND across filter names, OR within values).
	instances = applyInstanceFilters(instances, filters, h.Backend)

	// Pagination: MaxResults / NextToken.
	maxResults := 0
	if v := vals.Get("MaxResults"); v != "" {
		if _, scanErr := fmt.Sscan(v, &maxResults); scanErr != nil || maxResults < 1 {
			return nil, fmt.Errorf("%w: MaxResults must be a positive integer", ErrInvalidParameter)
		}
		if maxResults < describeInstancesMinResults || maxResults > describeInstancesMaxResults {
			return nil, fmt.Errorf(
				"%w: MaxResults must be between %d and %d",
				ErrInvalidParameter,
				describeInstancesMinResults,
				describeInstancesMaxResults,
			)
		}
	}

	offset := 0
	if tok := vals.Get("NextToken"); tok != "" {
		n := page.DecodeHMACToken(tok, ec2PaginationSalt)
		if n == 0 {
			return nil, fmt.Errorf("%w: the pagination token is not valid", ErrInvalidPaginationToken)
		}
		offset = n
	}

	var nextToken string

	if maxResults > 0 {
		if offset > len(instances) {
			offset = len(instances)
		}

		instances = instances[offset:]

		if len(instances) > maxResults {
			nextToken = page.EncodeHMACToken(offset+maxResults, ec2PaginationSalt)
			instances = instances[:maxResults]
		}
	}

	items := make([]instanceItem, 0, len(instances))
	for _, inst := range instances {
		items = append(
			items,
			toInstanceItem(inst, h.Backend.TagsForResource(inst.ID), h.activeIamInstanceProfile(inst.ID)),
		)
	}

	reservation := reservationItem{
		ReservationID: newReservationID(),
		OwnerID:       h.AccountID,
		InstancesSet:  instanceItemSet{Items: items},
	}

	return &describeInstancesResponse{
		Xmlns:          ec2XMLNS,
		RequestID:      reqID,
		ReservationSet: reservationSet{Items: []reservationItem{reservation}},
		NextToken:      nextToken,
	}, nil
}

func (h *Handler) handleTerminateInstances(vals url.Values, reqID string) (any, error) {
	ids := parseMemberList(vals, "InstanceId")
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: at least one InstanceId is required", ErrInvalidParameter)
	}

	cb, c := h.computeBackend()

	var (
		providerIDs map[string]string
		dnsNames    map[string]string
	)

	if c != nil {
		providerIDs = snapshotProviderIDs(cb, ids)

		if lookup, ok := h.Backend.(instanceLookup); ok {
			dnsNames = snapshotPublicDNSNames(lookup, ids)
		}
	}

	changes, err := h.Backend.TerminateInstances(ids)
	if err != nil {
		return nil, err
	}

	if c != nil {
		h.terminateOnCompute(h.svcCtx, cb, c, providerIDs, dnsNames)
	}

	items := make([]instanceStateChangeItem, 0, len(changes))
	for _, ch := range changes {
		items = append(items, instanceStateChangeItem{
			InstanceID:    ch.InstanceID,
			CurrentState:  stateItem{Code: ch.CurrentState.Code, Name: ch.CurrentState.Name},
			PreviousState: stateItem{Code: ch.PreviousState.Code, Name: ch.PreviousState.Name},
		})
	}

	return &terminateInstancesResponse{
		Xmlns:        ec2XMLNS,
		RequestID:    reqID,
		InstancesSet: instanceStateChangeSet{Items: items},
	}, nil
}

// ec2DescribeInstanceTypesMaxPageSize is the AWS-documented upper bound for
// MaxResults on DescribeInstanceTypes. The minimum is 5.
const (
	ec2DescribeInstanceTypesMaxPageSize = 100
	ec2DescribeInstanceTypesMinPageSize = 5
	ec2DefaultInstanceTypeFallback      = "t2.micro"
)

// handleDescribeInstanceTypes returns a stub response for the requested instance
// types. Multiple `InstanceType.N` values are echoed back. `MaxResults` and
// `NextToken` are honored so that callers iterating over instance-type catalogs
// see AWS-shaped pagination, with NextToken representing an opaque integer
// offset into the requested set.
func (h *Handler) handleDescribeInstanceTypes(vals url.Values, reqID string) (any, error) {
	requested := parseMemberList(vals, "InstanceType")

	// Backwards-compat: when a Filter.1.Value.1 is supplied (older callers), use it.
	if len(requested) == 0 {
		if v := vals.Get("Filter.1.Value.1"); v != "" {
			requested = []string{v}
		}
	}

	if len(requested) == 0 {
		requested = []string{ec2DefaultInstanceTypeFallback}
	}

	maxResults, nextToken, err := parseInstanceTypesPagination(vals)
	if err != nil {
		return nil, err
	}

	page, outToken := paginateInstanceTypes(requested, nextToken, maxResults)

	items := make([]instanceTypeItem, 0, len(page))
	for _, t := range page {
		items = append(items, instanceTypeItem{InstanceType: t})
	}

	return &describeInstanceTypesResponse{
		Xmlns:         ec2XMLNS,
		RequestID:     reqID,
		NextToken:     outToken,
		InstanceTypes: instanceTypeSet{Items: items},
	}, nil
}

// parseInstanceTypesPagination validates MaxResults bounds and decodes
// NextToken (which we serialize as a base-10 offset into the result set).
func parseInstanceTypesPagination(vals url.Values) (int, int, error) {
	maxResults := 0

	if v := vals.Get("MaxResults"); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < ec2DescribeInstanceTypesMinPageSize ||
			n > ec2DescribeInstanceTypesMaxPageSize {
			return 0, 0, fmt.Errorf(
				"%w: MaxResults=%q must be between %d and %d",
				ErrInvalidParameter, v,
				ec2DescribeInstanceTypesMinPageSize, ec2DescribeInstanceTypesMaxPageSize,
			)
		}

		maxResults = n
	}

	offset := 0

	if tok := vals.Get("NextToken"); tok != "" {
		n := page.DecodeHMACToken(tok, ec2PaginationSalt)
		if n == 0 {
			return 0, 0, fmt.Errorf("%w: NextToken %q is not valid", ErrInvalidPaginationToken, tok)
		}

		offset = n
	}

	return maxResults, offset, nil
}

// paginateInstanceTypes slices the instance-type catalog and returns the next
// pagination token (empty when fully consumed).
func paginateInstanceTypes(items []string, offset, maxResults int) ([]string, string) {
	if offset >= len(items) {
		return nil, ""
	}

	end := len(items)
	if maxResults > 0 && offset+maxResults < end {
		end = offset + maxResults
	}

	pageResult := items[offset:end]

	var token string
	if end < len(items) {
		token = page.EncodeHMACToken(end, ec2PaginationSalt)
	}

	return pageResult, token
}

// boolToEC2Attr renders a Go bool as the "true"/"false" string EC2 query-protocol
// attribute values use.
func boolToEC2Attr(v bool) string {
	if v {
		return ec2BooleanTrue
	}

	return ec2BooleanFalse
}

// handleDescribeInstanceAttribute returns the current value for the requested instance attribute.
// Terraform calls this after RunInstances to read instanceInitiatedShutdownBehavior.
func (h *Handler) handleDescribeInstanceAttribute(vals url.Values, reqID string) (any, error) {
	instanceID := vals.Get("InstanceId")
	attr := vals.Get("Attribute")

	if instanceID == "" {
		return nil, fmt.Errorf("%w: InstanceId is required", ErrInvalidParameter)
	}

	instances := h.Backend.DescribeInstances([]string{instanceID}, "")
	if len(instances) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
	}

	inst := instances[0]
	attrValue := h.instanceAttributeValue(inst, instanceID, attr)

	return &describeInstanceAttributeResponse{
		Xmlns:      ec2XMLNS,
		RequestID:  reqID,
		InstanceID: instanceID,
		Attribute:  namedStringAttr{XMLName: xml.Name{Local: attr}, Value: attrValue},
	}, nil
}

// instanceAttributeValue builds the DescribeInstanceAttribute string value
// from stored instance state when possible, falling back to AWS defaults for
// unmodelled attributes. Split out of handleDescribeInstanceAttribute to keep
// cyclomatic complexity down.
func (h *Handler) instanceAttributeValue(inst *Instance, instanceID, attr string) string {
	switch attr {
	case attrUserData:
		return inst.UserData
	case attrInstanceType:
		return inst.InstanceType
	case attrEnaSupport:
		return boolToEC2Attr(inst.EnaSupport)
	case attrSriovNetSupport:
		if inst.SriovNetSupport != "" {
			return inst.SriovNetSupport
		}

		return "simple"
	case attrDisableAPIStop:
		return boolToEC2Attr(inst.DisableAPIStop)
	case attrDisableAPITermination:
		return boolToEC2Attr(inst.DisableAPITermination)
	case attrEBSOptimized:
		return boolToEC2Attr(inst.EBSOptimized)
	case attrSourceDest:
		// sourceDestCheck lives on the primary ENI attachment; AWS defaults
		// it to true for VPC instances.
		return boolToEC2Attr(h.Backend.PrimaryNetworkInterfaceSourceDestCheck(instanceID))
	case attrInstanceInitiatedShutdownBehavior:
		if inst.InstanceInitiatedShutdownBehavior != "" {
			return inst.InstanceInitiatedShutdownBehavior
		}

		return "stop"
	case attrKernel, attrRamdisk:
		// Modern (HVM) instances have no kernel/ramdisk image; AWS returns an
		// empty value rather than "stop" (that default only applies to
		// instanceInitiatedShutdownBehavior).
		return ""
	default:
		return ""
	}
}

func toInstanceItem(inst *Instance, instanceTags map[string]string, iamProfile *iamProfileSpec) instanceItem {
	tagItems := make([]instanceTagItem, 0, len(instanceTags))
	for k, v := range instanceTags {
		tagItems = append(tagItems, instanceTagItem{Key: k, Value: v})
	}

	sort.Slice(tagItems, func(i, j int) bool { return tagItems[i].Key < tagItems[j].Key })

	groupItems := make([]instanceGroupItem, 0, len(inst.SecurityGroups))
	for _, sgID := range inst.SecurityGroups {
		groupItems = append(groupItems, instanceGroupItem{GroupID: sgID})
	}

	item := instanceItem{
		InstanceID:            inst.ID,
		ImageID:               inst.ImageID,
		InstanceType:          inst.InstanceType,
		StateItem:             stateItem{Code: inst.State.Code, Name: inst.State.Name},
		StateTransitionReason: inst.StateTransitionReason,
		VPCID:                 inst.VPCID,
		SubnetID:              inst.SubnetID,
		LaunchTime:            inst.LaunchTime.UTC().Format("2006-01-02T15:04:05.000Z"),
		OutpostArn:            inst.OutpostArn,
		PrivateIPAddress:      inst.PrivateIP,
		PublicIPAddress:       inst.PublicIPAddress,
		PublicDNSName:         inst.PublicDNSName,
		KeyName:               inst.KeyName,
		SriovNetSupport:       inst.SriovNetSupport,
		EBSOptimized:          inst.EBSOptimized,
		EnaSupport:            inst.EnaSupport,
		GroupSet:              instanceGroupSet{Items: groupItems},
		TagSet:                instanceTagItemSet{Items: tagItems},
		IamInstanceProfile:    iamProfile,
		Placement: instancePlacementItem{
			Tenancy:          inst.Placement.Tenancy,
			AvailabilityZone: inst.Placement.AvailabilityZone,
			GroupName:        inst.Placement.GroupName,
			Affinity:         inst.Placement.Affinity,
		},
	}

	if inst.StateReasonCode != "" {
		item.StateReasonItem = &stateReasonItem{
			Code:    inst.StateReasonCode,
			Message: inst.StateReasonMessage,
		}
	}

	if inst.CPUOptions.CoreCount > 0 || inst.CPUOptions.ThreadsPerCore > 0 {
		item.CPUOptions = &instanceCPUOptionsItem{
			CoreCount:      inst.CPUOptions.CoreCount,
			ThreadsPerCore: inst.CPUOptions.ThreadsPerCore,
		}
	}
	if inst.MaintenanceOptions.AutoRecovery != "" {
		item.MaintenanceOptions = &instanceMaintenanceOptionsItem{
			AutoRecovery: inst.MaintenanceOptions.AutoRecovery,
		}
	}
	if inst.NetworkPerformanceOptions.BandwidthWeighting != "" {
		item.NetworkPerformanceOptions = &instanceNetworkPerformanceOptionsItem{
			BandwidthWeighting: inst.NetworkPerformanceOptions.BandwidthWeighting,
		}
	}

	return item
}

type stateItem struct {
	Name string `xml:"name"`
	Code int    `xml:"code"`
}

type instanceGroupItem struct {
	GroupID   string `xml:"groupId"`
	GroupName string `xml:"groupName"`
}

type instanceGroupSet struct {
	Items []instanceGroupItem `xml:"item"`
}

type instancePlacementItem struct {
	Tenancy          string `xml:"tenancy,omitempty"`
	AvailabilityZone string `xml:"availabilityZone,omitempty"`
	GroupName        string `xml:"groupName,omitempty"`
	Affinity         string `xml:"affinity,omitempty"`
}

// stateReasonItem is the <stateReason> element carrying the structured
// code/message for an instance's most recent state transition.
type stateReasonItem struct {
	Code    string `xml:"code,omitempty"`
	Message string `xml:"message,omitempty"`
}

type instanceCPUOptionsItem struct {
	CoreCount      int32 `xml:"coreCount"`
	ThreadsPerCore int32 `xml:"threadsPerCore"`
}

type instanceMaintenanceOptionsItem struct {
	AutoRecovery string `xml:"autoRecovery,omitempty"`
}

type instanceNetworkPerformanceOptionsItem struct {
	BandwidthWeighting string `xml:"bandwidthWeighting,omitempty"`
}

type instanceItem struct {
	NetworkPerformanceOptions *instanceNetworkPerformanceOptionsItem `xml:"networkPerformanceOptions,omitempty"`
	MaintenanceOptions        *instanceMaintenanceOptionsItem        `xml:"maintenanceOptions,omitempty"`
	CPUOptions                *instanceCPUOptionsItem                `xml:"cpuOptions,omitempty"`
	StateReasonItem           *stateReasonItem                       `xml:"stateReason,omitempty"`
	IamInstanceProfile        *iamProfileSpec                        `xml:"iamInstanceProfile,omitempty"`
	Placement                 instancePlacementItem                  `xml:"placement"`
	// OutpostArn is a top-level field, sibling to Placement -- see
	// store.go's Instance.OutpostArn doc comment for the SDK confirmation.
	OutpostArn       string    `xml:"outpostArn,omitempty"`
	PublicDNSName    string    `xml:"dnsName,omitempty"`
	SubnetID         string    `xml:"subnetId,omitempty"`
	PrivateIPAddress string    `xml:"privateIpAddress,omitempty"`
	PublicIPAddress  string    `xml:"ipAddress,omitempty"`
	LaunchTime       string    `xml:"launchTime"`
	KeyName          string    `xml:"keyName,omitempty"`
	VPCID            string    `xml:"vpcId,omitempty"`
	InstanceType     string    `xml:"instanceType"`
	ImageID          string    `xml:"imageId"`
	InstanceID       string    `xml:"instanceId"`
	StateItem        stateItem `xml:"instanceState"`
	// StateTransitionReason is AWS's legacy free-text reason string, distinct
	// from the structured StateReasonItem above.
	StateTransitionReason string             `xml:"reason,omitempty"`
	SriovNetSupport       string             `xml:"sriovNetSupport,omitempty"`
	GroupSet              instanceGroupSet   `xml:"groupSet"`
	TagSet                instanceTagItemSet `xml:"tagSet"`
	EBSOptimized          bool               `xml:"ebsOptimized"`
	EnaSupport            bool               `xml:"enaSupport"`
}

// instanceTagItem is the embedded per-instance tag entry in DescribeInstances
// XML (no resourceId/resourceType fields, only key/value).
type instanceTagItem struct {
	Key   string `xml:"key"`
	Value string `xml:"value"`
}

type instanceTagItemSet struct {
	Items []instanceTagItem `xml:"item"`
}

type instanceItemSet struct {
	Items []instanceItem `xml:"item"`
}

type runInstancesResponse struct {
	XMLName       xml.Name        `xml:"RunInstancesResponse"`
	Xmlns         string          `xml:"xmlns,attr"`
	RequestID     string          `xml:"requestId"`
	ReservationID string          `xml:"reservationId"`
	OwnerID       string          `xml:"ownerId"`
	InstancesSet  instanceItemSet `xml:"instancesSet"`
}

type reservationItem struct {
	ReservationID string          `xml:"reservationId"`
	OwnerID       string          `xml:"ownerId"`
	InstancesSet  instanceItemSet `xml:"instancesSet"`
}

type reservationSet struct {
	Items []reservationItem `xml:"item"`
}

type describeInstancesResponse struct {
	XMLName        xml.Name       `xml:"DescribeInstancesResponse"`
	Xmlns          string         `xml:"xmlns,attr"`
	RequestID      string         `xml:"requestId"`
	NextToken      string         `xml:"nextToken,omitempty"`
	ReservationSet reservationSet `xml:"reservationSet"`
}

type instanceStateChangeItem struct {
	InstanceID    string    `xml:"instanceId"`
	CurrentState  stateItem `xml:"currentState"`
	PreviousState stateItem `xml:"previousState"`
}

type instanceStateChangeSet struct {
	Items []instanceStateChangeItem `xml:"item"`
}

type terminateInstancesResponse struct {
	XMLName      xml.Name               `xml:"TerminateInstancesResponse"`
	Xmlns        string                 `xml:"xmlns,attr"`
	RequestID    string                 `xml:"requestId"`
	InstancesSet instanceStateChangeSet `xml:"instancesSet"`
}

type instanceTypeItem struct {
	InstanceType string `xml:"instanceType"`
}

type instanceTypeSet struct {
	Items []instanceTypeItem `xml:"item"`
}

type describeInstanceTypesResponse struct {
	XMLName       xml.Name        `xml:"DescribeInstanceTypesResponse"`
	Xmlns         string          `xml:"xmlns,attr"`
	RequestID     string          `xml:"requestId"`
	NextToken     string          `xml:"nextToken,omitempty"`
	InstanceTypes instanceTypeSet `xml:"instanceTypeSet"`
}

type namedStringAttr struct {
	XMLName xml.Name `json:"xmlName"`
	Value   string   `json:"value,omitempty" xml:"value"`
}

type describeInstanceAttributeResponse struct {
	XMLName    xml.Name `xml:"DescribeInstanceAttributeResponse"`
	Xmlns      string   `xml:"xmlns,attr"`
	RequestID  string   `xml:"requestId"`
	InstanceID string   `xml:"instanceId"`
	Attribute  namedStringAttr
}
