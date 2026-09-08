package cognitoidp

import "time"

// User represents a Cognito user within a pool.
type User struct {
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
	ConfirmCodeExpiresAt time.Time `json:"confirmCodeExpiresAt"`
	LastAuthTime         time.Time `json:"lastAuthTime"`
	// TempPasswordIssuedAt is set whenever Status transitions to
	// UserStatusForceChangePassword; postCredentialCheckLocked uses it against the
	// pool's PasswordPolicy.TemporaryPasswordValidityDays to expire stale temp passwords.
	TempPasswordIssuedAt time.Time         `json:"tempPasswordIssuedAt"`
	Attributes           map[string]string `json:"attributes,omitempty"`
	UserPoolID           string            `json:"userPoolID,omitempty"`
	Sub                  string            `json:"sub,omitempty"`
	Username             string            `json:"username,omitempty"`
	PasswordHash         string            `json:"passwordHash,omitempty"`
	Status               string            `json:"status,omitempty"`
	ConfirmCode          string            `json:"confirmCode,omitempty"`
	PreferredMfaSetting  string            `json:"preferredMfaSetting,omitempty"`
	TOTPSecret           string            `json:"totpSecret,omitempty"`
	// SRPSalt and SRPVerifier are the padded-hex SRP-6a salt and verifier (v = g^x mod
	// N) derived from the plaintext password at every point PasswordHash is set (see
	// hashAndSRP in srp.go). USER_SRP_AUTH validates against these instead of PasswordHash.
	SRPSalt            string          `json:"srpSalt,omitempty"`
	SRPVerifier        string          `json:"srpVerifier,omitempty"`
	UserMFASettingList []string        `json:"userMFASettingList,omitempty"`
	MFAOptions         []MFAOptionType `json:"mfaOptions,omitempty"`
	LinkedProviders    []ProviderLink  `json:"linkedProviders,omitempty"`
	Enabled            bool            `json:"enabled,omitempty"`
	TOTPVerified       bool            `json:"totpVerified,omitempty"`
}

type adminGetUserInput struct {
	UserPoolID string `json:"UserPoolId,omitempty"`
	Username   string `json:"Username,omitempty"`
}

type adminGetUserOutput struct {
	Username             string          `json:"Username,omitempty"`
	UserStatus           string          `json:"UserStatus,omitempty"`
	UserAttributes       []attributeType `json:"UserAttributes,omitempty"`
	PreferredMfaSetting  string          `json:"PreferredMfaSetting,omitempty"`
	UserMFASettingList   []string        `json:"UserMFASettingList,omitempty"`
	UserCreateDate       float64         `json:"UserCreateDate,omitempty"`
	UserLastModifiedDate float64         `json:"UserLastModifiedDate,omitempty"`
	Enabled              bool            `json:"Enabled"`
}

type adminDeleteUserInput struct {
	UserPoolID string `json:"UserPoolId,omitempty"`
	Username   string `json:"Username,omitempty"`
}

type adminDeleteUserOutput struct{}

type listUsersInput struct {
	UserPoolID      string `json:"UserPoolId,omitempty"`
	Filter          string `json:"Filter,omitempty"`
	PaginationToken string `json:"PaginationToken,omitempty"`
	Limit           int    `json:"Limit,omitempty"`
}

type listUsersOutput struct {
	PaginationToken string         `json:"PaginationToken,omitempty"`
	Users           []*userSummary `json:"Users"`
}

type userSummary struct {
	Username         string          `json:"Username,omitempty"`
	UserStatus       string          `json:"UserStatus,omitempty"`
	Attributes       []attributeType `json:"Attributes,omitempty"`
	MFAOptions       []mfaOptionType `json:"MFAOptions,omitempty"`
	UserCreateDate   float64         `json:"UserCreateDate,omitempty"`
	UserLastModified float64         `json:"UserLastModifiedDate,omitempty"`
	Enabled          bool            `json:"Enabled"`
}

type providerUserIdentifierType struct {
	ProviderAttributeName  string `json:"ProviderAttributeName,omitempty"`
	ProviderAttributeValue string `json:"ProviderAttributeValue,omitempty"`
	ProviderName           string `json:"ProviderName,omitempty"`
}

type adminDisableUserInput struct {
	UserPoolID string `json:"UserPoolId,omitempty"`
	Username   string `json:"Username,omitempty"`
}

type adminDisableUserOutput struct{}

type adminEnableUserInput struct {
	UserPoolID string `json:"UserPoolId,omitempty"`
	Username   string `json:"Username,omitempty"`
}

type adminEnableUserOutput struct{}

type deleteUserInput struct {
	AccessToken string `json:"AccessToken,omitempty"`
}

type deleteUserOutput struct{}

// getUserWithMFAOutput is the wire format for GetUser, including MFA preference fields.
type getUserWithMFAOutput struct {
	Username            string          `json:"Username,omitempty"`
	PreferredMfaSetting string          `json:"PreferredMfaSetting,omitempty"`
	UserAttributes      []attributeType `json:"UserAttributes,omitempty"`
	UserMFASettingList  []string        `json:"UserMFASettingList,omitempty"`
}

type getUserAccurateInput struct {
	AccessToken string `json:"AccessToken,omitempty"`
}

type adminCreateUserFullInput struct {
	UserPoolID             string          `json:"UserPoolId,omitempty"`
	Username               string          `json:"Username,omitempty"`
	TemporaryPassword      string          `json:"TemporaryPassword,omitempty"`
	UserAttributes         []attributeType `json:"UserAttributes,omitempty"`
	MessageAction          string          `json:"MessageAction,omitempty"`
	DesiredDeliveryMediums []string        `json:"DesiredDeliveryMediums,omitempty"`
	ForceAliasCreation     bool            `json:"ForceAliasCreation,omitempty"`
}

type adminCreateUserFullOutput struct {
	User *adminUserJSON `json:"User,omitempty"`
}

// adminUserJSON is the wire shape for a UserType member (AdminCreateUser's
// User field, ListUsersInGroup's Users list) -- distinct from the
// standalone GetUser/AdminGetUser response types, which legitimately key
// their attribute list "UserAttributes". UserType's own member is
// "Attributes" (cognitoidentityprovider@v1.67.4 deserializers.go, case
// "Attributes" in awsAwsjson11_deserializeDocumentUserType), so tagging it
// "UserAttributes" here silently dropped every user's attributes for any
// real client.
type adminUserJSON struct {
	Username             string          `json:"Username,omitempty"`
	UserStatus           string          `json:"UserStatus,omitempty"`
	UserAttributes       []attributeType `json:"Attributes,omitempty"`
	MFAOptions           []mfaOptionType `json:"MFAOptions,omitempty"`
	UserCreateDate       float64         `json:"UserCreateDate,omitempty"`
	UserLastModifiedDate float64         `json:"UserLastModifiedDate,omitempty"`
	Enabled              bool            `json:"Enabled,omitempty"`
}

type adminSetUserPasswordFullInput struct {
	UserPoolID string `json:"UserPoolId,omitempty"`
	Username   string `json:"Username,omitempty"`
	Password   string `json:"Password,omitempty"`
	Permanent  bool   `json:"Permanent,omitempty"`
}

type adminSetUserPasswordFullOutput struct{}

type getUserAuthFactorsInput struct {
	AccessToken string `json:"AccessToken,omitempty"`
}

type getUserAuthFactorsOutput struct {
	Username                  string   `json:"Username,omitempty"`
	ConfiguredUserAuthFactors []string `json:"ConfiguredUserAuthFactors,omitempty"`
}

type adminGetUserAuthFactorsInput struct {
	UserPoolID string `json:"UserPoolId,omitempty"`
	Username   string `json:"Username,omitempty"`
}

// adminGetUserAuthFactorsOutput mirrors the AWS SDK's
// AdminGetUserAuthFactorsOutput (Username, ConfiguredUserAuthFactors,
// PreferredMfaSetting, UserMFASettingList -- field-diffed against
// aws-sdk-go-v2/service/cognitoidentityprovider.AdminGetUserAuthFactorsOutput).
type adminGetUserAuthFactorsOutput struct {
	Username                  string   `json:"Username,omitempty"`
	ConfiguredUserAuthFactors []string `json:"ConfiguredUserAuthFactors,omitempty"`
	PreferredMfaSetting       string   `json:"PreferredMfaSetting,omitempty"`
	UserMFASettingList        []string `json:"UserMFASettingList,omitempty"`
}
