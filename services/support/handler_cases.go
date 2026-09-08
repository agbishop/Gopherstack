package support

import (
	"context"
	"fmt"
	"time"
)

type caseView struct {
	RecentCommunications *recentCaseCommunicationsView `json:"recentCommunications,omitempty"`
	CreatedTime          string                        `json:"timeCreated"`
	CaseID               string                        `json:"caseId"`
	DisplayID            string                        `json:"displayId"`
	Subject              string                        `json:"subject"`
	Status               string                        `json:"status"`
	ServiceCode          string                        `json:"serviceCode"`
	CategoryCode         string                        `json:"categoryCode"`
	SeverityCode         string                        `json:"severityCode"`
	Language             string                        `json:"language"`
	SubmittedBy          string                        `json:"submittedBy"`
	CCEmails             []string                      `json:"ccEmailAddresses"`
}

type createCaseOutput struct {
	CaseID string `json:"caseId"`
}

type describeCasesOutput struct {
	NextToken string     `json:"nextToken,omitempty"`
	Cases     []caseView `json:"cases"`
}

type resolveCaseOutput struct {
	InitialCaseStatus string `json:"initialCaseStatus"`
	FinalCaseStatus   string `json:"finalCaseStatus"`
}

type handleCreateCaseInput struct {
	Subject           string   `json:"subject"`
	ServiceCode       string   `json:"serviceCode"`
	CategoryCode      string   `json:"categoryCode"`
	SeverityCode      string   `json:"severityCode"`
	CommunicationBody string   `json:"communicationBody"`
	AttachmentSetID   string   `json:"attachmentSetId,omitempty"`
	Language          string   `json:"language,omitempty"`
	IssueType         string   `json:"issueType,omitempty"`
	CCEmails          []string `json:"ccEmailAddresses,omitempty"`
}

func (h *Handler) handleCreateCase(_ context.Context, in *handleCreateCaseInput) (*createCaseOutput, error) {
	options := CreateCaseOptions{
		Subject: in.Subject, ServiceCode: in.ServiceCode, CategoryCode: in.CategoryCode,
		SeverityCode: in.SeverityCode, CommunicationBody: in.CommunicationBody,
		AttachmentSetID: in.AttachmentSetID, Language: in.Language, IssueType: in.IssueType,
		CCEmails: in.CCEmails,
	}
	if err := validateCreateCase(options); err != nil {
		return nil, err
	}
	c2, err := h.Backend.CreateCaseWithOptions(options)
	if err != nil {
		return nil, err
	}

	return &createCaseOutput{CaseID: c2.CaseID}, nil
}

type handleDescribeCasesInput struct {
	IncludeCommunications *bool    `json:"includeCommunications,omitempty"`
	Language              string   `json:"language"`
	NextToken             string   `json:"nextToken,omitempty"`
	DisplayID             string   `json:"displayId,omitempty"`
	AfterTime             string   `json:"afterTime,omitempty"`
	BeforeTime            string   `json:"beforeTime,omitempty"`
	CaseIDList            []string `json:"caseIdList"`
	MaxResults            int      `json:"maxResults,omitempty"`
	IncludeResolvedCases  bool     `json:"includeResolvedCases"`
}

func (h *Handler) handleDescribeCases(
	_ context.Context,
	in *handleDescribeCasesInput,
) (*describeCasesOutput, error) {
	if err := validatePageSize(in.MaxResults); err != nil {
		return nil, err
	}
	if err := validateCaseIDList(in.CaseIDList); err != nil {
		return nil, err
	}
	afterValue, err := parseFilterTime(in.AfterTime)
	if err != nil {
		return nil, err
	}
	beforeValue, err := parseFilterTime(in.BeforeTime)
	if err != nil {
		return nil, err
	}
	includeComms := in.IncludeCommunications == nil || *in.IncludeCommunications
	after := nonZeroTimePointer(afterValue)
	before := nonZeroTimePointer(beforeValue)
	cases, token, err := h.Backend.DescribeCasesWithOptions(DescribeCasesOptions{
		CaseIDs: in.CaseIDList, DisplayID: in.DisplayID, Language: in.Language,
		IncludeResolvedCases: in.IncludeResolvedCases, IncludeCommunications: includeComms,
		AfterTime: after, BeforeTime: before, MaxResults: in.MaxResults, NextToken: in.NextToken,
	})
	if err != nil {
		return nil, err
	}

	views := make([]caseView, 0, len(cases))
	for _, cs := range cases {
		v := caseView{
			CaseID:       cs.CaseID,
			DisplayID:    cs.DisplayID,
			Subject:      cs.Subject,
			Status:       cs.Status,
			ServiceCode:  cs.ServiceCode,
			CategoryCode: cs.CategoryCode,
			SeverityCode: cs.SeverityCode,
			Language:     cs.Language,
			SubmittedBy:  cs.SubmittedBy,
			CCEmails:     cs.CCEmails,
			CreatedTime:  cs.CreatedTime.UTC().Format(time.RFC3339),
		}
		if includeComms {
			comms, nextToken := h.Backend.RecentCommunications(cs.CaseID)
			v.RecentCommunications = &recentCaseCommunicationsView{
				Communications: communicationViews(comms),
				NextToken:      nextToken,
			}
		}
		views = append(views, v)
	}

	return &describeCasesOutput{Cases: views, NextToken: token}, nil
}

type handleResolveCaseInput struct {
	CaseID string `json:"caseId"`
}

func (h *Handler) handleResolveCase(_ context.Context, in *handleResolveCaseInput) (*resolveCaseOutput, error) {
	if in.CaseID == "" {
		return nil, fmt.Errorf("%w: caseId is required", ErrValidation)
	}
	initialStatus, cs, err := h.Backend.ResolveCaseWithStatus(in.CaseID)
	if err != nil {
		return nil, err
	}

	return &resolveCaseOutput{
		InitialCaseStatus: initialStatus,
		FinalCaseStatus:   cs.Status,
	}, nil
}

type describeCreateCaseOptionsInput struct {
	IssueType    string `json:"issueType"`
	ServiceCode  string `json:"serviceCode"`
	CategoryCode string `json:"categoryCode"`
	Language     string `json:"language"`
}

type communicationTypeOptionsView struct {
	Type                string          `json:"type"`
	SupportedHours      []SupportedHour `json:"supportedHours"`
	DatesWithoutSupport []DateInterval  `json:"datesWithoutSupport"`
}

type describeCreateCaseOptionsOutput struct {
	LanguageAvailability string                         `json:"languageAvailability"`
	CommunicationTypes   []communicationTypeOptionsView `json:"communicationTypes"`
}

func (h *Handler) handleDescribeCreateCaseOptions(
	_ context.Context,
	in *describeCreateCaseOptionsInput,
) (*describeCreateCaseOptionsOutput, error) {
	if !validIssueType(in.IssueType) || in.ServiceCode == "" || in.CategoryCode == "" || !validLanguage(in.Language) {
		return nil, fmt.Errorf("%w: issueType, serviceCode, categoryCode, and language are required", ErrValidation)
	}
	result := h.Backend.DescribeCreateCaseOptions(in.IssueType, in.ServiceCode, in.CategoryCode, in.Language)

	views := make([]communicationTypeOptionsView, 0, len(result.CommunicationTypes))
	for _, ct := range result.CommunicationTypes {
		views = append(views, communicationTypeOptionsView(ct))
	}

	return &describeCreateCaseOptionsOutput{
		CommunicationTypes:   views,
		LanguageAvailability: result.LanguageAvailability,
	}, nil
}
