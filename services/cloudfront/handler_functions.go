package cloudfront

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

type functionConfigXML struct {
	XMLName      xml.Name `xml:"FunctionConfig"`
	Comment      string   `xml:"Comment"`
	Runtime      string   `xml:"Runtime"`
	FunctionCode string   `xml:"FunctionCode"`
}

// functionRequestFields is shared by Create and Update, whose real request
// shapes carry identical fields but different root element names
// (CreateFunctionRequest vs UpdateFunctionRequest; cloudfront@v1.67.4
// serializers.go). A prior single struct fixed the root to
// "CreateFunctionRequest", so every real UpdateFunction call (root
// UpdateFunctionRequest) failed decode and was rejected as MalformedXML.
type functionRequestFields struct {
	Name           string            `xml:"Name"`
	FunctionCode   string            `xml:"FunctionCode"`
	FunctionConfig functionConfigXML `xml:"FunctionConfig"`
	Tags           tagsXML           `xml:"Tags"`
}

type createFunctionRequestXML struct {
	XMLName xml.Name `xml:"CreateFunctionRequest"`
	functionRequestFields
}

type updateFunctionRequestXML struct {
	XMLName xml.Name `xml:"UpdateFunctionRequest"`
	functionRequestFields
}

func (h *Handler) handleCreateFunction(c *echo.Context) error {
	body, err := readBody(c)
	if err != nil {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("MalformedXML", "failed to read body"))
	}

	if qErr := validateFunctionConfigQuantities(body); qErr != nil {
		return h.handleError(c, qErr)
	}

	var req createFunctionRequestXML
	if len(body) > 0 {
		if xmlErr := xml.Unmarshal(body, &req); xmlErr != nil {
			return xmlResp(
				c,
				http.StatusBadRequest,
				cfErrorXML("MalformedXML", "invalid CreateFunctionRequest XML"),
			)
		}
	}

	code := req.FunctionCode
	if code == "" {
		code = req.FunctionConfig.FunctionCode
	}

	fn, createErr := h.Backend.CreateFunction(
		req.Name,
		req.FunctionConfig.Comment,
		req.FunctionConfig.Runtime,
		code,
		tagsXMLToMap(req.Tags),
	)
	if createErr != nil {
		return h.handleError(c, createErr)
	}

	c.Response().Header().Set("ETag", fn.ETag)
	c.Response().Header().Set("Location", cfPathPrefix+"function/"+fn.Name)

	return xmlResp(c, http.StatusCreated, functionResponseXML(fn))
}

func (h *Handler) handleGetFunction(c *echo.Context, name string) error {
	fn, err := h.Backend.GetFunction(name)
	if err != nil {
		return h.handleError(c, err)
	}

	c.Response().Header().Set("ETag", fn.ETag)

	return xmlResp(c, http.StatusOK, functionResponseXML(fn))
}

func (h *Handler) handleDescribeFunction(c *echo.Context, name string) error {
	fn, err := h.Backend.GetFunction(name)
	if err != nil {
		return h.handleError(c, err)
	}

	c.Response().Header().Set("ETag", fn.ETag)

	return xmlResp(c, http.StatusOK, functionResponseXML(fn))
}

func (h *Handler) handleListFunctions(c *echo.Context) error {
	fns := h.Backend.ListFunctions()

	// Stage is a real query-bound filter (cloudfront@v1.67.4 serializers.go:
	// awsRestxml_serializeOpHttpBindingsListFunctionsInput), DEVELOPMENT or LIVE.
	if stage := c.QueryParam("Stage"); stage != "" {
		fns = filterSlice(fns, func(fn *Function) bool { return fn.Status == stage })
	}

	page, pageSize, isTruncated, nextMarker := paginateByMarkerID(c, fns, func(fn *Function) string { return fn.Name })

	var sb strings.Builder

	for _, fn := range page {
		fmt.Fprintf(&sb,
			`<FunctionSummary>`+
				`<Name>%s</Name>`+
				`<Status>%s</Status>`+
				`<FunctionConfig>`+
				`<Comment>%s</Comment>`+
				`<Runtime>%s</Runtime>`+
				`</FunctionConfig>`+
				`<FunctionMetadata>`+
				`<FunctionARN>%s</FunctionARN>`+
				`<Stage>%s</Stage>`+
				`<CreatedTime>%s</CreatedTime>`+
				`<LastModifiedTime>%s</LastModifiedTime>`+
				`</FunctionMetadata>`+
				`</FunctionSummary>`,
			fn.Name, fn.Status, fn.Comment, fn.Runtime,
			fn.ARN, fn.Status, fn.CreatedTime, fn.LastModifiedTime)
	}

	nextMarkerXML := ""
	if isTruncated {
		nextMarkerXML = fmt.Sprintf(`<NextMarker>%s</NextMarker>`, nextMarker)
	}

	resp := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<FunctionList xmlns="%s">`+
		`<MaxItems>%d</MaxItems>`+
		`<Quantity>%d</Quantity>`+
		`%s`+
		`<Items>%s</Items>`+
		`</FunctionList>`,
		cfNS, pageSize, len(page), nextMarkerXML, sb.String())

	return xmlResp(c, http.StatusOK, resp)
}

func (h *Handler) handlePublishFunction(c *echo.Context, name string) error {
	current, getErr := h.Backend.GetFunction(name)
	if getErr != nil {
		return h.handleError(c, getErr)
	}

	ifMatch := c.Request().Header.Get("If-Match")
	if ifMatch == "" || ifMatch != current.ETag {
		return xmlResp(
			c,
			http.StatusPreconditionFailed,
			cfErrorXML(
				"PreconditionFailed",
				"If-Match ETag did not match the current function ETag",
			),
		)
	}

	fn, err := h.Backend.PublishFunction(name)
	if err != nil {
		return h.handleError(c, err)
	}

	c.Response().Header().Set("ETag", fn.ETag)

	return xmlResp(c, http.StatusCreated, functionResponseXML(fn))
}

func (h *Handler) handleUpdateFunction(c *echo.Context, name string) error {
	current, getErr := h.Backend.GetFunction(name)
	if getErr != nil {
		return h.handleError(c, getErr)
	}

	ifMatch := c.Request().Header.Get("If-Match")
	if ifMatch == "" || ifMatch != current.ETag {
		return xmlResp(
			c,
			http.StatusPreconditionFailed,
			cfErrorXML(
				"PreconditionFailed",
				"If-Match ETag did not match the current function ETag",
			),
		)
	}

	body, err := readBody(c)
	if err != nil {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("MalformedXML", "failed to read body"))
	}

	if qErr := validateFunctionConfigQuantities(body); qErr != nil {
		return h.handleError(c, qErr)
	}

	var req updateFunctionRequestXML
	if len(body) > 0 {
		if xmlErr := xml.Unmarshal(body, &req); xmlErr != nil {
			return xmlResp(
				c,
				http.StatusBadRequest,
				cfErrorXML("MalformedXML", "invalid UpdateFunctionRequest XML"),
			)
		}
	}

	code := req.FunctionCode
	if code == "" {
		code = req.FunctionConfig.FunctionCode
	}

	fn, updateErr := h.Backend.UpdateFunction(
		name,
		req.FunctionConfig.Comment,
		req.FunctionConfig.Runtime,
		code,
	)
	if updateErr != nil {
		return h.handleError(c, updateErr)
	}

	c.Response().Header().Set("ETag", fn.ETag)

	return xmlResp(c, http.StatusOK, functionResponseXML(fn))
}

func (h *Handler) handleDeleteFunction(c *echo.Context, name string) error {
	current, getErr := h.Backend.GetFunction(name)
	if getErr != nil {
		return h.handleError(c, getErr)
	}

	ifMatch := c.Request().Header.Get("If-Match")
	if ifMatch == "" || ifMatch != current.ETag {
		return xmlResp(
			c,
			http.StatusPreconditionFailed,
			cfErrorXML(
				"PreconditionFailed",
				"If-Match ETag did not match the current function ETag",
			),
		)
	}

	if err := h.Backend.DeleteFunction(name); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// testFunctionRequestXML models the real TestFunctionRequest body (cloudfront@v1.67.4
// serializers.go:11847, awsRestxml_serializeOpDocumentTestFunctionInput). EventObject is
// base64-encoded binary on the wire (el.Base64EncodeBytes); encoding/xml does NOT decode
// base64 automatically on unmarshal (unlike the SDK client's own encode-on-send), so it's kept
// as the raw wire string here and decoded explicitly below.
type testFunctionRequestXML struct {
	XMLName     xml.Name `xml:"TestFunctionRequest"`
	EventObject string   `xml:"EventObject"`
	Stage       string   `xml:"Stage"`
}

// handleTestFunction validates the request (function exists, If-Match matches, EventObject is
// present and well-formed JSON) and then reports a structural gap: gopherstack vendors no
// JavaScript engine, so it cannot genuinely execute the function against the event and refuses
// to fabricate FunctionOutput/logs that would look like real execution. TestFunctionFailed is
// TestFunction's own declared error for exactly this case ("the CloudFront function failed",
// HTTP 500 per the API reference) -- distinct from InvalidArgument/InvalidIfMatchVersion, which
// cover malformed requests rather than an execution failure.
func (h *Handler) handleTestFunction(c *echo.Context, name string) error {
	current, err := h.Backend.GetFunction(name)
	if err != nil {
		return h.handleError(c, err)
	}

	ifMatch := c.Request().Header.Get("If-Match")
	if ifMatch == "" || ifMatch != current.ETag {
		return xmlResp(
			c,
			http.StatusBadRequest,
			cfErrorXML("InvalidIfMatchVersion", "the If-Match version is missing or not valid"),
		)
	}

	body, err := readBody(c)
	if err != nil {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("MalformedXML", "failed to read body"))
	}

	var req testFunctionRequestXML
	if len(body) > 0 {
		if xmlErr := xml.Unmarshal(body, &req); xmlErr != nil {
			return xmlResp(
				c,
				http.StatusBadRequest,
				cfErrorXML("MalformedXML", "invalid TestFunctionRequest XML"),
			)
		}
	}

	if req.EventObject == "" {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("InvalidArgument", "EventObject is required"))
	}

	eventObject, decodeErr := base64.StdEncoding.DecodeString(req.EventObject)
	if decodeErr != nil {
		return xmlResp(
			c,
			http.StatusBadRequest,
			cfErrorXML("InvalidArgument", "EventObject must be base64-encoded"),
		)
	}

	if !json.Valid(eventObject) {
		return xmlResp(c, http.StatusBadRequest, cfErrorXML("InvalidArgument", "EventObject must be valid JSON"))
	}

	return xmlResp(
		c,
		http.StatusInternalServerError,
		cfErrorXML(
			"TestFunctionFailed",
			"gopherstack does not implement a JavaScript engine and cannot execute CloudFront "+
				"Function code; TestFunction is a structural parity gap (see PARITY.md)",
		),
	)
}

func functionResponseXML(fn *Function) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<FunctionSummary xmlns="%s">`+
		`<Name>%s</Name>`+
		`<Status>%s</Status>`+
		`<FunctionConfig>`+
		`<Comment>%s</Comment>`+
		`<Runtime>%s</Runtime>`+
		`</FunctionConfig>`+
		`<FunctionMetadata>`+
		`<FunctionARN>%s</FunctionARN>`+
		`<Stage>%s</Stage>`+
		`<CreatedTime>%s</CreatedTime>`+
		`<LastModifiedTime>%s</LastModifiedTime>`+
		`</FunctionMetadata>`+
		`</FunctionSummary>`,
		cfNS, fn.Name, fn.Status, fn.Comment, fn.Runtime,
		fn.ARN, fn.Status, fn.CreatedTime, fn.LastModifiedTime)
}

// --- Origin Request Policy handlers ---
