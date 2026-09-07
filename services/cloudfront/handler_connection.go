package cloudfront

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// connectionFunctionConfigXML models the nested ConnectionFunctionConfig element carried by
// Create/UpdateConnectionFunctionRequest bodies (Comment + Runtime).
type connectionFunctionConfigXML struct {
	Comment string `xml:"Comment"`
	Runtime string `xml:"Runtime"`
}

// connectionFunctionRequestXML models a CreateConnectionFunctionRequest body. Comment is kept as
// both a top-level convenience field (for backward compatibility with earlier callers) and
// inside the nested, AWS-accurate ConnectionFunctionConfig element; the top-level value wins
// when both are present.
type connectionFunctionRequestXML struct {
	XMLName                  xml.Name                    `xml:"CreateConnectionFunctionRequest"`
	ConnectionFunctionConfig connectionFunctionConfigXML `xml:"ConnectionFunctionConfig"`
	Name                     string                      `xml:"Name"`
	Comment                  string                      `xml:"Comment"`
	// ConnectionFunctionCode is base64-encoded on the wire (matching real CloudFront); see
	// decodeConnectionFunctionCode.
	ConnectionFunctionCode string   `xml:"ConnectionFunctionCode"`
	Tags                   []tagXML `xml:"Tags>Items>Tag"`
}

// updateConnectionFunctionRequestXML models an UpdateConnectionFunctionRequest body.
type updateConnectionFunctionRequestXML struct {
	XMLName                  xml.Name                    `xml:"UpdateConnectionFunctionRequest"`
	ConnectionFunctionConfig connectionFunctionConfigXML `xml:"ConnectionFunctionConfig"`
	ConnectionFunctionCode   string                      `xml:"ConnectionFunctionCode"`
}

// decodeConnectionFunctionCode decodes a base64-encoded ConnectionFunctionCode payload, matching
// the real CloudFront wire format. If the payload is not valid base64 (e.g. a test sends raw
// text), it is used verbatim so the mock stays lenient.
func decodeConnectionFunctionCode(s string) []byte {
	if s == "" {
		return nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
		return decoded
	}

	return []byte(s)
}

// connectionGroupRequestXML models a CreateConnectionGroupRequest body. Comment is a
// gopherstack-only convenience field kept for backward compatibility; it is not part of the
// real AWS ConnectionGroup shape.
type connectionGroupRequestXML struct {
	XMLName         xml.Name `xml:"CreateConnectionGroupRequest"`
	Name            string   `xml:"Name"`
	Comment         string   `xml:"Comment"`
	AnycastIPListID string   `xml:"AnycastIpListId"`
	Enabled         *bool    `xml:"Enabled"`
	Ipv6Enabled     *bool    `xml:"Ipv6Enabled"`
	Tags            []tagXML `xml:"Tags>Items>Tag"`
}

// updateConnectionGroupRequestXML models an UpdateConnectionGroupRequest body.
type updateConnectionGroupRequestXML struct {
	Enabled         *bool    `xml:"Enabled"`
	Ipv6Enabled     *bool    `xml:"Ipv6Enabled"`
	XMLName         xml.Name `xml:"UpdateConnectionGroupRequest"`
	Comment         string   `xml:"Comment"`
	AnycastIPListID string   `xml:"AnycastIpListId"`
}

func (h *Handler) handleCreateConnectionFunction(c *echo.Context) error {
	body, err := readBody(c)
	if err != nil {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("MalformedXML", "failed to read body"))
	}

	if qErr := validateFunctionConfigQuantities(body); qErr != nil {
		return h.handleError(c, qErr)
	}

	var req connectionFunctionRequestXML
	if len(body) > 0 {
		if xmlErr := xml.Unmarshal(body, &req); xmlErr != nil {
			return xmlResp(
				c,
				http.StatusBadRequest,
				cfErrorXML("MalformedXML", "invalid CreateConnectionFunctionRequest XML"),
			)
		}
	}

	comment := req.Comment
	if comment == "" {
		comment = req.ConnectionFunctionConfig.Comment
	}

	tags := make(map[string]string, len(req.Tags))
	for _, tag := range req.Tags {
		tags[tag.Key] = tag.Value
	}

	code := decodeConnectionFunctionCode(req.ConnectionFunctionCode)
	fn, createErr := h.Backend.CreateConnectionFunctionWithCode(
		req.Name, comment, req.ConnectionFunctionConfig.Runtime, code, tags,
	)
	if createErr != nil {
		return h.handleError(c, createErr)
	}

	c.Response().Header().Set("Location", cfPathPrefix+"connection-function/"+fn.ID)
	c.Response().Header().Set("ETag", fn.ETag)

	return xmlResp(c, http.StatusCreated, connectionFunctionSummaryXML(fn))
}

func (h *Handler) handleCreateConnectionGroup(c *echo.Context) error {
	body, err := readBody(c)
	if err != nil {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("MalformedXML", "failed to read body"))
	}

	if qErr := validateQuantities(body); qErr != nil {
		return h.handleError(c, qErr)
	}

	var req connectionGroupRequestXML
	if len(body) > 0 {
		if xmlErr := xml.Unmarshal(body, &req); xmlErr != nil {
			return xmlResp(
				c,
				http.StatusBadRequest,
				cfErrorXML("MalformedXML", "invalid CreateConnectionGroupRequest XML"),
			)
		}
	}

	ipv6Enabled := true
	if req.Ipv6Enabled != nil {
		ipv6Enabled = *req.Ipv6Enabled
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	tags := make(map[string]string, len(req.Tags))
	for _, tag := range req.Tags {
		tags[tag.Key] = tag.Value
	}

	group, createErr := h.Backend.CreateConnectionGroupWithConfig(
		req.Name, req.Comment, req.AnycastIPListID, ipv6Enabled, enabled, tags,
	)
	if createErr != nil {
		return h.handleError(c, createErr)
	}

	c.Response().Header().Set("Location", cfPathPrefix+"connection-group/"+group.ID)
	c.Response().Header().Set("ETag", group.ETag)

	return xmlResp(c, http.StatusCreated, connectionGroupXML(group))
}

// connectionGroupPreconditionFailedXML is the shared If-Match error body for connection group
// mutations, mirroring the trust store / streaming distribution PreconditionFailed pattern.
func connectionGroupPreconditionFailedXML() string {
	return cfErrorXML("PreconditionFailed", "If-Match ETag did not match the current connection group ETag")
}

func connectionGroupXML(cg *ConnectionGroup) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<ConnectionGroup xmlns="%s">`+
		`<Id>%s</Id><Name>%s</Name><ARN>%s</ARN><Comment>%s</Comment>`+
		`<CreatedTime>%s</CreatedTime><LastModifiedTime>%s</LastModifiedTime>`+
		`<Status>%s</Status><Enabled>%v</Enabled><Ipv6Enabled>%v</Ipv6Enabled>`+
		`<IsDefault>%v</IsDefault><RoutingEndpoint>%s</RoutingEndpoint>`+
		`<AnycastIpListId>%s</AnycastIpListId>`+
		`</ConnectionGroup>`,
		cfNS, cg.ID, cg.Name, cg.ARN, cg.Comment,
		cg.CreatedTime, cg.LastModifiedTime,
		cg.Status, cg.Enabled, cg.IPv6Enabled,
		cg.IsDefault, cg.RoutingEndpoint,
		cg.AnycastIPListID)
}

func (h *Handler) handleGetConnectionGroup(c *echo.Context, id string) error {
	cg, err := h.Backend.GetConnectionGroup(id)
	if err != nil {
		return h.handleError(c, err)
	}
	c.Response().Header().Set("ETag", cg.ETag)

	return xmlResp(c, http.StatusOK, connectionGroupXML(cg))
}

func (h *Handler) handleGetConnectionGroupByRoutingEndpoint(c *echo.Context, endpoint string) error {
	cg, err := h.Backend.GetConnectionGroupByRoutingEndpoint(endpoint)
	if err != nil {
		return h.handleError(c, err)
	}
	c.Response().Header().Set("ETag", cg.ETag)

	return xmlResp(c, http.StatusOK, connectionGroupXML(cg))
}

// listConnectionGroupsRequestXML models a ListConnectionGroups request body.
// cloudfront@v1.67.4 serializers.go awsRestxml_serializeOpHttpBindingsListConnectionGroupsInput
// returns nil (no HTTP-bound fields), so AssociationFilter, Marker, and MaxItems all serialize
// into the XML body, not the query string.
type listConnectionGroupsRequestXML struct {
	XMLName           xml.Name `xml:"ListConnectionGroupsRequest"`
	AssociationFilter struct {
		AnycastIPListID string `xml:"AnycastIpListId"`
	} `xml:"AssociationFilter"`
	Marker   string `xml:"Marker"`
	MaxItems int    `xml:"MaxItems"`
}

func (h *Handler) handleListConnectionGroups(c *echo.Context) error {
	body, err := readBody(c)
	if err != nil {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("MalformedXML", "failed to read body"))
	}

	var req listConnectionGroupsRequestXML
	if len(body) > 0 {
		if xmlErr := xml.Unmarshal(body, &req); xmlErr != nil {
			return xmlResp(
				c,
				http.StatusBadRequest,
				cfErrorXML("MalformedXML", "invalid ListConnectionGroupsRequest XML"),
			)
		}
	}

	items := h.Backend.ListConnectionGroups()
	if anycastID := req.AssociationFilter.AnycastIPListID; anycastID != "" {
		items = filterSlice(items, func(cg *ConnectionGroup) bool { return cg.AnycastIPListID == anycastID })
	}

	page, _, isTruncated := paginateByMarkerValue(
		items,
		func(cg *ConnectionGroup) string { return cg.ID },
		req.Marker,
		req.MaxItems,
	)

	type cgSummary struct {
		XMLName          xml.Name `xml:"ConnectionGroupSummary"`
		ID               string   `xml:"Id"`
		Name             string   `xml:"Name"`
		ARN              string   `xml:"Arn"`
		ETag             string   `xml:"ETag"`
		RoutingEndpoint  string   `xml:"RoutingEndpoint"`
		Status           string   `xml:"Status"`
		AnycastIPListID  string   `xml:"AnycastIpListId,omitempty"`
		CreatedTime      string   `xml:"CreatedTime"`
		LastModifiedTime string   `xml:"LastModifiedTime"`
		Enabled          bool     `xml:"Enabled"`
		IsDefault        bool     `xml:"IsDefault"`
	}
	// Real ListConnectionGroupsOutput (api_op_ListConnectionGroups.go) is
	// ConnectionGroups []ConnectionGroupSummary + NextMarker, no Quantity/Items
	// wrapper: awsRestxml_deserializeOpDocumentListConnectionGroupsOutput reads
	// a direct <ConnectionGroups> child holding repeated <ConnectionGroupSummary>
	// elements (cloudfront@v1.67.4 deserializers.go).
	type cgList struct {
		XMLName          xml.Name    `xml:"ListConnectionGroupsResult"`
		XMLNS            string      `xml:"xmlns,attr"`
		NextMarker       string      `xml:"NextMarker,omitempty"`
		ConnectionGroups []cgSummary `xml:"ConnectionGroups>ConnectionGroupSummary"`
	}
	summaries := make([]cgSummary, 0, len(page))
	for _, cg := range page {
		summaries = append(summaries, cgSummary{
			ID: cg.ID, Name: cg.Name, ARN: cg.ARN, ETag: cg.ETag,
			RoutingEndpoint: cg.RoutingEndpoint, Status: cg.Status,
			AnycastIPListID: cg.AnycastIPListID, CreatedTime: cg.CreatedTime,
			LastModifiedTime: cg.LastModifiedTime, Enabled: cg.Enabled, IsDefault: cg.IsDefault,
		})
	}
	list := cgList{XMLNS: cfNS, ConnectionGroups: summaries}
	if isTruncated && len(page) > 0 {
		list.NextMarker = page[len(page)-1].ID
	}
	out, xmlErr := xml.Marshal(list)
	if xmlErr != nil {
		return h.handleError(c, xmlErr)
	}

	return xmlResp(c, http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?>`+string(out))
}

func (h *Handler) handleUpdateConnectionGroup(c *echo.Context, id string) error {
	current, getErr := h.Backend.GetConnectionGroup(id)
	if getErr != nil {
		return h.handleError(c, getErr)
	}

	if ifMatch := c.Request().Header.Get("If-Match"); ifMatch != "" && ifMatch != current.ETag {
		return xmlResp(c, http.StatusPreconditionFailed, connectionGroupPreconditionFailedXML())
	}

	body, err := readBody(c)
	if err != nil {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("MalformedXML", "failed to read body"))
	}

	if qErr := validateQuantities(body); qErr != nil {
		return h.handleError(c, qErr)
	}
	var req updateConnectionGroupRequestXML
	if len(body) > 0 {
		if xmlErr := xml.Unmarshal(body, &req); xmlErr != nil {
			return xmlResp(
				c,
				http.StatusBadRequest,
				cfErrorXML("MalformedXML", "invalid UpdateConnectionGroupRequest XML"),
			)
		}
	}

	var anycastIPListID *string
	if req.AnycastIPListID != "" {
		anycastIPListID = &req.AnycastIPListID
	}

	cg, updateErr := h.Backend.UpdateConnectionGroup(id, req.Comment, anycastIPListID, req.Ipv6Enabled, req.Enabled)
	if updateErr != nil {
		return h.handleError(c, updateErr)
	}
	c.Response().Header().Set("ETag", cg.ETag)

	return xmlResp(c, http.StatusOK, connectionGroupXML(cg))
}

func (h *Handler) handleDeleteConnectionGroup(c *echo.Context, id string) error {
	current, getErr := h.Backend.GetConnectionGroup(id)
	if getErr != nil {
		return h.handleError(c, getErr)
	}

	if ifMatch := c.Request().Header.Get("If-Match"); ifMatch != "" && ifMatch != current.ETag {
		return xmlResp(c, http.StatusPreconditionFailed, connectionGroupPreconditionFailedXML())
	}

	if err := h.Backend.DeleteConnectionGroup(id); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// ConnectionFunction extra handlers (Get/Describe/List/Update/Delete/Publish/Test)
// ---------------------------------------------------------------------------

// connectionFunctionPreconditionFailedXML is the shared If-Match error body for connection
// function mutations.
func connectionFunctionPreconditionFailedXML() string {
	return cfErrorXML(
		"PreconditionFailed",
		"If-Match ETag did not match the current connection function ETag",
	)
}

// connectionFunctionSummaryXML builds the ConnectionFunctionSummary XML representation used by
// DescribeConnectionFunction, Publish, and List responses.
func connectionFunctionSummaryXML(fn *ConnectionFunction) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<ConnectionFunctionSummary xmlns="%s">`+
		`<Id>%s</Id><ConnectionFunctionArn>%s</ConnectionFunctionArn><Name>%s</Name>`+
		`<ConnectionFunctionConfig><Comment>%s</Comment><Runtime>%s</Runtime></ConnectionFunctionConfig>`+
		`<Stage>%s</Stage><Status>%s</Status>`+
		`<CreatedTime>%s</CreatedTime><LastModifiedTime>%s</LastModifiedTime>`+
		`</ConnectionFunctionSummary>`,
		cfNS, fn.ID, fn.ARN, fn.Name, fn.Comment, fn.Runtime, fn.Stage, fn.Status,
		fn.CreatedTime, fn.LastModifiedTime)
}

// handleGetConnectionFunction returns the connection function's code and content type, mirroring
// GetConnectionFunctionOutput (ConnectionFunctionCode + ContentType), unlike
// DescribeConnectionFunction which returns metadata only.
func (h *Handler) handleGetConnectionFunction(c *echo.Context, id string) error {
	fn, err := h.Backend.GetConnectionFunction(id)
	if err != nil {
		return h.handleError(c, err)
	}
	c.Response().Header().Set("ETag", fn.ETag)

	return c.Blob(http.StatusOK, "application/octet-stream", fn.FunctionCode)
}

func (h *Handler) handleDescribeConnectionFunction(c *echo.Context, id string) error {
	fn, err := h.Backend.GetConnectionFunction(id)
	if err != nil {
		return h.handleError(c, err)
	}
	c.Response().Header().Set("ETag", fn.ETag)

	return xmlResp(c, http.StatusOK, connectionFunctionSummaryXML(fn))
}

// listConnectionFunctionsRequestXML models a ListConnectionFunctions request body.
// cloudfront@v1.67.4 serializers.go awsRestxml_serializeOpHttpBindingsListConnectionFunctionsInput
// returns nil (no HTTP-bound fields), so Marker, MaxItems, and Stage all serialize into the XML
// body, not the query string -- unlike sibling op ListFunctions, whose Stage is query-bound.
type listConnectionFunctionsRequestXML struct {
	XMLName  xml.Name `xml:"ListConnectionFunctionsRequest"`
	Marker   string   `xml:"Marker"`
	Stage    string   `xml:"Stage"`
	MaxItems int      `xml:"MaxItems"`
}

func (h *Handler) handleListConnectionFunctions(c *echo.Context) error {
	body, err := readBody(c)
	if err != nil {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("MalformedXML", "failed to read body"))
	}

	var req listConnectionFunctionsRequestXML
	if len(body) > 0 {
		if xmlErr := xml.Unmarshal(body, &req); xmlErr != nil {
			return xmlResp(
				c,
				http.StatusBadRequest,
				cfErrorXML("MalformedXML", "invalid ListConnectionFunctionsRequest XML"),
			)
		}
	}

	items := h.Backend.ListConnectionFunctions()
	if req.Stage != "" {
		items = filterSlice(items, func(fn *ConnectionFunction) bool { return fn.Stage == req.Stage })
	}

	// Name alone is not a unique cursor key (ConnectionFunction names may repeat, see
	// ListConnectionFunctions); Name+tab+ID matches the tiebreak that list applies and
	// keeps same-named functions from being dropped when a tie group straddles a page
	// boundary. Tab (not NUL) because Marker round-trips through the XML request/response
	// body and NUL is not a valid XML 1.0 character.
	page, _, isTruncated := paginateByMarkerValue(
		items,
		func(fn *ConnectionFunction) string { return fn.Name + "\t" + fn.ID },
		req.Marker,
		req.MaxItems,
	)

	type cfnConfig struct {
		Comment string `xml:"Comment"`
		Runtime string `xml:"Runtime"`
	}
	type cfnSummary struct {
		XMLName          xml.Name  `xml:"ConnectionFunctionSummary"`
		ID               string    `xml:"Id"`
		ARN              string    `xml:"ConnectionFunctionArn"`
		Name             string    `xml:"Name"`
		Config           cfnConfig `xml:"ConnectionFunctionConfig"`
		Stage            string    `xml:"Stage"`
		Status           string    `xml:"Status"`
		CreatedTime      string    `xml:"CreatedTime"`
		LastModifiedTime string    `xml:"LastModifiedTime"`
	}
	// Real ListConnectionFunctionsOutput (api_op_ListConnectionFunctions.go) is
	// ConnectionFunctions []ConnectionFunctionSummary + NextMarker, no
	// Quantity/Items wrapper: awsRestxml_deserializeOpDocumentListConnectionFunctionsOutput
	// reads a direct <ConnectionFunctions> child holding repeated
	// <ConnectionFunctionSummary> elements (cloudfront@v1.67.4 deserializers.go).
	type cfnList struct {
		XMLName             xml.Name     `xml:"ListConnectionFunctionsResult"`
		XMLNS               string       `xml:"xmlns,attr"`
		NextMarker          string       `xml:"NextMarker,omitempty"`
		ConnectionFunctions []cfnSummary `xml:"ConnectionFunctions>ConnectionFunctionSummary"`
	}
	summaries := make([]cfnSummary, 0, len(page))
	for _, fn := range page {
		summaries = append(summaries, cfnSummary{
			ID: fn.ID, ARN: fn.ARN, Name: fn.Name, Stage: fn.Stage, Status: fn.Status,
			Config:           cfnConfig{Comment: fn.Comment, Runtime: fn.Runtime},
			CreatedTime:      fn.CreatedTime,
			LastModifiedTime: fn.LastModifiedTime,
		})
	}
	list := cfnList{XMLNS: cfNS, ConnectionFunctions: summaries}
	if isTruncated && len(page) > 0 {
		last := page[len(page)-1]
		list.NextMarker = last.Name + "\t" + last.ID
	}
	out, xmlErr := xml.Marshal(list)
	if xmlErr != nil {
		return h.handleError(c, xmlErr)
	}

	return xmlResp(c, http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?>`+string(out))
}

func (h *Handler) handleUpdateConnectionFunction(c *echo.Context, id string) error {
	current, getErr := h.Backend.GetConnectionFunction(id)
	if getErr != nil {
		return h.handleError(c, getErr)
	}

	if ifMatch := c.Request().Header.Get("If-Match"); ifMatch != "" && ifMatch != current.ETag {
		return xmlResp(c, http.StatusPreconditionFailed, connectionFunctionPreconditionFailedXML())
	}

	body, err := readBody(c)
	if err != nil {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("MalformedXML", "failed to read body"))
	}

	if qErr := validateFunctionConfigQuantities(body); qErr != nil {
		return h.handleError(c, qErr)
	}
	var req updateConnectionFunctionRequestXML
	if len(body) > 0 {
		if xmlErr := xml.Unmarshal(body, &req); xmlErr != nil {
			return xmlResp(
				c,
				http.StatusBadRequest,
				cfErrorXML("MalformedXML", "invalid UpdateConnectionFunctionRequest XML"),
			)
		}
	}

	fn, updateErr := h.Backend.UpdateConnectionFunction(
		id, req.ConnectionFunctionConfig.Comment, req.ConnectionFunctionConfig.Runtime,
		decodeConnectionFunctionCode(req.ConnectionFunctionCode),
	)
	if updateErr != nil {
		return h.handleError(c, updateErr)
	}
	c.Response().Header().Set("ETag", fn.ETag)

	return xmlResp(c, http.StatusOK, connectionFunctionSummaryXML(fn))
}

func (h *Handler) handleDeleteConnectionFunction(c *echo.Context, id string) error {
	current, getErr := h.Backend.GetConnectionFunction(id)
	if getErr != nil {
		return h.handleError(c, getErr)
	}

	if ifMatch := c.Request().Header.Get("If-Match"); ifMatch != "" && ifMatch != current.ETag {
		return xmlResp(c, http.StatusPreconditionFailed, connectionFunctionPreconditionFailedXML())
	}

	if err := h.Backend.DeleteConnectionFunction(id); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) handlePublishConnectionFunction(c *echo.Context, id string) error {
	current, getErr := h.Backend.GetConnectionFunction(id)
	if getErr != nil {
		return h.handleError(c, getErr)
	}

	if ifMatch := c.Request().Header.Get("If-Match"); ifMatch != "" && ifMatch != current.ETag {
		return xmlResp(c, http.StatusPreconditionFailed, connectionFunctionPreconditionFailedXML())
	}

	fn, err := h.Backend.PublishConnectionFunction(id)
	if err != nil {
		return h.handleError(c, err)
	}
	c.Response().Header().Set("ETag", fn.ETag)

	return xmlResp(c, http.StatusOK, connectionFunctionSummaryXML(fn))
}

// testConnectionFunctionRequestXML models the TestConnectionFunction request body: the
// base64-encoded connection object to run the function against.
type testConnectionFunctionRequestXML struct {
	XMLName          xml.Name `xml:"TestConnectionFunctionRequest"`
	ConnectionObject string   `xml:"ConnectionObject"`
}

func (h *Handler) handleTestConnectionFunction(c *echo.Context, id string) error {
	current, getErr := h.Backend.GetConnectionFunction(id)
	if getErr != nil {
		return h.handleError(c, getErr)
	}

	if ifMatch := c.Request().Header.Get("If-Match"); ifMatch != "" && ifMatch != current.ETag {
		return xmlResp(c, http.StatusPreconditionFailed, connectionFunctionPreconditionFailedXML())
	}

	body, err := readBody(c)
	if err != nil {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("MalformedXML", "failed to read body"))
	}

	if qErr := validateQuantities(body); qErr != nil {
		return h.handleError(c, qErr)
	}
	var req testConnectionFunctionRequestXML
	if len(body) > 0 {
		if xmlErr := xml.Unmarshal(body, &req); xmlErr != nil {
			return xmlResp(
				c,
				http.StatusBadRequest,
				cfErrorXML("MalformedXML", "invalid TestConnectionFunctionRequest XML"),
			)
		}
	}

	result, testErr := h.Backend.TestConnectionFunction(id, decodeConnectionFunctionCode(req.ConnectionObject))
	if testErr != nil {
		return h.handleError(c, testErr)
	}

	var logsXML strings.Builder
	for _, l := range result.ExecutionLogs {
		fmt.Fprintf(&logsXML, "<member>%s</member>", l)
	}

	return xmlResp(c, http.StatusOK, fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?>`+
			`<TestResult xmlns="%s">`+
			`<ConnectionFunctionSummary><Id>%s</Id><Name>%s</Name><Stage>%s</Stage></ConnectionFunctionSummary>`+
			`<ConnectionFunctionExecutionLogs>%s</ConnectionFunctionExecutionLogs>`+
			`<ConnectionFunctionErrorMessage></ConnectionFunctionErrorMessage>`+
			`<ConnectionFunctionOutput>%s</ConnectionFunctionOutput>`+
			`<ComputeUtilization>%s</ComputeUtilization>`+
			`</TestResult>`,
		cfNS, current.ID, current.Name, current.Stage,
		logsXML.String(), result.FunctionOutput, result.ComputeUtilization,
	))
}

// ---------------------------------------------------------------------------
// AnycastIPList extra handlers (Get/List/Update/Delete)
// ---------------------------------------------------------------------------
