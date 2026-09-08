package iot

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

func (h *Handler) handleAddThingToBillingGroup(c *echo.Context) error {
	var body AddThingToBillingGroupInput

	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil &&
		!errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	if err := h.Backend.AddThingToBillingGroup(&body); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleListThingsInBillingGroup(c *echo.Context) error {
	trimmed := strings.TrimPrefix(c.Request().URL.Path, "/billing-groups/")
	groupName := strings.TrimSuffix(trimmed, "/things")
	things := h.Backend.ListThingsInBillingGroup(groupName)

	return c.JSON(http.StatusOK, map[string]any{keyThings: things})
}

func (h *Handler) handleRemoveThingFromBillingGroup(c *echo.Context) error {
	var req struct {
		BillingGroupName string `json:"billingGroupName"`
		ThingName        string `json:"thingName"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	if err := h.Backend.RemoveThingFromBillingGroup(req.ThingName, req.BillingGroupName); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func resolveBillingGroupOps(path, method string) string {
	switch {
	case path == "/billing-groups" && method == http.MethodGet:
		return opListBillingGroups
	case path == "/billing-groups/removeThingFromBillingGroup" && method == http.MethodPut:
		return opRemoveThingFromBillingGroup
	case strings.HasPrefix(path, "/billing-groups/") &&
		strings.HasSuffix(path, "/things") &&
		method == http.MethodGet:
		return opListThingsInBillingGroup
	}

	return resolveBillingGroupCrudOps(path, method)
}

// resolveBillingGroupCrudOps resolves the generic /billing-groups/{name} CRUD routes.
func resolveBillingGroupCrudOps(path, method string) string {
	switch {
	case strings.HasPrefix(path, "/billing-groups/") && method == http.MethodPost:
		return opCreateBillingGroup
	case strings.HasPrefix(path, "/billing-groups/") && method == http.MethodGet:
		return opDescribeBillingGroup
	case strings.HasPrefix(path, "/billing-groups/") && method == http.MethodPatch:
		return opUpdateBillingGroup
	case strings.HasPrefix(path, "/billing-groups/") && method == http.MethodDelete:
		return opDeleteBillingGroup
	}

	return unknownOperation
}

func (h *Handler) handleCreateBillingGroup(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/billing-groups/")
	var input CreateBillingGroupInput
	if err := readBody(c, &input); err != nil {
		return err
	}
	input.BillingGroupName = name
	bg, err := h.Backend.CreateBillingGroup(&input)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"billingGroupName": bg.BillingGroupName,
		"billingGroupArn":  bg.BillingGroupARN,
		"billingGroupId":   bg.BillingGroupID,
	})
}

func (h *Handler) handleDescribeBillingGroup(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/billing-groups/")
	bg, err := h.Backend.DescribeBillingGroup(name)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, bg)
}

func (h *Handler) handleListBillingGroups(c *echo.Context) error {
	groups := h.Backend.ListBillingGroups()
	summaries := make([]map[string]any, len(groups))
	for i, bg := range groups {
		summaries[i] = map[string]any{
			keyGroupName: bg.BillingGroupName,
			keyGroupArn:  bg.BillingGroupARN,
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"billingGroups": summaries})
}

func (h *Handler) handleUpdateBillingGroup(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/billing-groups/")
	var req struct {
		BillingGroupProperties BillingGroupProperties `json:"billingGroupProperties"`
		ExpectedVersion        int64                  `json:"expectedVersion"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	version, err := h.Backend.UpdateBillingGroup(name, req.BillingGroupProperties, req.ExpectedVersion)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{keyVersion: version})
}

func (h *Handler) handleDeleteBillingGroup(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/billing-groups/")
	expectedVersion := parseExpectedVersionQueryParam(c)
	if err := h.Backend.DeleteBillingGroup(name, expectedVersion); err != nil {
		// DeleteBillingGroup's own deserializeOpError switch declares no
		// ResourceNotFoundException case.
		return respondAsInvalidRequest(c, err, ErrResourceNotFound)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) dispatchBillingGroupOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateBillingGroup:
		return true, h.handleCreateBillingGroup(c)
	case opDescribeBillingGroup:
		return true, h.handleDescribeBillingGroup(c)
	case opListBillingGroups:
		return true, h.handleListBillingGroups(c)
	case opUpdateBillingGroup:
		return true, h.handleUpdateBillingGroup(c)
	case opDeleteBillingGroup:
		return true, h.handleDeleteBillingGroup(c)
	case opListThingsInBillingGroup:
		return true, h.handleListThingsInBillingGroup(c)
	case opRemoveThingFromBillingGroup:
		return true, h.handleRemoveThingFromBillingGroup(c)
	}

	return false, nil
}

// resolveBillingGroupMiscPathOps resolves the addThingToBillingGroup-adjacent misc paths.
func resolveBillingGroupMiscPathOps(path, method string) string {
	switch {
	case path == "/billing-groups/removeThingFromBillingGroup" && method == http.MethodPut:
		return opRemoveThingFromBillingGroup
	case strings.HasPrefix(path, "/billing-groups/") &&
		strings.HasSuffix(path, "/things") &&
		method == http.MethodGet:
		return opListThingsInBillingGroup
	}

	return unknownOperation
}
