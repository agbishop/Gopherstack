package ec2

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// spotFleetSupportedOperations returns the list of real (non-stub) spot fleet operations.
func spotFleetSupportedOperations() []string {
	return []string{
		"RequestSpotFleet",
		"DescribeSpotFleetRequests",
		"CancelSpotFleetRequests",
		"ModifySpotFleetRequest",
		"DescribeSpotFleetInstances",
		"DescribeSpotFleetRequestHistory",
		"CreateSpotDatafeedSubscription",
		"DeleteSpotDatafeedSubscription",
		"DescribeSpotDatafeedSubscription",
		"GetSpotPlacementScores",
	}
}

// registerSpotFleetOps registers the real spot fleet handlers, overriding stubs.
func registerSpotFleetOps(h *Handler, ops map[string]ec2ActionFn) {
	ops["RequestSpotFleet"] = h.handleRequestSpotFleet
	ops["DescribeSpotFleetRequests"] = h.handleDescribeSpotFleetRequests
	ops["CancelSpotFleetRequests"] = h.handleCancelSpotFleetRequests
	ops["ModifySpotFleetRequest"] = h.handleModifySpotFleetRequest
	ops["DescribeSpotFleetInstances"] = h.handleDescribeSpotFleetInstances
	ops["DescribeSpotFleetRequestHistory"] = h.handleDescribeSpotFleetRequestHistory
	ops["CreateSpotDatafeedSubscription"] = h.handleCreateSpotDatafeedSubscription
	ops["DeleteSpotDatafeedSubscription"] = h.handleDeleteSpotDatafeedSubscription
	ops["DescribeSpotDatafeedSubscription"] = h.handleDescribeSpotDatafeedSubscription
	ops["GetSpotPlacementScores"] = h.handleGetSpotPlacementScores
}

// parseSpotFleetTagSpecification extracts spot-fleet-request tags from
// SpotFleetRequestConfig.TagSpecification.N.Tag.M.Key/Value. Unlike every
// other create op's top-level TagSpecification.N, RequestSpotFleet's real
// serializer nests it under the SpotFleetRequestConfig object (serializers.go
// object.Key("SpotFleetRequestConfig") wrapping FlatKey("TagSpecification"),
// serializers.go:65330) -- the same nesting LaunchSpecifications already uses
// in this handler.
func parseSpotFleetTagSpecification(vals url.Values) map[string]string {
	tags := make(map[string]string)

	for i := 1; i <= maxTagsPerRequest; i++ {
		rt := vals.Get(fmt.Sprintf("SpotFleetRequestConfig.TagSpecification.%d.ResourceType", i))
		if rt == "" {
			break
		}

		if rt != "spot-fleet-request" {
			continue
		}

		for j := 1; j <= maxTagsPerRequest; j++ {
			key := vals.Get(fmt.Sprintf("SpotFleetRequestConfig.TagSpecification.%d.Tag.%d.Key", i, j))
			if key == "" {
				break
			}

			tags[key] = vals.Get(fmt.Sprintf("SpotFleetRequestConfig.TagSpecification.%d.Tag.%d.Value", i, j))
		}
	}

	return tags
}

// handleRequestSpotFleet parses and dispatches a RequestSpotFleet call.
func (h *Handler) handleRequestSpotFleet(vals url.Values, reqID string) (any, error) {
	config := SpotFleetRequestConfig{
		SpotPrice:          vals.Get("SpotFleetRequestConfig.SpotPrice"),
		AllocationStrategy: vals.Get("SpotFleetRequestConfig.AllocationStrategy"),
		ExcessCapacityTerminationPolicy: vals.Get(
			"SpotFleetRequestConfig.ExcessCapacityTerminationPolicy",
		),
		IamFleetRole: vals.Get("SpotFleetRequestConfig.IamFleetRole"),
		Type:         vals.Get("SpotFleetRequestConfig.Type")}

	if tcStr := vals.Get("SpotFleetRequestConfig.TargetCapacity"); tcStr != "" {
		tc, err := strconv.Atoi(tcStr)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid TargetCapacity: %s", ErrInvalidParameter, tcStr)
		}

		config.TargetCapacity = tc
	}

	if v := vals.Get("SpotFleetRequestConfig.TerminateInstancesWithExpiration"); v != "" {
		config.TerminateInstancesWithExpiration = strings.EqualFold(v, "true")
	}

	if v := vals.Get("SpotFleetRequestConfig.ReplaceUnhealthyInstances"); v != "" {
		config.ReplaceUnhealthyInstances = strings.EqualFold(v, "true")
	}

	// Parse launch specifications.
	for i := 1; ; i++ {
		prefix := fmt.Sprintf("SpotFleetRequestConfig.LaunchSpecifications.%d.", i)
		imageID := vals.Get(prefix + "ImageId")
		instanceType := vals.Get(prefix + "InstanceType")

		if imageID == "" && instanceType == "" {
			break
		}

		spec := SpotFleetLaunchSpecification{
			ImageID:      imageID,
			InstanceType: instanceType,
			SubnetID:     vals.Get(prefix + "SubnetId"),
			KeyName:      vals.Get(prefix + "KeyName"),
			SpotPrice:    vals.Get(prefix + "SpotPrice"),
		}

		if wcStr := vals.Get(prefix + "WeightedCapacity"); wcStr != "" {
			wc, err := strconv.ParseFloat(wcStr, 64)
			if err == nil {
				spec.WeightedCapacity = wc
			}
		}

		config.LaunchSpecifications = append(config.LaunchSpecifications, spec)
	}

	fleet, err := h.Backend.RequestSpotFleet(config)
	if err != nil {
		return nil, err
	}

	if tags := parseSpotFleetTagSpecification(vals); len(tags) > 0 {
		if tagErr := h.Backend.CreateTags([]string{fleet.SpotFleetRequestID}, tags); tagErr != nil {
			return nil, tagErr
		}
	}

	return &requestSpotFleetResponse{
		Xmlns:              ec2XMLNS,
		RequestID:          reqID,
		SpotFleetRequestID: fleet.SpotFleetRequestID,
	}, nil
}

// handleDescribeSpotFleetRequests handles DescribeSpotFleetRequests.
func (h *Handler) handleDescribeSpotFleetRequests(vals url.Values, reqID string) (any, error) {
	ids := parseMemberList(vals, "SpotFleetRequestId")
	fleets, err := h.Backend.DescribeSpotFleetRequests(ids)
	if err != nil {
		return nil, err
	}

	maxResults, offset, err := parseEC2Pagination(vals, ec2PageMinDefault, ec2PageMaxDefault, ec2PageMaxDefault)
	if err != nil {
		return nil, err
	}

	var nextToken string
	fleets, nextToken = pageSlice(fleets, offset, maxResults)

	items := make([]spotFleetRequestConfigSetItem, 0, len(fleets))
	for _, fleet := range fleets {
		specs := make(
			[]spotFleetLaunchSpecItem,
			0,
			len(fleet.SpotFleetRequestConfig.LaunchSpecifications),
		)
		for _, spec := range fleet.SpotFleetRequestConfig.LaunchSpecifications {
			specs = append(specs, spotFleetLaunchSpecItem{
				ImageID:          spec.ImageID,
				InstanceType:     spec.InstanceType,
				SubnetID:         spec.SubnetID,
				KeyName:          spec.KeyName,
				SpotPrice:        spec.SpotPrice,
				WeightedCapacity: fmt.Sprintf("%g", spec.WeightedCapacity),
			})
		}

		items = append(items, spotFleetRequestConfigSetItem{
			SpotFleetRequestID:    fleet.SpotFleetRequestID,
			SpotFleetRequestState: fleet.SpotFleetRequestState,
			ActivityStatus:        fleet.ActivityStatus,
			CreateTime:            fleet.CreateTime.Format(time.RFC3339),
			SpotFleetRequestConfig: spotFleetConfigItem{
				SpotPrice:                       fleet.SpotFleetRequestConfig.SpotPrice,
				TargetCapacity:                  fleet.SpotFleetRequestConfig.TargetCapacity,
				AllocationStrategy:              fleet.SpotFleetRequestConfig.AllocationStrategy,
				ExcessCapacityTerminationPolicy: fleet.SpotFleetRequestConfig.ExcessCapacityTerminationPolicy,
				IamFleetRole:                    fleet.SpotFleetRequestConfig.IamFleetRole,
				Type:                            fleet.SpotFleetRequestConfig.Type,
				LaunchSpecifications:            spotFleetLaunchSpecSet{Items: specs},
				FulfilledCapacity:               fmt.Sprintf("%g", fleet.FulfilledCapacity),
			},
			TagSet: tagItemsFromMap(h.Backend.TagsForResource(fleet.SpotFleetRequestID)),
		})
	}

	return &describeSpotFleetRequestsResponse{
		Xmlns:                     ec2XMLNS,
		RequestID:                 reqID,
		SpotFleetRequestConfigSet: spotFleetRequestConfigSet{Items: items},
		NextToken:                 nextToken,
	}, nil
}

// handleCancelSpotFleetRequests handles CancelSpotFleetRequests.
func (h *Handler) handleCancelSpotFleetRequests(vals url.Values, reqID string) (any, error) {
	ids := parseMemberList(vals, "SpotFleetRequestId")
	terminateInstances := strings.EqualFold(vals.Get("TerminateInstances"), "true")

	results, err := h.Backend.CancelSpotFleetRequests(ids, terminateInstances)
	if err != nil {
		return nil, err
	}

	successItems := make([]cancelSpotFleetSuccessItem, 0, len(results))
	errorItems := make([]cancelSpotFleetErrorItem, 0, len(results))

	for _, r := range results {
		if r.Error != "" {
			errorItems = append(errorItems, cancelSpotFleetErrorItem{
				SpotFleetRequestID: r.SpotFleetRequestID,
				Error:              cancelSpotFleetErrorDetail{Code: r.Error},
			})
		} else {
			successItems = append(successItems, cancelSpotFleetSuccessItem{
				SpotFleetRequestID:            r.SpotFleetRequestID,
				CurrentSpotFleetRequestState:  r.CurrentSpotFleetRequestState,
				PreviousSpotFleetRequestState: r.PreviousSpotFleetRequestState,
			})
		}
	}

	return &cancelSpotFleetRequestsResponse{
		Xmlns:                     ec2XMLNS,
		RequestID:                 reqID,
		SuccessfulFleetRequests:   cancelSpotFleetSuccessSet{Items: successItems},
		UnsuccessfulFleetRequests: cancelSpotFleetErrorSet{Items: errorItems},
	}, nil
}

// handleModifySpotFleetRequest handles ModifySpotFleetRequest.
func (h *Handler) handleModifySpotFleetRequest(vals url.Values, reqID string) (any, error) {
	fleetID := vals.Get("SpotFleetRequestId")
	tcStr := vals.Get("TargetCapacity")
	excessTermination := vals.Get("ExcessCapacityTerminationPolicy")

	tc := 0
	if tcStr != "" {
		parsed, err := strconv.Atoi(tcStr)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid TargetCapacity: %s", ErrInvalidParameter, tcStr)
		}

		tc = parsed
	}

	_, err := h.Backend.ModifySpotFleetRequest(fleetID, tc, excessTermination)
	if err != nil {
		return nil, err
	}

	return &modifySpotFleetRequestResponse{
		Xmlns:     ec2XMLNS,
		RequestID: reqID,
		Return:    true,
	}, nil
}

// handleDescribeSpotFleetInstances handles DescribeSpotFleetInstances.
func (h *Handler) handleDescribeSpotFleetInstances(vals url.Values, reqID string) (any, error) {
	fleetID := vals.Get("SpotFleetRequestId")

	instances, err := h.Backend.DescribeSpotFleetInstances(fleetID)
	if err != nil {
		return nil, err
	}

	maxResults, offset, err := parseEC2Pagination(vals, ec2PageMinDefault, ec2PageMaxDefault, ec2PageMaxDefault)
	if err != nil {
		return nil, err
	}

	var nextToken string
	instances, nextToken = pageSlice(instances, offset, maxResults)

	items := make([]spotFleetInstanceItem, 0, len(instances))
	for _, inst := range instances {
		items = append(items, spotFleetInstanceItem{
			InstanceID:            inst.InstanceID,
			InstanceType:          inst.InstanceType,
			SpotInstanceRequestID: inst.SpotInstanceRequestID,
			InstanceHealth:        inst.InstanceHealth,
		})
	}

	return &describeSpotFleetInstancesResponse{
		Xmlns:              ec2XMLNS,
		RequestID:          reqID,
		SpotFleetRequestID: fleetID,
		ActiveInstances:    spotFleetInstanceSet{Items: items},
		NextToken:          nextToken,
	}, nil
}

// handleDescribeSpotFleetRequestHistory handles DescribeSpotFleetRequestHistory.
func (h *Handler) handleDescribeSpotFleetRequestHistory(
	vals url.Values,
	reqID string,
) (any, error) {
	fleetID := vals.Get("SpotFleetRequestId")
	startTimeStr := vals.Get("StartTime")

	startTime := time.Time{}
	if startTimeStr != "" {
		parsed, err := time.Parse(time.RFC3339, startTimeStr)
		if err == nil {
			startTime = parsed
		}
	}

	records, err := h.Backend.DescribeSpotFleetRequestHistory(fleetID, startTime)
	if err != nil {
		return nil, err
	}

	maxResults, offset, err := parseEC2Pagination(vals, ec2PageMinDefault, ec2PageMaxDefault, ec2PageMaxDefault)
	if err != nil {
		return nil, err
	}

	var nextToken string
	records, nextToken = pageSlice(records, offset, maxResults)

	// LastEvaluatedTime is only present when nextToken is empty -- real AWS
	// documents it as "all records up to this time were retrieved".
	var lastEvaluatedTime string
	if nextToken == "" {
		lastEvaluatedTime = time.Now().UTC().Format(time.RFC3339)
	}

	items := make([]spotFleetHistoryRecordItem, 0, len(records))
	for _, rec := range records {
		items = append(items, spotFleetHistoryRecordItem{
			Timestamp:        rec.Timestamp.Format(time.RFC3339),
			EventType:        rec.EventType,
			EventInformation: spotFleetEventInformationItem{EventDescription: rec.EventInformation},
		})
	}

	return &describeSpotFleetRequestHistoryResponse{
		Xmlns:              ec2XMLNS,
		RequestID:          reqID,
		SpotFleetRequestID: fleetID,
		StartTime:          startTime.Format(time.RFC3339),
		HistoryRecords:     spotFleetHistoryRecordSet{Items: items},
		LastEvaluatedTime:  lastEvaluatedTime,
		NextToken:          nextToken,
	}, nil
}

// ---- XML response types ----

type requestSpotFleetResponse struct {
	XMLName            xml.Name `xml:"RequestSpotFleetResponse"`
	Xmlns              string   `xml:"xmlns,attr"`
	RequestID          string   `xml:"requestId"`
	SpotFleetRequestID string   `xml:"spotFleetRequestId"`
}

type spotFleetLaunchSpecItem struct {
	ImageID          string `xml:"imageId,omitempty"`
	InstanceType     string `xml:"instanceType,omitempty"`
	SubnetID         string `xml:"subnetId,omitempty"`
	KeyName          string `xml:"keyName,omitempty"`
	SpotPrice        string `xml:"spotPrice,omitempty"`
	WeightedCapacity string `xml:"weightedCapacity,omitempty"`
}

type spotFleetLaunchSpecSet struct {
	Items []spotFleetLaunchSpecItem `xml:"item"`
}

type spotFleetConfigItem struct {
	SpotPrice                       string                 `xml:"spotPrice,omitempty"`
	AllocationStrategy              string                 `xml:"allocationStrategy,omitempty"`
	ExcessCapacityTerminationPolicy string                 `xml:"excessCapacityTerminationPolicy,omitempty"`
	IamFleetRole                    string                 `xml:"iamFleetRole,omitempty"`
	Type                            string                 `xml:"type,omitempty"`
	FulfilledCapacity               string                 `xml:"fulfilledCapacity,omitempty"`
	LaunchSpecifications            spotFleetLaunchSpecSet `xml:"launchSpecifications"`
	TargetCapacity                  int                    `xml:"targetCapacity"`
}

type spotFleetRequestConfigSetItem struct {
	SpotFleetRequestID     string              `xml:"spotFleetRequestId"`
	SpotFleetRequestState  string              `xml:"spotFleetRequestState"`
	ActivityStatus         string              `xml:"activityStatus,omitempty"`
	CreateTime             string              `xml:"createTime"`
	TagSet                 []simpleTagItem     `xml:"tagSet>item"`
	SpotFleetRequestConfig spotFleetConfigItem `xml:"spotFleetRequestConfig"`
}

type spotFleetRequestConfigSet struct {
	Items []spotFleetRequestConfigSetItem `xml:"item"`
}

type describeSpotFleetRequestsResponse struct {
	XMLName                   xml.Name                  `xml:"DescribeSpotFleetRequestsResponse"`
	Xmlns                     string                    `xml:"xmlns,attr"`
	RequestID                 string                    `xml:"requestId"`
	NextToken                 string                    `xml:"nextToken,omitempty"`
	SpotFleetRequestConfigSet spotFleetRequestConfigSet `xml:"spotFleetRequestConfigSet"`
}

type cancelSpotFleetSuccessItem struct {
	SpotFleetRequestID            string `xml:"spotFleetRequestId"`
	CurrentSpotFleetRequestState  string `xml:"currentSpotFleetRequestState"`
	PreviousSpotFleetRequestState string `xml:"previousSpotFleetRequestState"`
}

type cancelSpotFleetSuccessSet struct {
	Items []cancelSpotFleetSuccessItem `xml:"item"`
}

// cancelSpotFleetErrorDetail matches CancelSpotFleetRequestsError
// (ec2@v1.319.1 types/types.go, deserializers.go:81882): the real
// deserializer reads <error> as a nested <code>/<message> structure via
// awsEc2query_deserializeDocumentCancelSpotFleetRequestsError, not a scalar
// value -- a bare string here silently loses the error code, since the
// deserializer finds no child elements to decode.
type cancelSpotFleetErrorDetail struct {
	Code    string `xml:"code"`
	Message string `xml:"message,omitempty"`
}

type cancelSpotFleetErrorItem struct {
	SpotFleetRequestID string                     `xml:"spotFleetRequestId"`
	Error              cancelSpotFleetErrorDetail `xml:"error"`
}

type cancelSpotFleetErrorSet struct {
	Items []cancelSpotFleetErrorItem `xml:"item"`
}

type cancelSpotFleetRequestsResponse struct {
	XMLName                   xml.Name                  `xml:"CancelSpotFleetRequestsResponse"`
	Xmlns                     string                    `xml:"xmlns,attr"`
	RequestID                 string                    `xml:"requestId"`
	SuccessfulFleetRequests   cancelSpotFleetSuccessSet `xml:"successfulFleetRequestSet"`
	UnsuccessfulFleetRequests cancelSpotFleetErrorSet   `xml:"unsuccessfulFleetRequestSet"`
}

type modifySpotFleetRequestResponse struct {
	XMLName   xml.Name `xml:"ModifySpotFleetRequestResponse"`
	Xmlns     string   `xml:"xmlns,attr"`
	RequestID string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

type spotFleetInstanceItem struct {
	InstanceID            string `xml:"instanceId"`
	InstanceType          string `xml:"instanceType,omitempty"`
	SpotInstanceRequestID string `xml:"spotInstanceRequestId,omitempty"`
	InstanceHealth        string `xml:"instanceHealth,omitempty"`
}

type spotFleetInstanceSet struct {
	Items []spotFleetInstanceItem `xml:"item"`
}

type describeSpotFleetInstancesResponse struct {
	XMLName            xml.Name             `xml:"DescribeSpotFleetInstancesResponse"`
	Xmlns              string               `xml:"xmlns,attr"`
	RequestID          string               `xml:"requestId"`
	SpotFleetRequestID string               `xml:"spotFleetRequestId"`
	NextToken          string               `xml:"nextToken,omitempty"`
	ActiveInstances    spotFleetInstanceSet `xml:"activeInstanceSet"`
}

// spotFleetEventInformationItem matches EventInformation (ec2@v1.319.1
// deserializers.go:99294): the real deserializer reads <eventInformation> as
// a nested element wrapping <eventDescription>, not a scalar value.
type spotFleetEventInformationItem struct {
	EventDescription string `xml:"eventDescription,omitempty"`
}

type spotFleetHistoryRecordItem struct {
	Timestamp        string                        `xml:"timestamp"`
	EventType        string                        `xml:"eventType,omitempty"`
	EventInformation spotFleetEventInformationItem `xml:"eventInformation"`
}

type spotFleetHistoryRecordSet struct {
	Items []spotFleetHistoryRecordItem `xml:"item"`
}

type describeSpotFleetRequestHistoryResponse struct {
	XMLName            xml.Name                  `xml:"DescribeSpotFleetRequestHistoryResponse"`
	Xmlns              string                    `xml:"xmlns,attr"`
	RequestID          string                    `xml:"requestId"`
	SpotFleetRequestID string                    `xml:"spotFleetRequestId"`
	StartTime          string                    `xml:"startTime"`
	LastEvaluatedTime  string                    `xml:"lastEvaluatedTime,omitempty"`
	NextToken          string                    `xml:"nextToken,omitempty"`
	HistoryRecords     spotFleetHistoryRecordSet `xml:"historyRecordSet"`
}

type createSpotDatafeedResponse struct {
	XMLName                  xml.Name         `xml:"CreateSpotDatafeedSubscriptionResponse"`
	RequestID                string           `xml:"requestId"`
	SpotDatafeedSubscription spotDatafeedItem `xml:"spotDatafeedSubscription"`
}

type describeSpotDatafeedResponse struct {
	XMLName                  xml.Name         `xml:"DescribeSpotDatafeedSubscriptionResponse"`
	RequestID                string           `xml:"requestId"`
	SpotDatafeedSubscription spotDatafeedItem `xml:"spotDatafeedSubscription"`
}

type importImageTaskItem struct {
	ImportTaskID string `xml:"importTaskId"`
	Description  string `xml:"description"`
	Architecture string `xml:"architecture,omitempty"`
	Platform     string `xml:"platform,omitempty"`
	Status       string `xml:"status"`
	KmsKeyID     string `xml:"kmsKeyId,omitempty"`
	Encrypted    bool   `xml:"encrypted"`
}

func (h *Handler) handleCreateSpotDatafeedSubscription(vals url.Values, reqID string) (any, error) {
	bucket := vals.Get("Bucket")
	prefix := vals.Get("Prefix")

	datafeed, err := h.Backend.CreateSpotDatafeedSubscription(bucket, prefix)
	if err != nil {
		return nil, err
	}

	return &createSpotDatafeedResponse{
		RequestID: reqID,
		SpotDatafeedSubscription: spotDatafeedItem{
			Bucket: datafeed.Bucket,
			Prefix: datafeed.Prefix,
			State:  datafeed.State,
		},
	}, nil
}

func (h *Handler) handleDeleteSpotDatafeedSubscription(_ url.Values, reqID string) (any, error) {
	h.Backend.DeleteSpotDatafeedSubscription()

	return &stubResponse{
		XMLName:   xml.Name{Local: "DeleteSpotDatafeedSubscriptionResponse"},
		RequestID: reqID,
		Return:    true,
	}, nil
}

func (h *Handler) handleDescribeSpotDatafeedSubscription(_ url.Values, reqID string) (any, error) {
	datafeed := h.Backend.DescribeSpotDatafeedSubscription()
	resp := &describeSpotDatafeedResponse{RequestID: reqID}
	if datafeed != nil {
		resp.SpotDatafeedSubscription = spotDatafeedItem{
			Bucket: datafeed.Bucket,
			Prefix: datafeed.Prefix,
			State:  datafeed.State,
		}
	}

	return resp, nil
}

type getSpotPlacementScoresResponse struct {
	XMLName               xml.Name `xml:"GetSpotPlacementScoresResponse"`
	RequestID             string   `xml:"requestId"`
	NextToken             string   `xml:"nextToken,omitempty"`
	SpotPlacementScoreSet struct {
		Items []spotPlacementScoreItem `xml:"item"`
	} `xml:"spotPlacementScoreSet"`
}

func (h *Handler) handleGetSpotPlacementScores(vals url.Values, reqID string) (any, error) {
	instanceTypes := parseMemberList(vals, "InstanceType")
	regionNames := parseMemberList(vals, "RegionName")
	singleAZ := vals.Get("SingleAvailabilityZone") == ec2BooleanTrue

	scores, err := h.Backend.GetSpotPlacementScores(instanceTypes, regionNames, singleAZ)
	if err != nil {
		return nil, err
	}

	maxResults, offset, err := parseEC2Pagination(vals, ec2PageMinDefault, ec2PageMaxDefault, ec2PageMaxDefault)
	if err != nil {
		return nil, err
	}

	var nextToken string
	scores, nextToken = pageSlice(scores, offset, maxResults)

	resp := &getSpotPlacementScoresResponse{RequestID: reqID, NextToken: nextToken}
	for _, s := range scores {
		resp.SpotPlacementScoreSet.Items = append(resp.SpotPlacementScoreSet.Items, spotPlacementScoreItem{
			Region:             s.Region,
			AvailabilityZoneID: s.AvailabilityZoneID,
			Score:              s.Score,
		})
	}

	return resp, nil
}
