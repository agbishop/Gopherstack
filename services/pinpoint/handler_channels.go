package pinpoint

import (
	"encoding/json"
	"maps"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

func (h *Handler) extractChannelOp(method, channelType string) string {
	if channelType == "" {
		return unknownOperation
	}

	switch method {
	case http.MethodGet:
		return "Get" + channelTypeOpName(channelType) + "Channel"
	case http.MethodPut:
		return "Update" + channelTypeOpName(channelType) + "Channel"
	case http.MethodDelete:
		return "Delete" + channelTypeOpName(channelType) + "Channel"
	}

	return unknownOperation
}

// channelTypeOpName converts a URL channel type segment to the AWS op suffix.
// e.g. "adm" → "Adm", "apns" → "Apns", "apns_sandbox" → "ApnsSandbox".
func channelTypeOpName(channelType string) string {
	switch strings.ToLower(channelType) {
	case channelKeyADM:
		return "Adm"
	case channelKeyAPNS:
		return "Apns"
	case channelKeyAPNSSandbox:
		return "ApnsSandbox"
	case channelKeyAPNSVoip:
		return "ApnsVoip"
	case channelKeyAPNSVoipSandbox:
		return "ApnsVoipSandbox"
	case channelKeyBaidu:
		return "Baidu"
	case templateTypeEmail:
		return "Email"
	case channelKeyGCM:
		return "Gcm"
	case templateTypeSMS:
		return "Sms"
	case templateTypeVoice:
		return "Voice"
	}

	return channelType
}

func (h *Handler) dispatchChannelByType(c *echo.Context, appID, channelType string) error {
	switch c.Request().Method {
	case http.MethodGet:
		return h.handleGetChannel(c, appID, channelType)
	case http.MethodPut:
		return h.handleUpdateChannel(c, appID, channelType)
	case http.MethodDelete:
		return h.handleDeleteChannel(c, appID, channelType)
	}

	return writeErrorResponse(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
}

// filterChannelExtraForEcho returns only the subset of extra that the real
// SDK's *ChannelResponse type for channelType actually declares, renamed to
// the real wire key where it differs from the request-side extra key
// (types.go: GCMChannelResponse and BaiduChannelResponse both call the
// credential "Credential", not the request's "ApiKey"). Credential/secret
// extras with no response member at all -- ADM's ClientId/ClientSecret,
// Baidu's SecretKey, APNS's BundleId/Certificate/TeamId/TokenKey/TokenKeyId,
// GCM's ServiceJson -- are dropped entirely: the real types echo only the
// HasCredential/HasTokenKey/HasFcmServiceCredentials booleans, never the raw
// secret.
func filterChannelExtraForEcho(channelType string, extra map[string]any) map[string]any {
	if extra == nil {
		return nil
	}

	switch strings.ToLower(channelType) {
	case templateTypeEmail, templateTypeSMS:
		return extra // every extra key here is already a real response member
	case channelKeyGCM:
		const gcmEchoFieldCount = 2

		out := make(map[string]any, gcmEchoFieldCount)

		if v, ok := extra[extraKeyDefaultAuthenticationMethod]; ok {
			out[extraKeyDefaultAuthenticationMethod] = v
		}

		if v, ok := extra[extraKeyAPIKey]; ok {
			out[wireKeyCredential] = v
		}

		return out
	case channelKeyBaidu:
		out := make(map[string]any, 1)

		if v, ok := extra[extraKeyAPIKey]; ok {
			out[wireKeyCredential] = v
		}

		return out
	case channelKeyAPNS, channelKeyAPNSSandbox, channelKeyAPNSVoip, channelKeyAPNSVoipSandbox:
		out := make(map[string]any, 1)

		if v, ok := extra[extraKeyDefaultAuthenticationMethod]; ok {
			out[extraKeyDefaultAuthenticationMethod] = v
		}

		return out
	default:
		return nil // ADM and others: no extra field is a real response member
	}
}

// toChannelResponse converts a Channel to its wire format including per-type extra fields.
func toChannelResponse(ch *Channel) map[string]any {
	resp := map[string]any{
		"ApplicationId":    ch.ApplicationID,
		"ChannelType":      ch.ChannelType,
		"Platform":         ch.Platform,
		"Enabled":          ch.Enabled,
		"IsArchived":       ch.IsArchived,
		"Version":          ch.Version,
		"CreationDate":     ch.CreationDate,
		"LastModifiedDate": ch.LastModifiedDate,
	}

	if ch.HasCredential {
		resp["HasCredential"] = true
	}

	if ch.HasTokenKey {
		resp["HasTokenKey"] = true
	}

	if ch.HasFcmServiceCredentials {
		resp["HasFcmServiceCredentials"] = true
	}

	if ch.MessagesPerSecond > 0 {
		resp["MessagesPerSecond"] = ch.MessagesPerSecond
	}

	maps.Copy(resp, filterChannelExtraForEcho(ch.ChannelType, ch.ExtraData))

	return resp
}

// handleGetChannel handles GET /v1/apps/{appId}/channels/{channelType}.
func (h *Handler) handleGetChannel(c *echo.Context, appID, channelType string) error {
	ch := h.Backend.GetChannel(appID, channelType)
	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toChannelResponse(ch))

	return nil
}

// handleGetChannels handles GET /v1/apps/{appId}/channels.
func (h *Handler) handleGetChannels(c *echo.Context, appID string) error {
	channels := h.Backend.GetAllChannels(appID)
	chMap := make(map[string]map[string]any, len(channels))

	for _, ch := range channels {
		chMap[ch.ChannelType] = toChannelResponse(ch)
	}

	return c.JSON(http.StatusOK, map[string]any{"Channels": chMap})
}

func parseGCMChannelExtra(body []byte) (bool, map[string]any) {
	var req updateGCMChannelRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return false, nil
	}

	extra := map[string]any{extraKeyDefaultAuthenticationMethod: req.DefaultAuthenticationMethod}

	if req.APIKey != "" {
		extra[extraKeyAPIKey] = req.APIKey
	}

	if req.ServiceJSON != "" {
		extra[extraKeyServiceJSON] = req.ServiceJSON
	}

	return req.Enabled, extra
}

func parseAPNSChannelExtra(body []byte) (bool, map[string]any) {
	var req updateAPNSChannelRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return false, nil
	}

	extra := map[string]any{extraKeyDefaultAuthenticationMethod: req.DefaultAuthenticationMethod}

	for k, v := range map[string]string{
		extraKeyBundleID: req.BundleID, extraKeyCertificate: req.Certificate,
		extraKeyTeamID: req.TeamID, extraKeyTokenKey: req.TokenKey, extraKeyTokenKeyID: req.TokenKeyID,
	} {
		if v != "" {
			extra[k] = v
		}
	}

	return req.Enabled, extra
}

func parseEmailChannelExtra(body []byte) (bool, map[string]any) {
	var req updateEmailChannelRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return false, nil
	}

	extra := map[string]any{}

	for k, v := range map[string]string{
		extraKeyFromAddress: req.FromAddress, "Identity": req.Identity,
		"RoleArn": req.RoleArn, "ConfigurationSet": req.ConfigurationSet,
		"OrchestrationSendingRoleArn": req.OrchestrationSendingRoleArn,
	} {
		if v != "" {
			extra[k] = v
		}
	}

	return req.Enabled, extra
}

func parseSMSChannelExtra(body []byte) (bool, map[string]any) {
	var req updateSMSChannelRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return false, nil
	}

	extra := map[string]any{}

	if req.SenderID != "" {
		extra["SenderId"] = req.SenderID
	}

	if req.ShortCode != "" {
		extra["ShortCode"] = req.ShortCode
	}

	return req.Enabled, extra
}

// parseChannelExtra extracts per-channel extra fields from the request body.
func parseChannelExtra(channelType string, body []byte) (bool, map[string]any) {
	switch strings.ToLower(channelType) {
	case channelKeyGCM:
		return parseGCMChannelExtra(body)
	case channelKeyAPNS, channelKeyAPNSSandbox, channelKeyAPNSVoip, channelKeyAPNSVoipSandbox:
		return parseAPNSChannelExtra(body)
	case templateTypeEmail:
		return parseEmailChannelExtra(body)
	case templateTypeSMS:
		return parseSMSChannelExtra(body)
	case channelKeyADM:
		var req updateADMChannelRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return false, nil
		}

		extra := map[string]any{}

		if req.ClientID != "" {
			extra[extraKeyClientID] = req.ClientID
		}

		if req.ClientSecret != "" {
			extra[extraKeyClientSecret] = req.ClientSecret
		}

		return req.Enabled, extra
	case channelKeyBaidu:
		var req updateBaiduChannelRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return false, nil
		}

		extra := map[string]any{}

		if req.APIKey != "" {
			extra[extraKeyAPIKey] = req.APIKey
		}

		if req.SecretKey != "" {
			extra[extraKeySecretKey] = req.SecretKey
		}

		return req.Enabled, extra
	default:
		var req updateChannelRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return false, nil
		}

		return req.Enabled, nil
	}
}

// handleUpdateChannel handles PUT /v1/apps/{appId}/channels/{channelType}.
func (h *Handler) handleUpdateChannel(c *echo.Context, appID, channelType string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	enabled, extra := parseChannelExtra(channelType, body)
	ch := h.Backend.UpsertChannel(appID, channelType, enabled, extra)
	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toChannelResponse(ch))

	return nil
}

// handleDeleteChannel handles DELETE /v1/apps/{appId}/channels/{channelType}.
func (h *Handler) handleDeleteChannel(c *echo.Context, appID, channelType string) error {
	ch := h.Backend.DeleteChannel(appID, channelType)
	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toChannelResponse(ch))

	return nil
}

// ──────────────────────────────────────────────────
// Campaign handlers
// ──────────────────────────────────────────────────
