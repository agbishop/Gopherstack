package quicksight

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
)

func isUserOp(op string) bool {
	switch op {
	case opRegisterUser, opDescribeUser, opUpdateUser, opDeleteUser,
		opDeleteUserByPrincipalID, opListUsers, opListUserGroups:
		return true
	}

	return false
}

func (h *Handler) dispatchUser(c *echo.Context, op string) error {
	switch op {
	case opRegisterUser:
		return h.handleRegisterUser(c)
	case opDescribeUser:
		return h.handleDescribeUser(c)
	case opUpdateUser:
		return h.handleUpdateUser(c)
	case opDeleteUser:
		return h.handleDeleteUser(c)
	case opDeleteUserByPrincipalID:
		return h.handleDeleteUserByPrincipalID(c)
	case opListUsers:
		return h.handleListUsers(c)
	case opListUserGroups:
		return h.handleListUserGroups(c)
	}

	return writeError(
		c,
		http.StatusNotImplemented,
		"UnsupportedOperationException",
		fmt.Sprintf("operation %q not implemented", op),
	)
}

// ---- User handlers ----

func (h *Handler) handleRegisterUser(c *echo.Context) error {
	segs := pathSegsFromCtx(c)
	accountID := seg(segs, segAccountID)
	namespace := seg(segs, segResID)

	body, err := readBody(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, errInvalidParam, errInvalidBody)
	}

	u, err := h.Backend.RegisterUser(
		accountID, namespace,
		strField(body, "UserName"),
		strField(body, "Email"),
		strField(body, "UserRole"),
		strField(body, "IdentityType"),
		strField(body, "SessionName"),
		tagsFromBody(body),
	)
	if err != nil {
		return httpErr(c, err)
	}

	return writeJSON(c, http.StatusOK, map[string]any{
		keyUser:      userToMap(u),
		keyRequestID: newReqID(),
		keyStatus:    http.StatusOK,
	})
}

func (h *Handler) handleDescribeUser(c *echo.Context) error {
	segs := pathSegsFromCtx(c)
	accountID := seg(segs, segAccountID)
	namespace := seg(segs, segResID)
	userName := seg(segs, segSubResID)

	u, err := h.Backend.DescribeUser(accountID, namespace, userName)
	if err != nil {
		return httpErr(c, err)
	}

	return writeJSON(c, http.StatusOK, map[string]any{
		keyUser:      userToMap(u),
		keyRequestID: newReqID(),
		keyStatus:    http.StatusOK,
	})
}

func (h *Handler) handleUpdateUser(c *echo.Context) error {
	segs := pathSegsFromCtx(c)
	accountID := seg(segs, segAccountID)
	namespace := seg(segs, segResID)
	userName := seg(segs, segSubResID)

	body, err := readBody(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, errInvalidParam, errInvalidBody)
	}

	u, err := h.Backend.UpdateUser(accountID, namespace, userName, strField(body, "Email"), strField(body, "Role"))
	if err != nil {
		return httpErr(c, err)
	}

	return writeJSON(c, http.StatusOK, map[string]any{
		keyUser:      userToMap(u),
		keyRequestID: newReqID(),
		keyStatus:    http.StatusOK,
	})
}

func (h *Handler) handleDeleteUser(c *echo.Context) error {
	segs := pathSegsFromCtx(c)
	accountID := seg(segs, segAccountID)
	namespace := seg(segs, segResID)
	userName := seg(segs, segSubResID)

	if err := h.Backend.DeleteUser(accountID, namespace, userName); err != nil {
		return httpErr(c, err)
	}

	return writeJSON(c, http.StatusOK, map[string]any{
		keyRequestID: newReqID(),
		keyStatus:    http.StatusOK,
	})
}

func (h *Handler) handleDeleteUserByPrincipalID(c *echo.Context) error {
	segs := pathSegsFromCtx(c)
	accountID := seg(segs, segAccountID)
	namespace := seg(segs, segResID)
	principalID := seg(segs, segSubResID)

	if err := h.Backend.DeleteUserByPrincipalID(accountID, namespace, principalID); err != nil {
		return httpErr(c, err)
	}

	return writeJSON(c, http.StatusOK, map[string]any{
		keyRequestID: newReqID(),
		keyStatus:    http.StatusOK,
	})
}

func (h *Handler) handleListUsers(c *echo.Context) error {
	segs := pathSegsFromCtx(c)
	accountID := seg(segs, segAccountID)
	namespace := seg(segs, segResID)

	users, next, err := h.Backend.ListUsers(accountID, namespace, maxResultsParam(c), nextTokenParam(c))
	if err != nil {
		return httpErr(c, err)
	}

	items := make([]map[string]any, 0, len(users))
	for _, u := range users {
		items = append(items, userToMap(u))
	}

	resp := map[string]any{
		keyUserList:  items,
		keyRequestID: newReqID(),
		keyStatus:    http.StatusOK,
	}
	if next != "" {
		resp[keyNextToken] = next
	}

	return writeJSON(c, http.StatusOK, resp)
}

func (h *Handler) handleListUserGroups(c *echo.Context) error {
	segs := pathSegsFromCtx(c)
	accountID := seg(segs, segAccountID)
	namespace := seg(segs, segResID)
	userName := seg(segs, segSubResID)

	groups, next, err := h.Backend.ListUserGroups(accountID, namespace, userName, maxResultsParam(c), nextTokenParam(c))
	if err != nil {
		return httpErr(c, err)
	}

	items := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		items = append(items, groupToMap(g))
	}

	resp := map[string]any{
		keyGroupList: items,
		keyRequestID: newReqID(),
		keyStatus:    http.StatusOK,
	}
	if next != "" {
		resp[keyNextToken] = next
	}

	return writeJSON(c, http.StatusOK, resp)
}

func userToMap(u *User) map[string]any {
	m := map[string]any{
		"Active":       u.Active,
		keyArn:         u.Arn,
		"Email":        u.Email,
		"IdentityType": u.IdentityType,
		keyNamespace:   u.Namespace,
		"PrincipalId":  u.PrincipalID,
		"Role":         u.Role,
		"UserName":     u.UserName,
	}
	if u.CustomPermissionsName != "" {
		m["CustomPermissionsName"] = u.CustomPermissionsName
	}

	return m
}
