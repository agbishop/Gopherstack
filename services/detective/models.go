package detective

import (
	"maps"
	"time"
)

const (
	memberStatusInvited          = "INVITED"
	memberStatusEnabled          = "ENABLED"
	memberStatusAcceptedDisabled = "ACCEPTED_BUT_DISABLED"

	investigationStateActive     = "ACTIVE"
	investigationStateArchived   = "ARCHIVED"
	investigationStatusRunning   = "RUNNING"
	investigationStatusFailed    = "FAILED"
	investigationStatusSucceeded = "SUCCESSFUL"

	datasourceIngestStateStarted  = "STARTED"
	datasourceIngestStateStopped  = "STOPPED"
	datasourceIngestStateDisabled = "DISABLED"

	entityTypeIAMRole = "IAM_ROLE"
	entityTypeIAMUser = "IAM_USER"

	// invitationTypeInvitation is the only InvitationType this emulator ever
	// produces: every member is created through the CreateMembers invite flow.
	// InvitationTypeOrganization ("ORGANIZATION") would require modeling
	// AWS Organizations account-join events driving auto-enablement, which
	// this single-account emulator does not simulate.
	invitationTypeInvitation = "INVITATION"

	// severityInformational is the only Severity value StartInvestigation ever
	// assigns (gopherstack-b6wo): real Detective derives Severity from the
	// likelihood/impact of indicators found during threat-intelligence
	// analysis this emulator does not perform, so LOW/MEDIUM/HIGH/CRITICAL
	// are unreachable and were removed rather than kept as unused consts.
	severityInformational = "INFORMATIONAL"

	maxGraphsPerPage         = 200
	maxMembersPerPage        = 200
	maxInvestigationsPerPage = 200
	maxIndicatorsPerPage     = 100
	maxInvitationsPerPage    = 200
	maxOrgAdminsPerPage      = 200
	maxDatasourcesPerPage    = 200

	maxTagCount              = 50
	maxTagKeyLen             = 128
	maxTagValueLen           = 256
	maxCreateMembersPerBatch = 50
	accountIDLen             = 12

	// iamARNPartsLen is the number of ":"-separated segments in a well-formed
	// IAM entity ARN: "arn", partition, "iam", region (empty), account, resource.
	iamARNPartsLen = 6

	// reasonMemberNotFoundInGraph is the UnprocessedAccount reason used by
	// DeleteMembers, GetMembers, and BatchGetGraphMemberDatasources when an
	// account ID has no member record in the graph.
	reasonMemberNotFoundInGraph = "Member account not found in behavior graph"
)

// storedGraph holds a behavior graph with all fields.
// CreatedTime is first: time.Time's non-pointer prefix (wall, ext) reduces GC pointer bytes.
type storedGraph struct {
	CreatedTime time.Time         `json:"createdTime"`
	Tags        map[string]string `json:"tags"`
	Arn         string            `json:"arn"`
}

func (g *storedGraph) toGraph() Graph {
	return Graph{
		Arn:         g.Arn,
		CreatedTime: g.CreatedTime,
		Tags:        g.Tags,
	}
}

// storedMember holds a member with all fields.
// time.Time fields are first: their non-pointer prefix (wall, ext) reduces GC pointer bytes.
type storedMember struct {
	InvitedTime     time.Time `json:"invitedTime"`
	UpdatedTime     time.Time `json:"updatedTime"`
	AccountID       string    `json:"accountId"`
	AdministratorID string    `json:"administratorId"`
	EmailAddress    string    `json:"emailAddress"`
	GraphARN        string    `json:"graphArn"`
	Status          string    `json:"status"`
}

// toMemberDetail builds the wire-facing MemberDetail. datasourceStates is the
// graph's per-package ingest state map (may be nil); it is copied so callers
// never share backend map storage with the returned value.
func (m *storedMember) toMemberDetail(datasourceStates map[string]string) MemberDetail {
	states := make(map[string]string, len(datasourceStates))
	maps.Copy(states, datasourceStates)

	return MemberDetail{
		AccountID:                     m.AccountID,
		AdministratorID:               m.AdministratorID,
		DatasourcePackageIngestStates: states,
		EmailAddress:                  m.EmailAddress,
		GraphARN:                      m.GraphARN,
		InvitationType:                invitationTypeInvitation,
		InvitedTime:                   m.InvitedTime,
		Status:                        m.Status,
		UpdatedTime:                   m.UpdatedTime,
	}
}

// storedInvestigation holds investigation state.
// time.Time fields are first so their non-pointer prefix reduces GC pointer bytes.
type storedInvestigation struct {
	CreatedTime     time.Time `json:"createdTime"`
	ScopeStartTime  time.Time `json:"scopeStartTime"`
	ScopeEndTime    time.Time `json:"scopeEndTime"`
	GraphARN        string    `json:"graphArn"`
	InvestigationID string    `json:"investigationId"`
	EntityARN       string    `json:"entityArn"`
	EntityType      string    `json:"entityType"`
	Severity        string    `json:"severity"`
	State           string    `json:"state"`
	Status          string    `json:"status"`
}

func (i *storedInvestigation) toInvestigation() Investigation {
	return Investigation{
		CreatedTime:     i.CreatedTime,
		ScopeStartTime:  i.ScopeStartTime,
		ScopeEndTime:    i.ScopeEndTime,
		GraphARN:        i.GraphARN,
		InvestigationID: i.InvestigationID,
		EntityARN:       i.EntityARN,
		EntityType:      i.EntityType,
		Severity:        i.Severity,
		State:           i.State,
		Status:          i.Status,
	}
}

func (i *storedInvestigation) toDetail() InvestigationDetail {
	return InvestigationDetail{
		CreatedTime:     i.CreatedTime,
		EntityARN:       i.EntityARN,
		EntityType:      i.EntityType,
		InvestigationID: i.InvestigationID,
		Severity:        i.Severity,
		State:           i.State,
		Status:          i.Status,
	}
}

// storedOrgAdmin holds an organization administrator record.
// DelegationTime is first so its non-pointer prefix reduces GC pointer bytes.
type storedOrgAdmin struct {
	DelegationTime time.Time `json:"delegationTime"`
	AccountID      string    `json:"accountId"`
	GraphARN       string    `json:"graphArn"`
}
