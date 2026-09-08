package appmesh

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrMeshNotFound is returned when a mesh does not exist.
	ErrMeshNotFound = awserr.New("mesh not found", awserr.ErrNotFound)
	// ErrMeshAlreadyExists is returned when a mesh already exists.
	ErrMeshAlreadyExists = awserr.New("mesh already exists", awserr.ErrAlreadyExists)
	// ErrMeshInUse is returned when a mesh has resources and cannot be deleted.
	ErrMeshInUse = awserr.New("mesh is in use", awserr.ErrConflict)
	// ErrVirtualNodeNotFound is returned when a virtual node does not exist.
	ErrVirtualNodeNotFound = awserr.New("virtual node not found", awserr.ErrNotFound)
	// ErrVirtualNodeAlreadyExists is returned when a virtual node already exists.
	ErrVirtualNodeAlreadyExists = awserr.New("virtual node already exists", awserr.ErrAlreadyExists)
	// ErrVirtualNodeInUse is returned when a virtual service still lists the
	// virtual node as its provider.
	ErrVirtualNodeInUse = awserr.New("virtual node is referenced by a virtual service", awserr.ErrConflict)
	// ErrVirtualRouterNotFound is returned when a virtual router does not exist.
	ErrVirtualRouterNotFound = awserr.New("virtual router not found", awserr.ErrNotFound)
	// ErrVirtualRouterAlreadyExists is returned when a virtual router already exists.
	ErrVirtualRouterAlreadyExists = awserr.New("virtual router already exists", awserr.ErrAlreadyExists)
	// ErrVirtualRouterInUse is returned when a virtual router has routes.
	ErrVirtualRouterInUse = awserr.New("virtual router has routes", awserr.ErrConflict)
	// ErrRouteNotFound is returned when a route does not exist.
	ErrRouteNotFound = awserr.New("route not found", awserr.ErrNotFound)
	// ErrRouteAlreadyExists is returned when a route already exists.
	ErrRouteAlreadyExists = awserr.New("route already exists", awserr.ErrAlreadyExists)
	// ErrVirtualServiceNotFound is returned when a virtual service does not exist.
	ErrVirtualServiceNotFound = awserr.New("virtual service not found", awserr.ErrNotFound)
	// ErrVirtualServiceAlreadyExists is returned when a virtual service already exists.
	ErrVirtualServiceAlreadyExists = awserr.New("virtual service already exists", awserr.ErrAlreadyExists)
	// ErrVirtualGatewayNotFound is returned when a virtual gateway does not exist.
	ErrVirtualGatewayNotFound = awserr.New("virtual gateway not found", awserr.ErrNotFound)
	// ErrVirtualGatewayAlreadyExists is returned when a virtual gateway already exists.
	ErrVirtualGatewayAlreadyExists = awserr.New("virtual gateway already exists", awserr.ErrAlreadyExists)
	// ErrVirtualGatewayInUse is returned when a virtual gateway has gateway routes.
	ErrVirtualGatewayInUse = awserr.New("virtual gateway has gateway routes", awserr.ErrConflict)
	// ErrGatewayRouteNotFound is returned when a gateway route does not exist.
	ErrGatewayRouteNotFound = awserr.New("gateway route not found", awserr.ErrNotFound)
	// ErrGatewayRouteAlreadyExists is returned when a gateway route already exists.
	ErrGatewayRouteAlreadyExists = awserr.New("gateway route already exists", awserr.ErrAlreadyExists)
	// ErrResourceNotFound is returned when a tagged resource does not exist.
	ErrResourceNotFound = awserr.New("resource not found for tagging", awserr.ErrNotFound)
	// ErrTooManyTags is returned when tagging a resource would exceed the
	// real App Mesh API's 50-tag-per-resource limit (see TagList's "max: 50"
	// constraint in the botocore service-2.json model). Deliberately does not
	// wrap awserr.ErrInvalidParameter: real clients distinguish the
	// TooManyTagsException wire code from the generic BadRequestException one,
	// so mapErr must be able to select it independently.
	ErrTooManyTags = errors.New("resource may have at most 50 tags")
	// ErrMeshOwnerMismatch is returned when a request's meshOwner query
	// parameter names an account other than this backend's own. gopherstack
	// has no AWS RAM cross-account mesh-sharing model, so no mesh can ever
	// really be shared from another account; a differing meshOwner can only
	// name a mesh this account has no access to. Not awserr.ErrNotFound: real
	// App Mesh declares ForbiddenException (not NotFoundException) on every
	// meshOwner-carrying op for this case.
	ErrMeshOwnerMismatch = errors.New("account is not the mesh owner and no mesh is shared with it")
)
