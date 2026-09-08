package timestreamquery

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// multiMeasureAttributeMappingInput mirrors types.MultiMeasureAttributeMapping.
type multiMeasureAttributeMappingInput struct {
	SourceColumn                    string `json:"SourceColumn"`
	TargetMultiMeasureAttributeName string `json:"TargetMultiMeasureAttributeName"`
	MeasureValueType                string `json:"MeasureValueType"`
}

// multiMeasureMappingsInput mirrors types.MultiMeasureMappings.
type multiMeasureMappingsInput struct {
	TargetMultiMeasureName        string                              `json:"TargetMultiMeasureName"`
	MultiMeasureAttributeMappings []multiMeasureAttributeMappingInput `json:"MultiMeasureAttributeMappings"`
}

// dimensionMappingInput mirrors types.DimensionMapping.
type dimensionMappingInput struct {
	Name               string `json:"Name"`
	DimensionValueType string `json:"DimensionValueType"`
}

// mixedMeasureMappingInput mirrors types.MixedMeasureMapping.
type mixedMeasureMappingInput struct {
	MeasureName                   string                              `json:"MeasureName"`
	SourceColumn                  string                              `json:"SourceColumn"`
	TargetMeasureName             string                              `json:"TargetMeasureName"`
	MeasureValueType              string                              `json:"MeasureValueType"`
	MultiMeasureAttributeMappings []multiMeasureAttributeMappingInput `json:"MultiMeasureAttributeMappings"`
}

// timestreamConfigurationInput mirrors types.TimestreamConfiguration.
type timestreamConfigurationInput struct {
	MultiMeasureMappings *multiMeasureMappingsInput `json:"MultiMeasureMappings"`
	DatabaseName         string                     `json:"DatabaseName"`
	TableName            string                     `json:"TableName"`
	TimeColumn           string                     `json:"TimeColumn"`
	MeasureNameColumn    string                     `json:"MeasureNameColumn"`
	DimensionMappings    []dimensionMappingInput    `json:"DimensionMappings"`
	MixedMeasureMappings []mixedMeasureMappingInput `json:"MixedMeasureMappings"`
}

type createScheduledQueryInput struct {
	ErrorReportConfiguration struct {
		S3Configuration *struct {
			BucketName       string `json:"BucketName"`
			EncryptionOption string `json:"EncryptionOption"`
			ObjectKeyPrefix  string `json:"ObjectKeyPrefix"`
		} `json:"S3Configuration"`
	} `json:"ErrorReportConfiguration"`
	NotificationConfiguration struct {
		SnsConfiguration *struct {
			TopicArn string `json:"TopicArn"`
		} `json:"SnsConfiguration"`
	} `json:"NotificationConfiguration"`
	ScheduledQueryExecutionRoleArn string `json:"ScheduledQueryExecutionRoleArn"`
	QueryString                    string `json:"QueryString"`
	Name                           string `json:"Name"`
	ClientToken                    string `json:"ClientToken"`
	KmsKeyID                       string `json:"KmsKeyId"`
	ScheduleConfiguration          struct {
		ScheduleExpression string `json:"ScheduleExpression"`
	} `json:"ScheduleConfiguration"`
	TargetConfiguration struct {
		TimestreamConfiguration *timestreamConfigurationInput `json:"TimestreamConfiguration"`
	} `json:"TargetConfiguration"`
	Tags []struct {
		Key   string `json:"Key"`
		Value string `json:"Value"`
	} `json:"Tags"`
}

// parseTargetConfiguration converts the parsed request-side TimestreamConfiguration
// into the (targetDatabase, targetTable, targetDetail) triple CreateScheduledQuery
// expects. Returns zero values / nil when tc is nil (TargetConfiguration is
// optional on CreateScheduledQueryInput).
func parseTargetConfiguration(tc *timestreamConfigurationInput) (string, string, *ScheduledQueryTargetDetail) {
	if tc == nil {
		return "", "", nil
	}

	detail := &ScheduledQueryTargetDetail{
		TimeColumn:           tc.TimeColumn,
		MeasureNameColumn:    tc.MeasureNameColumn,
		MultiMeasureMappings: convertMultiMeasureMappings(tc.MultiMeasureMappings),
	}

	for _, dm := range tc.DimensionMappings {
		detail.DimensionMappings = append(detail.DimensionMappings, DimensionMapping(dm))
	}

	for _, mm := range tc.MixedMeasureMappings {
		detail.MixedMeasureMappings = append(detail.MixedMeasureMappings, MixedMeasureMapping{
			MeasureName:                   mm.MeasureName,
			SourceColumn:                  mm.SourceColumn,
			TargetMeasureName:             mm.TargetMeasureName,
			MeasureValueType:              mm.MeasureValueType,
			MultiMeasureAttributeMappings: convertMultiMeasureAttributeMappings(mm.MultiMeasureAttributeMappings),
		})
	}

	return tc.DatabaseName, tc.TableName, detail
}

func convertMultiMeasureMappings(mm *multiMeasureMappingsInput) *MultiMeasureMappings {
	if mm == nil {
		return nil
	}

	return &MultiMeasureMappings{
		TargetMultiMeasureName:        mm.TargetMultiMeasureName,
		MultiMeasureAttributeMappings: convertMultiMeasureAttributeMappings(mm.MultiMeasureAttributeMappings),
	}
}

func convertMultiMeasureAttributeMappings(
	in []multiMeasureAttributeMappingInput,
) []MultiMeasureAttributeMapping {
	if len(in) == 0 {
		return nil
	}

	out := make([]MultiMeasureAttributeMapping, 0, len(in))
	for _, am := range in {
		out = append(out, MultiMeasureAttributeMapping(am))
	}

	return out
}

func (h *Handler) handleCreateScheduledQuery(ctx context.Context, body []byte) ([]byte, error) {
	var req createScheduledQueryInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidation)
	}

	if req.QueryString == "" {
		return nil, fmt.Errorf("%w: QueryString is required", ErrValidation)
	}

	if req.ScheduledQueryExecutionRoleArn == "" {
		return nil, fmt.Errorf("%w: ScheduledQueryExecutionRoleArn is required", ErrValidation)
	}

	if req.ScheduleConfiguration.ScheduleExpression == "" {
		return nil, fmt.Errorf("%w: ScheduleConfiguration.ScheduleExpression is required", ErrValidation)
	}

	if req.NotificationConfiguration.SnsConfiguration == nil ||
		req.NotificationConfiguration.SnsConfiguration.TopicArn == "" {
		return nil, fmt.Errorf(
			"%w: NotificationConfiguration.SnsConfiguration.TopicArn is required",
			ErrValidation,
		)
	}

	if req.ErrorReportConfiguration.S3Configuration == nil ||
		req.ErrorReportConfiguration.S3Configuration.BucketName == "" {
		return nil, fmt.Errorf(
			"%w: ErrorReportConfiguration.S3Configuration.BucketName is required",
			ErrValidation,
		)
	}

	notificationTopicArn := req.NotificationConfiguration.SnsConfiguration.TopicArn
	errorReportBucket := req.ErrorReportConfiguration.S3Configuration.BucketName
	errorReportEncryptionOption := req.ErrorReportConfiguration.S3Configuration.EncryptionOption
	errorReportObjectKeyPrefix := req.ErrorReportConfiguration.S3Configuration.ObjectKeyPrefix

	targetDB, targetTable, targetDetail := parseTargetConfiguration(req.TargetConfiguration.TimestreamConfiguration)

	tags := make(map[string]string, len(req.Tags))

	for _, t := range req.Tags {
		tags[t.Key] = t.Value
	}

	sq, err := h.Backend.CreateScheduledQuery(
		ctx,
		req.Name, req.QueryString,
		req.ScheduleConfiguration.ScheduleExpression,
		req.ScheduledQueryExecutionRoleArn,
		notificationTopicArn, errorReportBucket,
		targetDB, targetTable, req.ClientToken, req.KmsKeyID,
		errorReportEncryptionOption, errorReportObjectKeyPrefix,
		tags,
		targetDetail,
	)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		keyArn: sq.Arn,
	})
}

func (h *Handler) handleDeleteScheduledQuery(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		ScheduledQueryArn string `json:"ScheduledQueryArn"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.ScheduledQueryArn == "" {
		return nil, fmt.Errorf("%w: ScheduledQueryArn is required", ErrValidation)
	}

	if err := h.Backend.DeleteScheduledQuery(ctx, req.ScheduledQueryArn); err != nil {
		return nil, err
	}

	return nil, nil
}

func (h *Handler) handleDescribeScheduledQuery(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		ScheduledQueryArn string `json:"ScheduledQueryArn"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.ScheduledQueryArn == "" {
		return nil, fmt.Errorf("%w: ScheduledQueryArn is required", ErrValidation)
	}

	sq, err := h.Backend.DescribeScheduledQuery(ctx, req.ScheduledQueryArn)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		"ScheduledQuery": scheduledQueryToView(sq),
	})
}

func (h *Handler) handleExecuteScheduledQuery(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		ScheduledQueryArn string  `json:"ScheduledQueryArn"`
		InvocationTime    float64 `json:"InvocationTime"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.ScheduledQueryArn == "" {
		return nil, fmt.Errorf("%w: ScheduledQueryArn is required", ErrValidation)
	}

	if req.InvocationTime == 0 {
		return nil, fmt.Errorf("%w: InvocationTime is required", ErrValidation)
	}

	invocationTime := time.Unix(int64(req.InvocationTime), 0)

	if err := h.Backend.ExecuteScheduledQuery(ctx, req.ScheduledQueryArn, invocationTime); err != nil {
		return nil, err
	}

	return nil, nil
}

func (h *Handler) handleListScheduledQueries(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		NextToken  string `json:"NextToken"`
		MaxResults int32  `json:"MaxResults"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}

	result := h.Backend.ListScheduledQueriesEnriched(ctx, req.NextToken, req.MaxResults)

	resp := map[string]any{
		"ScheduledQueries": result.Items,
	}
	if result.NextToken != "" {
		resp["NextToken"] = result.NextToken
	}

	return json.Marshal(resp)
}

func (h *Handler) handleUpdateScheduledQuery(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		ScheduledQueryArn string `json:"ScheduledQueryArn"`
		State             string `json:"State"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.ScheduledQueryArn == "" {
		return nil, fmt.Errorf("%w: ScheduledQueryArn is required", ErrValidation)
	}

	if req.State == "" {
		return nil, fmt.Errorf("%w: State is required", ErrValidation)
	}

	if err := h.Backend.UpdateScheduledQuery(ctx, req.ScheduledQueryArn, req.State); err != nil {
		return nil, err
	}

	return nil, nil
}

// targetConfigurationView builds the TimestreamConfiguration response map for
// a scheduled query with a target database configured.
func targetConfigurationView(sq *ScheduledQuery) map[string]any {
	tsConfig := map[string]any{
		"DatabaseName": sq.TargetDatabase,
		"TableName":    sq.TargetTable,
	}

	td := sq.TargetDetail
	if td == nil {
		return tsConfig
	}

	if td.TimeColumn != "" {
		tsConfig["TimeColumn"] = td.TimeColumn
	}
	if td.MeasureNameColumn != "" {
		tsConfig["MeasureNameColumn"] = td.MeasureNameColumn
	}
	if len(td.DimensionMappings) > 0 {
		tsConfig["DimensionMappings"] = td.DimensionMappings
	}
	if len(td.MixedMeasureMappings) > 0 {
		tsConfig["MixedMeasureMappings"] = td.MixedMeasureMappings
	}
	if td.MultiMeasureMappings != nil {
		tsConfig["MultiMeasureMappings"] = td.MultiMeasureMappings
	}

	return tsConfig
}

// scheduledQueryToView converts a ScheduledQuery to an API response map.
// types.ScheduledQueryDescription (timestreamquery@v1.39.4 types/types.go:620)
// has no Tags member -- do not echo sq.Tags here.
func scheduledQueryToView(sq *ScheduledQuery) map[string]any {
	view := map[string]any{
		keyArn:         sq.Arn,
		"Name":         sq.Name,
		"State":        sq.State,
		"QueryString":  sq.QueryString,
		"CreationTime": epochSeconds(sq.CreationTime),
	}

	if sq.ScheduleExpression != "" {
		view["ScheduleConfiguration"] = map[string]any{
			"ScheduleExpression": sq.ScheduleExpression,
		}
	}

	if sq.ExecutionRoleArn != "" {
		view["ScheduledQueryExecutionRoleArn"] = sq.ExecutionRoleArn
	}

	if sq.KmsKeyID != "" {
		view["KmsKeyId"] = sq.KmsKeyID
	}

	if sq.NotificationTopicArn != "" {
		view["NotificationConfiguration"] = map[string]any{
			"SnsConfiguration": map[string]string{
				"TopicArn": sq.NotificationTopicArn,
			},
		}
	}

	if sq.ErrorReportS3BucketName != "" {
		s3Config := map[string]string{
			"BucketName": sq.ErrorReportS3BucketName,
		}
		if sq.ErrorReportEncryptionOption != "" {
			s3Config["EncryptionOption"] = sq.ErrorReportEncryptionOption
		}
		if sq.ErrorReportObjectKeyPrefix != "" {
			s3Config["ObjectKeyPrefix"] = sq.ErrorReportObjectKeyPrefix
		}
		view["ErrorReportConfiguration"] = map[string]any{
			"S3Configuration": s3Config,
		}
	}

	if sq.TargetDatabase != "" {
		view["TargetConfiguration"] = map[string]any{
			"TimestreamConfiguration": targetConfigurationView(sq),
		}
	}

	if summary := buildLastRunSummary(sq); summary != nil {
		view["LastRunSummary"] = summary
	}

	now := time.Now()
	view["NextInvocationTime"] = epochSeconds(nextInvocationTime(sq.ScheduleExpression, now))
	if !sq.LastRunTime.IsZero() {
		view["PreviousInvocationTime"] = epochSeconds(sq.LastRunTime)
	}

	return view
}
