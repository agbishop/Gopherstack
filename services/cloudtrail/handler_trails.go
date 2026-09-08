package cloudtrail

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// defaultTrailsPageSize is used when ListTrails omits MaxResults (ListTrails
// has no MaxResults input field in the real API, only NextToken, so every
// call effectively requests the default page).
const defaultTrailsPageSize = 1000

// maxS3KeyPrefixLength is CreateTrailInput.S3KeyPrefix's documented bound
// (cloudtrail@v1.58.4 types/types.go:912-914: "The maximum length is 200
// characters.").
const maxS3KeyPrefixLength = 200

const s3KeyPrefixTooLongMsg = "S3KeyPrefix exceeds maximum length of 200 characters"

// --- CreateTrail ---

type createTrailBody struct {
	Name                      string `json:"Name"`
	S3BucketName              string `json:"S3BucketName"`
	S3KeyPrefix               string `json:"S3KeyPrefix"`
	SnsTopicName              string `json:"SnsTopicName"`
	CloudWatchLogsLogGroupArn string `json:"CloudWatchLogsLogGroupArn"`
	CloudWatchLogsRoleArn     string `json:"CloudWatchLogsRoleArn"`
	KMSKeyID                  string `json:"KmsKeyId"`
	TagsList                  []struct {
		Key   string `json:"Key"`
		Value string `json:"Value"`
	} `json:"TagsList"`
	IncludeGlobalServiceEvents bool `json:"IncludeGlobalServiceEvents"`
	IsMultiRegionTrail         bool `json:"IsMultiRegionTrail"`
	EnableLogFileValidation    bool `json:"EnableLogFileValidation"`
}

func (h *Handler) handleCreateTrail(c *echo.Context, body []byte) error {
	var in createTrailBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "invalid request body"))
	}

	if in.Name == "" {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "Name is required"))
	}
	if in.S3BucketName == "" {
		return c.JSON(http.StatusBadRequest, errResp("InvalidS3BucketNameException", "S3BucketName is required"))
	}
	if len(in.S3KeyPrefix) > maxS3KeyPrefixLength {
		return c.JSON(http.StatusBadRequest, errResp("InvalidS3PrefixException", s3KeyPrefixTooLongMsg))
	}

	kv := make(map[string]string, len(in.TagsList))
	for _, tag := range in.TagsList {
		kv[tag.Key] = tag.Value
	}

	t, err := h.Backend.CreateTrail(
		in.Name, in.S3BucketName, in.S3KeyPrefix, in.SnsTopicName,
		in.CloudWatchLogsLogGroupArn, in.CloudWatchLogsRoleArn, in.KMSKeyID,
		in.IncludeGlobalServiceEvents, in.IsMultiRegionTrail, in.EnableLogFileValidation,
		kv,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	resp := trailToMap(t)

	return c.JSON(http.StatusOK, resp)
}

// --- GetTrail ---

type getTrailBody struct {
	Name string `json:"Name"`
}

func (h *Handler) handleGetTrail(c *echo.Context, body []byte) error {
	var in getTrailBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "invalid request body"))
	}

	t, err := h.Backend.GetTrail(in.Name)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"Trail": trailToMap(t)})
}

// --- DescribeTrails ---

type describeTrailsBody struct {
	TrailNameList       []string `json:"trailNameList"`
	IncludeShadowTrails bool     `json:"includeShadowTrails"`
}

func (h *Handler) handleDescribeTrails(c *echo.Context, body []byte) error {
	var in describeTrailsBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(
				http.StatusBadRequest,
				errResp("InvalidParameterCombinationException", "invalid request body"),
			)
		}
	}

	trails := h.Backend.DescribeTrails(in.TrailNameList)
	items := make([]map[string]any, 0, len(trails))
	for _, t := range trails {
		items = append(items, trailToMap(t))
	}

	return c.JSON(http.StatusOK, map[string]any{"trailList": items})
}

// --- UpdateTrail ---

type updateTrailBody struct {
	IncludeGlobalServiceEvents *bool  `json:"IncludeGlobalServiceEvents"`
	IsMultiRegionTrail         *bool  `json:"IsMultiRegionTrail"`
	EnableLogFileValidation    *bool  `json:"EnableLogFileValidation"`
	Name                       string `json:"Name"`
	S3BucketName               string `json:"S3BucketName"`
	S3KeyPrefix                string `json:"S3KeyPrefix"`
	SnsTopicName               string `json:"SnsTopicName"`
	CloudWatchLogsLogGroupArn  string `json:"CloudWatchLogsLogGroupArn"`
	CloudWatchLogsRoleArn      string `json:"CloudWatchLogsRoleArn"`
	KMSKeyID                   string `json:"KmsKeyId"`
}

func (h *Handler) handleUpdateTrail(c *echo.Context, body []byte) error {
	var in updateTrailBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "invalid request body"))
	}

	if in.Name == "" {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "Name is required"))
	}
	if len(in.S3KeyPrefix) > maxS3KeyPrefixLength {
		return c.JSON(http.StatusBadRequest, errResp("InvalidS3PrefixException", s3KeyPrefixTooLongMsg))
	}

	t, err := h.Backend.UpdateTrail(
		in.Name, in.S3BucketName, in.S3KeyPrefix, in.SnsTopicName,
		in.CloudWatchLogsLogGroupArn, in.CloudWatchLogsRoleArn, in.KMSKeyID,
		in.IncludeGlobalServiceEvents, in.IsMultiRegionTrail, in.EnableLogFileValidation,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, trailToMap(t))
}

// --- DeleteTrail ---

type deleteTrailBody struct {
	Name string `json:"Name"`
}

func (h *Handler) handleDeleteTrail(c *echo.Context, body []byte) error {
	var in deleteTrailBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "invalid request body"))
	}

	if err := h.Backend.DeleteTrail(in.Name); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

// --- StartLogging ---

type startLoggingBody struct {
	Name string `json:"Name"`
}

func (h *Handler) handleStartLogging(c *echo.Context, body []byte) error {
	var in startLoggingBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "invalid request body"))
	}

	if in.Name == "" {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "Name is required"))
	}

	if err := h.Backend.StartLogging(in.Name); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

// --- StopLogging ---

type stopLoggingBody struct {
	Name string `json:"Name"`
}

func (h *Handler) handleStopLogging(c *echo.Context, body []byte) error {
	var in stopLoggingBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "invalid request body"))
	}

	if in.Name == "" {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "Name is required"))
	}

	if err := h.Backend.StopLogging(in.Name); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

// --- GetTrailStatus ---

type getTrailStatusBody struct {
	Name string `json:"Name"`
}

func (h *Handler) handleGetTrailStatus(c *echo.Context, body []byte) error {
	var in getTrailStatusBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "invalid request body"))
	}

	t, err := h.Backend.GetTrailStatus(in.Name)
	if err != nil {
		return h.handleError(c, err)
	}

	resp := map[string]any{
		"IsLogging": t.IsLogging,
	}
	if t.StartLoggingTime != nil {
		resp["StartLoggingTime"] = float64(t.StartLoggingTime.Unix())
		resp["TimeLoggingStarted"] = t.StartLoggingTime.UTC().Format(time.RFC3339)
	}
	if t.StopLoggingTime != nil {
		resp["StopLoggingTime"] = float64(t.StopLoggingTime.Unix())
		resp["TimeLoggingStopped"] = t.StopLoggingTime.UTC().Format(time.RFC3339)
	}
	if t.LatestDeliveryTime != nil {
		resp["LatestDeliveryTime"] = float64(t.LatestDeliveryTime.Unix())
	}

	return c.JSON(http.StatusOK, resp)
}

// --- ListTrails ---

type listTrailsBody struct {
	NextToken string `json:"NextToken"`
}

func (h *Handler) handleListTrails(c *echo.Context, body []byte) error {
	var in listTrailsBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(
				http.StatusBadRequest,
				errResp("InvalidParameterCombinationException", "invalid request body"),
			)
		}
	}

	trails := h.Backend.ListTrails()
	p := page.New(trails, in.NextToken, 0, defaultTrailsPageSize)

	items := make([]map[string]any, 0, len(p.Data))
	for _, t := range p.Data {
		items = append(items, map[string]any{
			keyTrailARN:  t.TrailARN,
			keyName:      t.Name,
			"HomeRegion": t.HomeRegion,
		})
	}

	resp := map[string]any{"Trails": items}
	if p.Next != "" {
		resp["NextToken"] = p.Next
	}

	return c.JSON(http.StatusOK, resp)
}

// --- DeregisterOrganizationDelegatedAdmin ---

type deregisterOrgDelegatedAdminBody struct {
	DelegatedAdminAccountID string `json:"DelegatedAdminAccountId"`
}

func (h *Handler) handleDeregisterOrganizationDelegatedAdmin(c *echo.Context, body []byte) error {
	var in deregisterOrgDelegatedAdminBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "invalid request body"))
	}

	if err := h.Backend.DeregisterOrganizationDelegatedAdmin(in.DelegatedAdminAccountID); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

// --- RegisterOrganizationDelegatedAdmin ---

type registerOrgDelegatedAdminBody struct {
	MemberAccountID string `json:"MemberAccountId"`
}

func (h *Handler) handleRegisterOrganizationDelegatedAdmin(c *echo.Context, body []byte) error {
	var in registerOrgDelegatedAdminBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterCombinationException", "invalid request body"))
	}

	if err := h.Backend.RegisterOrganizationDelegatedAdmin(in.MemberAccountID); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

// --- ListPublicKeys ---

func (h *Handler) handleListPublicKeys(c *echo.Context, _ []byte) error {
	keys := h.Backend.ListPublicKeys()

	return c.JSON(http.StatusOK, map[string]any{"PublicKeyList": keys})
}

// trailToMap converts a Trail to the JSON map used in API responses.
func trailToMap(t *Trail) map[string]any {
	m := map[string]any{
		keyName:                      t.Name,
		"S3BucketName":               t.S3BucketName,
		keyTrailARN:                  t.TrailARN,
		"HomeRegion":                 t.HomeRegion,
		"IncludeGlobalServiceEvents": t.IncludeGlobalServiceEvents,
		"IsMultiRegionTrail":         t.IsMultiRegionTrail,
		"LogFileValidationEnabled":   t.LogFileValidationEnabled,
		"HasCustomEventSelectors":    t.HasCustomEventSelectors,
		"HasInsightSelectors":        t.HasInsightSelectors,
		"IsOrganizationTrail":        t.IsOrganizationTrail,
	}
	if t.S3KeyPrefix != "" {
		m["S3KeyPrefix"] = t.S3KeyPrefix
	}
	if t.SnsTopicName != "" {
		m["SnsTopicName"] = t.SnsTopicName
		m["SnsTopicARN"] = t.SnsTopicARN
	}
	if t.CloudWatchLogsLogGroupARN != "" {
		m["CloudWatchLogsLogGroupArn"] = t.CloudWatchLogsLogGroupARN
	}
	if t.CloudWatchLogsRoleARN != "" {
		m["CloudWatchLogsRoleArn"] = t.CloudWatchLogsRoleARN
	}
	if t.KMSKeyID != "" {
		m["KmsKeyId"] = t.KMSKeyID
	}

	return m
}
