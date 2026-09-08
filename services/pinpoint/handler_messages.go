package pinpoint

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// handleSendMessages handles POST /v1/apps/{appId}/messages.
func (h *Handler) handleSendMessages(c *echo.Context, appID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req sendMessagesRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
	}

	resp, backendErr := h.Backend.SendMessages(appID, req)
	if backendErr != nil {
		if errors.Is(backendErr, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", backendErr.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", backendErr.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleSendUsersMessages handles POST /v1/apps/{appId}/users-messages.
func (h *Handler) handleSendUsersMessages(c *echo.Context, appID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req sendUsersMessagesRequest
	if len(body) > 0 {
		if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
			return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
		}
	}

	resp, backendErr := h.Backend.SendUsersMessages(appID, req)
	if backendErr != nil {
		if errors.Is(backendErr, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", backendErr.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", backendErr.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleSendOTPMessage handles POST /v1/apps/{appId}/otp.
func (h *Handler) handleSendOTPMessage(c *echo.Context, appID string) error {
	body, _ := httputils.ReadBody(c.Request())

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	resp, err := h.Backend.SendOTPMessage(appID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleVerifyOTPMessage handles POST /v1/apps/{appId}/verify-otp.
func (h *Handler) handleVerifyOTPMessage(c *echo.Context, appID string) error {
	body, _ := httputils.ReadBody(c.Request())

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req verifyOTPMessageRequest
	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}

	code := req.Otp

	resp, err := h.Backend.VerifyOTPMessage(appID, code)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handlePhoneNumberValidate handles POST /v1/phone/number/validate.
func (h *Handler) handlePhoneNumberValidate(c *echo.Context) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req phoneNumberValidateRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
	}

	resp, backendErr := h.Backend.PhoneNumberValidate(req.PhoneNumber)
	if backendErr != nil {
		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", backendErr.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}
