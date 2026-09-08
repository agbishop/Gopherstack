package ses_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ses"
)

func TestInMemoryBackend_SnapshotRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(b *ses.InMemoryBackend) string
		verify func(t *testing.T, b *ses.InMemoryBackend, id string)
		name   string
	}{
		{
			name: "round_trip_preserves_state",
			setup: func(b *ses.InMemoryBackend) string {
				err := b.VerifyEmailIdentity("test@example.com")
				if err != nil {
					return ""
				}

				return "test@example.com"
			},
			verify: func(t *testing.T, b *ses.InMemoryBackend, id string) {
				t.Helper()

				identities := b.ListIdentities("", 0, "").Data
				require.Len(t, identities, 1)
				assert.Equal(t, id, identities[0])
			},
		},
		{
			name:  "empty_backend_round_trip",
			setup: func(_ *ses.InMemoryBackend) string { return "" },
			verify: func(t *testing.T, b *ses.InMemoryBackend, _ string) {
				t.Helper()

				identities := b.ListIdentities("", 0, "").Data
				assert.Empty(t, identities)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := ses.NewInMemoryBackend()
			id := tt.setup(original)

			snap := original.Snapshot(t.Context())
			require.NotNil(t, snap)

			fresh := ses.NewInMemoryBackend()
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tt.verify(t, fresh, id)
		})
	}
}

func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	h := ses.NewHandler(ses.NewInMemoryBackend())
	require.NoError(t, h.Backend.VerifyEmailIdentity("delegate@test.com"))

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := ses.NewHandler(ses.NewInMemoryBackend())
	require.NoError(t, h2.Restore(t.Context(), snap))

	identities := h2.Backend.ListIdentities("", 0, "").Data
	require.Len(t, identities, 1)
	assert.Equal(t, "delegate@test.com", identities[0])
}

// TestInMemoryBackend_SnapshotRestore_FullState exercises a Snapshot->Restore
// round trip across every resource family the Phase 3.3 pkgs/store
// conversion touched, including the "dirty" tables (identities, configSets,
// trackingOptions, eventDestinations) whose live value type deliberately
// excludes its own identity field from JSON (json:"-") and instead relies on
// a DTO type in persistence.go to carry it through the round trip -- this is
// exactly the class of bug (identity silently dropped on restore) a
// same-named-fields-only test would miss.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := ses.NewInMemoryBackend().WithRegion("us-west-2").WithAccountID("999999999999")

	require.NoError(t, original.VerifyEmailIdentity("verified@example.com"))
	require.NoError(t, original.SetIdentityMailFromDomain("verified@example.com", "mail.example.com", ""))
	require.NoError(t, original.SetIdentityDkimEnabled("verified@example.com", true))

	require.NoError(t, original.CreateTemplate(ses.EmailTemplate{
		TemplateName: "tmpl1", SubjectPart: "Subj", TextPart: "Text", HTMLPart: "<p>HTML</p>",
	}))

	require.NoError(t, original.CreateConfigurationSet("cs1"))
	require.NoError(t, original.PutConfigurationSetDeliveryOptions("cs1", ses.TLSPolicyRequire))
	require.NoError(t, original.UpdateConfigurationSetReputationMetricsEnabled("cs1", true))
	require.NoError(t, original.UpdateConfigurationSetSendingEnabled("cs1", false))

	require.NoError(t, original.CreateConfigurationSetEventDestination("cs1", ses.EventDestination{
		Name: "dest1", SNSTopicARN: "arn:aws:sns:us-west-2:999999999999:topic", Enabled: true,
		MatchingEventTypes: []string{"send", "bounce"},
	}))

	require.NoError(t, original.CreateConfigurationSetTrackingOptions("cs1", "track.example.com"))

	require.NoError(t, original.CreateReceiptRuleSet("rs1"))
	require.NoError(t, original.CreateReceiptRule("rs1", ses.ReceiptRule{
		Name: "rule1", Enabled: true, TLSPolicy: ses.TLSPolicyOptional, Recipients: []string{"a@example.com"},
	}, ""))
	require.NoError(t, original.SetActiveReceiptRuleSet("rs1"))

	require.NoError(t, original.CreateReceiptFilter(ses.ReceiptFilter{
		Name: "filter1", Policy: ses.FilterPolicyBlock, CIDR: "10.0.0.0/8",
	}))

	require.NoError(t, original.CreateCustomVerificationEmailTemplate(ses.CustomVerificationEmailTemplate{
		TemplateName: "cvt1", FromEmailAddress: "verify@example.com", TemplateSubject: "Verify",
		TemplateContent: "click here", SuccessRedirectionURL: "https://ok", FailureRedirectionURL: "https://fail",
	}))

	require.NoError(t, original.PutIdentityPolicy("verified@example.com", "policy1", `{"Version":"2012-10-17"}`))

	original.UpdateAccountSendingEnabled(false)

	_, err := original.SendEmail(ses.SendEmailInput{
		From: "verified@example.com", To: []string{"to@example.com"}, Subject: "Hi", BodyText: "body",
	})
	require.Error(t, err) // sending is paused above; the email should NOT be recorded

	original.UpdateAccountSendingEnabled(true)

	_, err = original.SendEmail(ses.SendEmailInput{
		From: "verified@example.com", To: []string{"to@example.com"}, Subject: "Hi", BodyText: "body",
	})
	require.NoError(t, err)

	original.UpdateAccountSendingEnabled(false) // exercise a non-default bool value across the round trip

	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := ses.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	assert.Equal(t, "us-west-2", fresh.Region())
	assert.Equal(t, "999999999999", fresh.AccountID())
	assert.False(t, fresh.GetAccountSendingEnabled())

	mailFrom := fresh.GetIdentityMailFromDomainAttributes([]string{"verified@example.com"})["verified@example.com"]
	assert.Equal(t, "mail.example.com", mailFrom.MailFromDomain)

	dkim := fresh.GetIdentityDkimAttributes([]string{"verified@example.com"})["verified@example.com"]
	assert.True(t, dkim.DkimEnabled)

	tmpl, err := fresh.GetTemplate("tmpl1")
	require.NoError(t, err)
	assert.Equal(t, "Subj", tmpl.SubjectPart)

	csDesc, err := fresh.DescribeConfigurationSet("cs1")
	require.NoError(t, err)
	require.NotNil(t, csDesc.DeliveryOptions)
	assert.Equal(t, ses.TLSPolicyRequire, csDesc.DeliveryOptions.TLSPolicy)
	assert.True(t, csDesc.ReputationMetricsEnabled)
	assert.False(t, csDesc.SendingEnabled)
	require.Len(t, csDesc.EventDestinations, 1)
	assert.Equal(t, "dest1", csDesc.EventDestinations[0].Name)
	assert.ElementsMatch(t, []string{"send", "bounce"}, csDesc.EventDestinations[0].MatchingEventTypes)
	require.NotNil(t, csDesc.TrackingOptions)
	assert.Equal(t, "track.example.com", csDesc.TrackingOptions.CustomRedirectDomain)

	rs, err := fresh.DescribeReceiptRuleSet("rs1")
	require.NoError(t, err)
	require.Len(t, rs.Rules, 1)
	assert.Equal(t, "rule1", rs.Rules[0].Name)
	assert.Equal(t, "rs1", fresh.ActiveRuleSet())

	filters := fresh.ListReceiptFilters()
	require.Len(t, filters, 1)
	assert.Equal(t, "10.0.0.0/8", filters[0].CIDR)

	cvt, err := fresh.GetCustomVerificationEmailTemplate("cvt1")
	require.NoError(t, err)
	assert.Equal(t, "verify@example.com", cvt.FromEmailAddress)

	policies, err := fresh.GetIdentityPolicies("verified@example.com", []string{"policy1"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"Version":"2012-10-17"}`, policies["policy1"])

	emails := fresh.ListEmails()
	require.Len(t, emails, 1)
	assert.Equal(t, "Hi", emails[0].Subject)

	emailByID, err := fresh.GetEmailByID(emails[0].MessageID)
	require.NoError(t, err)
	assert.Equal(t, emails[0].MessageID, emailByID.MessageID)
}

func TestInMemoryBackend_RestorePreservesEmails(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("persist@test.com"))

	_, err := b.SendEmail(ses.SendEmailInput{
		From: "persist@test.com", To: []string{"to@test.com"}, Subject: "Test", BodyText: "body",
	})
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := ses.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	emails := fresh.ListEmails()
	require.Len(t, emails, 1)
	assert.Equal(t, "persist@test.com", emails[0].From)
	assert.Equal(t, "Test", emails[0].Subject)
}

func TestInMemoryBackend_RestorePreservesBouncedComplained(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("persist@test.com"))

	_, err := b.SendEmail(ses.SendEmailInput{
		From: "persist@test.com", To: []string{"bounce@simulator.amazonses.com"}, Subject: "b", BodyText: "body",
	})
	require.NoError(t, err)

	_, err = b.SendEmail(ses.SendEmailInput{
		From: "persist@test.com", To: []string{"complaint@simulator.amazonses.com"}, Subject: "c", BodyText: "body",
	})
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := ses.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	emails := fresh.ListEmails()
	require.Len(t, emails, 2)

	points := fresh.GetSendStatistics()
	require.Len(t, points, 1)
	assert.InDelta(t, float64(1), points[0].Bounces, 0, "bounce flag must survive snapshot/restore")
	assert.InDelta(t, float64(1), points[0].Complaints, 0, "complaint flag must survive snapshot/restore")
}

func TestPersistence_EnsureNonNilMaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "snapshot_restore_roundtrip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ses.NewInMemoryBackend()
			require.NoError(t, b.VerifyEmailIdentity("snap@example.com"))

			snap := b.Snapshot(t.Context())

			b2 := ses.NewInMemoryBackend()
			require.NoError(t, b2.Restore(t.Context(), snap))

			// After restore all maps should be non-nil and usable.
			require.NoError(t, b2.VerifyEmailIdentity("post-restore@example.com"))
			assert.Equal(t, 2, b2.IdentityCount())
		})
	}
}

func TestSnapshot_IncludesPolicies(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("snap@example.com"))
	require.NoError(t, b.PutIdentityPolicy("snap@example.com", "pol1", `{"v":1}`))
	require.NoError(t, b.PutIdentityPolicy("snap@example.com", "pol2", `{"v":2}`))

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := ses.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	names, err := fresh.ListIdentityPolicies("snap@example.com")
	require.NoError(t, err)
	assert.Equal(t, []string{"pol1", "pol2"}, names)

	pols, err := fresh.GetIdentityPolicies("snap@example.com", []string{"pol1", "pol2"})
	require.NoError(t, err)
	assert.Equal(t, `{"v":1}`, pols["pol1"])
	assert.Equal(t, `{"v":2}`, pols["pol2"])
}

func TestSnapshot_IncludesAccountSending(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	b.UpdateAccountSendingEnabled(false)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := ses.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))
	assert.False(t, fresh.GetAccountSendingEnabled())
}

func TestSnapshot_FullRoundtrip(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, b.VerifyEmailIdentity("b.com"))
	require.NoError(t, b.PutIdentityPolicy("a@example.com", "p1", `{}`))
	require.NoError(t, b.CreateTemplate(ses.EmailTemplate{TemplateName: "t", SubjectPart: "s"}))
	require.NoError(t, b.CreateConfigurationSet("cs1"))
	require.NoError(t, b.CreateReceiptRuleSet("rs1"))
	require.NoError(t, b.CreateReceiptFilter(ses.ReceiptFilter{Name: "f1", Policy: "Allow", CIDR: "10.0.0.0/8"}))
	b.UpdateAccountSendingEnabled(false)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := ses.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	assert.Equal(t, 2, fresh.IdentityCount())
	assert.Equal(t, 1, fresh.PolicyCount())
	assert.Equal(t, 1, fresh.TemplateCount())
	assert.Equal(t, 1, fresh.ConfigSetCount())
	assert.Equal(t, 1, fresh.ReceiptRuleSetCount())
	assert.Equal(t, 1, fresh.ReceiptFilterCount())
	assert.False(t, fresh.GetAccountSendingEnabled())
}

func TestReset_ClearsPoliciesAndAccountSending(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, b.PutIdentityPolicy("a@example.com", "p1", `{}`))
	b.UpdateAccountSendingEnabled(false)

	b.Reset()

	assert.Equal(t, 0, b.PolicyCount())
	assert.True(t, b.GetAccountSendingEnabled())
	assert.Equal(t, 0, b.IdentityCount())
}

// TestSESNewOps_SnapshotRestore verifies that new maps are included in Snapshot/Restore.
func TestSnapshotRestore_NewOps(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()

	require.NoError(t, b.CreateReceiptRuleSet("rs1"))
	require.NoError(t, b.CreateReceiptFilter(ses.ReceiptFilter{Name: "f1", Policy: "Block", CIDR: "192.168.0.0/16"}))
	require.NoError(t, b.CreateConfigurationSet("cs1"))
	require.NoError(
		t,
		b.CreateConfigurationSetEventDestination(
			"cs1", ses.EventDestination{Name: "dest1", Enabled: true, MatchingEventTypes: []string{"send"}},
		),
	)
	require.NoError(t, b.CreateConfigurationSetTrackingOptions("cs1", "track.example.com"))
	require.NoError(t, b.CreateCustomVerificationEmailTemplate(ses.CustomVerificationEmailTemplate{
		TemplateName:          "my-tmpl",
		FromEmailAddress:      "noreply@example.com",
		TemplateSubject:       "Please verify",
		TemplateContent:       "<html>Verify</html>",
		SuccessRedirectionURL: "https://example.com/success",
		FailureRedirectionURL: "https://example.com/failure",
	}))

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := ses.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), snap))

	assert.Equal(t, 1, b2.ReceiptRuleSetCount())
	assert.Equal(t, 1, b2.ReceiptFilterCount())
	assert.Equal(t, 1, b2.EventDestinationCount())
	assert.Equal(t, 1, b2.TrackingOptionsCount())
	assert.Equal(t, 1, b2.CustomVerifTemplateCount())
}

// TestBackend_Persistence_ActiveRuleSet tests that ActiveRuleSet survives Snapshot/Restore.
func TestBackend_Persistence_ActiveRuleSet(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	b.AddReceiptRuleSetInternal(ses.ReceiptRuleSet{Name: "rs1", CreatedAt: time.Now()})
	require.NoError(t, b.SetActiveReceiptRuleSet("rs1"))

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := ses.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), snap))
	assert.Equal(t, "rs1", b2.ActiveRuleSet())
}

func TestSESPersistence_TemplatesAndConfigSetsRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(b *ses.InMemoryBackend)
		verify func(t *testing.T, b *ses.InMemoryBackend)
		name   string
	}{
		{
			name: "templates_persisted",
			setup: func(b *ses.InMemoryBackend) {
				require.NoError(t, b.CreateTemplate(ses.EmailTemplate{
					TemplateName: "t1",
					SubjectPart:  "Hello",
				}))
			},
			verify: func(t *testing.T, b *ses.InMemoryBackend) {
				t.Helper()

				tmpl, err := b.GetTemplate("t1")
				require.NoError(t, err)
				assert.Equal(t, "Hello", tmpl.SubjectPart)
			},
		},
		{
			name: "config_sets_persisted",
			setup: func(b *ses.InMemoryBackend) {
				require.NoError(t, b.CreateConfigurationSet("cs1"))
			},
			verify: func(t *testing.T, b *ses.InMemoryBackend) {
				t.Helper()

				p := b.ListConfigurationSets("", 0)
				require.Len(t, p.Data, 1)
				assert.Equal(t, "cs1", p.Data[0])
			},
		},
		{
			name: "emails_by_id_rebuilt_on_restore",
			setup: func(b *ses.InMemoryBackend) {
				require.NoError(t, b.VerifyEmailIdentity("p@test.com"))
				_, err := b.SendEmail(ses.SendEmailInput{
					From: "p@test.com", To: []string{"q@test.com"}, Subject: "s", BodyText: "b",
				})
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *ses.InMemoryBackend) {
				t.Helper()

				assert.Equal(t, b.EmailCount(), b.EmailsByIDCount())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := ses.NewInMemoryBackend()
			tt.setup(original)

			snap := original.Snapshot(t.Context())
			require.NotNil(t, snap)

			fresh := ses.NewInMemoryBackend()
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tt.verify(t, fresh)
		})
	}
}

func TestSESPersistence_RestorePrunesExpiredEmails(t *testing.T) {
	t.Parallel()

	original := ses.NewInMemoryBackend()
	require.NoError(t, original.VerifyEmailIdentity("prune@test.com"))

	_, err := original.SendEmail(ses.SendEmailInput{
		From: "prune@test.com", To: []string{"to@test.com"}, Subject: "keep", BodyText: "b",
	})
	require.NoError(t, err)

	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	// Restore into a backend with a very short TTL so the snapshot email is
	// considered expired at restore time.
	fresh := ses.NewInMemoryBackend()
	fresh.SetEmailTTL(time.Nanosecond) // instant expiry

	time.Sleep(time.Millisecond) // ensure TTL has passed

	require.NoError(t, fresh.Restore(t.Context(), snap))

	// The expired email must have been pruned.
	assert.Equal(t, 0, fresh.EmailCount())
	assert.Equal(t, 0, fresh.EmailsByIDCount())
}

func TestSESPersistence_RestoreCapsToBound(t *testing.T) {
	t.Parallel()

	original := ses.NewInMemoryBackend()
	require.NoError(t, original.VerifyEmailIdentity("cap@test.com"))

	// Append MaxRetainedEmails+10 emails and snapshot, via AppendEmailForTest
	// so this retention-cap test isn't gated by the simulated 200/day send
	// quota that real SendEmail now enforces (see TestSESBackend_EmailsByIDSyncAfterEviction).
	for range ses.MaxRetainedEmails + 10 {
		original.AppendEmailForTest("cap@test.com", []string{"to@test.com"})
	}

	// The original should already be capped by SendEmail eviction.
	assert.Equal(t, ses.MaxRetainedEmails, original.EmailCount())

	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := ses.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	assert.Equal(t, ses.MaxRetainedEmails, fresh.EmailCount())
	assert.Equal(t, fresh.EmailCount(), fresh.EmailsByIDCount())
}
