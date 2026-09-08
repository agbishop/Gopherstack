package cloudwatch

import (
	"errors"
	"net/http"
	"strings"

	"github.com/aws/smithy-go/encoding/cbor"
	"github.com/labstack/echo/v5"
)

// dashboardValidationMessagesCBOR converts DashboardValidationMessage entries
// to their rpc-v2-cbor wire shape (DataPath + Message only, no IsError member —
// matches aws-sdk-go-v2's types.DashboardValidationMessage).
func dashboardValidationMessagesCBOR(msgs []DashboardValidationMessage) cbor.List {
	out := make(cbor.List, 0, len(msgs))
	for _, m := range msgs {
		entry := cbor.Map{"Message": cbor.String(m.Message)}
		if m.DataPath != "" {
			entry["DataPath"] = cbor.String(m.DataPath)
		}
		out = append(out, entry)
	}

	return out
}

func (h *Handler) cborPutDashboard(input cbor.Map, c *echo.Context) error {
	name := cborStr(input, "DashboardName")
	if name == "" {
		return h.cborError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"DashboardName is required",
		)
	}

	body := cborStr(input, "DashboardBody")

	messages, err := h.Backend.PutDashboard(name, body)
	if err != nil {
		if valErr, ok := errors.AsType[*DashboardValidationError](err); ok {
			return h.cborDashboardValidationError(c, valErr.Messages)
		}

		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	if entry, _, entryErr := h.Backend.GetDashboard(name); entryErr == nil {
		h.applyCreationTags(input, entry.DashboardArn)
	}

	return writeCBOR(c, cbor.Map{"DashboardValidationMessages": dashboardValidationMessagesCBOR(messages)})
}

// cborDashboardValidationError writes the DashboardInvalidInputError error
// response for a PutDashboard call whose body failed schema validation,
// embedding the full DashboardValidationMessages list in the error body
// (matches the SDK's DashboardInvalidInputError exception shape).
func (h *Handler) cborDashboardValidationError(
	c *echo.Context,
	messages []DashboardValidationMessage,
) error {
	summary := make([]string, 0, len(messages))
	for _, m := range messages {
		if m.IsError {
			summary = append(summary, m.Message)
		}
	}

	c.Response().Header().Set("Content-Type", "application/cbor")
	c.Response().Header().Set("Smithy-Protocol", "rpc-v2-cbor")
	c.Response().Header().Set("X-Amzn-Errortype", "DashboardInvalidInputError")
	c.Response().WriteHeader(http.StatusBadRequest)
	_, err := c.Response().Write(cbor.Encode(cbor.Map{
		"message":                     cbor.String("The dashboard body is invalid: " + strings.Join(summary, "; ")),
		"DashboardValidationMessages": dashboardValidationMessagesCBOR(messages),
	}))

	return err
}

func (h *Handler) cborGetDashboard(input cbor.Map, c *echo.Context) error {
	name := cborStr(input, "DashboardName")
	if name == "" {
		return h.cborError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"DashboardName is required",
		)
	}

	entry, body, err := h.Backend.GetDashboard(name)
	if err != nil {
		return h.cborError(c, http.StatusBadRequest, "ResourceNotFoundException", err.Error())
	}

	return writeCBOR(c, cbor.Map{
		"DashboardArn":  cbor.String(entry.DashboardArn),
		"DashboardBody": cbor.String(body),
		"DashboardName": cbor.String(entry.DashboardName),
	})
}

func (h *Handler) cborListDashboards(input cbor.Map, c *echo.Context) error {
	prefix := cborStr(input, "DashboardNamePrefix")
	nextToken := cborStr(input, "NextToken")

	p, err := h.Backend.ListDashboards(prefix, nextToken)
	if err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	entries := make(cbor.List, 0, len(p.Data))
	for _, e := range p.Data {
		entries = append(entries, cbor.Map{
			"DashboardArn":  cbor.String(e.DashboardArn),
			"DashboardName": cbor.String(e.DashboardName),
			"LastModified":  cborFromTime(e.LastModified),
			"Size": cbor.Uint(
				uint64(e.Size), //nolint:gosec // Size is always non-negative (len of body)
			),
		})
	}

	resp := cbor.Map{"DashboardEntries": entries}
	if p.Next != "" {
		resp["NextToken"] = cbor.String(p.Next)
	}

	return writeCBOR(c, resp)
}

func (h *Handler) cborDeleteDashboards(input cbor.Map, c *echo.Context) error {
	names := cborStrList(input, "DashboardNames")

	if b, ok := h.Backend.(*InMemoryBackend); ok {
		for _, a := range b.GetDashboardARNs(names) {
			h.deleteResourceTags(a)
		}
	}

	if err := h.Backend.DeleteDashboards(names); err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	return writeCBOR(c, cbor.Map{})
}
