package iot

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

func resolveThingGroupOps(path, method string) string {
	// Handle exact and special-suffix paths first.
	if op := resolveThingGroupSpecialPaths(path, method); op != unknownOperation {
		return op
	}

	// Handle generic /thing-groups/{name} CRUD paths.
	if !strings.HasPrefix(path, "/thing-groups/") {
		return unknownOperation
	}

	switch method {
	case http.MethodPost:

		return opCreateThingGroup
	case http.MethodGet:

		return opDescribeThingGroup
	case http.MethodPatch:

		return opUpdateThingGroup
	case http.MethodDelete:

		return opDeleteThingGroup
	}

	return unknownOperation
}

func resolveThingGroupSpecialPaths(path, method string) string {
	switch {
	case path == "/thing-groups" && method == http.MethodGet:

		return opListThingGroups
	case path == "/thing-groups/removeThingFromThingGroup" && method == http.MethodPut:

		return opRemoveThingFromThingGroup
	case strings.HasSuffix(path, "/things") && method == http.MethodGet &&
		strings.HasPrefix(path, "/thing-groups/"):

		return opListThingsInThingGroup
	}

	return unknownOperation
}

func (h *Handler) dispatchThingGroupOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateThingGroup:

		return true, h.handleCreateThingGroup(c)
	case opDescribeThingGroup:

		return true, h.handleDescribeThingGroup(c)
	case opListThingGroups:

		return true, h.handleListThingGroups(c)
	case opUpdateThingGroup:

		return true, h.handleUpdateThingGroup(c)
	case opDeleteThingGroup:

		return true, h.handleDeleteThingGroup(c)
	case opRemoveThingFromThingGroup:

		return true, h.handleRemoveThingFromThingGroup(c)
	case opListThingsInThingGroup:

		return true, h.handleListThingsInThingGroup(c)
	}

	return false, nil
}

func (h *Handler) handleAddThingToThingGroup(c *echo.Context) error {
	var body AddThingToThingGroupInput

	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil &&
		!errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	if err := h.Backend.AddThingToThingGroup(&body); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleCreateThingGroup(c *echo.Context) error {
	thingGroupName := strings.TrimPrefix(c.Request().URL.Path, "/thing-groups/")

	var body struct {
		ThingGroupProperties *struct {
			AttributePayload      *AttributePayload `json:"attributePayload"`
			ThingGroupDescription string            `json:"thingGroupDescription"`
		} `json:"thingGroupProperties"`
		ParentGroupName string `json:"parentGroupName"`
		// []types.Tag on the wire, not a map (serializers.go:4871, aws-sdk-go-v2/service/iot@v1.77.4).
		Tags []tags.KV `json:"tags,omitempty"`
	}

	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil &&
		!errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	desc := ""
	var attrs map[string]string
	if body.ThingGroupProperties != nil {
		desc = body.ThingGroupProperties.ThingGroupDescription
		if body.ThingGroupProperties.AttributePayload != nil {
			attrs = body.ThingGroupProperties.AttributePayload.Attributes
		}
	}

	tg, err := h.Backend.CreateThingGroup(&CreateThingGroupInput{
		ThingGroupName:  thingGroupName,
		ParentGroupName: body.ParentGroupName,
		Description:     desc,
		Attributes:      attrs,
		Tags:            tags.MapFromKV(body.Tags),
	})
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		keyThingGroupName: tg.ThingGroupName,
		keyThingGroupArn:  tg.ThingGroupARN,
		keyThingGroupID:   tg.ThingGroupID,
	})
}

func (h *Handler) handleDescribeThingGroup(c *echo.Context) error {
	thingGroupName := strings.TrimPrefix(c.Request().URL.Path, "/thing-groups/")
	tg, err := h.Backend.DescribeThingGroup(thingGroupName)
	if err != nil {
		return h.handleError(c, err)
	}

	metadata := map[string]any{
		keyCreationDate:   awstime.Epoch(tg.CreatedAt),
		"parentGroupName": tg.ParentGroupName,
	}

	// rootToParentThingGroups is only present (per real AWS behavior) when
	// the group actually has ancestors -- verified against
	// aws-sdk-go-v2/service/iot@v1.77.4's ThingGroupMetadata; a root-level
	// group has no ParentGroupName and an empty ancestor chain.
	if roots := h.Backend.RootToParentThingGroups(thingGroupName); len(roots) > 0 {
		metadata["rootToParentThingGroups"] = roots
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyThingGroupName: tg.ThingGroupName,
		keyThingGroupArn:  tg.ThingGroupARN,
		keyThingGroupID:   tg.ThingGroupID,
		keyVersion:        tg.Version,
		"thingGroupProperties": map[string]any{
			"thingGroupDescription": tg.Description,
			"attributePayload":      map[string]any{keyAttributes: tg.Attributes},
		},
		"thingGroupMetadata": metadata,
	})
}

func (h *Handler) handleListThingGroups(c *echo.Context) error {
	groups := h.Backend.ListThingGroups()
	out := make([]map[string]string, 0, len(groups))
	for _, tg := range groups {
		// ListThingGroups' items deserialize as types.GroupNameAndArn
		// (iot@v1.77.4 deserializers.go's awsRestjson1_deserializeDocumentGroupNameAndArn:
		// "groupName"/"groupArn"), a different shape from
		// CreateThingGroupOutput/DescribeThingGroupOutput's "thingGroupName"/
		// "thingGroupArn" -- every real client's GroupName/GroupArn decoded
		// empty before this fix.
		out = append(out, map[string]string{
			keyGroupName: tg.ThingGroupName,
			keyGroupArn:  tg.ThingGroupARN,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{"thingGroups": out})
}

func (h *Handler) handleUpdateThingGroup(c *echo.Context) error {
	thingGroupName := strings.TrimPrefix(c.Request().URL.Path, "/thing-groups/")

	var body struct {
		ThingGroupProperties *struct {
			AttributePayload      *AttributePayload `json:"attributePayload"`
			ThingGroupDescription string            `json:"thingGroupDescription"`
		} `json:"thingGroupProperties"`
		ExpectedVersion int64 `json:"expectedVersion"`
	}

	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil &&
		!errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	desc := ""
	var attrs map[string]string
	var merge *bool
	if body.ThingGroupProperties != nil {
		desc = body.ThingGroupProperties.ThingGroupDescription
		if body.ThingGroupProperties.AttributePayload != nil {
			attrs = body.ThingGroupProperties.AttributePayload.Attributes
			merge = body.ThingGroupProperties.AttributePayload.Merge
		}
	}

	newVersion, err := h.Backend.UpdateThingGroup(&UpdateThingGroupInput{
		ThingGroupName:  thingGroupName,
		Description:     desc,
		Attributes:      attrs,
		Merge:           merge,
		ExpectedVersion: body.ExpectedVersion,
	})
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{keyVersion: newVersion})
}

func (h *Handler) handleDeleteThingGroup(c *echo.Context) error {
	thingGroupName := strings.TrimPrefix(c.Request().URL.Path, "/thing-groups/")
	expectedVersion := parseExpectedVersionQueryParam(c)
	if err := h.Backend.DeleteThingGroup(thingGroupName, expectedVersion); err != nil {
		// DeleteThingGroup's own deserializeOpError switch declares no
		// ResourceNotFoundException case.
		return respondAsInvalidRequest(c, err, ErrThingGroupNotFound)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) handleRemoveThingFromThingGroup(c *echo.Context) error {
	var body RemoveThingFromThingGroupInput
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil &&
		!errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}
	if err := h.Backend.RemoveThingFromThingGroup(&body); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleListThingsInThingGroup(c *echo.Context) error {
	after := strings.TrimPrefix(c.Request().URL.Path, "/thing-groups/")
	thingGroupName := strings.TrimSuffix(after, "/things")
	things, err := h.Backend.ListThingsInThingGroup(
		&ListThingsInThingGroupInput{ThingGroupName: thingGroupName},
	)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"things": things})
}

func (h *Handler) handleListThingGroupsForThing(c *echo.Context) error {
	thingName := extractThingName(c.Request().URL.Path)
	names := h.Backend.ListThingGroupsForThing(thingName)

	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		entry := map[string]any{keyGroupName: name}
		if tg, err := h.Backend.DescribeThingGroup(name); err == nil {
			entry[keyGroupArn] = tg.ThingGroupARN
		}
		out = append(out, entry)
	}

	pageSize, start := parseIoTPagination(c)
	page, nextToken := paginateMaps(out, pageSize, start)

	resp := map[string]any{"thingGroups": page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

// resolveBatch3DynamicGroupOps handles dynamic thing group ops.
func resolveBatch3DynamicGroupOps(path, method string) string {
	switch {
	case strings.HasPrefix(path, "/dynamic-thing-groups/") && method == http.MethodPost:

		return opCreateDynamicThingGroup
	case strings.HasPrefix(path, "/dynamic-thing-groups/") && method == http.MethodDelete:

		return opDeleteDynamicThingGroup
	case strings.HasPrefix(path, "/dynamic-thing-groups/") && method == http.MethodPatch:

		return opUpdateDynamicThingGroup
	}

	return unknownOperation
}

func (h *Handler) handleCreateDynamicThingGroup(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/dynamic-thing-groups/")
	var req struct {
		ThingGroupProperties *struct {
			AttributePayload      *AttributePayload `json:"attributePayload"`
			ThingGroupDescription string            `json:"thingGroupDescription"`
		} `json:"thingGroupProperties"`
		QueryString  string `json:"queryString"`
		IndexName    string `json:"indexName"`
		QueryVersion string `json:"queryVersion"`
		// []types.Tag on the wire, not a map (serializers.go:2625, aws-sdk-go-v2/service/iot@v1.77.4).
		Tags []tags.KV `json:"tags,omitempty"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}

	desc := ""
	var attrs map[string]string
	if req.ThingGroupProperties != nil {
		desc = req.ThingGroupProperties.ThingGroupDescription
		if req.ThingGroupProperties.AttributePayload != nil {
			attrs = req.ThingGroupProperties.AttributePayload.Attributes
		}
	}

	tg, err := h.Backend.CreateDynamicThingGroup(&CreateThingGroupInput{
		ThingGroupName: name,
		Description:    desc,
		Attributes:     attrs,
		QueryString:    req.QueryString,
		IndexName:      req.IndexName,
		QueryVersion:   req.QueryVersion,
		Tags:           tags.MapFromKV(req.Tags),
	})
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"thingGroupName": tg.ThingGroupName,
		"thingGroupArn":  tg.ThingGroupARN,
		keyThingGroupID:  tg.ThingGroupID,
		"queryString":    tg.QueryString,
		"indexName":      tg.IndexName,
		"queryVersion":   tg.QueryVersion,
	})
}

func (h *Handler) handleDeleteDynamicThingGroup(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/dynamic-thing-groups/")
	expectedVersion := parseExpectedVersionQueryParam(c)
	if err := h.Backend.DeleteDynamicThingGroup(name, expectedVersion); err != nil {
		// DeleteDynamicThingGroup's own deserializeOpError switch declares
		// no ResourceNotFoundException case.
		return respondAsInvalidRequest(c, err, ErrThingGroupNotFound)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleUpdateDynamicThingGroup(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/dynamic-thing-groups/")
	var req struct {
		ThingGroupProperties *struct {
			AttributePayload      *AttributePayload `json:"attributePayload"`
			ThingGroupDescription string            `json:"thingGroupDescription"`
		} `json:"thingGroupProperties"`
		QueryString     string `json:"queryString"`
		IndexName       string `json:"indexName"`
		QueryVersion    string `json:"queryVersion"`
		ExpectedVersion int64  `json:"expectedVersion"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}

	desc := ""
	var attrs map[string]string
	var merge *bool
	if req.ThingGroupProperties != nil {
		desc = req.ThingGroupProperties.ThingGroupDescription
		if req.ThingGroupProperties.AttributePayload != nil {
			attrs = req.ThingGroupProperties.AttributePayload.Attributes
			merge = req.ThingGroupProperties.AttributePayload.Merge
		}
	}

	version, err := h.Backend.UpdateDynamicThingGroup(&UpdateThingGroupInput{
		ThingGroupName:  name,
		Description:     desc,
		Attributes:      attrs,
		Merge:           merge,
		QueryString:     req.QueryString,
		IndexName:       req.IndexName,
		QueryVersion:    req.QueryVersion,
		ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{keyVersion: version})
}

// resolveThingGroupUpdateOp resolves the PUT
// /thing-groups/updateThingGroupsForThing route.
func resolveThingGroupUpdateOp(path, method string) string {
	if path == pathUpdateThingGroups && method == http.MethodPut {
		return opUpdateThingGroupsForThing
	}

	return unknownOperation
}

func (h *Handler) handleUpdateThingGroupsForThing(c *echo.Context) error {
	var body struct {
		ThingName             string   `json:"thingName"`
		ThingGroupsToAdd      []string `json:"thingGroupsToAdd"`
		ThingGroupsToRemove   []string `json:"thingGroupsToRemove"`
		OverrideDynamicGroups bool     `json:"overrideDynamicGroups"`
	}
	if err := readBody(c, &body); err != nil {
		return err
	}

	if err := h.Backend.UpdateThingGroupsForThing(&UpdateThingGroupsForThingInput{
		ThingName:             body.ThingName,
		ThingGroupsToAdd:      body.ThingGroupsToAdd,
		ThingGroupsToRemove:   body.ThingGroupsToRemove,
		OverrideDynamicGroups: body.OverrideDynamicGroups,
	}); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) dispatchDynamicThingGroupOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateDynamicThingGroup:
		return true, h.handleCreateDynamicThingGroup(c)
	case opDeleteDynamicThingGroup:
		return true, h.handleDeleteDynamicThingGroup(c)
	case opUpdateDynamicThingGroup:
		return true, h.handleUpdateDynamicThingGroup(c)
	case opListThingGroupsForThing:
		return true, h.handleListThingGroupsForThing(c)
	}

	return false, nil
}

// resolveThingGroupForThingOps resolves ListThingGroupsForThing.
func resolveThingGroupForThingOps(path, method string) string {
	if strings.HasPrefix(path, "/things/") &&
		strings.HasSuffix(path, "/thing-groups") &&
		method == http.MethodGet {
		return opListThingGroupsForThing
	}

	return unknownOperation
}
