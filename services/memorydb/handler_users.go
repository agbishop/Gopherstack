package memorydb

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"slices"

	"github.com/labstack/echo/v5"
)

func (h *Handler) handleCreateUser(ctx context.Context, c *echo.Context, body []byte) error {
	var req createUserRequest

	if err := json.Unmarshal(body, &req); err != nil {
		return writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	if req.UserName == "" {
		return writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "UserName is required")
	}

	if err := validateTagEntries(req.Tags); err != nil {
		return writeError(c, http.StatusBadRequest, "InvalidParameterValueException", err.Error())
	}

	user, err := h.Backend.CreateUser(ctx, &req)
	if err != nil {
		return h.writeBackendError(c, err)
	}

	return c.JSON(http.StatusOK, createUserResponse{User: toUserObject(user, []string{})})
}

func (h *Handler) handleDescribeUsers(ctx context.Context, c *echo.Context, body []byte) error {
	var req describeUserRequest

	if err := json.Unmarshal(body, &req); err != nil {
		return writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	users, err := h.Backend.DescribeUsers(ctx, req.UserName)
	if err != nil {
		return h.writeBackendError(c, err)
	}

	users, nextToken := paginateItems(users, req.NextToken, req.MaxResults, func(u *User) string { return u.Name })

	allACLs, _ := h.Backend.DescribeACLs(ctx, "")

	objs := make([]userObject, 0, len(users))

	for _, u := range users {
		names := aclNamesForUser(allACLs, u.Name)
		objs = append(objs, toUserObject(u, names))
	}

	return c.JSON(http.StatusOK, describeUserResponse{Users: objs, NextToken: nextToken})
}

func (h *Handler) handleDeleteUser(ctx context.Context, c *echo.Context, body []byte) error {
	var req deleteUserRequest

	if err := json.Unmarshal(body, &req); err != nil {
		return writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	if req.UserName == "" {
		return writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "UserName is required")
	}

	user, err := h.Backend.DeleteUser(ctx, req.UserName)
	if err != nil {
		return h.writeBackendError(c, err)
	}

	return c.JSON(http.StatusOK, deleteUserResponse{User: toUserObject(user, []string{})})
}

func (h *Handler) handleUpdateUser(ctx context.Context, c *echo.Context, body []byte) error {
	var req updateUserRequest

	if err := json.Unmarshal(body, &req); err != nil {
		return writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	if req.UserName == "" {
		return writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "UserName is required")
	}

	user, err := h.Backend.UpdateUser(ctx, &req)
	if err != nil {
		return h.writeBackendError(c, err)
	}

	return c.JSON(http.StatusOK, updateUserResponse{User: toUserObject(user, []string{})})
}

// -- ParameterGroup handlers -----------------------------------------------------

// toUserObject converts a User to its JSON representation. userObject has no
// "Engine" field: confirmed absent from the real SDK's User type
// (deserializers.go's awsAwsjson11_deserializeDocumentUser only recognizes
// AccessString, ACLNames, ARN, Authentication, MinimumEngineVersion, Name,
// Status) -- a prior pass fabricated one.
func toUserObject(u *User, aclNames []string) userObject {
	auth := &authenticationObject{Type: u.AuthType}
	if u.AuthType == "password" && len(u.Passwords) > 0 {
		count := min(len(u.Passwords), math.MaxInt32)
		auth.PasswordCount = int32(count)
	}

	names := aclNames
	if names == nil {
		names = []string{}
	}

	return userObject{
		Name:                 u.Name,
		ARN:                  u.ARN,
		AccessString:         u.AccessString,
		Status:               u.Status,
		Authentication:       auth,
		MinimumEngineVersion: engineVersion62,
		ACLNames:             names,
	}
}

// aclNamesForUser returns the names of all ACLs that contain userName.
func aclNamesForUser(acls []*ACL, userName string) []string {
	names := []string{}

	for _, a := range acls {
		if slices.Contains(a.UserNames, userName) {
			names = append(names, a.Name)
		}
	}

	return names
}
