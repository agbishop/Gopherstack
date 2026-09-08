package accessanalyzer

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// errPolicyGenerationJobNotModeled translates ErrPolicyGenerationNotFound for
// GetGeneratedPolicy and CancelPolicyGeneration, whose own deserializeOpError
// switches (aws-sdk-go-v2/service/accessanalyzer@v1.51.4 deserializers.go) do
// not type ResourceNotFoundException -- only AccessDeniedException,
// InternalServerException, ThrottlingException, ValidationException. An
// unrecognized jobId is reported as invalid input, matching account's
// errRegionNotFound precedent.
func errPolicyGenerationJobNotModeled(err error) error {
	if errors.Is(err, ErrPolicyGenerationNotFound) {
		return ErrValidation
	}

	return err
}

const (
	opStartPolicyGeneration  = "StartPolicyGeneration"
	opGetGeneratedPolicy     = "GetGeneratedPolicy"
	opCancelPolicyGeneration = "CancelPolicyGeneration"
	opListPolicyGenerations  = "ListPolicyGenerations"

	pathGeneration = "generation"
)

// wireTrail is the request-body shape of types.Trail (types/types.go:2357).
type wireTrail struct {
	CloudTrailArn string   `json:"cloudTrailArn"`
	AllRegions    *bool    `json:"allRegions"`
	Regions       []string `json:"regions"`
}

// wireCloudTrailDetails is the request-body shape of types.CloudTrailDetails
// (types/types.go:493, v1.51.4).
type wireCloudTrailDetails struct {
	AccessRole string      `json:"accessRole"`
	StartTime  string      `json:"startTime"`
	EndTime    string      `json:"endTime"`
	Trails     []wireTrail `json:"trails"`
}

// dispatchGeneratedPolicyOps routes policy generation job operations.
func (h *Handler) dispatchGeneratedPolicyOps(op, path, query string, body []byte) (any, int, bool, error) {
	switch op {
	case opStartPolicyGeneration:
		r, c, e := h.handleStartPolicyGeneration(body)

		return r, c, true, e
	case opGetGeneratedPolicy:
		r, c, e := h.handleGetGeneratedPolicy(path)

		return r, c, true, e
	case opCancelPolicyGeneration:
		c, e := h.handleCancelPolicyGeneration(path)

		return nil, c, true, e
	case opListPolicyGenerations:
		r, c, e := h.handleListPolicyGenerations(query)

		return r, c, true, e
	}

	return nil, 0, false, nil
}

// ---- operation handlers ----

func (h *Handler) handleStartPolicyGeneration(body []byte) (any, int, error) {
	var req struct {
		CloudTrailDetails       *wireCloudTrailDetails `json:"cloudTrailDetails"`
		PolicyGenerationDetails struct {
			PrincipalArn string `json:"principalArn"`
		} `json:"policyGenerationDetails"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, 0, ErrValidation
	}

	pg, err := h.Backend.StartPolicyGeneration(
		req.PolicyGenerationDetails.PrincipalArn, cloudTrailDetailsFromRequest(req.CloudTrailDetails),
	)
	if err != nil {
		return nil, 0, err
	}

	return map[string]string{"jobId": pg.JobID}, http.StatusOK, nil
}

// cloudTrailDetailsFromRequest converts the parsed wire body into the
// backend's PolicyGenerationCloudTrailDetails, defaulting endTime to now
// when absent (matches types.CloudTrailDetails.EndTime's documented
// behavior: "If this is not included in the request, the default value is
// the current time").
func cloudTrailDetailsFromRequest(req *wireCloudTrailDetails) *PolicyGenerationCloudTrailDetails {
	if req == nil {
		return nil
	}

	endTime := parseWireTime(req.EndTime)
	if endTime.IsZero() {
		endTime = time.Now().UTC()
	}

	ctd := &PolicyGenerationCloudTrailDetails{
		AccessRole: req.AccessRole,
		StartTime:  parseWireTime(req.StartTime),
		EndTime:    endTime,
	}

	for _, tr := range req.Trails {
		ctd.Trails = append(ctd.Trails, PolicyGenerationTrail{
			CloudTrailArn: tr.CloudTrailArn,
			AllRegions:    tr.AllRegions,
			Regions:       tr.Regions,
		})
	}

	return ctd
}

func parseWireTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}

	return t
}

func (h *Handler) handleGetGeneratedPolicy(path string) (any, int, error) {
	jobID := extractLastSegment(path, pathGeneration)

	pg, err := h.Backend.GetPolicyGeneration(jobID)
	if err != nil {
		return nil, 0, errPolicyGenerationJobNotModeled(err)
	}

	properties := map[string]any{
		"principalArn": pg.PrincipalArn,
	}

	if pg.CloudTrailDetails != nil {
		properties["cloudTrailProperties"] = cloudTrailPropertiesToJSON(pg.CloudTrailDetails)
	}

	return map[string]any{
		"generatedPolicyResult": map[string]any{
			"generatedPolicies": []any{},
			"properties":        properties,
		},
		"jobDetails": jobDetailsToJSON(pg),
	}, http.StatusOK, nil
}

// cloudTrailPropertiesToJSON builds the types.CloudTrailProperties wire
// shape (startTime/endTime/trailProperties -- no accessRole, unlike the
// request-side types.CloudTrailDetails it's derived from).
func cloudTrailPropertiesToJSON(ctd *PolicyGenerationCloudTrailDetails) map[string]any {
	trails := make([]any, 0, len(ctd.Trails))

	for _, tr := range ctd.Trails {
		m := map[string]any{"cloudTrailArn": tr.CloudTrailArn}

		if tr.AllRegions != nil {
			m["allRegions"] = *tr.AllRegions
		}

		if tr.Regions != nil {
			m["regions"] = tr.Regions
		}

		trails = append(trails, m)
	}

	return map[string]any{
		"startTime":       ctd.StartTime.Format(time.RFC3339),
		"endTime":         ctd.EndTime.Format(time.RFC3339),
		"trailProperties": trails,
	}
}

func (h *Handler) handleCancelPolicyGeneration(path string) (int, error) {
	jobID := extractLastSegment(path, pathGeneration)

	if err := h.Backend.CancelPolicyGeneration(jobID); err != nil {
		return 0, errPolicyGenerationJobNotModeled(err)
	}

	return http.StatusOK, nil
}

func (h *Handler) handleListPolicyGenerations(query string) (any, int, error) {
	principalArn := queryParamValue(query, "principalArn")
	nextToken := queryParamValue(query, "nextToken")

	maxResults := 0
	if v := queryParamValue(query, "maxResults"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, 0, ErrValidation
		}

		maxResults = n
	}

	pgs, next, err := h.Backend.ListPolicyGenerations(principalArn, maxResults, nextToken)
	if err != nil {
		return nil, 0, err
	}

	list := make([]any, 0, len(pgs))

	for _, pg := range pgs {
		list = append(list, policyGenerationToJSON(pg))
	}

	resp := map[string]any{"policyGenerations": list}

	if next != "" {
		resp["nextToken"] = next
	}

	return resp, http.StatusOK, nil
}

// ---- URL path parsing ----

// parsePolicyGenerationPath parses /policy/generation and
// /policy/generation/{jobId} paths, reached via parsePolicyPath's
// pathGeneration case.
func parsePolicyGenerationPath(method string, segments []string) (string, string, bool) {
	switch len(segments) {
	case segmentDepthResource:
		switch method {
		case http.MethodGet:
			return opListPolicyGenerations, "", true
		case http.MethodPut:
			return opStartPolicyGeneration, "", true
		}
	case 3: //nolint:mnd // existing issue.
		switch method {
		case http.MethodPut:
			return opCancelPolicyGeneration, segments[2], true
		case http.MethodGet:
			return opGetGeneratedPolicy, segments[2], true
		}
	}

	return "", "", false
}

// ---- JSON serialization ----

// policyGenerationToJSON builds the types.PolicyGeneration wire shape used
// by ListPolicyGenerations, which (unlike types.JobDetails, see
// jobDetailsToJSON) does carry principalArn.
func policyGenerationToJSON(pg *PolicyGeneration) map[string]any {
	m := jobFieldsJSON(pg)
	m["principalArn"] = pg.PrincipalArn

	return m
}

// jobDetailsToJSON builds the types.JobDetails wire shape used by
// GetGeneratedPolicy's "jobDetails" member. Unlike types.PolicyGeneration
// (see policyGenerationToJSON), JobDetails has no principalArn field --
// that value is only reported under generatedPolicyResult.properties for
// this operation.
func jobDetailsToJSON(pg *PolicyGeneration) map[string]any {
	return jobFieldsJSON(pg)
}

// jobFieldsJSON builds the fields common to both types.PolicyGeneration and
// types.JobDetails (jobId/status/startedOn/completedOn).
func jobFieldsJSON(pg *PolicyGeneration) map[string]any {
	m := map[string]any{
		"jobId":     pg.JobID,
		keyStatus:   string(pg.Status),
		"startedOn": pg.StartedOn.Format(time.RFC3339),
	}

	if pg.CompletedOn != nil {
		m["completedOn"] = pg.CompletedOn.Format(time.RFC3339)
	}

	return m
}
