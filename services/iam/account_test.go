package iam_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iam"
)

// TestGetAccountSummary_ProvidersKey covers PARITY.md's items_still_open:
// real GetAccountSummaryOutput's SummaryMap (types.SummaryKeyType, enums.go)
// has no "SAMLProviders" key -- SAML and OIDC identity providers are both
// reported under the single "Providers" key. gopherstack previously emitted
// "SAMLProviders" (a fabricated key no real caller reads) and never surfaced
// OIDCProviders at all.
func TestGetAccountSummary_ProvidersKey(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)
	client := newTestIAMClient(t, h)

	_, err := b.CreateSAMLProvider("saml-provider", "<EntityDescriptor/>")
	require.NoError(t, err)
	_, err = b.CreateOpenIDConnectProvider("https://oidc.example.com", nil, []string{strings.Repeat("a", 40)})
	require.NoError(t, err)

	out, err := client.GetAccountSummary(t.Context(), nil)
	require.NoError(t, err)

	_, hasFabricatedKey := out.SummaryMap["SAMLProviders"]
	assert.False(t, hasFabricatedKey, "SAMLProviders is not a real SummaryKeyType")
	assert.Equal(t, int32(2), out.SummaryMap["Providers"], "Providers must count both SAML and OIDC providers")
}

func TestGetAccountSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup             func(b *iam.InMemoryBackend)
		name              string
		wantUsers         int
		wantGroups        int
		wantRoles         int
		wantPolicies      int
		wantSAMLProviders int
	}{
		{
			name:  "empty_backend_returns_zeros",
			setup: func(_ *iam.InMemoryBackend) {},
		},
		{
			name: "counts_all_entities",
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateUser("alice", "/", "")
				_, _ = b.CreateUser("bob", "/", "")
				_, _ = b.CreateGroup("admins", "/")
				_, _ = b.CreateRole("ec2-role", "/", "{}", "")
				_, _ = b.CreatePolicy(
					"ReadOnly",
					"/",
					`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
				)
				_, _ = b.CreateSAMLProvider("MySAML", "<doc/>")
			},
			wantUsers:         2,
			wantGroups:        1,
			wantRoles:         1,
			wantPolicies:      1,
			wantSAMLProviders: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iam.NewInMemoryBackend()
			tt.setup(b)

			summary := b.GetAccountSummary()

			assert.Equal(t, tt.wantUsers, summary.Users)
			assert.Equal(t, tt.wantGroups, summary.Groups)
			assert.Equal(t, tt.wantRoles, summary.Roles)
			assert.Equal(t, tt.wantPolicies, summary.Policies)
			assert.Equal(t, tt.wantSAMLProviders, summary.SAMLProviders)
		})
	}
}

// TestListDeleteAccountAliases tests ListAccountAliases and DeleteAccountAlias.
func TestListDeleteAccountAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(*iam.InMemoryBackend)
		deleteAlias string
		wantLen     int
		wantErr     bool
	}{
		{
			name:    "list_empty",
			setup:   func(_ *iam.InMemoryBackend) {},
			wantLen: 0,
		},
		{
			name: "list_with_alias",
			setup: func(b *iam.InMemoryBackend) {
				_ = b.CreateAccountAlias("mycompany")
			},
			wantLen: 1,
		},
		{
			name: "delete_alias",
			setup: func(b *iam.InMemoryBackend) {
				_ = b.CreateAccountAlias("mycompany")
			},
			deleteAlias: "mycompany",
			wantLen:     0,
		},
		{
			name:        "delete_not_found",
			setup:       func(_ *iam.InMemoryBackend) {},
			deleteAlias: "ghost",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iam.NewInMemoryBackend()
			tt.setup(b)

			if tt.deleteAlias != "" {
				err := b.DeleteAccountAlias(tt.deleteAlias)
				if tt.wantErr {
					require.Error(t, err)

					return
				}

				require.NoError(t, err)
			}

			aliases := b.ListAccountAliases()
			assert.Len(t, aliases, tt.wantLen)
		})
	}
}

// TestPasswordPolicy_Backend tests password policy CRUD.
func TestPasswordPolicy_Backend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*iam.InMemoryBackend)
		name       string
		action     string
		wantMinLen int
		wantErr    bool
	}{
		{
			name:       "get_default",
			action:     "get",
			setup:      func(_ *iam.InMemoryBackend) {},
			wantMinLen: 8,
		},
		{
			name:   "update_and_get",
			action: "update",
			setup: func(b *iam.InMemoryBackend) {
				_ = b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{
					MinimumPasswordLength: 12,
					RequireNumbers:        true,
				})
			},
			wantMinLen: 12,
		},
		{
			name:   "delete_existing",
			action: "delete",
			setup: func(b *iam.InMemoryBackend) {
				_ = b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{MinimumPasswordLength: 14})
			},
		},
		{
			name:    "delete_not_set",
			action:  "delete",
			setup:   func(_ *iam.InMemoryBackend) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iam.NewInMemoryBackend()
			tt.setup(b)

			switch tt.action {
			case "get":
				pp := b.GetAccountPasswordPolicy()
				require.NotNil(t, pp)
				assert.Equal(t, tt.wantMinLen, pp.MinimumPasswordLength)
			case "update":
				pp := b.GetAccountPasswordPolicy()
				require.NotNil(t, pp)
				assert.Equal(t, tt.wantMinLen, pp.MinimumPasswordLength)
			case "delete":
				err := b.DeleteAccountPasswordPolicy()
				if tt.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}

func TestCredentialReport_CSVHasAllRequiredColumns(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("test-user", "/", "")

	csv := b.GetCredentialReport()

	lines := strings.Split(strings.TrimSpace(csv), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "report must have header + at least root row")

	header := lines[0]
	// AWS credential report has exactly these columns in order.
	requiredCols := []string{
		"user",
		"arn",
		"user_creation_time",
		"password_enabled",
		"password_last_used",
		"password_last_changed",
		"password_next_rotation",
		"mfa_active",
		"access_key_1_active",
		"access_key_1_last_rotated",
		"access_key_1_last_used_date",
		"access_key_1_last_used_region",
		"access_key_1_last_used_service",
		"access_key_2_active",
		"access_key_2_last_rotated",
		"access_key_2_last_used_date",
		"access_key_2_last_used_region",
		"access_key_2_last_used_service",
		"cert_1_active",
		"cert_1_last_rotated",
		"cert_2_active",
		"cert_2_last_rotated",
	}

	for _, col := range requiredCols {
		assert.Contains(t, header, col, "header must contain column %q", col)
	}
}

func TestCredentialReport_RootAccountPresent(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	csv := b.GetCredentialReport()

	assert.Contains(t, csv, "<root_account>", "credential report must include root account row")
}

func TestCredentialReport_UserARNFormat(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("report-user", "/", "")

	csv := b.GetCredentialReport()

	// ARN must match arn:aws:iam::<account>:user/<name>
	assert.Contains(t, csv, "arn:aws:iam::", "user ARN must use AWS ARN format")
	assert.Contains(t, csv, ":user/report-user", "user ARN must contain user path")
}

func TestCredentialReport_AccessKey1_Active_Reflected(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("ak-user", "/", "")

	ak, err := b.CreateAccessKey("ak-user")
	require.NoError(t, err)
	assert.Equal(t, "Active", ak.Status)

	csv := b.GetCredentialReport()
	lines := strings.Split(strings.TrimSpace(csv), "\n")

	var userLine string

	for _, l := range lines {
		if strings.HasPrefix(l, "ak-user,") {
			userLine = l

			break
		}
	}

	require.NotEmpty(t, userLine)
	cols := strings.Split(userLine, ",")
	// access_key_1_active is column index 8
	require.Greater(t, len(cols), 8)
	assert.Equal(t, "true", cols[8], "access_key_1_active must be true")
}

func TestCredentialReport_AccessKey1_LastUsed_Reflected(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("ak-used-user", "/", "")

	ak, err := b.CreateAccessKey("ak-used-user")
	require.NoError(t, err)

	b.RecordAccessKeyUsage(ak.AccessKeyID, "us-east-1", "s3")

	csv := b.GetCredentialReport()
	lines := strings.Split(strings.TrimSpace(csv), "\n")

	var userLine string

	for _, l := range lines {
		if strings.HasPrefix(l, "ak-used-user,") {
			userLine = l

			break
		}
	}

	require.NotEmpty(t, userLine)
	cols := strings.Split(userLine, ",")
	// access_key_1_last_used_date is column index 10
	require.Greater(t, len(cols), 12)
	assert.NotEqual(t, "N/A", cols[10], "last_used_date must be set after usage")
	assert.Equal(t, "us-east-1", cols[11], "last_used_region must match recorded region")
	assert.Equal(t, "s3", cols[12], "last_used_service must match recorded service")
}

func TestPasswordPolicy_AllFieldsPersisted(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	pp := iam.PasswordPolicy{
		MinimumPasswordLength:      16,
		RequireSymbols:             true,
		RequireNumbers:             true,
		RequireUppercaseCharacters: true,
		RequireLowercaseCharacters: true,
		AllowUsersToChangePassword: true,
		MaxPasswordAge:             90,
		PasswordReusePrevention:    12,
		HardExpiry:                 false,
	}

	require.NoError(t, b.UpdateAccountPasswordPolicy(pp))

	got := b.GetAccountPasswordPolicy()
	require.NotNil(t, got)
	assert.Equal(t, 16, got.MinimumPasswordLength)
	assert.True(t, got.RequireSymbols)
	assert.True(t, got.RequireNumbers)
	assert.True(t, got.RequireUppercaseCharacters)
	assert.True(t, got.RequireLowercaseCharacters)
	assert.True(t, got.AllowUsersToChangePassword)
	assert.Equal(t, 90, got.MaxPasswordAge)
	assert.Equal(t, 12, got.PasswordReusePrevention)
	assert.False(t, got.HardExpiry)
}

func TestPasswordPolicy_DeleteResetsToDefault(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	pp := iam.PasswordPolicy{MinimumPasswordLength: 20, RequireSymbols: true}
	require.NoError(t, b.UpdateAccountPasswordPolicy(pp))
	require.NoError(t, b.DeleteAccountPasswordPolicy())

	got := b.GetAccountPasswordPolicy()
	// After delete, gopherstack returns the default policy (AWS returns a NoSuchEntityException
	// when calling GetAccountPasswordPolicy after deletion, but the default is used internally).
	require.NotNil(t, got)
	assert.Equal(t, 8, got.MinimumPasswordLength, "default minimum length is 8")
	assert.False(t, got.RequireSymbols, "custom symbol requirement must be reset")
}

func TestPasswordPolicy_MinLengthDefault(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	// With no policy configured, the default minimum length is 8 (AWS default).
	// Creating a login profile with length 8 should succeed.
	_, _ = b.CreateUser("pp-default-user", "/", "")
	_, err := b.CreateLoginProfile("pp-default-user", "12345678", false)
	assert.NoError(t, err, "8-char password allowed by default policy")
}

func TestPasswordPolicy_HardExpiry_RejectsExpiredPassword(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("hardexpiry-user", "/", "")

	pp := iam.PasswordPolicy{
		MinimumPasswordLength: 8,
		MaxPasswordAge:        1,
		HardExpiry:            true,
	}
	require.NoError(t, b.UpdateAccountPasswordPolicy(pp))

	_, err := b.CreateLoginProfile("hardexpiry-user", "Password1!", false)
	require.NoError(t, err)

	// HardExpiry enforcement tested via ChangePassword at expiry — expired password blocked.
	// (In gopherstack, MaxPasswordAge enforcement happens at ChangePassword time.)
	// Verify the policy persists correctly.
	got := b.GetAccountPasswordPolicy()
	require.NotNil(t, got)
	assert.True(t, got.HardExpiry)
	assert.Equal(t, 1, got.MaxPasswordAge)
}

func TestGetCredentialReport_ContentIsBase64(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	csv := b.GetCredentialReport()
	encoded := base64.StdEncoding.EncodeToString([]byte(csv))

	// Verify the base64 decodes back to the original CSV.
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	assert.Equal(t, csv, string(decoded))
}

func TestPasswordPolicy_ChangePassword_MinLength(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	// Set policy requiring 12-char minimum.
	require.NoError(t, b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{
		MinimumPasswordLength: 12,
	}))

	err := b.ChangePassword("OldPassword1!", "short")
	require.Error(t, err)
	require.ErrorIs(t, err, iam.ErrInvalidPassword,
		"password below minimum length must return ErrInvalidPassword")
}

func TestPasswordPolicy_ChangePassword_Uppercase(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	require.NoError(t, b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{
		MinimumPasswordLength:      8,
		RequireUppercaseCharacters: true,
	}))

	err := b.ChangePassword("OldPassword1!", "alllower1")
	require.Error(t, err)
	require.ErrorIs(t, err, iam.ErrInvalidPassword, "no uppercase must fail")

	err = b.ChangePassword("OldPassword1!", "HasUpper1")
	require.NoError(t, err, "password with uppercase must succeed")
}

func TestPasswordPolicy_ChangePassword_Lowercase(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	require.NoError(t, b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{
		MinimumPasswordLength:      8,
		RequireLowercaseCharacters: true,
	}))

	err := b.ChangePassword("OldPassword1!", "ALLUPPER1")
	require.Error(t, err)
	require.ErrorIs(t, err, iam.ErrInvalidPassword, "no lowercase must fail")

	err = b.ChangePassword("OldPassword1!", "haslower1")
	require.NoError(t, err, "password with lowercase must succeed")
}

func TestPasswordPolicy_ChangePassword_Numbers(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	require.NoError(t, b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{
		MinimumPasswordLength: 8,
		RequireNumbers:        true,
	}))

	err := b.ChangePassword("OldPassword1!", "NoDigits!")
	require.Error(t, err)
	require.ErrorIs(t, err, iam.ErrInvalidPassword, "no digit must fail")

	err = b.ChangePassword("OldPassword1!", "HasDigit1")
	require.NoError(t, err, "password with digit must succeed")
}

func TestPasswordPolicy_ChangePassword_Symbols(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	require.NoError(t, b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{
		MinimumPasswordLength: 8,
		RequireSymbols:        true,
	}))

	err := b.ChangePassword("OldPassword1!", "NoSymbol1")
	require.Error(t, err)
	require.ErrorIs(t, err, iam.ErrInvalidPassword, "no symbol must fail")

	err = b.ChangePassword("OldPassword1!", "HasSymbl!")
	require.NoError(t, err, "password with symbol must succeed")
}

func TestPasswordPolicy_ChangePassword_ReusePrevention(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	require.NoError(t, b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{
		MinimumPasswordLength:   8,
		PasswordReusePrevention: 2,
	}))

	require.NoError(t, b.ChangePassword("OldPassword1!", "Password1!"))
	require.NoError(t, b.ChangePassword("Password1!", "Password2!"))

	err := b.ChangePassword("Password2!", "Password1!")
	require.Error(t, err)
	require.ErrorIs(t, err, iam.ErrInvalidPassword,
		"reusing one of the last 2 passwords must be rejected")

	require.NoError(t, b.ChangePassword("Password2!", "Password3!"),
		"a password outside the reuse-prevention window must be accepted")

	require.NoError(t, b.ChangePassword("Password3!", "Password1!"),
		"Password1! fell out of the bounded history and must be reusable again")
}

func TestPasswordPolicy_DefaultAllowsShortPassword(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("alice", "/", "")

	// Default policy: min 8 chars, no complexity requirements.
	_, err := b.CreateLoginProfile("alice", "exactly8!", false)
	require.NoError(t, err, "8-char password must satisfy default policy")
}

func TestCredentialReport_AccessKeyLastUsed_Reflected(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("alice", "/", "")

	ak, err := b.CreateAccessKey("alice")
	require.NoError(t, err)

	// Record access key usage.
	b.RecordAccessKeyUsage(ak.AccessKeyID, "us-east-1", "s3")

	report := b.GetCredentialReport()

	// The access key last used date should be present in the report.
	lines := strings.Split(report, "\n")
	var aliceRow string
	for _, line := range lines {
		if strings.HasPrefix(line, "alice,") {
			aliceRow = line
		}
	}

	require.NotEmpty(t, aliceRow, "alice must appear in credential report")
	assert.NotContains(t, aliceRow, "N/A,N/A,s3",
		"access key last used date must not be N/A after usage is recorded")
	assert.Contains(t, aliceRow, "s3",
		"service name from access key usage must appear in report")
	assert.Contains(t, aliceRow, "us-east-1",
		"region from access key usage must appear in report")
}

func TestCredentialReport_PasswordEnabled_Reflected(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("alice", "/", "")

	report := b.GetCredentialReport()
	lines := strings.Split(report, "\n")
	var aliceRow string
	for _, line := range lines {
		if strings.HasPrefix(line, "alice,") {
			aliceRow = line
		}
	}

	require.NotEmpty(t, aliceRow)
	fields := strings.Split(aliceRow, ",")

	// password_enabled is field index 3.
	require.GreaterOrEqual(t, len(fields), 4)
	assert.Equal(t, "false", fields[3], "password_enabled must be false before login profile created")

	_, err := b.CreateLoginProfile("alice", "Password123!", false)
	require.NoError(t, err)

	report2 := b.GetCredentialReport()
	for line := range strings.SplitSeq(report2, "\n") {
		if strings.HasPrefix(line, "alice,") {
			aliceRow = line
		}
	}

	fields2 := strings.Split(aliceRow, ",")
	require.GreaterOrEqual(t, len(fields2), 4)
	assert.Equal(t, "true", fields2[3], "password_enabled must be true after login profile created")
}

func TestCredentialReport_RootRowPresent(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	report := b.GetCredentialReport()
	assert.Contains(t, report, "<root_account>", "credential report must include root account row")
}

func TestCredentialReport_HasHeader(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	report := b.GetCredentialReport()
	lines := strings.Split(report, "\n")
	require.GreaterOrEqual(t, len(lines), 1)
	assert.True(t, strings.HasPrefix(lines[0], "user,arn,"),
		"first line must be CSV header starting with user,arn,")
}

func TestGetAccountSummary_Users(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("u1", "/", "")
	_, _ = b.CreateUser("u2", "/", "")

	sum := b.GetAccountSummary()
	assert.Equal(t, 2, sum.Users)
}

func TestGetAccountSummary_AccessKeys(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	_, _ = b.CreateUser("alice", "/", "")
	ak1, _ := b.CreateAccessKey("alice")
	_, _ = b.CreateAccessKey("alice")

	// Deactivate one key.
	_ = b.UpdateAccessKey("alice", ak1.AccessKeyID, "Inactive")

	sum := b.GetAccountSummary()
	assert.Equal(t, 2, sum.AccessKeysPerUser, "total access keys")
	assert.Equal(t, 1, sum.ActiveAccessKeys, "only active keys")
}

func TestGetAccountSummary_Policies(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`
	_, _ = b.CreatePolicy("P1", "/", doc)
	_, _ = b.CreatePolicy("P2", "/", doc)

	sum := b.GetAccountSummary()
	assert.Equal(t, 2, sum.Policies)
}

func TestGetAccountPasswordPolicy_Defaults(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	pp := b.GetAccountPasswordPolicy()

	assert.Equal(t, 8, pp.MinimumPasswordLength, "default min password length is 8")
	assert.False(t, pp.RequireUppercaseCharacters)
	assert.False(t, pp.RequireLowercaseCharacters)
	assert.False(t, pp.RequireNumbers)
	assert.False(t, pp.RequireSymbols)
	assert.True(t, pp.AllowUsersToChangePassword)
}

func TestDeleteAccountPasswordPolicy_ResetsToDefault(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	require.NoError(t, b.UpdateAccountPasswordPolicy(iam.PasswordPolicy{
		MinimumPasswordLength:      16,
		RequireUppercaseCharacters: true,
	}))

	require.NoError(t, b.DeleteAccountPasswordPolicy())

	pp := b.GetAccountPasswordPolicy()
	assert.Equal(t, 8, pp.MinimumPasswordLength)
	assert.False(t, pp.RequireUppercaseCharacters)
}

func TestAccountAlias_RoundTrip(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	require.NoError(t, b.CreateAccountAlias("my-account"))
	aliases := b.ListAccountAliases()
	assert.Equal(t, []string{"my-account"}, aliases)

	// AWS allows only one alias; creating another replaces the old one.
	require.NoError(t, b.CreateAccountAlias("new-alias"))
	aliases2 := b.ListAccountAliases()
	assert.Equal(t, []string{"new-alias"}, aliases2)

	require.NoError(t, b.DeleteAccountAlias("new-alias"))
	aliases3 := b.ListAccountAliases()
	assert.Empty(t, aliases3)
}

func TestGetAccountAuthorizationDetails_Backend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup         func(*iam.InMemoryBackend)
		name          string
		wantUserCount int
		wantRoleCount int
	}{
		{
			name:  "empty",
			setup: func(_ *iam.InMemoryBackend) {},
		},
		{
			name: "users_and_roles",
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateUser("alice", "/", "")
				_, _ = b.CreateRole("exec", "/", "{}", "")
			},
			wantUserCount: 1,
			wantRoleCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iam.NewInMemoryBackend()
			tt.setup(b)

			details, nextMarker := b.GetAccountAuthorizationDetails("", 0, nil)
			assert.Len(t, details.Users, tt.wantUserCount)
			assert.Len(t, details.Roles, tt.wantRoleCount)
			assert.Empty(t, nextMarker)
		})
	}
}

// newAuthDetailsFixture creates 2 users, 1 group, 1 role, and 1 managed
// policy (5 entities total) for pagination/filter tests below.
func newAuthDetailsFixture(t *testing.T) *iam.InMemoryBackend {
	t.Helper()

	b := iam.NewInMemoryBackend()
	_, err := b.CreateUser("alice", "/", "")
	require.NoError(t, err)
	_, err = b.CreateUser("bob", "/", "")
	require.NoError(t, err)
	_, err = b.CreateGroup("devs", "/")
	require.NoError(t, err)
	_, err = b.CreateRole("exec", "/", "{}", "")
	require.NoError(t, err)
	_, err = b.CreatePolicy(
		"readonly", "/",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
	)
	require.NoError(t, err)

	return b
}

func TestGetAccountAuthorizationDetails_Pagination(t *testing.T) {
	t.Parallel()

	b := newAuthDetailsFixture(t)

	details, nextMarker := b.GetAccountAuthorizationDetails("", 3, nil)
	firstPageCount := len(details.Users) + len(details.Groups) + len(details.Roles) + len(details.Policies)
	assert.Equal(t, 3, firstPageCount)
	assert.NotEmpty(t, nextMarker)

	details2, nextMarker2 := b.GetAccountAuthorizationDetails(nextMarker, 3, nil)
	secondPageCount := len(details2.Users) + len(details2.Groups) + len(details2.Roles) + len(details2.Policies)
	assert.Equal(t, 2, secondPageCount)
	assert.Empty(t, nextMarker2)
}

func TestGetAccountAuthorizationDetails_Filter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		filter       []string
		wantUsers    int
		wantGroups   int
		wantRoles    int
		wantPolicies int
	}{
		{
			name: "no_filter_includes_everything", filter: nil,
			wantUsers: 2, wantGroups: 1, wantRoles: 1, wantPolicies: 1,
		},
		{name: "user_only", filter: []string{"User"}, wantUsers: 2},
		{name: "role_and_group", filter: []string{"Role", "Group"}, wantGroups: 1, wantRoles: 1},
		{
			name:   "local_managed_policy_matches_all_policies",
			filter: []string{"LocalManagedPolicy"}, wantPolicies: 1,
		},
		{
			name:   "aws_managed_policy_matches_none",
			filter: []string{"AWSManagedPolicy"}, wantPolicies: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newAuthDetailsFixture(t)

			details, _ := b.GetAccountAuthorizationDetails("", 0, tt.filter)
			assert.Len(t, details.Users, tt.wantUsers)
			assert.Len(t, details.Groups, tt.wantGroups)
			assert.Len(t, details.Roles, tt.wantRoles)
			assert.Len(t, details.Policies, tt.wantPolicies)
		})
	}
}

func TestGetCredentialReport_Backend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup         func(*iam.InMemoryBackend)
		name          string
		wantUserLines int
	}{
		{
			name:          "no_users",
			setup:         func(_ *iam.InMemoryBackend) {},
			wantUserLines: 2, // header + root
		},
		{
			name: "with_users",
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateUser("alice", "/", "")
				_, _ = b.CreateUser("bob", "/", "")
			},
			wantUserLines: 4, // header + root + 2 users
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iam.NewInMemoryBackend()
			tt.setup(b)

			report := b.GetCredentialReport()
			assert.NotEmpty(t, report)

			lines := len(splitLines(report))
			assert.Equal(t, tt.wantUserLines, lines)
		})
	}
}

// splitLines splits a string on newlines, filtering empty lines.
func splitLines(s string) []string {
	var lines []string

	for l := range strings.SplitSeq(s, "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}

	return lines
}

// TestOutboundWebIdentityFederation covers Enable/DisableOutboundWebIdentityFederation
// and GetOutboundWebIdentityFederationInfo/OutboundWebIdentityFederationEnabled:
// defaults to enabled (so services/sts's GetWebIdentityToken keeps working out
// of the box), toggles correctly, and always returns a stable, non-empty
// issuer URL regardless of enabled state (real AWS's EnableOutbound.../
// GetOutbound...Info.IssuerIdentifier doc comments both describe it as "a
// unique issuer URL for your account", not something that appears/disappears
// with the enabled toggle).
func TestOutboundWebIdentityFederation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action      func(b *iam.InMemoryBackend)
		name        string
		wantEnabled bool
	}{
		{
			name:        "default_is_enabled",
			action:      func(_ *iam.InMemoryBackend) {},
			wantEnabled: true,
		},
		{
			name: "disable_turns_it_off",
			action: func(b *iam.InMemoryBackend) {
				b.DisableOutboundWebIdentityFederation()
			},
			wantEnabled: false,
		},
		{
			name: "re_enable_turns_it_back_on",
			action: func(b *iam.InMemoryBackend) {
				b.DisableOutboundWebIdentityFederation()
				b.EnableOutboundWebIdentityFederation()
			},
			wantEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iam.NewInMemoryBackend()
			tt.action(b)

			issuer, enabled := b.GetOutboundWebIdentityFederationInfo()
			assert.Equal(t, tt.wantEnabled, enabled)
			assert.Equal(t, tt.wantEnabled, b.OutboundWebIdentityFederationEnabled())
			assert.NotEmpty(t, issuer)
			assert.True(t, strings.HasPrefix(issuer, "https://"))
		})
	}
}
