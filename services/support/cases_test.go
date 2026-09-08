package support_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/support"
)

// opaqueToken decodes a base64url token and asserts it wraps a JSON offset object.
func opaqueToken(t *testing.T, token string) int {
	t.Helper()

	raw, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err, "token must be valid base64url")

	var obj struct {
		O int `json:"o"`
	}

	require.NoError(t, json.Unmarshal(raw, &obj), "token must be JSON with 'o' key")

	return obj.O
}

func TestSupport_DescribeCases(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)

	rec1 := doSupportRequest(t, h, "CreateCase", map[string]any{
		"subject":           "Case One",
		"communicationBody": "Initial question",
	})
	require.Equal(t, http.StatusOK, rec1.Code)

	createResp := decodeSupportResponse(t, rec1)
	caseID := createResp["caseId"].(string)

	rec := doSupportRequest(t, h, "DescribeCases", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeSupportResponse(t, rec)
	cases, ok := resp["cases"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, cases)

	// Describe by caseId
	rec2 := doSupportRequest(t, h, "DescribeCases", map[string]any{
		"caseIdList": []string{caseID},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	resp2 := decodeSupportResponse(t, rec2)
	cases2, ok := resp2["cases"].([]any)
	require.True(t, ok)
	assert.Len(t, cases2, 1)
}

func TestSupport_ResolveCase(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)

	createRec := doSupportRequest(t, h, "CreateCase", map[string]any{
		"subject":           "Resolve me",
		"communicationBody": "Please resolve this.",
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	createResp := decodeSupportResponse(t, createRec)
	caseID := createResp["caseId"].(string)

	rec := doSupportRequest(t, h, "ResolveCase", map[string]any{
		"caseId": caseID,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeSupportResponse(t, rec)
	assert.Equal(t, "opened", resp["initialCaseStatus"])
	assert.Equal(t, "resolved", resp["finalCaseStatus"])
}

func TestSupport_DescribeCreateCaseOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "all_fields",
			body: map[string]any{
				"issueType":    "technical",
				"serviceCode":  "amazon-s3",
				"categoryCode": "general-guidance",
				"language":     "en",
			},
			wantCode: http.StatusOK,
		},
		{
			name:     "empty_body",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSupportHandler(t)
			rec := doSupportRequest(t, h, "DescribeCreateCaseOptions", tt.body)
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				resp := decodeSupportResponse(t, rec)
				assert.NotEmpty(t, resp["communicationTypes"])
				assert.Equal(t, "available", resp["languageAvailability"])
			}
		})
	}
}

// TestSupport_DescribeCasesWithOptions_OpaquePageToken verifies pagination emits JSON-wrapped opaque tokens.
func TestSupport_DescribeCasesWithOptions_OpaquePageToken(t *testing.T) {
	t.Parallel()

	b := support.NewInMemoryBackend()

	for range 15 {
		_, err := b.CreateCaseWithOptions(support.CreateCaseOptions{
			Subject:           "subject",
			CommunicationBody: "body",
			SeverityCode:      "low",
		})
		require.NoError(t, err)
	}

	cases, token, err := b.DescribeCasesWithOptions(support.DescribeCasesOptions{
		MaxResults:           10,
		IncludeResolvedCases: true,
	})
	require.NoError(t, err)
	assert.Len(t, cases, 10)
	require.NotEmpty(t, token)

	offset := opaqueToken(t, token)
	assert.Equal(t, 10, offset)
}

// TestSupport_DescribeCasesWithOptions_InvalidOpaqueToken verifies a plain-integer token is rejected.
func TestSupport_DescribeCasesWithOptions_InvalidOpaqueToken(t *testing.T) {
	t.Parallel()

	b := support.NewInMemoryBackend()

	// Old-style plain-integer token (base64url("5")) must be rejected.
	plainIntToken := base64.RawURLEncoding.EncodeToString([]byte("5"))
	_, _, err := b.DescribeCasesWithOptions(support.DescribeCasesOptions{
		NextToken:            plainIntToken,
		IncludeResolvedCases: true,
	})
	assert.Error(t, err, "plain integer token should be invalid")
}

// TestSupport_DescribeCreateCaseOptions_ChatHours verifies language-specific chat hours.
func TestSupport_DescribeCreateCaseOptions_ChatHours(t *testing.T) {
	t.Parallel()

	b := support.NewInMemoryBackend()

	tests := []struct {
		name      string
		language  string
		wantStart string
		wantEnd   string
	}{
		{"english_default", "en", "06:00", "22:00"},
		{"japanese_locale", "ja", "07:00", "21:00"},
		{"empty_language", "", "06:00", "22:00"},
		{"chinese_locale", "zh", "06:00", "22:00"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := b.DescribeCreateCaseOptions("", "", "", tc.language)
			require.NotNil(t, result)

			var chatType *support.CommunicationTypeOptions

			for i := range result.CommunicationTypes {
				if result.CommunicationTypes[i].Type == "chat" {
					chatType = &result.CommunicationTypes[i]

					break
				}
			}

			require.NotNil(t, chatType, "chat communication type must be present")
			require.NotEmpty(t, chatType.SupportedHours)
			assert.Equal(t, tc.wantStart, chatType.SupportedHours[0].StartTime)
			assert.Equal(t, tc.wantEnd, chatType.SupportedHours[0].EndTime)
		})
	}
}

func TestSupport_CreateCase_Validation(t *testing.T) {
	t.Parallel()

	elevenEmails := make([]string, 11)
	for i := range elevenEmails {
		elevenEmails[i] = "copy@example.test"
	}
	tests := []struct {
		body map[string]any
		name string
	}{
		{name: "missing communication body", body: map[string]any{"subject": "invalid"}},
		{
			name: "invalid language",
			body: map[string]any{"subject": "invalid", "communicationBody": "body", "language": "xx"},
		},
		{
			name: "invalid issue type",
			body: map[string]any{"subject": "invalid", "communicationBody": "body", "issueType": "incident"},
		},
		{
			name: "invalid severity",
			body: map[string]any{"subject": "invalid", "communicationBody": "body", "severityCode": "blocker"},
		},
		{
			name: "too many cc recipients",
			body: map[string]any{
				"subject": "invalid", "communicationBody": "body", "ccEmailAddresses": elevenEmails,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doSupportRequest(t, newTestSupportHandler(t), "CreateCase", tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestSupport_CaseDetails_InitialCommunicationAndReopen(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)
	caseID := createAccurateCase(t, h, map[string]any{
		"ccEmailAddresses": []string{"billing@example.test"},
	})

	describeRec := doSupportRequest(t, h, "DescribeCases", map[string]any{"caseIdList": []string{caseID}})
	require.Equal(t, http.StatusOK, describeRec.Code)
	cases := decodeSupportResponse(t, describeRec)["cases"].([]any)
	require.Len(t, cases, 1)
	cs := cases[0].(map[string]any)
	assert.NotEmpty(t, cs["displayId"])
	assert.Equal(t, "en", cs["language"])
	assert.Equal(t, "customer", cs["submittedBy"])
	recent := cs["recentCommunications"].(map[string]any)["communications"].([]any)
	require.Len(t, recent, 1)
	assert.Equal(t, "Initial customer report", recent[0].(map[string]any)["body"])

	resolveRec := doSupportRequest(t, h, "ResolveCase", map[string]any{"caseId": caseID})
	require.Equal(t, http.StatusOK, resolveRec.Code)
	addRec := doSupportRequest(t, h, "AddCommunicationToCase", map[string]any{
		"caseId": caseID, "communicationBody": "Issue persists",
	})
	require.Equal(t, http.StatusOK, addRec.Code)
	resolveAgain := doSupportRequest(t, h, "ResolveCase", map[string]any{"caseId": caseID})
	require.Equal(t, http.StatusOK, resolveAgain.Code)
	assert.Equal(t, "reopened", decodeSupportResponse(t, resolveAgain)["initialCaseStatus"])
}

func TestSupport_DescribeCases_PaginationAndValidation(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)
	for i := range 11 {
		createAccurateCase(t, h, map[string]any{"subject": "Case " + string(rune('A'+i))})
	}

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
		wantLen  int
	}{
		{
			name: "first page emits token", body: map[string]any{"maxResults": 10},
			wantCode: http.StatusOK, wantLen: 10,
		},
		{
			name: "invalid page size", body: map[string]any{"maxResults": 1},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "invalid time filter", body: map[string]any{"afterTime": "not-time"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		rec := doSupportRequest(t, h, "DescribeCases", tt.body)
		require.Equal(t, tt.wantCode, rec.Code, tt.name)
		if tt.wantLen == 0 {
			continue
		}
		body := decodeSupportResponse(t, rec)
		assert.Len(t, body["cases"], tt.wantLen, tt.name)
		token := body["nextToken"].(string)
		second := doSupportRequest(t, h, "DescribeCases", map[string]any{"maxResults": 10, "nextToken": token})
		assert.Len(t, decodeSupportResponse(t, second)["cases"], 1, tt.name)
	}
}

// TestSupport_AddCaseInternal verifies seed helper.
func TestSupport_AddCaseInternal(t *testing.T) {
	t.Parallel()

	b := support.NewInMemoryBackend()

	b.AddCaseInternal(&support.Case{
		CaseID:      "case-seed001",
		Subject:     "Seeded Case",
		Status:      "opened",
		CreatedTime: time.Now(),
	})

	assert.Equal(t, 1, support.CaseCount(b))
	cases := b.DescribeCases([]string{"case-seed001"}, false)
	require.Len(t, cases, 1)
	assert.Equal(t, "Seeded Case", cases[0].Subject)
}

// TestSupport_DescribeCases_IncludeResolved verifies includeResolvedCases filter.
func TestSupport_DescribeCases_IncludeResolved(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)

	// Create and resolve a case.
	rec := doSupportRequest(t, h, "CreateCase", map[string]any{"subject": "Resolve me", "communicationBody": "Initial"})
	require.Equal(t, http.StatusOK, rec.Code)

	createResp := decodeSupportResponse(t, rec)
	caseID := createResp["caseId"].(string)

	rec2 := doSupportRequest(t, h, "ResolveCase", map[string]any{"caseId": caseID})
	require.Equal(t, http.StatusOK, rec2.Code)

	// Without includeResolvedCases, resolved case should not appear.
	rec3 := doSupportRequest(t, h, "DescribeCases", map[string]any{
		"includeResolvedCases": false,
	})
	require.Equal(t, http.StatusOK, rec3.Code)

	resp3 := decodeSupportResponse(t, rec3)
	cases3 := resp3["cases"].([]any)
	assert.Empty(t, cases3)

	// With includeResolvedCases, resolved case should appear.
	rec4 := doSupportRequest(t, h, "DescribeCases", map[string]any{
		"includeResolvedCases": true,
	})
	require.Equal(t, http.StatusOK, rec4.Code)

	resp4 := decodeSupportResponse(t, rec4)
	cases4 := resp4["cases"].([]any)
	assert.Len(t, cases4, 1)
}

// TestSupport_DescribeCases_IncludeCommunications verifies the documented
// "By default, communications are included" default: omitting
// includeCommunications must behave like true, and it must still be
// possible to turn it off with an explicit false (the omitted-vs-false
// distinction DescribeCasesInput.IncludeCommunications's *bool is typed to
// preserve).
func TestSupport_DescribeCases_IncludeCommunications(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)

	rec := doSupportRequest(t, h, "CreateCase", map[string]any{"subject": "Comms", "communicationBody": "Initial"})
	require.Equal(t, http.StatusOK, rec.Code)

	// Omitted entirely: defaults to true, so recentCommunications is present.
	recOmitted := doSupportRequest(t, h, "DescribeCases", map[string]any{})
	require.Equal(t, http.StatusOK, recOmitted.Code)

	respOmitted := decodeSupportResponse(t, recOmitted)
	casesOmitted := respOmitted["cases"].([]any)
	require.Len(t, casesOmitted, 1)
	_, hasComms := casesOmitted[0].(map[string]any)["recentCommunications"]
	assert.True(t, hasComms, "includeCommunications omitted must default to true")

	// Explicit false: recentCommunications must be absent.
	recFalse := doSupportRequest(t, h, "DescribeCases", map[string]any{"includeCommunications": false})
	require.Equal(t, http.StatusOK, recFalse.Code)

	respFalse := decodeSupportResponse(t, recFalse)
	casesFalse := respFalse["cases"].([]any)
	require.Len(t, casesFalse, 1)
	_, hasCommsFalse := casesFalse[0].(map[string]any)["recentCommunications"]
	assert.False(t, hasCommsFalse, "includeCommunications: false must omit recentCommunications")

	// Explicit true: recentCommunications must be present.
	recTrue := doSupportRequest(t, h, "DescribeCases", map[string]any{"includeCommunications": true})
	require.Equal(t, http.StatusOK, recTrue.Code)

	respTrue := decodeSupportResponse(t, recTrue)
	casesTrue := respTrue["cases"].([]any)
	require.Len(t, casesTrue, 1)
	_, hasCommsTrue := casesTrue[0].(map[string]any)["recentCommunications"]
	assert.True(t, hasCommsTrue, "includeCommunications: true must include recentCommunications")
}

// TestSupport_DescribeCases_ReturnsTimestamps verifies timeCreated is populated.
func TestSupport_DescribeCases_ReturnsTimestamps(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)

	rec := doSupportRequest(
		t,
		h,
		"CreateCase",
		map[string]any{"subject": "Timestamp test", "communicationBody": "Initial"},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSupportRequest(t, h, "DescribeCases", map[string]any{})
	require.Equal(t, http.StatusOK, rec2.Code)

	resp := decodeSupportResponse(t, rec2)
	cases := resp["cases"].([]any)
	require.Len(t, cases, 1)
	cs := cases[0].(map[string]any)
	assert.NotEmpty(t, cs["timeCreated"])
}

// TestSupport_DescribeCases_ResolvedStatus verifies resolved cases expose AWS status without internal timestamp.
func TestSupport_DescribeCases_ResolvedStatus(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)

	rec := doSupportRequest(
		t,
		h,
		"CreateCase",
		map[string]any{"subject": "Resolve timestamp", "communicationBody": "Initial"},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	createResp := decodeSupportResponse(t, rec)
	caseID := createResp["caseId"].(string)

	doSupportRequest(t, h, "ResolveCase", map[string]any{"caseId": caseID})

	rec2 := doSupportRequest(t, h, "DescribeCases", map[string]any{
		"includeResolvedCases": true,
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	resp := decodeSupportResponse(t, rec2)
	cases := resp["cases"].([]any)
	require.Len(t, cases, 1)
	cs := cases[0].(map[string]any)
	assert.Equal(t, "resolved", cs["status"])
	assert.NotContains(t, cs, "timeResolved")
}

// TestSupport_CreateCase_ValidationError verifies subject validation uses ErrValidation.
func TestSupport_CreateCase_ValidationError(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)
	rec := doSupportRequest(t, h, "CreateCase", map[string]any{"serviceCode": "amazon-s3"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestSupport_ResolveCase_AlreadyResolved verifies double-resolve is idempotent: real
// AWS Support's ResolveCase has no "already resolved" error (its modeled
// exceptions are only CaseIdNotFound and InternalServerError), so resolving
// an already-resolved case succeeds and reports "resolved" for both the
// initial and final status.
func TestSupport_ResolveCase_AlreadyResolved(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)

	rec := doSupportRequest(
		t,
		h,
		"CreateCase",
		map[string]any{"subject": "Double resolve", "communicationBody": "Initial"},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	createResp := decodeSupportResponse(t, rec)
	caseID := createResp["caseId"].(string)

	rec2 := doSupportRequest(t, h, "ResolveCase", map[string]any{"caseId": caseID})
	require.Equal(t, http.StatusOK, rec2.Code)

	rec3 := doSupportRequest(t, h, "ResolveCase", map[string]any{"caseId": caseID})
	require.Equal(t, http.StatusOK, rec3.Code)

	resp3 := decodeSupportResponse(t, rec3)
	assert.Equal(t, "resolved", resp3["initialCaseStatus"])
	assert.Equal(t, "resolved", resp3["finalCaseStatus"])
}

// TestSupport_CreateCase_CaseCreationLimitExceeded verifies CaseCreationLimitExceeded
// is real (not a stub): once the account is at the open-case cap, CreateCase is
// rejected with the correct wire type, and resolving a case frees a slot again.
// TestSupport_CreateCase_CaseCreationLimitExceeded and
// TestSupport_CreateCase_CaseCreationLimitExceeded_WireType live in
// whitebox_test.go: they need direct access to the unexported maxOpenCases
// constant.

// TestSupport_DescribeCases_Pagination verifies nextToken/maxResults fields accepted.
func TestSupport_DescribeCases_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)

	doSupportRequest(t, h, "CreateCase", map[string]any{"subject": "Page1", "communicationBody": "Initial"})
	doSupportRequest(t, h, "CreateCase", map[string]any{"subject": "Page2", "communicationBody": "Initial"})

	rec := doSupportRequest(t, h, "DescribeCases", map[string]any{
		"maxResults": 10,
		"nextToken":  "",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestSupport_DescribeCases_UnknownCaseID verifies that requesting a caseId
// that doesn't exist returns CaseIdNotFound rather than silently omitting it
// from the results: DescribeCases's only case-specific modeled exception
// (aws-sdk-go-v2/service/support/deserializers.go
// awsAwsjson11_deserializeOpErrorDescribeCases) is CaseIdNotFound.
func TestSupport_DescribeCases_UnknownCaseID(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)

	rec := doSupportRequest(t, h, "DescribeCases", map[string]any{
		"caseIdList": []string{"case-does-not-exist"},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	resp := decodeSupportResponse(t, rec)
	assert.Equal(t, "CaseIdNotFound", resp["__type"])
}

// TestSupport_DescribeCases_CaseIdListTooLarge verifies the documented
// "maximum number of cases is 100" limit on caseIdList
// (aws-sdk-go-v2/service/support/api_op_DescribeCases.go) is enforced.
func TestSupport_DescribeCases_CaseIdListTooLarge(t *testing.T) {
	t.Parallel()

	h := newTestSupportHandler(t)

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "case-" + strconv.Itoa(i)
	}

	rec := doSupportRequest(t, h, "DescribeCases", map[string]any{"caseIdList": ids})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	resp := decodeSupportResponse(t, rec)
	assert.Equal(t, "ValidationException", resp["__type"])
}
