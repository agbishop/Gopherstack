package redshift

import (
	"encoding/xml"
	"net/url"
	"strconv"
	"time"
)

// ----- Scheduled Actions -----

// xmlPauseClusterAction/xmlResumeClusterAction/xmlResizeClusterAction mirror
// types.PauseClusterMessage/ResumeClusterMessage/ResizeClusterMessage. Wire element
// names and the "TargetAction.PauseCluster.*" / "TargetAction.ResizeCluster.*" /
// "TargetAction.ResumeCluster.*" request param nesting are confirmed against
// aws-sdk-go-v2/service/redshift@v1.62.3 serializers.go
// (awsAwsquery_serializeDocumentScheduledActionType) and deserializers.go
// (awsAwsquery_deserializeDocumentScheduledActionType).
type xmlPauseClusterAction struct {
	ClusterIdentifier string `xml:"ClusterIdentifier"`
}

type xmlResumeClusterAction struct {
	ClusterIdentifier string `xml:"ClusterIdentifier"`
}

type xmlResizeClusterAction struct {
	ClusterIdentifier            string `xml:"ClusterIdentifier"`
	ClusterType                  string `xml:"ClusterType,omitempty"`
	NodeType                     string `xml:"NodeType,omitempty"`
	ReservedNodeID               string `xml:"ReservedNodeId,omitempty"`
	TargetReservedNodeOfferingID string `xml:"TargetReservedNodeOfferingId,omitempty"`
	NumberOfNodes                int    `xml:"NumberOfNodes,omitempty"`
	Classic                      bool   `xml:"Classic,omitempty"`
}

type xmlScheduledActionTarget struct {
	PauseCluster  *xmlPauseClusterAction  `xml:"PauseCluster,omitempty"`
	ResumeCluster *xmlResumeClusterAction `xml:"ResumeCluster,omitempty"`
	ResizeCluster *xmlResizeClusterAction `xml:"ResizeCluster,omitempty"`
}

type scheduledActionXML struct {
	TargetAction               *xmlScheduledActionTarget `xml:"TargetAction,omitempty"`
	ScheduledActionName        string                    `xml:"ScheduledActionName"`
	Schedule                   string                    `xml:"Schedule"`
	IamRole                    string                    `xml:"IamRole,omitempty"`
	ScheduledActionDescription string                    `xml:"ScheduledActionDescription,omitempty"`
	State                      string                    `xml:"State"`
	NextInvocations            []string                  `xml:"NextInvocations>ScheduledActionTime,omitempty"`
}

type createScheduledActionResponse struct {
	XMLName xml.Name           `xml:"CreateScheduledActionResponse"`
	Xmlns   string             `xml:"xmlns,attr"`
	Result  scheduledActionXML `xml:"CreateScheduledActionResult"`
}

func targetActionToXML(t *ScheduledActionTarget) *xmlScheduledActionTarget {
	if t == nil {
		return nil
	}

	x := &xmlScheduledActionTarget{}

	if t.PauseCluster != nil {
		x.PauseCluster = &xmlPauseClusterAction{ClusterIdentifier: t.PauseCluster.ClusterIdentifier}
	}

	if t.ResumeCluster != nil {
		x.ResumeCluster = &xmlResumeClusterAction{ClusterIdentifier: t.ResumeCluster.ClusterIdentifier}
	}

	if t.ResizeCluster != nil {
		r := t.ResizeCluster
		x.ResizeCluster = &xmlResizeClusterAction{
			ClusterIdentifier:            r.ClusterIdentifier,
			ClusterType:                  r.ClusterType,
			NodeType:                     r.NodeType,
			ReservedNodeID:               r.ReservedNodeID,
			TargetReservedNodeOfferingID: r.TargetReservedNodeOfferingID,
			NumberOfNodes:                r.NumberOfNodes,
			Classic:                      r.Classic,
		}
	}

	return x
}

func scheduledActionToXML(a *ScheduledAction) scheduledActionXML {
	return scheduledActionXML{
		ScheduledActionName:        a.ScheduledActionName,
		Schedule:                   a.Schedule,
		IamRole:                    a.IamRole,
		ScheduledActionDescription: a.ScheduledActionDescription,
		State:                      a.State,
		TargetAction:               targetActionToXML(a.TargetAction),
		NextInvocations:            nextInvocationsXML(a.Schedule),
	}
}

// nextInvocationsXML formats nextInvocations' computed times the same way this
// service formats every other query-XML timestamp (RFC3339, see PARITY.md Notes).
func nextInvocationsXML(schedule string) []string {
	times := nextInvocations(schedule, time.Now().UTC())
	if len(times) == 0 {
		return nil
	}

	out := make([]string, 0, len(times))
	for _, t := range times {
		out = append(out, t.Format(time.RFC3339))
	}

	return out
}

// Values of types.ScheduledActionTypeValues, the DescribeScheduledActionsInput.
// TargetActionType filter.
const (
	scheduledActionTypeResizeCluster = "ResizeCluster"
	scheduledActionTypePauseCluster  = "PauseCluster"
	scheduledActionTypeResumeCluster = "ResumeCluster"
)

// scheduledActionTargetType returns the DescribeScheduledActionsInput.TargetActionType
// value matching which sub-action of a is set. Returns "" for nil or an unset target.
func scheduledActionTargetType(a *ScheduledActionTarget) string {
	switch {
	case a == nil:
		return ""
	case a.ResizeCluster != nil:
		return scheduledActionTypeResizeCluster
	case a.PauseCluster != nil:
		return scheduledActionTypePauseCluster
	case a.ResumeCluster != nil:
		return scheduledActionTypeResumeCluster
	default:
		return ""
	}
}

// parseTargetAction parses the TargetAction.{PauseCluster,ResumeCluster,ResizeCluster}
// nested request parameters into a ScheduledActionTarget. Returns nil if none of the
// three sub-actions were present (a real client always sends exactly one, but this
// mirrors the SDK's "all fields optional" struct rather than enforcing that here).
func parseTargetAction(vals url.Values) *ScheduledActionTarget {
	var target ScheduledActionTarget

	present := false

	if id := vals.Get("TargetAction.PauseCluster.ClusterIdentifier"); id != "" {
		target.PauseCluster = &PauseClusterAction{ClusterIdentifier: id}
		present = true
	}

	if id := vals.Get("TargetAction.ResumeCluster.ClusterIdentifier"); id != "" {
		target.ResumeCluster = &ResumeClusterAction{ClusterIdentifier: id}
		present = true
	}

	if id := vals.Get("TargetAction.ResizeCluster.ClusterIdentifier"); id != "" {
		numberOfNodes, _ := strconv.Atoi(vals.Get("TargetAction.ResizeCluster.NumberOfNodes"))
		target.ResizeCluster = &ResizeClusterAction{
			ClusterIdentifier:            id,
			ClusterType:                  vals.Get("TargetAction.ResizeCluster.ClusterType"),
			NodeType:                     vals.Get("TargetAction.ResizeCluster.NodeType"),
			NumberOfNodes:                numberOfNodes,
			Classic:                      vals.Get("TargetAction.ResizeCluster.Classic") == paramValueTrue,
			ReservedNodeID:               vals.Get("TargetAction.ResizeCluster.ReservedNodeId"),
			TargetReservedNodeOfferingID: vals.Get("TargetAction.ResizeCluster.TargetReservedNodeOfferingId"),
		}
		present = true
	}

	if !present {
		return nil
	}

	return &target
}

// parseEnable parses the optional Enable request parameter into a tri-state *bool,
// matching CreateScheduledActionInput/ModifyScheduledActionInput's *bool Enable
// field (nil means "not specified" as distinct from explicit false).
func parseEnable(vals url.Values) *bool {
	v := vals.Get("Enable")
	if v == "" {
		return nil
	}

	b := v == paramValueTrue

	return &b
}

func (h *Handler) handleCreateScheduledAction(vals url.Values) (any, error) {
	action, err := h.Backend.CreateScheduledAction(
		vals.Get("ScheduledActionName"),
		vals.Get("Schedule"),
		vals.Get("IamRole"),
		vals.Get("ScheduledActionDescription"),
		parseTargetAction(vals),
		parseEnable(vals),
	)
	if err != nil {
		return nil, err
	}

	return &createScheduledActionResponse{
		Xmlns:  redshiftXMLNS,
		Result: scheduledActionToXML(action),
	}, nil
}

type deleteScheduledActionResponse struct {
	XMLName xml.Name `xml:"DeleteScheduledActionResponse"`
	Xmlns   string   `xml:"xmlns,attr"`
}

func (h *Handler) handleDeleteScheduledAction(vals url.Values) (any, error) {
	name := vals.Get("ScheduledActionName")
	if err := h.Backend.DeleteScheduledAction(name); err != nil {
		return nil, err
	}

	return &deleteScheduledActionResponse{Xmlns: redshiftXMLNS}, nil
}

type describeScheduledActionsResponse struct {
	XMLName xml.Name `xml:"DescribeScheduledActionsResponse"`
	Xmlns   string   `xml:"xmlns,attr"`
	Result  struct {
		ScheduledActions []scheduledActionXML `xml:"ScheduledActions>ScheduledAction"`
	} `xml:"DescribeScheduledActionsResult"`
}

func (h *Handler) handleDescribeScheduledActions(vals url.Values) (any, error) {
	name := vals.Get("ScheduledActionName")
	actions, err := h.Backend.DescribeScheduledActions(name)
	if err != nil {
		return nil, err
	}

	var active *bool
	if v := vals.Get("Active"); v != "" {
		b := v == paramValueTrue
		active = &b
	}

	targetActionType := vals.Get("TargetActionType")

	members := make([]scheduledActionXML, 0, len(actions))

	for i := range actions {
		if active != nil && (actions[i].State == scheduledActionStateActiveValue) != *active {
			continue
		}

		if targetActionType != "" && scheduledActionTargetType(actions[i].TargetAction) != targetActionType {
			continue
		}

		members = append(members, scheduledActionToXML(&actions[i]))
	}

	resp := &describeScheduledActionsResponse{Xmlns: redshiftXMLNS}
	resp.Result.ScheduledActions = members

	return resp, nil
}

type modifyScheduledActionResponse struct {
	XMLName xml.Name           `xml:"ModifyScheduledActionResponse"`
	Xmlns   string             `xml:"xmlns,attr"`
	Result  scheduledActionXML `xml:"ModifyScheduledActionResult"`
}

func (h *Handler) handleModifyScheduledAction(vals url.Values) (any, error) {
	action, err := h.Backend.ModifyScheduledAction(
		vals.Get("ScheduledActionName"),
		vals.Get("Schedule"),
		vals.Get("IamRole"),
		vals.Get("ScheduledActionDescription"),
		parseTargetAction(vals),
		parseEnable(vals),
	)
	if err != nil {
		return nil, err
	}

	return &modifyScheduledActionResponse{
		Xmlns:  redshiftXMLNS,
		Result: scheduledActionToXML(action),
	}, nil
}
