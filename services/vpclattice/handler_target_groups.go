package vpclattice

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// ------- TargetGroup handlers -------

func (h *Handler) handleCreateTargetGroup(c *echo.Context, body map[string]any) error {
	name, _ := body[keyName].(string)
	tgType, _ := body[keyType].(string)

	if name == "" || tgType == "" {
		return validationError(c, "name and type are required")
	}

	ctx := c.Request().Context()
	config := extractTargetGroupConfig(body)
	tags := extractTags(body)

	tg, err := h.Backend.CreateTargetGroup(ctx, name, tgType, config, tags)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusCreated, targetGroupToJSON(tg))
}

func (h *Handler) handleGetTargetGroup(c *echo.Context, id string) error {
	tg, err := h.Backend.GetTargetGroup(id)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, targetGroupToJSON(tg))
}

func (h *Handler) handleUpdateTargetGroup(c *echo.Context, id string, body map[string]any) error {
	var healthCheck *HealthCheckConfig
	if hcRaw, ok := body["healthCheck"].(map[string]any); ok {
		healthCheck = extractHealthCheckConfig(hcRaw)
	}

	tg, err := h.Backend.UpdateTargetGroup(id, healthCheck)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, targetGroupToJSON(tg))
}

func (h *Handler) handleDeleteTargetGroup(c *echo.Context, id string) error {
	if err := h.Backend.DeleteTargetGroup(id); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"id": id, keyStatus: statusDeleteInProgress})
}

func (h *Handler) handleListTargetGroups(c *echo.Context) error {
	ctx := c.Request().Context()
	maxResults := queryInt32(c)
	nextToken := c.QueryParam("nextToken")
	tgType := c.QueryParam("targetGroupType")
	vpcID := c.QueryParam("vpcIdentifier")

	items, next, err := h.Backend.ListTargetGroups(ctx, tgType, vpcID, maxResults, nextToken)
	if err != nil {
		return h.handleError(c, err)
	}

	summaries := make([]any, 0, len(items))
	for _, tg := range items {
		summaries = append(summaries, targetGroupSummaryToJSON(tg))
	}

	resp := map[string]any{keyItems: summaries}
	if next != "" {
		resp["nextToken"] = next
	}

	return c.JSON(http.StatusOK, resp)
}

// ------- TargetGroup JSON serialization -------

func targetGroupToJSON(tg *TargetGroup) map[string]any {
	m := map[string]any{
		keyARN:           tg.ARN,
		"id":             tg.ID,
		keyName:          tg.Name,
		keyType:          tg.Type,
		keyStatus:        tg.Status,
		"serviceArns":    tg.ServiceARNs,
		keyCreatedAt:     tg.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		keyLastUpdatedAt: tg.LastUpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}

	if tg.Config != nil {
		m["config"] = targetGroupConfigToJSON(tg.Config)
	}

	return m
}

func targetGroupSummaryToJSON(tg *TargetGroupSummary) map[string]any {
	m := map[string]any{
		keyARN:           tg.ARN,
		"id":             tg.ID,
		keyName:          tg.Name,
		keyType:          tg.Type,
		keyStatus:        tg.Status,
		keyPort:          tg.Port,
		keyProtocol:      tg.Protocol,
		keyVpcIdentifier: tg.VpcID,
		"serviceArns":    tg.ServiceARNs,
		keyCreatedAt:     tg.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		keyLastUpdatedAt: tg.LastUpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}

	if tg.IPAddressType != "" {
		m[keyIPAddressType] = tg.IPAddressType
	}

	if tg.LambdaEventStructureVersion != "" {
		m["lambdaEventStructureVersion"] = tg.LambdaEventStructureVersion
	}

	return m
}

func targetGroupConfigToJSON(c *TargetGroupConfig) map[string]any {
	m := map[string]any{
		keyPort:           c.Port,
		keyProtocol:       c.Protocol,
		"protocolVersion": c.ProtocolVersion,
		keyVpcIdentifier:  c.VpcID,
	}

	if c.IPAddressType != "" {
		m[keyIPAddressType] = c.IPAddressType
	}

	if c.LambdaEventStructureVersion != "" {
		m["lambdaEventStructureVersion"] = c.LambdaEventStructureVersion
	}

	if c.HealthCheck != nil {
		m["healthCheck"] = healthCheckToJSON(c.HealthCheck)
	}

	return m
}

func healthCheckToJSON(hc *HealthCheckConfig) map[string]any {
	m := map[string]any{
		"enabled":                    hc.Enabled,
		keyProtocol:                  hc.Protocol,
		"protocolVersion":            hc.ProtocolVersion,
		"path":                       hc.Path,
		keyPort:                      hc.Port,
		"healthyThresholdCount":      hc.HealthyThresholdCount,
		"unhealthyThresholdCount":    hc.UnhealthyThresholdCount,
		"healthCheckIntervalSeconds": hc.HealthCheckIntervalSeconds,
		"healthCheckTimeoutSeconds":  hc.HealthCheckTimeoutSeconds,
	}

	if hc.MatcherHTTPCode != "" {
		m["matcher"] = map[string]any{"httpCode": hc.MatcherHTTPCode}
	}

	return m
}

// ------- TargetGroup body extraction helpers -------

func extractTargetGroupConfig(body map[string]any) *TargetGroupConfig {
	raw, ok := body["config"].(map[string]any)
	if !ok {
		return nil
	}

	cfg := &TargetGroupConfig{}
	cfg.Port = bodyInt32(raw, keyPort)
	cfg.Protocol, _ = raw[keyProtocol].(string)
	cfg.ProtocolVersion, _ = raw["protocolVersion"].(string)
	cfg.VpcID, _ = raw[keyVpcIdentifier].(string)
	cfg.IPAddressType, _ = raw[keyIPAddressType].(string)
	cfg.LambdaEventStructureVersion, _ = raw["lambdaEventStructureVersion"].(string)

	if hcRaw, ok2 := raw["healthCheck"].(map[string]any); ok2 {
		cfg.HealthCheck = extractHealthCheckConfig(hcRaw)
	}

	return cfg
}

func extractHealthCheckConfig(raw map[string]any) *HealthCheckConfig {
	hc := &HealthCheckConfig{}
	if v, ok := raw["enabled"].(bool); ok {
		hc.Enabled = v
	}

	hc.Protocol, _ = raw[keyProtocol].(string)
	hc.ProtocolVersion, _ = raw["protocolVersion"].(string)
	hc.Path, _ = raw["path"].(string)
	hc.Port = bodyInt32(raw, keyPort)

	if matcher, ok2 := raw["matcher"].(map[string]any); ok2 {
		hc.MatcherHTTPCode, _ = matcher["httpCode"].(string)
	}

	hc.HealthyThresholdCount = bodyInt32(raw, "healthyThresholdCount")
	hc.UnhealthyThresholdCount = bodyInt32(raw, "unhealthyThresholdCount")
	hc.HealthCheckIntervalSeconds = bodyInt32(raw, "healthCheckIntervalSeconds")
	hc.HealthCheckTimeoutSeconds = bodyInt32(raw, "healthCheckTimeoutSeconds")

	return hc
}
