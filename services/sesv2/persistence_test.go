package sesv2_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sesv2"
)

func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := sesv2.NewInMemoryBackend()
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend (including the pre-Phase-3.3
// format, which decodes with Version == 0) is discarded cleanly rather than
// partially decoded: the backend resets to empty state and Restore returns
// no error.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b := sesv2.NewInMemoryBackend()
	_, err := b.CreateEmailIdentity("seed@example.com", "", nil)
	require.NoError(t, err)

	// A syntactically valid but version-less/mismatched snapshot.
	err = b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.Equal(t, 0, sesv2.IdentityCount(b))
}

func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	h := sesv2.NewHandler(sesv2.NewInMemoryBackend())
	_, err := h.Backend.CreateEmailIdentity("delegate@test.com", "", nil)
	require.NoError(t, err)

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := sesv2.NewHandler(sesv2.NewInMemoryBackend())
	require.NoError(t, h2.Restore(t.Context(), snap))

	page := h2.Backend.ListEmailIdentities("", 0)
	require.Len(t, page.Data, 1)
	assert.Equal(t, "delegate@test.com", page.Data[0].Identity)
}

// TestInMemoryBackend_SnapshotRestore_FullState exercises a Snapshot->Restore
// round trip across every resource family the Phase 3.3 pkgs/store
// conversion touched, including the two flattened composite-key tables
// (eventDestinations, contacts) and their secondary indexes, plus every
// plain map left un-converted (emailIdentityPolicies, resourceTags,
// multiRegionEndpoints, tenants, tenantResources, resourceTenants) and the
// single accountDetails pointer.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := sesv2.NewInMemoryBackend()

	_, err := original.CreateEmailIdentity("verified@example.com", "", map[string]string{"env": "test"})
	require.NoError(t, err)

	_, err = original.CreateConfigurationSet("cs1", nil)
	require.NoError(t, err)
	require.NoError(t, original.PutConfigurationSetSendingOptions("cs1", false))

	_, err = original.CreateConfigurationSetEventDestination(
		"cs1", "dest1", true, []string{"SEND", "BOUNCE"},
	)
	require.NoError(t, err)

	_, err = original.CreateContactList("list1", "desc1", nil)
	require.NoError(t, err)

	_, err = original.CreateContact("list1", "contact@example.com", []sesv2.TopicPreference{
		{TopicName: "news", SubscriptionStatus: "OPT_IN"},
	})
	require.NoError(t, err)

	_, err = original.CreateCustomVerificationEmailTemplate(&sesv2.CustomVerificationEmailTemplate{
		TemplateName: "cvt1", FromEmailAddress: "verify@example.com", TemplateSubject: "Verify",
	})
	require.NoError(t, err)

	_, err = original.CreateDedicatedIPPool("pool1", "STANDARD", nil)
	require.NoError(t, err)
	require.NoError(t, original.PutDedicatedIPWarmupAttributes("10.0.0.1", 42))
	require.NoError(t, original.PutDedicatedIPInPool("10.0.0.1", "pool1"))

	require.NoError(t, original.UpdateReputationEntityCustomerManagedStatus("cs1", "ENABLED"))

	dtReport, err := original.CreateDeliverabilityTestReport("report1", "from@example.com")
	require.NoError(t, err)

	_, err = original.CreateEmailTemplate("tmpl1", &sesv2.EmailTemplateContent{Subject: "Hi"}, nil)
	require.NoError(t, err)

	original.AddExportJobInternal("job1", "CREATED")

	_, err = original.CreateImportJob(sesv2.ImportDestination{SuppressionListImportAction: "PUT"})
	require.NoError(t, err)

	require.NoError(t, original.PutSuppressedDestination("suppressed@example.com", "BOUNCE"))

	require.NoError(t, original.CreateEmailIdentityPolicy("verified@example.com", "policy1", `{"a":1}`))

	require.NoError(t, original.TagResource("arn:aws:ses:us-east-1:123:identity/verified@example.com",
		map[string]string{"k": "v"}))

	_, err = original.CreateMultiRegionEndpoint("mre1", nil, nil)
	require.NoError(t, err)

	_, err = original.CreateTenant("tenant1", nil)
	require.NoError(t, err)
	require.NoError(t, original.CreateTenantResourceAssociation("tenant1", "arn:aws:ses:us-east-1:123:x"))

	require.NoError(t, original.PutAccountDetails(&sesv2.AccountDetails{
		MailType: "TRANSACTIONAL", SendingEnabled: true,
	}))

	msgID, err := original.SendEmail("verified@example.com", []string{"to@example.com"}, "Hi", "", "body", nil)
	require.NoError(t, err)

	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := sesv2.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	ei, err := fresh.GetEmailIdentity("verified@example.com")
	require.NoError(t, err)
	assert.Equal(t, "test", ei.Tags["env"])

	cs, err := fresh.GetConfigurationSet("cs1")
	require.NoError(t, err)
	assert.False(t, cs.SendingEnabled)

	dests, err := fresh.GetConfigurationSetEventDestinations("cs1")
	require.NoError(t, err)
	require.Len(t, dests, 1)
	assert.Equal(t, "dest1", dests[0].Name)
	assert.ElementsMatch(t, []string{"SEND", "BOUNCE"}, dests[0].MatchingEventTypes)

	cl, err := fresh.GetContactList("list1")
	require.NoError(t, err)
	assert.Equal(t, "desc1", cl.Description)

	contact, err := fresh.GetContact("list1", "contact@example.com")
	require.NoError(t, err)
	require.Len(t, contact.TopicPreferences, 1)
	assert.Equal(t, "news", contact.TopicPreferences[0].TopicName)

	cvt, err := fresh.GetCustomVerificationEmailTemplate("cvt1")
	require.NoError(t, err)
	assert.Equal(t, "verify@example.com", cvt.FromEmailAddress)

	pool, err := fresh.GetDedicatedIPPool("pool1")
	require.NoError(t, err)
	assert.Equal(t, "STANDARD", pool.ScalingMode)

	ip, err := fresh.GetDedicatedIP("10.0.0.1")
	require.NoError(t, err)
	assert.InEpsilon(t, float64(42), ip["WarmupPercentage"], 0)
	assert.Equal(t, "pool1", ip["PoolName"])

	entity, err := fresh.GetReputationEntity("cs1")
	require.NoError(t, err)
	require.NotNil(t, entity.CustomerManagedStatus)
	assert.Equal(t, "ENABLED", entity.CustomerManagedStatus.Status)

	report, err := fresh.GetDeliverabilityTestReport(dtReport.ReportID)
	require.NoError(t, err)
	assert.Equal(t, "from@example.com", report.FromEmailAddress)

	tmpl, err := fresh.GetEmailTemplate("tmpl1")
	require.NoError(t, err)
	assert.Equal(t, "Hi", tmpl.TemplateContent.Subject)

	job, err := fresh.GetExportJob("job1")
	require.NoError(t, err)
	assert.Equal(t, "CREATED", job.JobStatus)

	importJobs := fresh.ListImportJobs("", "", 0)
	require.Len(t, importJobs.Data, 1)

	suppressed, err := fresh.GetSuppressedDestination("suppressed@example.com")
	require.NoError(t, err)
	assert.Equal(t, "BOUNCE", suppressed.Reason)

	policies, err := fresh.GetEmailIdentityPolicies("verified@example.com")
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, policies["policy1"])

	tags, err := fresh.ListTagsForResource("arn:aws:ses:us-east-1:123:identity/verified@example.com")
	require.NoError(t, err)
	assert.Equal(t, "v", tags["k"])

	_, err = fresh.GetMultiRegionEndpoint("mre1")
	require.NoError(t, err)

	_, err = fresh.GetTenant("tenant1")
	require.NoError(t, err)

	resources, _, err := fresh.ListTenantResources("tenant1", nil, "", 0)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	acct, err := fresh.GetAccount()
	require.NoError(t, err)
	assert.Equal(t, "TRANSACTIONAL", acct.MailType)

	emails := fresh.ListEmails()
	require.Len(t, emails, 1)
	assert.Equal(t, msgID, emails[0].MessageID)
}

// TestInMemoryBackend_DeleteConfigurationSet_CascadesEventDestinations
// exercises the cascade-delete path added by the Phase 3.3 conversion: the
// eventDestinationsByConfigSet index must be cloned before iterating, since
// deleting from the underlying table mutates the index's backing slice.
func TestInMemoryBackend_DeleteConfigurationSet_CascadesEventDestinations(t *testing.T) {
	t.Parallel()

	b := sesv2.NewInMemoryBackend()

	_, err := b.CreateConfigurationSet("cs1", nil)
	require.NoError(t, err)

	_, err = b.CreateConfigurationSetEventDestination("cs1", "d1", true, nil)
	require.NoError(t, err)
	_, err = b.CreateConfigurationSetEventDestination("cs1", "d2", true, nil)
	require.NoError(t, err)

	require.NoError(t, b.DeleteConfigurationSet("cs1"))

	_, err = b.CreateConfigurationSet("cs1", nil)
	require.NoError(t, err)

	dests, err := b.GetConfigurationSetEventDestinations("cs1")
	require.NoError(t, err)
	assert.Empty(t, dests)
}

// TestInMemoryBackend_DeleteContactList_CascadesContacts mirrors the
// event-destination cascade test above for the other composite-key table.
func TestInMemoryBackend_DeleteContactList_CascadesContacts(t *testing.T) {
	t.Parallel()

	b := sesv2.NewInMemoryBackend()

	_, err := b.CreateContactList("list1", "", nil)
	require.NoError(t, err)

	_, err = b.CreateContact("list1", "a@example.com", nil)
	require.NoError(t, err)
	_, err = b.CreateContact("list1", "b@example.com", nil)
	require.NoError(t, err)

	require.NoError(t, b.DeleteContactList("list1"))

	_, err = b.CreateContactList("list1", "", nil)
	require.NoError(t, err)

	page, err := b.ListContacts("list1", "", 0)
	require.NoError(t, err)
	assert.Empty(t, page.Data)
}
