package support

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultAccountID    = "000000000000"
	defaultCaseLanguage = "en"
	defaultIssueType    = "technical"
	defaultSubmittedBy  = "customer"
	defaultPageSize     = 100
	minPageSize         = 10
	maxPageSize         = 100

	// maxOpenCases backs CaseCreationLimitExceeded ("the case creation limit
	// for the account has been exceeded" / "you have exceeded the number of
	// cases you can have open" per the real AWS documentation, which
	// publishes no exact number). It caps concurrently OPEN (non-resolved)
	// cases, not lifetime case creation -- resolving a case frees a slot,
	// matching the "cases you can have open" wording.
	maxOpenCases = 500

	// maxCaseIDList backs DescribeCasesInput.CaseIdList's documented "The
	// maximum number of cases is 100" limit (api_op_DescribeCases.go).
	maxCaseIDList = 100
)

// openCaseCountLocked returns the number of cases not currently in the
// resolved status. The caller must hold b.mu.
func (b *InMemoryBackend) openCaseCountLocked() int {
	count := 0

	for _, cs := range b.cases.All() {
		if cs.Status != caseStatusResolved {
			count++
		}
	}

	return count
}

// CreateCase creates a new support case.
func (b *InMemoryBackend) CreateCase(subject, serviceCode, categoryCode, severityCode, body string) (*Case, error) {
	b.mu.Lock("CreateCase")
	defer b.mu.Unlock()

	if b.openCaseCountLocked() >= maxOpenCases {
		return nil, fmt.Errorf("%w: too many open cases", ErrCaseCreationLimitExceeded)
	}

	caseID := "case-" + uuid.New().String()[:8]
	c := &Case{
		CaseID:       caseID,
		Subject:      subject,
		Status:       caseStatusOpened,
		ServiceCode:  serviceCode,
		CategoryCode: categoryCode,
		SeverityCode: severityCode,
		Body:         body,
		CreatedTime:  time.Now(),
	}
	b.cases.Put(c)

	cp := *c

	return &cp, nil
}

// DescribeCases returns all support cases, optionally filtered by caseIds.
// When includeResolvedCases is false, resolved cases are excluded.
//
// Fast path: when caseIDs are supplied, look them up directly in the
// case-ID-keyed map instead of scanning every case in the backend.
func (b *InMemoryBackend) DescribeCases(caseIDs []string, includeResolvedCases bool) []Case {
	b.mu.RLock("DescribeCases")
	defer b.mu.RUnlock()

	if len(caseIDs) > 0 {
		out := make([]Case, 0, len(caseIDs))

		for _, id := range caseIDs {
			c, ok := b.cases.Get(id)
			if !ok {
				continue
			}

			if !includeResolvedCases && c.Status == caseStatusResolved {
				continue
			}

			out = append(out, *c)
		}

		return out
	}

	all := b.cases.All()
	out := make([]Case, 0, len(all))

	for _, c := range all {
		if !includeResolvedCases && c.Status == caseStatusResolved {
			continue
		}

		out = append(out, *c)
	}

	return out
}

// ResolveCase resolves a support case by caseId. Resolving an
// already-resolved case is a no-op (see ResolveCaseWithStatus): real AWS
// Support has no "already resolved" error for this operation.
func (b *InMemoryBackend) ResolveCase(caseID string) (*Case, error) {
	b.mu.Lock("ResolveCase")
	defer b.mu.Unlock()

	c, ok := b.cases.Get(caseID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, caseID)
	}

	if c.Status != caseStatusResolved {
		now := time.Now()
		c.Status = caseStatusResolved
		c.ResolvedTime = &now
	}

	cp := *c

	return &cp, nil
}

// AddCaseInternal seeds a case directly into the backend (for testing).
func (b *InMemoryBackend) AddCaseInternal(c *Case) {
	b.mu.Lock("AddCaseInternal")
	defer b.mu.Unlock()

	cp := *c
	b.cases.Put(&cp)
}

// DescribeCreateCaseOptions returns available case creation options.
// Chat support hours differ by locale: Japanese support runs 07:00–21:00 JST,
// all other locales use 06:00–22:00 UTC.
func (b *InMemoryBackend) DescribeCreateCaseOptions(_, _, _, language string) *DescribeCreateCaseOptionsResult {
	chatStart, chatEnd := "06:00", "22:00"
	if language == "ja" {
		chatStart, chatEnd = "07:00", "21:00"
	}

	return &DescribeCreateCaseOptionsResult{
		LanguageAvailability: "available",
		CommunicationTypes: []CommunicationTypeOptions{
			{
				Type: "web",
				SupportedHours: []SupportedHour{
					{StartTime: "00:00", EndTime: "24:00"},
				},
				DatesWithoutSupport: []DateInterval{},
			},
			{
				Type: "chat",
				SupportedHours: []SupportedHour{
					{StartTime: chatStart, EndTime: chatEnd},
				},
				DatesWithoutSupport: []DateInterval{},
			},
		},
	}
}

// CreateCaseWithOptions stores an AWS-shaped support case and its initial communication.
func (b *InMemoryBackend) CreateCaseWithOptions(in CreateCaseOptions) (*Case, error) {
	b.mu.Lock("CreateCaseWithOptions")
	defer b.mu.Unlock()

	// Checked before consuming the attachment set: a rejected case creation
	// must not burn a single-use staged attachment set.
	if b.openCaseCountLocked() >= maxOpenCases {
		return nil, fmt.Errorf("%w: too many open cases", ErrCaseCreationLimitExceeded)
	}

	attachments, err := b.consumeAttachmentSetLocked(in.AttachmentSetID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	b.nextDisplayID++
	cs := &Case{
		CaseID: fmt.Sprintf(
			"case-%s-%d-%s",
			defaultAccountID,
			now.Year(),
			strings.ReplaceAll(uuid.NewString(), "-", "")[:16],
		),
		DisplayID:    fmt.Sprintf("%010d", b.nextDisplayID),
		Subject:      in.Subject,
		Status:       caseStatusOpened,
		ServiceCode:  in.ServiceCode,
		CategoryCode: in.CategoryCode,
		SeverityCode: in.SeverityCode,
		Language:     defaultIfEmpty(in.Language, defaultCaseLanguage),
		IssueType:    defaultIfEmpty(in.IssueType, defaultIssueType),
		SubmittedBy:  defaultSubmittedBy,
		CCEmails:     append([]string(nil), in.CCEmails...),
		Body:         in.CommunicationBody,
		CreatedTime:  now,
	}
	b.cases.Put(cs)
	b.communications[cs.CaseID] = []Communication{
		newCommunication(cs.CaseID, in.CommunicationBody, in.CCEmails, attachments, now),
	}

	out := *cs
	out.CCEmails = append([]string(nil), cs.CCEmails...)

	return &out, nil
}

// DescribeCasesWithOptions returns ordered and paginated cases matching API filters.
func (b *InMemoryBackend) DescribeCasesWithOptions(in DescribeCasesOptions) ([]Case, string, error) {
	b.mu.RLock("DescribeCasesWithOptions")
	defer b.mu.RUnlock()

	// CaseIdNotFound is the only case-specific exception DescribeCases models
	// (aws-sdk-go-v2/service/support/deserializers.go
	// awsAwsjson11_deserializeOpErrorDescribeCases: CaseIdNotFound,
	// InternalServerError) -- an explicitly requested case ID that doesn't
	// exist must error, not be silently dropped from the results.
	for _, id := range in.CaseIDs {
		if !b.cases.Has(id) {
			return nil, "", fmt.Errorf("%w: %s", ErrNotFound, id)
		}
	}

	all := b.cases.All()
	cases := make([]Case, 0, len(all))
	for _, cs := range all {
		if matchCase(cs, in) {
			cp := *cs
			cp.CCEmails = append([]string(nil), cs.CCEmails...)
			cases = append(cases, cp)
		}
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].CreatedTime.After(cases[j].CreatedTime) })

	start, err := pageOffset(in.NextToken)
	if err != nil {
		return nil, "", err
	}

	return paginate(cases, start, pageLimit(in.MaxResults))
}

// ResolveCaseWithStatus resolves a case and returns its real prior status.
// Real AWS Support has no "already resolved" error for ResolveCase: the
// operation's modeled exceptions are only CaseIdNotFound and
// InternalServerError, and resolving an already-resolved case is a no-op that
// reports resolved for both initialCaseStatus and finalCaseStatus.
func (b *InMemoryBackend) ResolveCaseWithStatus(caseID string) (string, *Case, error) {
	b.mu.Lock("ResolveCaseWithStatus")
	defer b.mu.Unlock()

	cs, ok := b.cases.Get(caseID)
	if !ok {
		return "", nil, fmt.Errorf("%w: %s", ErrNotFound, caseID)
	}
	initialStatus := cs.Status
	if cs.Status != caseStatusResolved {
		now := time.Now()
		cs.Status = caseStatusResolved
		cs.ResolvedTime = &now
	}
	out := *cs

	return initialStatus, &out, nil
}

func validateCreateCase(in CreateCaseOptions) error {
	switch {
	case in.Subject == "":
		return fmt.Errorf("%w: subject is required", ErrValidation)
	case in.CommunicationBody == "":
		return fmt.Errorf("%w: communicationBody is required", ErrValidation)
	case len(in.CommunicationBody) > maxCommunicationBodySize:
		return fmt.Errorf("%w: communicationBody exceeds %d characters", ErrValidation, maxCommunicationBodySize)
	case len(in.CCEmails) > maxCCEmailAddresses:
		return fmt.Errorf("%w: too many ccEmailAddresses", ErrValidation)
	case in.Language != "" && !validLanguage(in.Language):
		return fmt.Errorf("%w: invalid language", ErrValidation)
	case in.IssueType != "" && !validIssueType(in.IssueType):
		return fmt.Errorf("%w: invalid issueType", ErrValidation)
	case in.SeverityCode != "" && !validSeverity(in.SeverityCode):
		return fmt.Errorf("%w: invalid severityCode", ErrValidation)
	}

	return nil
}

func validIssueType(value string) bool {
	return value == "technical" || value == "customer-service" || value == "account-and-billing"
}

func matchCase(cs *Case, in DescribeCasesOptions) bool {
	if !in.IncludeResolvedCases && cs.Status == caseStatusResolved {
		return false
	}
	if in.DisplayID != "" && cs.DisplayID != in.DisplayID {
		return false
	}
	if in.Language != "" && cs.Language != in.Language {
		return false
	}
	if in.AfterTime != nil && !cs.CreatedTime.After(*in.AfterTime) {
		return false
	}
	if in.BeforeTime != nil && !cs.CreatedTime.Before(*in.BeforeTime) {
		return false
	}
	if len(in.CaseIDs) == 0 {
		return true
	}

	return slices.Contains(in.CaseIDs, cs.CaseID)
}

func pageLimit(maxResults int) int {
	if maxResults == 0 {
		return defaultPageSize
	}

	return maxResults
}

func validateCaseIDList(caseIDs []string) error {
	if len(caseIDs) > maxCaseIDList {
		return fmt.Errorf("%w: caseIdList exceeds %d cases", ErrValidation, maxCaseIDList)
	}

	return nil
}

func validatePageSize(maxResults int) error {
	if maxResults != 0 && (maxResults < minPageSize || maxResults > maxPageSize) {
		return fmt.Errorf("%w: maxResults must be between %d and %d", ErrValidation, minPageSize, maxPageSize)
	}

	return nil
}

func pageOffset(token string) (int, error) {
	if token == "" {
		return 0, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid nextToken", ErrValidation)
	}

	var t pageTokenJSON
	if unmarshalErr := json.Unmarshal(decoded, &t); unmarshalErr != nil || t.Offset < 0 {
		return 0, fmt.Errorf("%w: invalid nextToken", ErrValidation)
	}

	return t.Offset, nil
}

func encodePageToken(offset int) string {
	data, _ := json.Marshal(pageTokenJSON{Offset: offset})

	return base64.RawURLEncoding.EncodeToString(data)
}

func paginate[T any](values []T, start, limit int) ([]T, string, error) {
	if start > len(values) {
		return nil, "", fmt.Errorf("%w: invalid nextToken", ErrValidation)
	}

	end := min(start+limit, len(values))
	nextToken := ""

	if end < len(values) {
		nextToken = encodePageToken(end)
	}

	return values[start:end], nextToken, nil
}

func defaultIfEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}
