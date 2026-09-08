package managedblockchain

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	opCreateProposal      = "CreateProposal"
	opDeleteAccessor      = "DeleteAccessor"
	opDeleteMember        = "DeleteMember"
	opDeleteNode          = "DeleteNode"
	opGetAccessor         = "GetAccessor"
	opGetMember           = "GetMember"
	opGetNetwork          = "GetNetwork"
	opGetNode             = "GetNode"
	opGetProposal         = "GetProposal"
	opListAccessors       = "ListAccessors"
	opListInvitations     = "ListInvitations"
	opListMembers         = "ListMembers"
	opListNetworks        = "ListNetworks"
	opListNodes           = "ListNodes"
	opListProposalVotes   = "ListProposalVotes"
	opListProposals       = "ListProposals"
	opListTagsForResource = "ListTagsForResource"
	opRejectInvitation    = "RejectInvitation"
	opTagResource         = "TagResource"
	opUntagResource       = "UntagResource"
	opUpdateMember        = "UpdateMember"
	opUpdateNode          = "UpdateNode"
	opVoteOnProposal      = "VoteOnProposal"
)

const (
	managedblockchainService       = "managedblockchain"
	managedblockchainMatchPriority = 87

	opCreateAccessor = "CreateAccessor"
	opCreateMember   = "CreateMember"
	opCreateNetwork  = "CreateNetwork"
	opCreateNode     = "CreateNode"
)

// Handler is the HTTP handler for the Managed Blockchain REST API.
type Handler struct {
	Backend       StorageBackend
	AccountID     string
	DefaultRegion string
}

// NewHandler creates a new Managed Blockchain handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// Name returns the service name.
func (h *Handler) Name() string { return "ManagedBlockchain" }

// GetSupportedOperations returns the list of supported Managed Blockchain operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateAccessor,
		opCreateMember,
		opCreateNetwork,
		opCreateNode,
		opCreateProposal,
		opDeleteAccessor,
		opDeleteMember,
		opDeleteNode,
		opGetAccessor,
		opGetMember,
		opGetNetwork,
		opGetNode,
		opGetProposal,
		opListAccessors,
		opListInvitations,
		opListMembers,
		opListNetworks,
		opListNodes,
		opListProposalVotes,
		opListProposals,
		opListTagsForResource,
		opRejectInvitation,
		opTagResource,
		opUntagResource,
		opUpdateMember,
		opUpdateNode,
		opVoteOnProposal,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return managedblockchainService }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler handles.
func (h *Handler) ChaosRegions() []string { return []string{h.DefaultRegion} }

// RouteMatcher returns a function that matches Managed Blockchain REST API requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		if path == "/networks" || strings.HasPrefix(path, "/networks/") {
			return httputils.ExtractServiceFromRequest(c.Request()) == managedblockchainService
		}

		if strings.HasPrefix(path, "/tags/") {
			return httputils.ExtractServiceFromRequest(c.Request()) == managedblockchainService
		}

		if path == "/accessors" || strings.HasPrefix(path, "/accessors/") {
			return httputils.ExtractServiceFromRequest(c.Request()) == managedblockchainService
		}

		if path == "/invitations" || strings.HasPrefix(path, "/invitations/") {
			return httputils.ExtractServiceFromRequest(c.Request()) == managedblockchainService
		}

		return false
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return managedblockchainMatchPriority }

// ExtractOperation extracts the operation name from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	op, _ := parsePath(c.Request().Method, c.Request().URL.Path)

	return op
}

// ExtractResource extracts the resource ID from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	_, resource := parsePath(c.Request().Method, c.Request().URL.Path)

	return resource
}

// Handler returns the Echo handler function for Managed Blockchain requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		method := c.Request().Method
		path := c.Request().URL.Path

		op, resource := parsePath(method, path)
		if op == "" {
			return writeError(c, http.StatusNotFound, "ResourceNotFoundException", "resource not found")
		}

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "managedblockchain: failed to read request body", "error", err)

			return writeError(c,
				http.StatusInternalServerError,
				"InternalServiceErrorException",
				"failed to read request body",
			)
		}

		log.DebugContext(ctx, "managedblockchain request", "op", op, "resource", resource)

		return h.dispatch(c, op, resource, body, c.Request().URL.Query())
	}
}

const (
	// maxPathParts is the maximum number of segments to split when parsing paths.
	maxPathParts = 7

	// networkIDSegment is the index of the network ID in the path parts.
	networkIDSegment = 2
)

// parsePath maps a method+path to an (operation, resource) pair.
//
// Supported path shapes:
//
//	POST   /networks                                                               → CreateNetwork, ""
//	GET    /networks                                                               → ListNetworks, ""
//	GET    /networks/{networkId}                                                   → GetNetwork, networkId
//	POST   /networks/{networkId}/members                                           → CreateMember, networkId
//	GET    /networks/{networkId}/members                                           → ListMembers, networkId
//	GET    /networks/{networkId}/members/{memberId}                                → GetMember, networkId/memberId
//	DELETE /networks/{networkId}/members/{memberId}                                → DeleteMember, networkId/memberId
//	POST   /networks/{networkId}/nodes                                             → CreateNode, networkId
//	GET    /networks/{networkId}/nodes                                             → ListNodes, networkId
//	GET    /networks/{networkId}/nodes/{nodeId}                                    → GetNode, networkId/nodeId
//	DELETE /networks/{networkId}/nodes/{nodeId}                                    → DeleteNode, networkId/nodeId
//	PATCH  /networks/{networkId}/nodes/{nodeId}                                    → UpdateNode, networkId/nodeId
//
// The member that owns a node is NOT part of the node's path (unlike member
// itself, which nests under its network): CreateNode carries MemberId in its
// JSON body, and GetNode/ListNodes/DeleteNode/UpdateNode carry it as the
// "memberId" query parameter. This matches the real aws-sdk-go-v2
// managedblockchain wire shape exactly (see serializers.go's opPath
// constants for CreateNode/GetNode/ListNodes/DeleteNode/UpdateNode, all of
// which resolve to "/networks/{NetworkId}/nodes[/{NodeId}]" with MemberId
// bound via SetQuery("memberId") or the request body, never SetURI).
//
//	POST   /networks/{networkId}/proposals                                         → CreateProposal, networkId
//	GET    /networks/{networkId}/proposals                                         → ListProposals, networkId
//	GET    /networks/{networkId}/proposals/{proposalId}   → GetProposal, networkId/proposalId
//	GET    /networks/{networkId}/proposals/{proposalId}/votes → ListProposalVotes, networkId/proposalId
//	GET    /tags/{resourceArn}                                                     → ListTagsForResource, arn
//	POST   /tags/{resourceArn}                                                     → TagResource, arn
//	DELETE /tags/{resourceArn}                                                     → UntagResource, arn
//	POST   /accessors                                                              → CreateAccessor, ""
//	GET    /accessors                                                              → ListAccessors, ""
//	GET    /accessors/{accessorId}                                                 → GetAccessor, accessorId
//	DELETE /accessors/{accessorId}                                                 → DeleteAccessor, accessorId
//	GET    /invitations                                                            → ListInvitations, ""
//	DELETE /invitations/{invitationId}                                             → RejectInvitation, invitationId
func parsePath(method, path string) (string, string) {
	trimmed := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(trimmed, "/", maxPathParts)

	if len(parts) == 0 {
		return "", ""
	}

	base := parts[0]

	switch base {
	case "tags":
		if len(parts) < 2 || parts[1] == "" {
			return "", ""
		}

		arnEncoded := strings.Join(parts[1:], "/")

		switch method {
		case http.MethodGet:
			return opListTagsForResource, arnEncoded
		case http.MethodPost:
			return opTagResource, arnEncoded
		case http.MethodDelete:
			return opUntagResource, arnEncoded
		}

		return "", ""

	case "networks":
		return parseNetworksPath(method, parts)

	case "accessors":
		return parseAccessorsPath(method, parts)

	case "invitations":
		return parseInvitationsPath(method, parts)
	}

	return "", ""
}

// parseNetworksPath handles routing for /networks and /networks/{id}/... paths.
func parseNetworksPath(method string, parts []string) (string, string) {
	// /networks or /networks/
	if len(parts) == 1 || (len(parts) == networkIDSegment && parts[1] == "") {
		return parseRootNetworksMethod(method)
	}

	networkID := parts[1]

	// /networks/{networkId}
	if len(parts) == networkIDSegment {
		if method == http.MethodGet {
			return opGetNetwork, networkID
		}

		return "", ""
	}

	// /networks/{networkId}/members/...
	if parts[2] == "members" {
		return parseMembersPath(method, parts, networkID)
	}

	// /networks/{networkId}/proposals/...
	if parts[2] == "proposals" {
		return parseProposalsPath(method, parts, networkID)
	}

	// /networks/{networkId}/nodes/...
	if parts[2] == "nodes" {
		return parseNetworkNodesPath(method, parts, networkID)
	}

	return "", ""
}

// parseRootNetworksMethod returns the operation for POST/GET /networks.
func parseRootNetworksMethod(method string) (string, string) {
	switch method {
	case http.MethodPost:
		return opCreateNetwork, ""
	case http.MethodGet:
		return opListNetworks, ""
	}

	return "", ""
}

// parseMembersPath handles routing for /networks/{networkId}/members/... paths.
func parseMembersPath(method string, parts []string, networkID string) (string, string) {
	if len(parts) == 3 || (len(parts) == 4 && parts[3] == "") {
		switch method {
		case http.MethodPost:
			return opCreateMember, networkID
		case http.MethodGet:
			return opListMembers, networkID
		}

		return "", ""
	}

	// /networks/{networkId}/members/{memberId} -- exactly this depth (plus an
	// optional trailing slash). Anything deeper (e.g. a stray extra segment)
	// falls through to "unknown operation" rather than being silently
	// accepted as a member resource.
	if (len(parts) == 4 || (len(parts) == 5 && parts[4] == "")) && parts[3] != "" {
		resource := networkID + "/" + parts[3]

		switch method {
		case http.MethodGet:
			return opGetMember, resource
		case http.MethodDelete:
			return opDeleteMember, resource
		case http.MethodPatch:
			return opUpdateMember, resource
		}
	}

	return "", ""
}

// parseNetworkNodesPath handles routing for /networks/{networkId}/nodes/... paths.
// Unlike members, a node's path never carries its owning member's ID -- the
// real API binds MemberId via the "memberId" query parameter (Get/List/
// Delete/Update) or the JSON request body (Create), never the URI. See
// parsePath's doc comment for the wire-shape citation.
func parseNetworkNodesPath(method string, parts []string, networkID string) (string, string) {
	// /networks/{networkId}/nodes
	if len(parts) == 3 || (len(parts) == 4 && parts[3] == "") {
		switch method {
		case http.MethodPost:
			return opCreateNode, networkID
		case http.MethodGet:
			return opListNodes, networkID
		}

		return "", ""
	}

	// /networks/{networkId}/nodes/{nodeId}
	if len(parts) >= 4 && parts[3] != "" {
		nodeID := parts[3]
		resource := networkID + "/" + nodeID

		switch method {
		case http.MethodGet:
			return opGetNode, resource
		case http.MethodDelete:
			return opDeleteNode, resource
		case http.MethodPatch:
			return opUpdateNode, resource
		}
	}

	return "", ""
}

// parseProposalsPath handles routing for /networks/{networkId}/proposals/... paths.
func parseProposalsPath(method string, parts []string, networkID string) (string, string) {
	// /networks/{networkId}/proposals  or  /networks/{networkId}/proposals/
	if len(parts) == 3 || (len(parts) == 4 && parts[3] == "") {
		switch method {
		case http.MethodPost:
			return opCreateProposal, networkID
		case http.MethodGet:
			return opListProposals, networkID
		}

		return "", ""
	}

	// /networks/{networkId}/proposals/{proposalId}[/votes]
	if len(parts) >= 4 && parts[3] != "" {
		return parseProposalIDPath(method, parts, networkID)
	}

	return "", ""
}

// parseProposalIDPath handles routing for /networks/{networkId}/proposals/{proposalId}[/votes].
func parseProposalIDPath(method string, parts []string, networkID string) (string, string) {
	proposalID := parts[3]
	resource := networkID + "/" + proposalID

	// /networks/{networkId}/proposals/{proposalId}/votes
	if len(parts) >= 5 && parts[4] == "votes" {
		switch method {
		case http.MethodGet:
			return opListProposalVotes, resource
		case http.MethodPost:
			return opVoteOnProposal, resource
		}
	}

	// /networks/{networkId}/proposals/{proposalId}
	if len(parts) == 4 || (len(parts) == 5 && parts[4] == "") {
		if method == http.MethodGet {
			return opGetProposal, resource
		}
	}

	return "", ""
}

// parseAccessorsPath handles routing for /accessors and /accessors/{id} paths.
func parseAccessorsPath(method string, parts []string) (string, string) {
	// /accessors  or  /accessors/
	if len(parts) == 1 || (len(parts) == 2 && parts[1] == "") {
		switch method {
		case http.MethodPost:
			return opCreateAccessor, ""
		case http.MethodGet:
			return opListAccessors, ""
		}

		return "", ""
	}

	// /accessors/{accessorId}
	if len(parts) >= 2 && parts[1] != "" {
		accessorID := parts[1]

		switch method {
		case http.MethodGet:
			return opGetAccessor, accessorID
		case http.MethodDelete:
			return opDeleteAccessor, accessorID
		}
	}

	return "", ""
}

// parseInvitationsPath handles routing for /invitations and /invitations/{id} paths.
func parseInvitationsPath(method string, parts []string) (string, string) {
	// /invitations  or  /invitations/
	if len(parts) == 1 || (len(parts) == 2 && parts[1] == "") {
		if method == http.MethodGet {
			return opListInvitations, ""
		}

		return "", ""
	}

	// /invitations/{invitationId}
	if len(parts) >= 2 && parts[1] != "" {
		invitationID := parts[1]

		if method == http.MethodDelete {
			return opRejectInvitation, invitationID
		}
	}

	return "", ""
}

// dispatch routes to the appropriate handler based on the operation name.
func (h *Handler) dispatch(c *echo.Context, op, resource string, body []byte, query url.Values) error {
	if err := h.dispatchNetworkOps(c, op, resource, body, query); !errors.Is(err, errUnknownOp) {
		return err
	}

	if err := h.dispatchAccessorOps(c, op, resource, body); !errors.Is(err, errUnknownOp) {
		return err
	}

	if err := h.dispatchProposalOps(c, op, resource, body); !errors.Is(err, errUnknownOp) {
		return err
	}

	if err := h.dispatchInvitationOps(c, op, resource); !errors.Is(err, errUnknownOp) {
		return err
	}

	return writeError(c, http.StatusNotFound, "ResourceNotFoundException", "unknown operation")
}

// errUnknownOp is a sentinel returned by sub-dispatch helpers when the operation is not handled.
var errUnknownOp = errors.New("unknown operation")

// dispatchNetworkOps handles network, member, node, and tag operations.
func (h *Handler) dispatchNetworkOps(
	c *echo.Context, op, resource string, body []byte, query url.Values,
) error {
	switch op {
	case opCreateNetwork:
		return h.handleCreateNetwork(c, body)
	case opGetNetwork:
		return h.handleGetNetwork(c, resource)
	case opListNetworks:
		return h.handleListNetworks(c)
	case opListTagsForResource:
		return h.handleListTagsForResource(c, resource)
	case opTagResource:
		return h.handleTagResource(c, resource, body)
	case opUntagResource:
		return h.handleUntagResource(c, resource, query)
	}

	if err := h.dispatchMemberNodeOps(c, op, resource, body); !errors.Is(err, errUnknownOp) {
		return err
	}

	return errUnknownOp
}

// dispatchMemberNodeOps handles member and node operations.
func (h *Handler) dispatchMemberNodeOps(c *echo.Context, op, resource string, body []byte) error {
	switch op {
	case opCreateMember:
		return h.handleCreateMember(c, resource, body)
	case opGetMember:
		return h.handleGetMember(c, resource)
	case opListMembers:
		return h.handleListMembers(c, resource)
	case opDeleteMember:
		return h.handleDeleteMember(c, resource)
	case opUpdateMember:
		return h.handleUpdateMember(c, resource, body)
	case opCreateNode:
		return h.handleCreateNode(c, resource, body)
	case opGetNode:
		return h.handleGetNode(c, resource)
	case opListNodes:
		return h.handleListNodes(c, resource)
	case opDeleteNode:
		return h.handleDeleteNode(c, resource)
	case opUpdateNode:
		return h.handleUpdateNode(c, resource, body)
	}

	return errUnknownOp
}

// dispatchAccessorOps handles accessor operations.
func (h *Handler) dispatchAccessorOps(c *echo.Context, op, resource string, body []byte) error {
	switch op {
	case opCreateAccessor:
		return h.handleCreateAccessor(c, body)
	case opGetAccessor:
		return h.handleGetAccessor(c, resource)
	case opDeleteAccessor:
		return h.handleDeleteAccessor(c, resource)
	case opListAccessors:
		return h.handleListAccessors(c)
	}

	return errUnknownOp
}

// dispatchProposalOps handles proposal operations.
func (h *Handler) dispatchProposalOps(c *echo.Context, op, resource string, body []byte) error {
	switch op {
	case opCreateProposal:
		return h.handleCreateProposal(c, resource, body)
	case opGetProposal:
		return h.handleGetProposal(c, resource)
	case opListProposals:
		return h.handleListProposals(c, resource)
	case opListProposalVotes:
		return h.handleListProposalVotes(c, resource)
	case opVoteOnProposal:
		return h.handleVoteOnProposal(c, resource, body)
	}

	return errUnknownOp
}

// dispatchInvitationOps handles invitation operations.
func (h *Handler) dispatchInvitationOps(c *echo.Context, op, resource string) error {
	switch op {
	case opListInvitations:
		return h.handleListInvitations(c)
	case opRejectInvitation:
		return h.handleRejectInvitation(c, resource)
	}

	return errUnknownOp
}

// writeBackendError translates a backend error to an HTTP response.
func (h *Handler) writeBackendError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, awserr.ErrNotFound):
		return writeError(c, http.StatusNotFound, "ResourceNotFoundException", err.Error())
	case errors.Is(err, awserr.ErrAlreadyExists):
		return writeError(c, http.StatusConflict, "ResourceAlreadyExistsException", err.Error())
	case errors.Is(err, ErrTooManyTags):
		return writeError(c, http.StatusBadRequest, "TooManyTagsException", err.Error())
	case errors.Is(err, awserr.ErrInvalidParameter):
		return writeError(c, http.StatusBadRequest, "InvalidRequestException", err.Error())
	default:
		return writeError(c, http.StatusInternalServerError, "InternalServiceErrorException", err.Error())
	}
}

// writeError writes a JSON error response.
func writeError(c *echo.Context, status int, code, message string) error {
	return c.JSON(status, errorResponse{Message: message, Code: code})
}

// splitResource splits a "networkId/memberId" resource string into its parts.
func splitResource(resource string) (string, string, bool) {
	idx := strings.Index(resource, "/")
	if idx <= 0 || idx == len(resource)-1 {
		return "", "", false
	}

	return resource[:idx], resource[idx+1:], true
}
