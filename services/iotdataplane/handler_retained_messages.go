package iotdataplane

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// handleGetRetainedMessage processes GET /retainedMessage/{topic} requests.
func (h *Handler) handleGetRetainedMessage(c *echo.Context) error {
	if c.Request().Method != http.MethodGet {
		return methodNotAllowedResponse(c)
	}

	topic := strings.TrimPrefix(c.Request().URL.Path, retainedMessagePathSlash)
	if topic == "" {
		return invalidRequestResponse(c, "topic is required")
	}

	msg, err := h.Backend.GetRetainedMessage(topic)
	if err != nil {
		return h.handleError(c, err)
	}

	// Typed response per AWS GetRetainedMessageOutput shape; payload and
	// userProperties are base64 encoded by json.Marshal ([]byte -> string).
	// userProperties serializes as JSON null when nil, matching AWS docs
	// ("...or null if the retained message doesn't include any user properties").
	resp := map[string]any{
		"topic":            msg.Topic,
		"payload":          msg.Payload,
		"qos":              msg.Qos,
		"lastModifiedTime": msg.LastModifiedTime,
		"userProperties":   msg.UserProperties,
	}

	return c.JSON(http.StatusOK, resp)
}

// handleListRetainedMessages processes GET /retainedMessage requests.
// Pagination: pageSize (primary) or maxResults (alias); default 25, max 100.
func (h *Handler) handleListRetainedMessages(c *echo.Context) error {
	if c.Request().Method != http.MethodGet {
		return methodNotAllowedResponse(c)
	}

	msgs, err := h.Backend.ListRetainedMessages()
	if err != nil {
		return h.handleError(c, err)
	}

	q := c.Request().URL.Query()
	nextTokenIn := q.Get("nextToken")
	pageSize := parsePageSize(q, defaultPageSize)

	// Extract topic strings for cursor lookup.
	topics := make([]string, len(msgs))
	for i, m := range msgs {
		topics[i] = m.Topic
	}

	startIdx := findCursorIndex(topics, nextTokenIn)
	end := min(startIdx+pageSize, len(msgs))

	page := msgs[startIdx:end]

	// AWS RetainedMessageSummary: {topic, payloadSize, qos, lastModifiedTime}
	// (confirmed against awsRestjson1_deserializeDocumentRetainedMessageSummary
	// in the SDK's deserializers.go -- qos IS present on the summary shape).
	summaries := make([]map[string]any, 0, len(page))
	for _, msg := range page {
		summaries = append(summaries, map[string]any{
			"topic":            msg.Topic,
			"payloadSize":      int64(len(msg.Payload)),
			"qos":              msg.Qos,
			"lastModifiedTime": msg.LastModifiedTime,
		})
	}

	resp := map[string]any{
		"retainedTopics": summaries,
	}

	if end < len(msgs) {
		resp["nextToken"] = msgs[end].Topic
	}

	return c.JSON(http.StatusOK, resp)
}
