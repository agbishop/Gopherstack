package identitystore

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/labstack/echo/v5"
)

// ----------------------------------------
// Smithy `pattern` trait constraints
// ----------------------------------------
//
// The regexes below are transcribed from the identitystore service model's
// `pattern` trait constraints for API version 2020-06-15, verified against
// botocore's vendored service-2.json (site-packages/botocore/data/
// identitystore/2020-06-15/service-2.json.gz) rather than the older
// aws-sdk-go@v1.55.5 bundled api-2.json -- the v1 model turned out to be
// stale (it is missing the User.Website/Birthdate/Photos/Roles members
// entirely, which DO exist on both the real service and on
// aws-sdk-go-v2/service/identitystore/types.User; trusting it would have
// produced a false "gopherstack invented these fields" diagnosis). This is
// parity-principles.md rule 2 applied one level deeper: verify against the
// real, CURRENT service model, not merely against whichever bundled SDK
// model happens to be on disk.
//
// Go's regexp package (RE2) supports the \p{L}/\p{M}/\p{N}/\p{P}/\p{S}
// Unicode general-category classes used throughout these patterns.
var (
	// patternUserName matches the UserName shape (also reused for
	// ExternalIdIssuer/ExternalIdIdentifier, which share the identical
	// character class): letters, marks, symbols, numbers, and punctuation.
	// No whitespace.
	patternUserName = regexp.MustCompile(`^[\p{L}\p{M}\p{S}\p{N}\p{P}]+$`)

	// patternGroupDisplayName matches the GroupDisplayName shape:
	// UserName's class plus tab, newline, carriage return, space, and
	// non-breaking space (U+00A0).
	patternGroupDisplayName = regexp.MustCompile("^[\\p{L}\\p{M}\\p{S}\\p{N}\\p{P}\t\n\r \u00A0]+$")

	// patternSensitiveString matches the SensitiveStringType shape shared by
	// most other free-text User/Group fields (User.DisplayName, NickName,
	// Title, ProfileUrl, Locale, PreferredLanguage, Timezone, UserType,
	// Website, Birthdate, Group.Description, Name.*, and the
	// Email/Address/PhoneNumber/Photo/Role sub-fields): GroupDisplayName's
	// class plus the ideographic space U+3000.
	patternSensitiveString = regexp.MustCompile("^[\\p{L}\\p{M}\\p{S}\\p{N}\\p{P}\t\n\r \u00A0\u3000]+$")

	// patternAttributePath matches the AttributePath shape used by
	// Filter.AttributePath, AttributeOperation.AttributePath, and
	// UniqueAttribute.AttributePath.
	patternAttributePath = regexp.MustCompile(
		`^(?:\p{L}+:\p{L}+:\p{L}+(?:\.\p{L}+){0,3}|\p{L}+(?:\.\p{L}+){0,2})$`,
	)

	// patternIdentityStoreID matches the IdentityStoreId shape: either the
	// short "d-" + 10 hex characters form, or a full UUID.
	patternIdentityStoreID = regexp.MustCompile(
		`^(?:d-[0-9a-f]{10}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`,
	)
)

// validatePattern reports a ValidationException-wrapped error when value is
// non-empty and does not match re. Every field validated through this
// helper is optional at the smithy level (or its required-ness is already
// checked by a separate non-empty check), so an empty string always passes.
func validatePattern(re *regexp.Regexp, fieldName, value string) error {
	if value == "" || re.MatchString(value) {
		return nil
	}

	return fmt.Errorf("%w: %s %q does not match the required pattern", ErrValidation, fieldName, value)
}

func validateSensitive(fieldName, value string) error {
	return validatePattern(patternSensitiveString, fieldName, value)
}

// ----------------------------------------
// Reserved names
// ----------------------------------------

// reservedName reports whether name is one of the two literal values that
// the real API documentation states are reserved. This is NOT encoded as a
// smithy enum/pattern trait, but it IS a real, documented constraint: the
// CreateUserRequest.UserName and CreateGroupRequest.DisplayName member
// documentation strings (see botocore's service-2.json) both say verbatim
// "Administrator and AWSAdministrators are reserved names and can't be used
// for users or groups." A prior audit pass dismissed this as unverified
// "operator guidance" because it isn't in the smithy structural constraints;
// it IS in the operation request shape's own reference documentation, which
// is a stronger source than general prose docs, so it is implemented here.
// It applies only to the two identifier-like fields the docs name --
// User.UserName (via CreateUser/UpdateUser) and Group.DisplayName (via
// CreateGroup/UpdateGroup) -- not to User.DisplayName, which is a distinct,
// non-identifying field the docs never mention in this context.
func reservedName(name string) bool {
	return name == "Administrator" || name == "AWSAdministrators"
}

func errReservedName(field, name string) error {
	return fmt.Errorf("%w: %s %q is a reserved name and cannot be used for users or groups", ErrValidation, field, name)
}

func validateUserNameValue(name string) error {
	if err := validatePattern(patternUserName, "UserName", name); err != nil {
		return err
	}

	if reservedName(name) {
		return errReservedName("UserName", name)
	}

	return nil
}

func validateGroupDisplayNameValue(name string) error {
	if err := validatePattern(patternGroupDisplayName, "DisplayName", name); err != nil {
		return err
	}

	if reservedName(name) {
		return errReservedName("DisplayName", name)
	}

	return nil
}

// ----------------------------------------
// CreateUser / CreateGroup payload validation
// ----------------------------------------

// sensitiveField pairs a wire field name with its value for batch pattern
// validation via validateSensitiveFields.
type sensitiveField struct {
	name  string
	value string
}

func validateSensitiveFields(fields []sensitiveField) error {
	for _, f := range fields {
		if err := validateSensitive(f.name, f.value); err != nil {
			return err
		}
	}

	return nil
}

// validateUserPayloadStrings validates every free-text CreateUserRequest
// field against its smithy pattern constraint (and UserName's reserved-name
// restriction). Called from CreateUser before any state is mutated.
func validateUserPayloadStrings(req *CreateUserRequest) error {
	if err := validateUserNameValue(req.UserName); err != nil {
		return err
	}

	if err := validateSensitiveFields([]sensitiveField{
		{"DisplayName", req.DisplayName},
		{"NickName", req.NickName},
		{"Title", req.Title},
		{"ProfileUrl", req.ProfileURL},
		{"Locale", req.Locale},
		{"PreferredLanguage", req.PreferredLang},
		{"Timezone", req.Timezone},
		{"UserType", req.UserType},
		{"Website", req.Website},
	}); err != nil {
		return err
	}

	if req.Name != nil {
		if err := validateNameFields(req.Name); err != nil {
			return err
		}
	}

	if err := validateEmails(req.Emails); err != nil {
		return err
	}

	if err := validateAddresses(req.Addresses); err != nil {
		return err
	}

	if err := validatePhoneNumbers(req.PhoneNumbers); err != nil {
		return err
	}

	if err := validatePhotos(req.Photos); err != nil {
		return err
	}

	return validateRoles(req.Roles)
}

// validateGroupPayloadStrings validates every free-text CreateGroupRequest
// field against its smithy pattern constraint (and DisplayName's
// reserved-name restriction).
func validateGroupPayloadStrings(req *CreateGroupRequest) error {
	if err := validateGroupDisplayNameValue(req.DisplayName); err != nil {
		return err
	}

	return validateSensitive("Description", req.Description)
}

func validateNameFields(n *Name) error {
	return validateSensitiveFields([]sensitiveField{
		{"Name.Formatted", n.Formatted},
		{"Name.FamilyName", n.FamilyName},
		{"Name.GivenName", n.GivenName},
		{"Name.MiddleName", n.MiddleName},
		{"Name.HonorificPrefix", n.HonorificPrefix},
		{"Name.HonorificSuffix", n.HonorificSuffix},
	})
}

func validateEmails(emails []Email) error {
	for i, e := range emails {
		if err := validateSensitiveFields([]sensitiveField{
			{fmt.Sprintf("Emails[%d].Value", i), e.Value},
			{fmt.Sprintf("Emails[%d].Type", i), e.Type},
		}); err != nil {
			return err
		}
	}

	return nil
}

func validateAddresses(addresses []Address) error {
	for i, a := range addresses {
		if err := validateSensitiveFields([]sensitiveField{
			{fmt.Sprintf("Addresses[%d].Formatted", i), a.Formatted},
			{fmt.Sprintf("Addresses[%d].StreetAddress", i), a.StreetAddress},
			{fmt.Sprintf("Addresses[%d].Locality", i), a.Locality},
			{fmt.Sprintf("Addresses[%d].Region", i), a.Region},
			{fmt.Sprintf("Addresses[%d].PostalCode", i), a.PostalCode},
			{fmt.Sprintf("Addresses[%d].Country", i), a.Country},
			{fmt.Sprintf("Addresses[%d].Type", i), a.Type},
		}); err != nil {
			return err
		}
	}

	return nil
}

func validatePhoneNumbers(numbers []PhoneNumber) error {
	for i, p := range numbers {
		if err := validateSensitiveFields([]sensitiveField{
			{fmt.Sprintf("PhoneNumbers[%d].Value", i), p.Value},
			{fmt.Sprintf("PhoneNumbers[%d].Type", i), p.Type},
		}); err != nil {
			return err
		}
	}

	return nil
}

func validatePhotos(photos []Photo) error {
	for i, p := range photos {
		if err := validateSensitiveFields([]sensitiveField{
			{fmt.Sprintf("Photos[%d].Value", i), p.Value},
			{fmt.Sprintf("Photos[%d].Type", i), p.Type},
			{fmt.Sprintf("Photos[%d].Display", i), p.Display},
		}); err != nil {
			return err
		}
	}

	return nil
}

func validateRoles(roles []Role) error {
	for i, r := range roles {
		if err := validateSensitiveFields([]sensitiveField{
			{fmt.Sprintf("Roles[%d].Value", i), r.Value},
			{fmt.Sprintf("Roles[%d].Type", i), r.Type},
		}); err != nil {
			return err
		}
	}

	return nil
}

// validateExternalIDs validates every ExternalId.Issuer/Id pair against the
// ExternalIdIssuer/ExternalIdIdentifier patterns (both share UserName's
// no-whitespace character class).
func validateExternalIDs(ids []ExternalID) error {
	for i, ext := range ids {
		if err := validatePattern(patternUserName, fmt.Sprintf("ExternalIds[%d].Issuer", i), ext.Issuer); err != nil {
			return err
		}

		if err := validatePattern(patternUserName, fmt.Sprintf("ExternalIds[%d].Id", i), ext.ID); err != nil {
			return err
		}
	}

	return nil
}

// ----------------------------------------
// UpdateUser / UpdateGroup AttributeOperation validation
// ----------------------------------------

// validateAttributeOperation checks an AttributeOperation's AttributePath
// against the real AttributePath pattern, and -- for paths that carry a
// string or string-collection value with its own pattern constraint -- the
// operation's AttributeValue too. Shared by UpdateUser and UpdateGroup;
// validateGroupAttributeOperation layers Group.DisplayName's reserved-name
// check on top, since that restriction does not apply to User.DisplayName.
func validateAttributeOperation(op attributeOperation) error {
	if err := validatePattern(patternAttributePath, "AttributePath", op.AttributePath); err != nil {
		return err
	}

	path := strings.ToLower(op.AttributePath)
	strVal, isStr := op.AttributeValue.(string)

	switch path {
	case attrUserNameKey:
		if isStr {
			return validateUserNameValue(strVal)
		}
	case attrDisplayName,
		attrNickName, attrTitle, attrProfileURL, attrLocale, attrPreferredLanguage,
		attrTimezone, attrUserType, attrWebsite, attrBirthdate, attrDescription,
		attrNameGivenName, attrNameFamilyName, attrNameMiddleName,
		attrNameFormatted, attrNameHonorificPrefix, attrNameHonorificSuffix:
		if isStr {
			return validateSensitive(op.AttributePath, strVal)
		}
	case attrEmails:
		return validateEmails(parseEmails(op.AttributeValue))
	case attrAddresses:
		return validateAddresses(parseAddresses(op.AttributeValue))
	case attrPhoneNumbers:
		return validatePhoneNumbers(parsePhoneNumbers(op.AttributeValue))
	case attrPhotos:
		return validatePhotos(parsePhotos(op.AttributeValue))
	case attrRoles:
		return validateRoles(parseRoles(op.AttributeValue))
	case attrExternalIDs:
		return validateExternalIDs(parseExternalIDs(op.AttributeValue))
	}

	return nil
}

// validateGroupAttributeOperation extends validateAttributeOperation with
// Group.DisplayName's reserved-name restriction (Administrator /
// AWSAdministrators), which real AWS documents only for the two
// identifier-like fields User.UserName and Group.DisplayName -- see
// reservedName's doc comment.
func validateGroupAttributeOperation(op attributeOperation) error {
	if err := validateAttributeOperation(op); err != nil {
		return err
	}

	if strings.ToLower(op.AttributePath) != attrDisplayName {
		return nil
	}

	if strVal, ok := op.AttributeValue.(string); ok && reservedName(strVal) {
		return errReservedName("DisplayName", strVal)
	}

	return nil
}

// ----------------------------------------
// ListUsers / ListGroups Filters validation
// ----------------------------------------

// validateFilters checks every Filter.AttributePath in a ListUsers/ListGroups
// request against the real AttributePath pattern. This is purely a
// syntax-level check (does the string have the right shape?), independent
// of whether gopherstack's matching logic recognizes the path as one it can
// actually filter on -- see userMatchesFilter/groupMatchesFilter for that
// semantic layer, and PARITY.md for why the supported-path superset itself
// is intentionally not narrowed by this pass.
func validateFilters(filters []ListFilter) error {
	for i, f := range filters {
		fieldName := fmt.Sprintf("Filters[%d].AttributePath", i)
		if err := validatePattern(patternAttributePath, fieldName, f.AttributePath); err != nil {
			return err
		}
	}

	return nil
}

// ----------------------------------------
// IdentityStoreId validation
// ----------------------------------------

// errResponseWritten is returned by requireIdentityStoreID and
// parseAlternateIDRequest instead of the writer's nil-on-success result, so
// that their ~20 callers' existing `if err != nil { return err }` checks
// actually stop instead of silently falling through to a second, corrupting
// write -- or, on the 8 mutation paths, the mutation itself -- on an
// already-rejected request (gopherstack-n7nk, same class as elasticache's
// gopherstack-8haq). Handler() translates it back to nil at the top of the
// dispatch chain.
var errResponseWritten = errors.New("identitystore: response already written")

// requireIdentityStoreID checks that id is present and matches the real
// IdentityStoreId shape's pattern (either "d-" + 10 hex chars, or a UUID),
// writing the appropriate ValidationException response and returning
// errResponseWritten when it does not. Centralizes what was previously an
// empty-check-only block repeated at every one of this service's ~18
// operation handlers.
func (h *Handler) requireIdentityStoreID(c *echo.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		_ = h.writeError(c, http.StatusBadRequest, "ValidationException", "IdentityStoreId is required")

		return errResponseWritten
	}

	if err := validatePattern(patternIdentityStoreID, "IdentityStoreId", id); err != nil {
		_ = h.writeError(c, http.StatusBadRequest, "ValidationException", err.Error())

		return errResponseWritten
	}

	return nil
}
