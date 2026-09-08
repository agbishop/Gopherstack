package macie2

import (
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) customDataIDARN(id string) string {
	return arn.Build("macie2", b.region, b.accountID, fmt.Sprintf("custom-data-identifier/%s", id))
}

// CreateCustomDataIdentifier creates a new custom data identifier.
func (b *InMemoryBackend) CreateCustomDataIdentifier(
	name, description, regex string,
	ignoreWords, keywords []string,
	severityLevels []SeverityLevel,
	maxMatchDistance *int32,
	tags map[string]string,
) (string, error) {
	if _, compileErr := regexp.Compile(regex); compileErr != nil {
		return "", fmt.Errorf("%w: regex is invalid: %s", ErrValidation, compileErr.Error())
	}

	dist := defaultMatchDist
	if maxMatchDistance != nil {
		if *maxMatchDistance < minMatchDist || *maxMatchDistance > maxMatchDist {
			return "", fmt.Errorf(
				"%w: maximumMatchDistance must be between %d and %d",
				ErrValidation, minMatchDist, maxMatchDist,
			)
		}

		dist = *maxMatchDistance
	}

	b.mu.Lock("CreateCustomDataIdentifier")
	defer b.mu.Unlock()

	id := uuid.New().String()
	now := time.Now().UTC()

	cdi := &storedCustomDataID{
		ID:                   id,
		Arn:                  b.customDataIDARN(id),
		Name:                 name,
		Description:          description,
		Regex:                regex,
		IgnoreWords:          ignoreWords,
		Keywords:             keywords,
		SeverityLevels:       severityLevels,
		MaximumMatchDistance: dist,
		CreatedAt:            now,
		Tags:                 maps.Clone(tags),
	}

	b.customDataIDs.Put(cdi)
	if len(tags) > 0 {
		b.tags[cdi.Arn] = maps.Clone(tags)
	}

	return id, nil
}

// GetCustomDataIdentifier retrieves a custom data identifier. Real AWS soft
// deletes custom data identifiers (DeleteCustomDataIdentifier never removes
// them), so Get keeps succeeding for a deleted identifier and reports
// deleted:true instead of 404ing -- only an identifier that never existed
// 404s.
func (b *InMemoryBackend) GetCustomDataIdentifier(id string) (*CustomDataIdentifier, error) {
	b.mu.RLock("GetCustomDataIdentifier")
	defer b.mu.RUnlock()

	cdi, ok := b.customDataIDs.Get(id)
	if !ok {
		return nil, ErrCustomDataIDNotFound
	}

	cp := cdi.CustomDataIdentifier
	cp.Tags = maps.Clone(cdi.Tags)

	return &cp, nil
}

// DeleteCustomDataIdentifier soft-deletes a custom data identifier.
func (b *InMemoryBackend) DeleteCustomDataIdentifier(id string) error {
	b.mu.Lock("DeleteCustomDataIdentifier")
	defer b.mu.Unlock()

	cdi, ok := b.customDataIDs.Get(id)
	if !ok {
		return ErrCustomDataIDNotFound
	}

	cdi.Deleted = true

	return nil
}

// ListCustomDataIdentifiers returns summaries of all non-deleted custom data identifiers.
func (b *InMemoryBackend) ListCustomDataIdentifiers(
	limit int,
	token string,
) ([]*CustomDataIdentifierSummary, string, error) {
	b.mu.RLock("ListCustomDataIdentifiers")
	defer b.mu.RUnlock()

	data, next := mapSortPaginate(
		b.customDataIDs.All(),
		func(cdi *storedCustomDataID) (*CustomDataIdentifierSummary, bool) {
			if cdi.Deleted {
				return nil, false
			}

			return &CustomDataIdentifierSummary{
				Arn:         cdi.Arn,
				CreatedAt:   cdi.CreatedAt,
				Description: cdi.Description,
				ID:          cdi.ID,
				Name:        cdi.Name,
			}, true
		},
		func(result []*CustomDataIdentifierSummary) {
			// Name has no uniqueness constraint (CreateCustomDataIdentifier never
			// checks for an existing Name); ID as a tiebreaker keeps a total order
			// so the offset-based page.NewHMAC cursor can't drop or duplicate
			// identifiers that tie on Name.
			sort.Slice(result, func(i, j int) bool {
				if result[i].Name != result[j].Name {
					return result[i].Name < result[j].Name
				}

				return result[i].ID < result[j].ID
			})
		},
		token,
		b.paginationSecret,
		limit,
	)

	return data, next, nil
}

// TestCustomDataIdentifier tests a regex against sample text.
func (b *InMemoryBackend) TestCustomDataIdentifier(
	regex string,
	ignoreWords, keywords []string,
	maxMatchDistance *int32,
	sampleText string,
) (int32, error) {
	re, err := regexp.Compile(regex)
	if err != nil {
		return 0, ErrValidation
	}

	matches := re.FindAllStringIndex(sampleText, -1)
	if len(matches) == 0 {
		return 0, nil
	}

	dist := defaultMatchDist
	if maxMatchDistance != nil {
		dist = *maxMatchDistance
	}

	count := int32(0)

	for _, match := range matches {
		matchedText := sampleText[match[0]:match[1]]

		if containsIgnoreWord(matchedText, ignoreWords) {
			continue
		}

		if len(keywords) > 0 && !hasKeywordBefore(sampleText, match[0], keywords, int(dist)) {
			continue
		}

		count++
	}

	return count, nil
}

func containsIgnoreWord(text string, ignoreWords []string) bool {
	lower := strings.ToLower(text)

	for _, iw := range ignoreWords {
		if strings.Contains(lower, strings.ToLower(iw)) {
			return true
		}
	}

	return false
}

func hasKeywordBefore(text string, matchStart int, keywords []string, dist int) bool {
	start := max(matchStart-dist, 0)

	preceding := strings.ToLower(text[start:matchStart])

	for _, kw := range keywords {
		if strings.Contains(preceding, strings.ToLower(kw)) {
			return true
		}
	}

	return false
}

// BatchGetCustomDataIdentifiers returns full details for the given IDs. Like
// GetCustomDataIdentifier, soft-deleted identifiers are still returned (with
// deleted:true); only genuinely unknown IDs are omitted (and reported by the
// caller via notFoundIdentifierIds).
func (b *InMemoryBackend) BatchGetCustomDataIdentifiers(ids []string) ([]*CustomDataIdentifier, error) {
	b.mu.RLock("BatchGetCustomDataIdentifiers")
	defer b.mu.RUnlock()

	result := make([]*CustomDataIdentifier, 0, len(ids))

	for _, id := range ids {
		cdi, ok := b.customDataIDs.Get(id)
		if !ok {
			continue
		}

		cp := cdi.CustomDataIdentifier
		cp.Tags = maps.Clone(cdi.Tags)
		result = append(result, &cp)
	}

	return result, nil
}

// managedDataIdentifiers is the static list of built-in Macie data identifiers.
var managedDataIdentifiers = []ManagedDataIdentifier{ //nolint:gochecknoglobals // existing issue.
	{Category: "CREDENTIALS", ID: "AWS_CREDENTIALS"}, //nolint:goconst // existing issue.
	{Category: "CREDENTIALS", ID: "PRIVATE_KEY"},
	{Category: "CREDENTIALS", ID: "AWS_SECRET_ACCESS_KEY"},
	{Category: "FINANCIAL_INFORMATION", ID: "CREDIT_CARD_NUMBER"},
	{Category: "FINANCIAL_INFORMATION", ID: "BANK_ACCOUNT_NUMBER_US"},
	{Category: "PERSONAL_INFORMATION", ID: "EMAIL_ADDRESS"}, //nolint:goconst // existing issue.
	{Category: "PERSONAL_INFORMATION", ID: "PHONE_NUMBER_US"},
	{Category: "PERSONAL_INFORMATION", ID: "NAME"},
	{Category: "PERSONAL_INFORMATION", ID: "US_SOCIAL_SECURITY_NUMBER"},
	{Category: "PERSONAL_INFORMATION", ID: "US_DRIVERS_LICENSE"},
	{Category: "PERSONAL_INFORMATION", ID: "US_PASSPORT_NUMBER"},
	{Category: "PERSONAL_INFORMATION", ID: "DATE_OF_BIRTH"},
	{Category: "PERSONAL_INFORMATION", ID: "ADDRESS"},
}

// ListManagedDataIdentifiers returns the built-in Macie data identifiers.
func (b *InMemoryBackend) ListManagedDataIdentifiers() ([]ManagedDataIdentifier, error) {
	result := make([]ManagedDataIdentifier, len(managedDataIdentifiers))
	copy(result, managedDataIdentifiers)

	return result, nil
}
