package cloudwatch

import (
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// dashboardValidationMessageXML is the XML wire shape of a single
// DashboardValidationMessage (DataPath + Message only — no error/warning
// discriminator on the wire, matching aws-sdk-go-v2's types.DashboardValidationMessage).
type dashboardValidationMessageXML struct {
	DataPath string `xml:"DataPath,omitempty"`
	Message  string `xml:"Message"`
}

func dashboardValidationMessagesXML(msgs []DashboardValidationMessage) []dashboardValidationMessageXML {
	out := make([]dashboardValidationMessageXML, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, dashboardValidationMessageXML{DataPath: m.DataPath, Message: m.Message})
	}

	return out
}

func (h *Handler) handlePutDashboard(form url.Values, c *echo.Context) error {
	name := form.Get("DashboardName")
	if name == "" {
		return h.xmlError(
			c,
			http.StatusBadRequest,
			"InvalidParameterInput",
			"DashboardName is required",
		)
	}
	body := form.Get("DashboardBody")

	messages, err := h.Backend.PutDashboard(name, body)
	if err != nil {
		if valErr, ok := errors.AsType[*DashboardValidationError](err); ok {
			return h.xmlDashboardValidationError(c, valErr.Messages)
		}

		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type response struct {
		XMLName   xml.Name `xml:"PutDashboardResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
		//nolint:lll // XML path tag cannot be wrapped
		DashboardValidationMessages []dashboardValidationMessageXML `xml:"PutDashboardResult>DashboardValidationMessages>member"`
	}

	return writeXML(c, response{
		Xmlns:                       cloudwatchNS,
		RequestID:                   uuid.New().String(),
		DashboardValidationMessages: dashboardValidationMessagesXML(messages),
	})
}

// xmlDashboardValidationError writes the DashboardInvalidInputError error
// response for a PutDashboard call whose body failed schema validation. Unlike
// h.xmlError, this embeds the full DashboardValidationMessages list inside the
// <Error> element, matching the SDK's DashboardInvalidInputError exception shape.
func (h *Handler) xmlDashboardValidationError(
	c *echo.Context,
	messages []DashboardValidationMessage,
) error {
	type errorBody struct {
		XMLName                     xml.Name                        `xml:"ErrorResponse"`
		Code                        string                          `xml:"Error>Code"`
		Message                     string                          `xml:"Error>Message"`
		RequestID                   string                          `xml:"RequestId"`
		DashboardValidationMessages []dashboardValidationMessageXML `xml:"Error>DashboardValidationMessages>member"`
	}

	summary := make([]string, 0, len(messages))
	for _, m := range messages {
		if m.IsError {
			summary = append(summary, m.Message)
		}
	}

	w := c.Response()
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusBadRequest)
	enc := xml.NewEncoder(w)
	_ = enc.Encode(errorBody{
		Code:                        "DashboardInvalidInputError",
		Message:                     "The dashboard body is invalid: " + strings.Join(summary, "; "),
		DashboardValidationMessages: dashboardValidationMessagesXML(messages),
		RequestID:                   uuid.New().String(),
	})

	return nil
}

func (h *Handler) handleGetDashboard(form url.Values, c *echo.Context) error {
	name := form.Get("DashboardName")
	if name == "" {
		return h.xmlError(
			c,
			http.StatusBadRequest,
			"InvalidParameterInput",
			"DashboardName is required",
		)
	}

	entry, body, err := h.Backend.GetDashboard(name)
	if err != nil {
		return h.xmlError(c, http.StatusBadRequest, "ResourceNotFoundException", err.Error())
	}

	type result struct {
		DashboardArn  string `xml:"DashboardArn"`
		DashboardBody string `xml:"DashboardBody"`
		DashboardName string `xml:"DashboardName"`
	}
	type response struct {
		XMLName   xml.Name `xml:"GetDashboardResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
		Result    result   `xml:"GetDashboardResult"`
	}

	return writeXML(c, response{
		Xmlns:     cloudwatchNS,
		RequestID: uuid.New().String(),
		Result: result{
			DashboardArn:  entry.DashboardArn,
			DashboardBody: body,
			DashboardName: entry.DashboardName,
		},
	})
}

func (h *Handler) handleListDashboards(form url.Values, c *echo.Context) error {
	prefix := form.Get("DashboardNamePrefix")
	nextToken := form.Get("NextToken")

	p, err := h.Backend.ListDashboards(prefix, nextToken)
	if err != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type entryXML struct {
		DashboardArn  string `xml:"DashboardArn"`
		DashboardName string `xml:"DashboardName"`
		LastModified  string `xml:"LastModified"`
		Size          int64  `xml:"Size"`
	}
	members := make([]entryXML, 0, len(p.Data))
	for _, e := range p.Data {
		members = append(members, entryXML{
			DashboardArn:  e.DashboardArn,
			DashboardName: e.DashboardName,
			LastModified:  e.LastModified.UTC().Format(time.RFC3339),
			Size:          e.Size,
		})
	}

	type listResult struct {
		NextToken        string     `xml:"NextToken,omitempty"`
		DashboardEntries []entryXML `xml:"DashboardEntries>member"`
	}
	type response struct {
		XMLName   xml.Name   `xml:"ListDashboardsResponse"`
		Xmlns     string     `xml:"xmlns,attr"`
		RequestID string     `xml:"ResponseMetadata>RequestId"`
		Result    listResult `xml:"ListDashboardsResult"`
	}

	return writeXML(c, response{
		Xmlns:     cloudwatchNS,
		RequestID: uuid.New().String(),
		Result:    listResult{DashboardEntries: members, NextToken: p.Next},
	})
}

func (h *Handler) handleDeleteDashboards(form url.Values, c *echo.Context) error {
	names := parseMemberList(form, "DashboardNames.")

	// Clean up tag entries for dashboards being deleted.
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		for _, arn := range b.GetDashboardARNs(names) {
			h.deleteResourceTags(arn)
		}
	}

	if err := h.Backend.DeleteDashboards(names); err != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type response struct {
		XMLName   xml.Name       `xml:"DeleteDashboardsResponse"`
		Result    xmlEmptyResult `xml:"DeleteDashboardsResult"`
		Xmlns     string         `xml:"xmlns,attr"`
		RequestID string         `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cloudwatchNS, RequestID: uuid.New().String()})
}
