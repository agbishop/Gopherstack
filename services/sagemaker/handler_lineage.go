package sagemaker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// Key constants for ML Lineage resource ARNs, reused across request/response bodies.
const (
	keyActionArn       = "ActionArn"
	keyArtifactArn     = "ArtifactArn"
	keyContextArn      = "ContextArn"
	keyLineageGroupArn = "LineageGroupArn"
	keySource          = "Source"
)

// lineageOpsSupported returns the ML Lineage operations implemented with real
// backend state (CreateAction/AddAssociation are declared in the core op list
// in handler.go since they predate this family).
func lineageOpsSupported() []string {
	return []string{
		"CreateArtifact",
		"DescribeArtifact",
		"UpdateArtifact",
		"DeleteArtifact",
		"ListArtifacts",
		"CreateContext",
		"DescribeContext",
		"UpdateContext",
		"DeleteContext",
		"ListContexts",
		"DescribeAction",
		"UpdateAction",
		"DeleteAction",
		"ListActions",
		"DeleteAssociation",
		"ListAssociations",
		"QueryLineage",
		"DescribeLineageGroup",
		"ListLineageGroups",
		"GetLineageGroupPolicy",
	}
}

// lineageHandlerFunc is the signature shared by every ML Lineage op handler.
type lineageHandlerFunc func(*Handler, context.Context, []byte) ([]byte, error)

// lineageHandlers builds the op-name -> handler routing table for the ML
// Lineage family. It is data-driven (rather than a switch) so that adding an
// op is a one-line table entry instead of another near-identical case arm.
func lineageHandlers() map[string]lineageHandlerFunc {
	return map[string]lineageHandlerFunc{
		"CreateArtifact":        (*Handler).handleCreateArtifact,
		"DescribeArtifact":      (*Handler).handleDescribeArtifact,
		"UpdateArtifact":        (*Handler).handleUpdateArtifact,
		"DeleteArtifact":        (*Handler).handleDeleteArtifact,
		"ListArtifacts":         (*Handler).handleListArtifacts,
		"CreateContext":         (*Handler).handleCreateContext,
		"DescribeContext":       (*Handler).handleDescribeContext,
		"UpdateContext":         (*Handler).handleUpdateContext,
		"DeleteContext":         (*Handler).handleDeleteContext,
		"ListContexts":          (*Handler).handleListContexts,
		"DescribeAction":        (*Handler).handleDescribeAction,
		"UpdateAction":          (*Handler).handleUpdateAction,
		"DeleteAction":          (*Handler).handleDeleteAction,
		"ListActions":           (*Handler).handleListActions,
		"DeleteAssociation":     (*Handler).handleDeleteAssociation,
		"ListAssociations":      (*Handler).handleListAssociations,
		"QueryLineage":          (*Handler).handleQueryLineage,
		"DescribeLineageGroup":  (*Handler).handleDescribeLineageGroup,
		"ListLineageGroups":     (*Handler).handleListLineageGroups,
		"GetLineageGroupPolicy": (*Handler).handleGetLineageGroupPolicy,
	}
}

// dispatchLineageOps handles the ML Lineage family (Action/Artifact/Context/
// Association/LineageGroup CRUD and QueryLineage graph traversal).
func (h *Handler) dispatchLineageOps(
	ctx context.Context, op string, body []byte,
) ([]byte, bool, error) {
	fn, ok := lineageHandlers()[op]
	if !ok {
		return nil, false, nil
	}

	r, err := fn(h, ctx, body)

	return r, true, err
}

// ---------------------------------------------------------------------------
// Artifact handlers
// ---------------------------------------------------------------------------

type artifactSourceTypeObject struct {
	SourceIDType string `json:"SourceIdType"`
	Value        string `json:"Value"`
}

type artifactSourceObject struct {
	SourceURI   string                     `json:"SourceUri"`
	SourceTypes []artifactSourceTypeObject `json:"SourceTypes"`
}

// metadataPropertiesObject is the wire shape of MetadataProperties
// (aws-sdk-go-v2/service/sagemaker types/types.go:13617), shared by
// CreateAction/CreateArtifact.
type metadataPropertiesObject struct {
	CommitID    string `json:"CommitId,omitempty"`
	GeneratedBy string `json:"GeneratedBy,omitempty"`
	ProjectID   string `json:"ProjectId,omitempty"`
	Repository  string `json:"Repository,omitempty"`
}

func fromMetadataProperties(mp *metadataPropertiesObject) *MetadataProperties {
	if mp == nil || *mp == (metadataPropertiesObject{}) {
		return nil
	}

	return &MetadataProperties{
		CommitID:    mp.CommitID,
		GeneratedBy: mp.GeneratedBy,
		ProjectID:   mp.ProjectID,
		Repository:  mp.Repository,
	}
}

func toMetadataProperties(mp *MetadataProperties) *metadataPropertiesObject {
	if mp == nil {
		return nil
	}

	return &metadataPropertiesObject{
		CommitID:    mp.CommitID,
		GeneratedBy: mp.GeneratedBy,
		ProjectID:   mp.ProjectID,
		Repository:  mp.Repository,
	}
}

func toArtifactSource(src ArtifactSource) artifactSourceObject {
	types := make([]artifactSourceTypeObject, 0, len(src.SourceTypes))
	for _, st := range src.SourceTypes {
		types = append(types, artifactSourceTypeObject(st))
	}

	return artifactSourceObject{SourceURI: src.SourceURI, SourceTypes: types}
}

func fromArtifactSource(src artifactSourceObject) ArtifactSource {
	types := make([]ArtifactSourceType, 0, len(src.SourceTypes))
	for _, st := range src.SourceTypes {
		types = append(types, ArtifactSourceType(st))
	}

	return ArtifactSource{SourceURI: src.SourceURI, SourceTypes: types}
}

// createArtifactInput is the CreateArtifact request shape (named, not inline,
// so wire-field-audit tooling that only inspects named types can see it —
// see gopherstack-oc9v).
type createArtifactInput struct {
	ArtifactName       string                    `json:"ArtifactName"`
	ArtifactType       string                    `json:"ArtifactType"`
	Source             artifactSourceObject      `json:"Source"`
	MetadataProperties *metadataPropertiesObject `json:"MetadataProperties"`
	Properties         map[string]string         `json:"Properties"`
	Tags               []tagObject               `json:"Tags"`
}

func (h *Handler) handleCreateArtifact(ctx context.Context, body []byte) ([]byte, error) {
	var req createArtifactInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ArtifactType == "" {
		return nil, fmt.Errorf("%w: ArtifactType is required", errInvalidRequest)
	}

	if req.Source.SourceURI == "" {
		return nil, fmt.Errorf("%w: Source.SourceUri is required", errInvalidRequest)
	}

	ar, err := h.Backend.CreateArtifact(
		ctx,
		req.ArtifactName,
		req.ArtifactType,
		fromArtifactSource(req.Source),
		req.Properties,
		fromTagObjects(req.Tags),
		fromMetadataProperties(req.MetadataProperties),
	)
	if err != nil {
		return nil, err
	}

	logger.Load(ctx).InfoContext(ctx, "sagemaker: created artifact", "arn", ar.ArtifactArn)

	return json.Marshal(map[string]string{keyArtifactArn: ar.ArtifactArn})
}

// describeArtifactInput is the DescribeArtifact request shape (named, not
// inline — see gopherstack-oc9v).
type describeArtifactInput struct {
	ArtifactArn string `json:"ArtifactArn"`
}

func (h *Handler) handleDescribeArtifact(ctx context.Context, body []byte) ([]byte, error) {
	var req describeArtifactInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ArtifactArn == "" {
		return nil, fmt.Errorf("%w: ArtifactArn is required", errInvalidRequest)
	}

	ar, err := h.Backend.DescribeArtifact(ctx, req.ArtifactArn)
	if err != nil {
		return nil, err
	}

	resp := map[string]any{
		keyArtifactArn:      ar.ArtifactArn,
		"ArtifactType":      ar.ArtifactType,
		keySource:           toArtifactSource(ar.Source),
		keyCreationTime:     epochSeconds(ar.CreationTime),
		keyLastModifiedTime: epochSeconds(ar.LastModifiedTime),
	}
	if ar.ArtifactName != "" {
		resp["ArtifactName"] = ar.ArtifactName
	}

	if len(ar.Properties) > 0 {
		resp["Properties"] = ar.Properties
	}

	if ar.MetadataProperties != nil {
		resp["MetadataProperties"] = toMetadataProperties(ar.MetadataProperties)
	}

	return json.Marshal(resp)
}

// updateArtifactInput is the UpdateArtifact request shape (named, not
// inline — see gopherstack-oc9v).
type updateArtifactInput struct {
	ArtifactArn        string            `json:"ArtifactArn"`
	ArtifactName       string            `json:"ArtifactName"`
	Properties         map[string]string `json:"Properties"`
	PropertiesToRemove []string          `json:"PropertiesToRemove"`
}

func (h *Handler) handleUpdateArtifact(ctx context.Context, body []byte) ([]byte, error) {
	var req updateArtifactInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ArtifactArn == "" {
		return nil, fmt.Errorf("%w: ArtifactArn is required", errInvalidRequest)
	}

	ar, err := h.Backend.UpdateArtifact(ctx, req.ArtifactArn, req.ArtifactName, req.Properties, req.PropertiesToRemove)
	if err != nil {
		return nil, err
	}

	logger.Load(ctx).InfoContext(ctx, "sagemaker: updated artifact", "arn", ar.ArtifactArn)

	return json.Marshal(map[string]string{keyArtifactArn: ar.ArtifactArn})
}

// deleteArtifactInput is the DeleteArtifact request shape (named, not
// inline — see gopherstack-oc9v). Source is the real alternative identity to
// ArtifactArn (api_op_DeleteArtifact.go:28-37: "Either ArtifactArn or Source
// must be specified"); previously absent from this wire struct entirely.
type deleteArtifactInput struct {
	ArtifactArn string               `json:"ArtifactArn"`
	Source      artifactSourceObject `json:"Source"`
}

func (h *Handler) handleDeleteArtifact(ctx context.Context, body []byte) ([]byte, error) {
	var req deleteArtifactInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ArtifactArn == "" && req.Source.SourceURI == "" {
		return nil, fmt.Errorf("%w: either ArtifactArn or Source is required", errInvalidRequest)
	}

	source := fromArtifactSource(req.Source)

	ar, err := h.Backend.DeleteArtifact(ctx, req.ArtifactArn, &source)
	if err != nil {
		return nil, err
	}

	logger.Load(ctx).InfoContext(ctx, "sagemaker: deleted artifact", "arn", ar.ArtifactArn)

	return json.Marshal(map[string]string{keyArtifactArn: ar.ArtifactArn})
}

type artifactSummary struct {
	ArtifactArn      string               `json:"ArtifactArn"`
	ArtifactName     string               `json:"ArtifactName,omitempty"`
	ArtifactType     string               `json:"ArtifactType"`
	Source           artifactSourceObject `json:"Source"`
	CreationTime     float64              `json:"CreationTime"`
	LastModifiedTime float64              `json:"LastModifiedTime"`
}

// marshalSummaryPage converts items to their wire summary shape via conv and
// marshals them under key, alongside NextToken when paginated.
func marshalSummaryPage[T, S any](key, nextToken string, items []*T, conv func(*T) S) ([]byte, error) {
	summaries := make([]S, 0, len(items))
	for _, item := range items {
		summaries = append(summaries, conv(item))
	}

	resp := map[string]any{key: summaries}
	if nextToken != "" {
		resp["NextToken"] = nextToken
	}

	return json.Marshal(resp)
}

// listArtifactsInput is the ListArtifacts request shape (named, not inline —
// see gopherstack-oc9v).
type listArtifactsInput struct {
	CreatedAfter  *float64 `json:"CreatedAfter"`
	CreatedBefore *float64 `json:"CreatedBefore"`
	ArtifactType  string   `json:"ArtifactType"`
	SourceURI     string   `json:"SourceUri"`
	NextToken     string   `json:"NextToken"`
	SortBy        string   `json:"SortBy"`
	SortOrder     string   `json:"SortOrder"`
	MaxResults    int32    `json:"MaxResults"`
}

func (h *Handler) handleListArtifacts(ctx context.Context, body []byte) ([]byte, error) {
	var req listArtifactsInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	artifacts, nextToken := h.Backend.ListArtifacts(ctx, ListArtifactsParams{
		ArtifactType:  req.ArtifactType,
		SourceURI:     req.SourceURI,
		NextToken:     req.NextToken,
		SortBy:        req.SortBy,
		SortOrder:     req.SortOrder,
		MaxResults:    req.MaxResults,
		CreatedAfter:  timeFromEpochSecondsPtr(req.CreatedAfter),
		CreatedBefore: timeFromEpochSecondsPtr(req.CreatedBefore),
	})

	return marshalSummaryPage("ArtifactSummaries", nextToken, artifacts, artifactToSummary)
}

func artifactToSummary(ar *Artifact) artifactSummary {
	return artifactSummary{
		ArtifactArn:      ar.ArtifactArn,
		ArtifactName:     ar.ArtifactName,
		ArtifactType:     ar.ArtifactType,
		Source:           toArtifactSource(ar.Source),
		CreationTime:     epochSeconds(ar.CreationTime),
		LastModifiedTime: epochSeconds(ar.LastModifiedTime),
	}
}

// ---------------------------------------------------------------------------
// Context handlers
// ---------------------------------------------------------------------------

type contextSourceObject struct {
	SourceURI  string `json:"SourceUri"`
	SourceID   string `json:"SourceId,omitempty"`
	SourceType string `json:"SourceType,omitempty"`
}

func toContextSource(src ContextSource) contextSourceObject {
	return contextSourceObject(src)
}

func fromContextSource(src contextSourceObject) ContextSource {
	return ContextSource(src)
}

// createContextInput is the CreateContext request shape (named, not inline —
// see gopherstack-oc9v).
type createContextInput struct {
	ContextName string              `json:"ContextName"`
	ContextType string              `json:"ContextType"`
	Description string              `json:"Description"`
	Source      contextSourceObject `json:"Source"`
	Properties  map[string]string   `json:"Properties"`
	Tags        []tagObject         `json:"Tags"`
}

func (h *Handler) handleCreateContext(ctx context.Context, body []byte) ([]byte, error) {
	var req createContextInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ContextName == "" {
		return nil, fmt.Errorf("%w: ContextName is required", errInvalidRequest)
	}

	if req.ContextType == "" {
		return nil, fmt.Errorf("%w: ContextType is required", errInvalidRequest)
	}

	if req.Source.SourceURI == "" {
		return nil, fmt.Errorf("%w: Source.SourceUri is required", errInvalidRequest)
	}

	c, err := h.Backend.CreateContext(
		ctx,
		req.ContextName,
		req.ContextType,
		req.Description,
		fromContextSource(req.Source),
		req.Properties,
		fromTagObjects(req.Tags),
	)
	if err != nil {
		return nil, err
	}

	logger.Load(ctx).InfoContext(ctx, "sagemaker: created context", "name", c.ContextName)

	return json.Marshal(map[string]string{keyContextArn: c.ContextArn})
}

// describeContextInput is the DescribeContext request shape (named, not
// inline — see gopherstack-oc9v).
type describeContextInput struct {
	ContextName string `json:"ContextName"`
}

func (h *Handler) handleDescribeContext(ctx context.Context, body []byte) ([]byte, error) {
	var req describeContextInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ContextName == "" {
		return nil, fmt.Errorf("%w: ContextName is required", errInvalidRequest)
	}

	c, err := h.Backend.DescribeContext(ctx, req.ContextName)
	if err != nil {
		return nil, err
	}

	resp := map[string]any{
		"ContextName":       c.ContextName,
		keyContextArn:       c.ContextArn,
		"ContextType":       c.ContextType,
		keySource:           toContextSource(c.Source),
		keyCreationTime:     epochSeconds(c.CreationTime),
		keyLastModifiedTime: epochSeconds(c.LastModifiedTime),
	}
	if c.Description != "" {
		resp["Description"] = c.Description
	}

	if len(c.Properties) > 0 {
		resp["Properties"] = c.Properties
	}

	return json.Marshal(resp)
}

// updateContextInput is the UpdateContext request shape (named, not
// inline — see gopherstack-oc9v).
type updateContextInput struct {
	ContextName        string            `json:"ContextName"`
	Description        string            `json:"Description"`
	Properties         map[string]string `json:"Properties"`
	PropertiesToRemove []string          `json:"PropertiesToRemove"`
}

func (h *Handler) handleUpdateContext(ctx context.Context, body []byte) ([]byte, error) {
	var req updateContextInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ContextName == "" {
		return nil, fmt.Errorf("%w: ContextName is required", errInvalidRequest)
	}

	c, err := h.Backend.UpdateContext(ctx, req.ContextName, req.Description, req.Properties, req.PropertiesToRemove)
	if err != nil {
		return nil, err
	}

	logger.Load(ctx).InfoContext(ctx, "sagemaker: updated context", "name", c.ContextName)

	return json.Marshal(map[string]string{keyContextArn: c.ContextArn})
}

// deleteContextInput is the DeleteContext request shape (named, not
// inline — see gopherstack-oc9v).
type deleteContextInput struct {
	ContextName string `json:"ContextName"`
}

func (h *Handler) handleDeleteContext(ctx context.Context, body []byte) ([]byte, error) {
	var req deleteContextInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ContextName == "" {
		return nil, fmt.Errorf("%w: ContextName is required", errInvalidRequest)
	}

	c, err := h.Backend.DeleteContext(ctx, req.ContextName)
	if err != nil {
		return nil, err
	}

	logger.Load(ctx).InfoContext(ctx, "sagemaker: deleted context", "name", req.ContextName)

	return json.Marshal(map[string]string{keyContextArn: c.ContextArn})
}

type contextSummary struct {
	Source           contextSourceObject `json:"Source"`
	ContextName      string              `json:"ContextName"`
	ContextArn       string              `json:"ContextArn"`
	ContextType      string              `json:"ContextType"`
	CreationTime     float64             `json:"CreationTime"`
	LastModifiedTime float64             `json:"LastModifiedTime"`
}

// listContextsInput is the ListContexts request shape (named, not inline —
// see gopherstack-oc9v).
type listContextsInput struct {
	CreatedAfter  *float64 `json:"CreatedAfter"`
	CreatedBefore *float64 `json:"CreatedBefore"`
	ContextType   string   `json:"ContextType"`
	SourceURI     string   `json:"SourceUri"`
	NextToken     string   `json:"NextToken"`
	SortBy        string   `json:"SortBy"`
	SortOrder     string   `json:"SortOrder"`
	MaxResults    int32    `json:"MaxResults"`
}

func (h *Handler) handleListContexts(ctx context.Context, body []byte) ([]byte, error) {
	var req listContextsInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	contexts, nextToken := h.Backend.ListContexts(ctx, ListContextsParams{
		ContextType:   req.ContextType,
		SourceURI:     req.SourceURI,
		NextToken:     req.NextToken,
		SortBy:        req.SortBy,
		SortOrder:     req.SortOrder,
		MaxResults:    req.MaxResults,
		CreatedAfter:  timeFromEpochSecondsPtr(req.CreatedAfter),
		CreatedBefore: timeFromEpochSecondsPtr(req.CreatedBefore),
	})

	return marshalSummaryPage("ContextSummaries", nextToken, contexts, contextToSummary)
}

func contextToSummary(c *Context) contextSummary {
	return contextSummary{
		ContextName:      c.ContextName,
		ContextArn:       c.ContextArn,
		ContextType:      c.ContextType,
		Source:           toContextSource(c.Source),
		CreationTime:     epochSeconds(c.CreationTime),
		LastModifiedTime: epochSeconds(c.LastModifiedTime),
	}
}

// ---------------------------------------------------------------------------
// Action handlers (Describe/Update/Delete/List; Create lives in handler.go)
// ---------------------------------------------------------------------------

// describeActionInput is the DescribeAction request shape (named, not
// inline — see gopherstack-oc9v).
type describeActionInput struct {
	ActionName string `json:"ActionName"`
}

func (h *Handler) handleDescribeAction(ctx context.Context, body []byte) ([]byte, error) {
	var req describeActionInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ActionName == "" {
		return nil, fmt.Errorf("%w: ActionName is required", errInvalidRequest)
	}

	a, err := h.Backend.DescribeAction(ctx, req.ActionName)
	if err != nil {
		return nil, err
	}

	resp := map[string]any{
		"ActionName": a.ActionName,
		keyActionArn: a.ActionArn,
		"ActionType": a.ActionType,
		keySource: map[string]any{
			"SourceUri":  a.Source.SourceURI,
			"SourceType": a.Source.SourceType,
		},
		keyCreationTime:     epochSeconds(a.CreationTime),
		keyLastModifiedTime: epochSeconds(a.LastModifiedTime),
	}
	if a.Description != "" {
		resp["Description"] = a.Description
	}

	if a.Status != "" {
		resp[keyStatus] = a.Status
	}

	if len(a.Properties) > 0 {
		resp["Properties"] = a.Properties
	}

	if a.MetadataProperties != nil {
		resp["MetadataProperties"] = toMetadataProperties(a.MetadataProperties)
	}

	return json.Marshal(resp)
}

// updateActionInput is the UpdateAction request shape (named, not
// inline — see gopherstack-oc9v).
type updateActionInput struct {
	ActionName         string            `json:"ActionName"`
	Description        string            `json:"Description"`
	Status             string            `json:"Status"`
	Properties         map[string]string `json:"Properties"`
	PropertiesToRemove []string          `json:"PropertiesToRemove"`
}

func (h *Handler) handleUpdateAction(ctx context.Context, body []byte) ([]byte, error) {
	var req updateActionInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ActionName == "" {
		return nil, fmt.Errorf("%w: ActionName is required", errInvalidRequest)
	}

	a, err := h.Backend.UpdateAction(
		ctx, req.ActionName, req.Description, req.Status, req.Properties, req.PropertiesToRemove,
	)
	if err != nil {
		return nil, err
	}

	logger.Load(ctx).InfoContext(ctx, "sagemaker: updated action", "name", a.ActionName)

	return json.Marshal(map[string]string{keyActionArn: a.ActionArn})
}

// deleteActionInput is the DeleteAction request shape (named, not
// inline — see gopherstack-oc9v).
type deleteActionInput struct {
	ActionName string `json:"ActionName"`
}

func (h *Handler) handleDeleteAction(ctx context.Context, body []byte) ([]byte, error) {
	var req deleteActionInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ActionName == "" {
		return nil, fmt.Errorf("%w: ActionName is required", errInvalidRequest)
	}

	a, err := h.Backend.DeleteAction(ctx, req.ActionName)
	if err != nil {
		return nil, err
	}

	logger.Load(ctx).InfoContext(ctx, "sagemaker: deleted action", "name", req.ActionName)

	return json.Marshal(map[string]string{keyActionArn: a.ActionArn})
}

type actionSummary struct {
	ActionName       string  `json:"ActionName"`
	ActionArn        string  `json:"ActionArn"`
	ActionType       string  `json:"ActionType"`
	Status           string  `json:"Status,omitempty"`
	CreationTime     float64 `json:"CreationTime"`
	LastModifiedTime float64 `json:"LastModifiedTime"`
}

// listActionsInput is the ListActions request shape (named, not inline —
// see gopherstack-oc9v).
type listActionsInput struct {
	CreatedAfter  *float64 `json:"CreatedAfter"`
	CreatedBefore *float64 `json:"CreatedBefore"`
	ActionType    string   `json:"ActionType"`
	SourceURI     string   `json:"SourceUri"`
	NextToken     string   `json:"NextToken"`
	SortBy        string   `json:"SortBy"`
	SortOrder     string   `json:"SortOrder"`
	MaxResults    int32    `json:"MaxResults"`
}

func (h *Handler) handleListActions(ctx context.Context, body []byte) ([]byte, error) {
	var req listActionsInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	actions, nextToken := h.Backend.ListActions(ctx, ListActionsParams{
		ActionType:    req.ActionType,
		SourceURI:     req.SourceURI,
		NextToken:     req.NextToken,
		SortBy:        req.SortBy,
		SortOrder:     req.SortOrder,
		MaxResults:    req.MaxResults,
		CreatedAfter:  timeFromEpochSecondsPtr(req.CreatedAfter),
		CreatedBefore: timeFromEpochSecondsPtr(req.CreatedBefore),
	})

	return marshalSummaryPage("ActionSummaries", nextToken, actions, actionToSummary)
}

func actionToSummary(a *Action) actionSummary {
	return actionSummary{
		ActionName:       a.ActionName,
		ActionArn:        a.ActionArn,
		ActionType:       a.ActionType,
		Status:           a.Status,
		CreationTime:     epochSeconds(a.CreationTime),
		LastModifiedTime: epochSeconds(a.LastModifiedTime),
	}
}

// ---------------------------------------------------------------------------
// Association handlers (Delete/List; AddAssociation lives in handler.go)
// ---------------------------------------------------------------------------

// deleteAssociationInput is the DeleteAssociation request shape (named, not
// inline — see gopherstack-oc9v).
type deleteAssociationInput struct {
	SourceArn      string `json:"SourceArn"`
	DestinationArn string `json:"DestinationArn"`
}

func (h *Handler) handleDeleteAssociation(ctx context.Context, body []byte) ([]byte, error) {
	var req deleteAssociationInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.SourceArn == "" {
		return nil, fmt.Errorf("%w: SourceArn is required", errInvalidRequest)
	}

	if req.DestinationArn == "" {
		return nil, fmt.Errorf("%w: DestinationArn is required", errInvalidRequest)
	}

	if err := h.Backend.DeleteAssociation(ctx, req.SourceArn, req.DestinationArn); err != nil {
		return nil, err
	}

	logger.Load(ctx).InfoContext(ctx, "sagemaker: deleted association",
		"source", req.SourceArn, "destination", req.DestinationArn)

	return json.Marshal(map[string]string{"SourceArn": req.SourceArn, "DestinationArn": req.DestinationArn})
}

type associationSummary struct {
	SourceArn       string  `json:"SourceArn"`
	SourceType      string  `json:"SourceType,omitempty"`
	SourceName      string  `json:"SourceName,omitempty"`
	DestinationArn  string  `json:"DestinationArn"`
	DestinationType string  `json:"DestinationType,omitempty"`
	DestinationName string  `json:"DestinationName,omitempty"`
	AssociationType string  `json:"AssociationType,omitempty"`
	CreationTime    float64 `json:"CreationTime"`
}

// listAssociationsInput is the ListAssociations request shape (named, not
// inline, so wire-field-audit tooling that only inspects named types can see
// it — see gopherstack-oc9v).
type listAssociationsInput struct {
	CreatedAfter    *float64 `json:"CreatedAfter"`
	CreatedBefore   *float64 `json:"CreatedBefore"`
	SourceArn       string   `json:"SourceArn"`
	DestinationArn  string   `json:"DestinationArn"`
	SourceType      string   `json:"SourceType"`
	DestinationType string   `json:"DestinationType"`
	AssociationType string   `json:"AssociationType"`
	SortBy          string   `json:"SortBy"`
	SortOrder       string   `json:"SortOrder"`
	NextToken       string   `json:"NextToken"`
	MaxResults      int32    `json:"MaxResults"`
}

func (h *Handler) handleListAssociations(ctx context.Context, body []byte) ([]byte, error) {
	var req listAssociationsInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	assocs, nextToken := h.Backend.ListAssociations(ctx, ListAssociationsParams{
		SourceArn:       req.SourceArn,
		DestinationArn:  req.DestinationArn,
		SourceType:      req.SourceType,
		DestinationType: req.DestinationType,
		AssociationType: req.AssociationType,
		SortBy:          req.SortBy,
		SortOrder:       req.SortOrder,
		NextToken:       req.NextToken,
		CreatedAfter:    timeFromEpochSecondsPtr(req.CreatedAfter),
		CreatedBefore:   timeFromEpochSecondsPtr(req.CreatedBefore),
		MaxResults:      req.MaxResults,
	})

	return marshalSummaryPage("AssociationSummaries", nextToken, assocs, func(a *Association) associationSummary {
		srcName, srcType, _, _ := h.Backend.LineageEntityInfo(ctx, a.SourceArn)
		dstName, dstType, _, _ := h.Backend.LineageEntityInfo(ctx, a.DestinationArn)

		return associationSummary{
			SourceArn:       a.SourceArn,
			SourceName:      srcName,
			SourceType:      srcType,
			DestinationArn:  a.DestinationArn,
			DestinationName: dstName,
			DestinationType: dstType,
			AssociationType: a.AssociationType,
			CreationTime:    epochSeconds(a.CreationTime),
		}
	})
}

// ---------------------------------------------------------------------------
// LineageGroup handlers
// ---------------------------------------------------------------------------

// describeLineageGroupInput is the DescribeLineageGroup request shape
// (named, not inline — see gopherstack-oc9v).
type describeLineageGroupInput struct {
	LineageGroupName string `json:"LineageGroupName"`
}

func (h *Handler) handleDescribeLineageGroup(ctx context.Context, body []byte) ([]byte, error) {
	var req describeLineageGroupInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.LineageGroupName == "" {
		return nil, fmt.Errorf("%w: LineageGroupName is required", errInvalidRequest)
	}

	lineageGroupARN, createdAt, err := h.Backend.DescribeLineageGroup(ctx, req.LineageGroupName)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		keyLineageGroupArn:  lineageGroupARN,
		"LineageGroupName":  req.LineageGroupName,
		keyCreationTime:     epochSeconds(createdAt),
		keyLastModifiedTime: epochSeconds(createdAt),
	})
}

// listLineageGroupsInput is the ListLineageGroups request shape (named, not
// inline — see gopherstack-oc9v).
type listLineageGroupsInput struct {
	CreatedAfter  *float64 `json:"CreatedAfter"`
	CreatedBefore *float64 `json:"CreatedBefore"`
	NextToken     string   `json:"NextToken"`
	SortBy        string   `json:"SortBy"`
	SortOrder     string   `json:"SortOrder"`
	MaxResults    int32    `json:"MaxResults"`
}

func (h *Handler) handleListLineageGroups(ctx context.Context, body []byte) ([]byte, error) {
	var req listLineageGroupsInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	groups, nextToken := h.Backend.ListLineageGroups(ctx, ListLineageGroupsParams{
		NextToken:     req.NextToken,
		SortBy:        req.SortBy,
		SortOrder:     req.SortOrder,
		MaxResults:    req.MaxResults,
		CreatedAfter:  timeFromEpochSecondsPtr(req.CreatedAfter),
		CreatedBefore: timeFromEpochSecondsPtr(req.CreatedBefore),
	})

	summaries := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		summaries = append(summaries, map[string]any{
			keyLineageGroupArn:  g.LineageGroupArn,
			"LineageGroupName":  defaultLineageGroupName,
			keyCreationTime:     epochSeconds(g.CreationTime),
			keyLastModifiedTime: epochSeconds(g.CreationTime),
		})
	}

	resp := map[string]any{"LineageGroupSummaries": summaries}
	if nextToken != "" {
		resp["NextToken"] = nextToken
	}

	return json.Marshal(resp)
}

// getLineageGroupPolicyInput is the GetLineageGroupPolicy request shape
// (named, not inline — see gopherstack-oc9v).
type getLineageGroupPolicyInput struct {
	LineageGroupName string `json:"LineageGroupName"`
}

func (h *Handler) handleGetLineageGroupPolicy(ctx context.Context, body []byte) ([]byte, error) {
	var req getLineageGroupPolicyInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.LineageGroupName == "" {
		return nil, fmt.Errorf("%w: LineageGroupName is required", errInvalidRequest)
	}

	// GetLineageGroupPolicy always errors: no policy-attachment op exists yet, so a
	// lineage group never has a policy attached. SA4023 below is a false positive
	// against that deliberately defensive shape (see its doc in lineage.go).
	resourcePolicy, err := h.Backend.GetLineageGroupPolicy(ctx, req.LineageGroupName) //nolint:staticcheck // see above
	if err != nil {                                                                   //nolint:staticcheck // see above
		return nil, err
	}

	return json.Marshal(map[string]string{"ResourcePolicy": resourcePolicy})
}

// ---------------------------------------------------------------------------
// QueryLineage handler
// ---------------------------------------------------------------------------

// queryFiltersObject is the wire shape of QueryFilters (aws-sdk-go-v2/service/sagemaker
// types/types.go:19078). Types (entity-type filter for non-lineage-tracked
// vertices such as TrainingJob/Model/Endpoint ARNs) is parsed but NOT
// enforced — see handleQueryLineage's Filters.Types disclosure below.
type queryFiltersObject struct {
	CreatedAfter   *float64          `json:"CreatedAfter"`
	CreatedBefore  *float64          `json:"CreatedBefore"`
	ModifiedAfter  *float64          `json:"ModifiedAfter"`
	ModifiedBefore *float64          `json:"ModifiedBefore"`
	LineageTypes   []string          `json:"LineageTypes"`
	Properties     map[string]string `json:"Properties"`
	Types          []string          `json:"Types"`
}

// queryLineageInput is the QueryLineage request shape (named, not inline —
// see gopherstack-oc9v).
type queryLineageInput struct {
	MaxDepth     *int32              `json:"MaxDepth"`
	MaxResults   *int32              `json:"MaxResults"`
	IncludeEdges *bool               `json:"IncludeEdges"`
	Filters      *queryFiltersObject `json:"Filters"`
	Direction    string              `json:"Direction"`
	NextToken    string              `json:"NextToken"`
	StartArns    []string            `json:"StartArns"`
}

func (h *Handler) handleQueryLineage(ctx context.Context, body []byte) ([]byte, error) {
	var req queryLineageInput

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if len(req.StartArns) == 0 {
		return nil, fmt.Errorf("%w: StartArns is required", errInvalidRequest)
	}

	maxDepth := 0
	if req.MaxDepth != nil {
		maxDepth = int(*req.MaxDepth)
	}

	includeEdges := true
	if req.IncludeEdges != nil {
		includeEdges = *req.IncludeEdges
	}

	var maxResults int32
	if req.MaxResults != nil {
		maxResults = *req.MaxResults
	}

	vertices, edges, nextToken, err := h.Backend.QueryLineage(ctx, QueryLineageParams{
		StartArns:    req.StartArns,
		Direction:    req.Direction,
		MaxDepth:     maxDepth,
		IncludeEdges: includeEdges,
		Filters:      fromQueryFilters(req.Filters),
		MaxResults:   maxResults,
		NextToken:    req.NextToken,
	})
	if err != nil {
		return nil, err
	}

	if vertices == nil {
		vertices = []Vertex{}
	}

	if edges == nil {
		edges = []Edge{}
	}

	resp := map[string]any{"Vertices": vertices, "Edges": edges}
	if nextToken != "" {
		resp["NextToken"] = nextToken
	}

	return json.Marshal(resp)
}

// fromQueryFilters converts the wire Filters object to backend QueryLineageFilters.
func fromQueryFilters(f *queryFiltersObject) *QueryLineageFilters {
	if f == nil {
		return nil
	}

	return &QueryLineageFilters{
		CreatedAfter:   timeFromEpochSecondsPtr(f.CreatedAfter),
		CreatedBefore:  timeFromEpochSecondsPtr(f.CreatedBefore),
		ModifiedAfter:  timeFromEpochSecondsPtr(f.ModifiedAfter),
		ModifiedBefore: timeFromEpochSecondsPtr(f.ModifiedBefore),
		LineageTypes:   f.LineageTypes,
		Properties:     f.Properties,
	}
}

// addAssociationRequest is the request body for AddAssociation. Tags is
// deliberately absent: AddAssociationInput has no Tags member at all
// (api_op_AddAssociation.go) -- an association can only be tagged
// afterward, via AddTags against its resulting association ARN.
type addAssociationRequest struct {
	SourceArn       string `json:"SourceArn"`
	DestinationArn  string `json:"DestinationArn"`
	AssociationType string `json:"AssociationType"`
}

func (h *Handler) handleAddAssociation(ctx context.Context, body []byte) ([]byte, error) {
	var req addAssociationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.SourceArn == "" {
		return nil, fmt.Errorf("%w: SourceArn is required", errInvalidRequest)
	}

	if req.DestinationArn == "" {
		return nil, fmt.Errorf("%w: DestinationArn is required", errInvalidRequest)
	}

	assoc, err := h.Backend.AddAssociation(
		ctx,
		req.SourceArn,
		req.DestinationArn,
		req.AssociationType,
		nil,
	)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "sagemaker: added association", "arn", assoc.AssociationArn)

	// AddAssociationOutput has no AssociationArn member at all -- it echoes
	// back SourceArn and DestinationArn (api_op_AddAssociation.go).
	return json.Marshal(map[string]string{
		"SourceArn":      req.SourceArn,
		"DestinationArn": req.DestinationArn,
	})
}

// associateTrialComponentRequest is the request body for AssociateTrialComponent.
type associateTrialComponentRequest struct {
	TrialName          string `json:"TrialName"`
	TrialComponentName string `json:"TrialComponentName"`
}

func (h *Handler) handleAssociateTrialComponent(ctx context.Context, body []byte) ([]byte, error) {
	var req associateTrialComponentRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.TrialName == "" {
		return nil, fmt.Errorf("%w: TrialName is required", errInvalidRequest)
	}

	if req.TrialComponentName == "" {
		return nil, fmt.Errorf("%w: TrialComponentName is required", errInvalidRequest)
	}

	assoc, err := h.Backend.AssociateTrialComponent(ctx, req.TrialName, req.TrialComponentName)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "sagemaker: associated trial component",
		"trial", assoc.TrialArn, "component", assoc.TrialComponentArn)

	return json.Marshal(map[string]string{
		"TrialArn":          assoc.TrialArn,
		"TrialComponentArn": assoc.TrialComponentArn,
	})
}

// actionSourceRequest is the source for a CreateAction request.
type actionSourceRequest struct {
	SourceURI  string `json:"SourceUri"`
	SourceType string `json:"SourceType,omitempty"`
}

// createActionRequest is the request body for CreateAction. MetadataProperties
// was absent here too — the same gap parity-5 disclosed for CreateArtifact
// (both CreateActionInput/CreateArtifactInput carry it,
// aws-sdk-go-v2/service/sagemaker types/types.go:13617) — fixed alongside
// CreateArtifact's since it's the same root cause.
type createActionRequest struct {
	Properties         map[string]string         `json:"Properties"`
	Source             actionSourceRequest       `json:"Source"`
	MetadataProperties *metadataPropertiesObject `json:"MetadataProperties"`
	ActionName         string                    `json:"ActionName"`
	ActionType         string                    `json:"ActionType"`
	Description        string                    `json:"Description,omitempty"`
	Status             string                    `json:"Status,omitempty"`
	Tags               []tagObject               `json:"Tags"`
}

func (h *Handler) handleCreateAction(ctx context.Context, body []byte) ([]byte, error) {
	var req createActionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ActionName == "" {
		return nil, fmt.Errorf("%w: ActionName is required", errInvalidRequest)
	}

	tags := fromTagObjects(req.Tags)
	source := ActionSource{
		SourceURI:  req.Source.SourceURI,
		SourceType: req.Source.SourceType,
	}

	a, err := h.Backend.CreateAction(
		ctx,
		req.ActionName,
		req.ActionType,
		req.Description,
		req.Status,
		source,
		req.Properties,
		tags,
		fromMetadataProperties(req.MetadataProperties),
	)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "sagemaker: created action", "name", a.ActionName, "arn", a.ActionArn)

	return json.Marshal(map[string]string{"ActionArn": a.ActionArn})
}
