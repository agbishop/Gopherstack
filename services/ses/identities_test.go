package ses_test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ses"
)

func TestHandler_PutIdentityPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *ses.Handler)
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "put_policy_succeeds_even_for_unknown_identity",
			body:         "Action=PutIdentityPolicy&Version=2010-12-01&Identity=nonexistent@example.com&PolicyName=p1&Policy={}",
			wantCode:     http.StatusOK,
			wantContains: "PutIdentityPolicyResponse",
		},
		{
			name: "put_policy_for_verified_identity",
			setup: func(h *ses.Handler) {
				require.NoError(t, h.Backend.VerifyEmailIdentity("policy@example.com"))
			},
			body:         "Action=PutIdentityPolicy&Version=2010-12-01&Identity=policy@example.com&PolicyName=mypol&Policy={\"Version\":\"2012-10-17\"}", //nolint:lll // existing issue.
			wantCode:     http.StatusOK,
			wantContains: "PutIdentityPolicyResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_DeleteIdentityPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "idempotent_delete_nonexistent",
			body:         "Action=DeleteIdentityPolicy&Version=2010-12-01&Identity=nonexistent@example.com&PolicyName=p1",
			wantCode:     http.StatusOK,
			wantContains: "DeleteIdentityPolicyResponse",
		},
		{
			name:         "missing_identity_param",
			body:         "Action=DeleteIdentityPolicy&Version=2010-12-01&Identity=&PolicyName=p1",
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_GetIdentityPolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "unknown_identity_returns_empty",
			body:         "Action=GetIdentityPolicies&Version=2010-12-01&Identity=nonexistent@example.com&PolicyNames.member.1=p1", //nolint:lll // existing issue.
			wantCode:     http.StatusOK,
			wantContains: "GetIdentityPoliciesResponse",
		},
		{
			name:         "missing_identity_param",
			body:         "Action=GetIdentityPolicies&Version=2010-12-01&Identity=&PolicyNames.member.1=p1",
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
		{
			// PolicyNames is a required member (api_op_GetIdentityPolicies.go:
			// "This member is required"); a real SDK client refuses to even
			// build this request, but a raw wire client can still omit it.
			name:         "missing_policy_names_param",
			body:         "Action=GetIdentityPolicies&Version=2010-12-01&Identity=someone@example.com",
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_ListIdentityPolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "unknown_identity_returns_empty",
			body:         "Action=ListIdentityPolicies&Version=2010-12-01&Identity=nonexistent@example.com",
			wantCode:     http.StatusOK,
			wantContains: "ListIdentityPoliciesResponse",
		},
		{
			name:         "missing_identity_param",
			body:         "Action=ListIdentityPolicies&Version=2010-12-01&Identity=",
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_SetIdentityFeedbackForwardingEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "creates_identity_if_missing",
			body:         "Action=SetIdentityFeedbackForwardingEnabled&Version=2010-12-01&Identity=new@example.com&ForwardingEnabled=true", //nolint:lll // existing issue.
			wantCode:     http.StatusOK,
			wantContains: "SetIdentityFeedbackForwardingEnabledResponse",
		},
		{
			name:         "missing_identity_param",
			body:         "Action=SetIdentityFeedbackForwardingEnabled&Version=2010-12-01&Identity=&ForwardingEnabled=true",
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_SetIdentityHeadersInNotificationsEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "creates_identity_if_missing",
			body:         "Action=SetIdentityHeadersInNotificationsEnabled&Version=2010-12-01&Identity=new@example.com&NotificationType=Bounce&Enabled=true", //nolint:lll // existing issue.
			wantCode:     http.StatusOK,
			wantContains: "SetIdentityHeadersInNotificationsEnabledResponse",
		},
		{
			name:         "missing_identity_param",
			body:         "Action=SetIdentityHeadersInNotificationsEnabled&Version=2010-12-01&Identity=&NotificationType=Bounce&Enabled=true", //nolint:lll // existing issue.
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
		{
			name:         "missing_notification_type",
			body:         "Action=SetIdentityHeadersInNotificationsEnabled&Version=2010-12-01&Identity=x@example.com&NotificationType=&Enabled=true", //nolint:lll // existing issue.
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
		{
			name:         "invalid_notification_type",
			body:         "Action=SetIdentityHeadersInNotificationsEnabled&Version=2010-12-01&Identity=x@example.com&NotificationType=NotARealType&Enabled=true", //nolint:lll // existing issue.
			wantCode:     http.StatusBadRequest,
			wantContains: "ValidationError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_SetIdentityMailFromDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "creates_identity_if_missing",
			body:         "Action=SetIdentityMailFromDomain&Version=2010-12-01&Identity=new@example.com&MailFromDomain=mail.example.com", //nolint:lll // existing issue.
			wantCode:     http.StatusOK,
			wantContains: "SetIdentityMailFromDomainResponse",
		},
		{
			name:         "missing_identity_param",
			body:         "Action=SetIdentityMailFromDomain&Version=2010-12-01&Identity=&MailFromDomain=mail.example.com",
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_SetIdentityNotificationTopic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "creates_identity_if_missing",
			body:         "Action=SetIdentityNotificationTopic&Version=2010-12-01&Identity=new@example.com&NotificationType=Bounce&SnsTopic=arn:aws:sns:us-east-1:123:topic", //nolint:lll // existing issue.
			wantCode:     http.StatusOK,
			wantContains: "SetIdentityNotificationTopicResponse",
		},
		{
			name:         "missing_identity_param",
			body:         "Action=SetIdentityNotificationTopic&Version=2010-12-01&Identity=&NotificationType=Bounce&SnsTopic=arn:aws:sns:us-east-1:123:topic", //nolint:lll // existing issue.
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
		{
			name:         "missing_notification_type",
			body:         "Action=SetIdentityNotificationTopic&Version=2010-12-01&Identity=x@example.com&NotificationType=&SnsTopic=arn:aws:sns:us-east-1:123:topic", //nolint:lll // existing issue.
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
		{
			name:         "invalid_notification_type",
			body:         "Action=SetIdentityNotificationTopic&Version=2010-12-01&Identity=x@example.com&NotificationType=NotARealType&SnsTopic=arn:aws:sns:us-east-1:123:topic", //nolint:lll // existing issue.
			wantCode:     http.StatusBadRequest,
			wantContains: "ValidationError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestIdentityPolicy_CRUD(t *testing.T) {
	t.Parallel()

	h := newHandler()
	postForm(t, h, "Action=VerifyEmailIdentity&EmailAddress=test@example.com")

	// PutIdentityPolicy
	rec := postForm(t, h, url.Values{
		"Action":     []string{"PutIdentityPolicy"},
		"Identity":   []string{"test@example.com"},
		"PolicyName": []string{"my-policy"},
		"Policy":     []string{`{"Version":"2012-10-17","Statement":[]}`},
	}.Encode())
	assert.Equal(t, 200, rec.Code)

	// GetIdentityPolicies
	rec = postForm(t, h, url.Values{
		"Action":               []string{"GetIdentityPolicies"},
		"Identity":             []string{"test@example.com"},
		"PolicyNames.member.1": []string{"my-policy"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)

	// ListIdentityPolicies
	rec = postForm(t, h, url.Values{
		"Action":   []string{"ListIdentityPolicies"},
		"Identity": []string{"test@example.com"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)

	// DeleteIdentityPolicy
	rec = postForm(t, h, url.Values{
		"Action":     []string{"DeleteIdentityPolicy"},
		"Identity":   []string{"test@example.com"},
		"PolicyName": []string{"my-policy"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)
}

func TestIdentityAttributes_Aggregate(t *testing.T) {
	t.Parallel()

	h := newHandler()
	postForm(t, h, "Action=VerifyEmailIdentity&EmailAddress=attr@example.com")

	// GetIdentityDkimAttributes
	rec := postForm(t, h, url.Values{
		"Action":              []string{"GetIdentityDkimAttributes"},
		"Identities.member.1": []string{"attr@example.com"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)

	// GetIdentityMailFromDomainAttributes
	rec = postForm(t, h, url.Values{
		"Action":              []string{"GetIdentityMailFromDomainAttributes"},
		"Identities.member.1": []string{"attr@example.com"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)

	// GetIdentityNotificationAttributes
	rec = postForm(t, h, url.Values{
		"Action":              []string{"GetIdentityNotificationAttributes"},
		"Identities.member.1": []string{"attr@example.com"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)

	// SetIdentityDkimEnabled
	rec = postForm(t, h, url.Values{
		"Action":      []string{"SetIdentityDkimEnabled"},
		"Identity":    []string{"attr@example.com"},
		"DkimEnabled": []string{"true"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)

	// SetIdentityFeedbackForwardingEnabled
	rec = postForm(t, h, url.Values{
		"Action":            []string{"SetIdentityFeedbackForwardingEnabled"},
		"Identity":          []string{"attr@example.com"},
		"ForwardingEnabled": []string{"true"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)

	// SetIdentityMailFromDomain
	rec = postForm(t, h, url.Values{
		"Action":         []string{"SetIdentityMailFromDomain"},
		"Identity":       []string{"attr@example.com"},
		"MailFromDomain": []string{"mail.example.com"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)
}

func TestVerifyEmailIdentity_Basic(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, url.Values{
		"Action":       {"VerifyEmailIdentity"},
		"Version":      {"2010-12-01"},
		"EmailAddress": {"alice@example.com"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "VerifyEmailIdentityResponse")

	attrs := h.Backend.GetIdentityVerificationAttributes([]string{"alice@example.com"})
	assert.Equal(t, "Success", attrs["alice@example.com"])
}

func TestVerifyEmailIdentity_MissingAddress(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, "Action=VerifyEmailIdentity&Version=2010-12-01&EmailAddress=")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestVerifyEmailIdentity_Idempotent(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("bob@example.com"))
	require.NoError(t, b.VerifyEmailIdentity("bob@example.com"))

	attrs := b.GetIdentityVerificationAttributes([]string{"bob@example.com"})
	assert.Equal(t, "Success", attrs["bob@example.com"])
}

func TestDeleteIdentity(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("gone@example.com"))

	rec := postForm(t, h, url.Values{
		"Action":   {"DeleteIdentity"},
		"Version":  {"2010-12-01"},
		"Identity": {"gone@example.com"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DeleteIdentityResponse")

	p := h.Backend.ListIdentities("", 100, "")
	assert.NotContains(t, p.Data, "gone@example.com")
}

func TestDeleteIdentity_NonExistent_IsIdempotent(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, url.Values{
		"Action":   {"DeleteIdentity"},
		"Version":  {"2010-12-01"},
		"Identity": {"nobody@example.com"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDeleteIdentity_ClearsPoliciesOnRecreate(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("reused@example.com"))
	require.NoError(t, h.Backend.PutIdentityPolicy("reused@example.com", "my-policy", `{"Version":"2012-10-17"}`))

	h.Backend.DeleteIdentity("reused@example.com")

	require.NoError(t, h.Backend.VerifyEmailIdentity("reused@example.com"))

	names, err := h.Backend.ListIdentityPolicies("reused@example.com")
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestListIdentities_IdentityTypeFilter(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("user@example.com"))
	_, err := h.Backend.VerifyDomainIdentity("example.com")
	require.NoError(t, err)

	rec := postForm(t, h, url.Values{
		"Action":       {"ListIdentities"},
		"Version":      {"2010-12-01"},
		"IdentityType": {"Domain"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "example.com")
	assert.NotContains(t, body, "user@example.com",
		"IdentityType=Domain must exclude email-address identities")
}

func TestListIdentities_InvalidIdentityType(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, url.Values{
		"Action":       {"ListIdentities"},
		"Version":      {"2010-12-01"},
		"IdentityType": {"NotARealType"},
	}.Encode())
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ValidationError")
}

func TestListIdentities_Pagination(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	for i := range 5 {
		require.NoError(t, b.VerifyEmailIdentity(fmt.Sprintf("user%d@example.com", i)))
	}

	p := b.ListIdentities("", 3, "")
	assert.Len(t, p.Data, 3)
	assert.NotEmpty(t, p.Next)

	p2 := b.ListIdentities(p.Next, 3, "")
	assert.Len(t, p2.Data, 2)
	assert.Empty(t, p2.Next)
}

func TestListIdentities_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, h.Backend.VerifyEmailIdentity("b@example.com"))

	rec := postForm(t, h, url.Values{
		"Action":   {"ListIdentities"},
		"Version":  {"2010-12-01"},
		"MaxItems": {"10"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "a@example.com")
	assert.Contains(t, rec.Body.String(), "b@example.com")
}

func TestGetIdentityVerificationAttributes_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("v@example.com"))

	rec := postForm(t, h, url.Values{
		"Action":              {"GetIdentityVerificationAttributes"},
		"Version":             {"2010-12-01"},
		"Identities.member.1": {"v@example.com"},
		"Identities.member.2": {"unknown@example.com"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Success")
	assert.Contains(t, rec.Body.String(), "NotStarted")
}

func TestSetIdentityMailFromDomain_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("mf@example.com"))

	rec := postForm(t, h, url.Values{
		"Action":         {"SetIdentityMailFromDomain"},
		"Version":        {"2010-12-01"},
		"Identity":       {"mf@example.com"},
		"MailFromDomain": {"mail.example.com"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	attrs := h.Backend.GetIdentityMailFromDomainAttributes([]string{"mf@example.com"})
	assert.Equal(t, "mail.example.com", attrs["mf@example.com"].MailFromDomain)
	assert.Equal(t, "Success", attrs["mf@example.com"].MailFromDomainStatus)
}

func TestGetIdentityMailFromDomainAttributes_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("mf2@example.com"))
	require.NoError(t, h.Backend.SetIdentityMailFromDomain("mf2@example.com", "bounce.example.com", ""))

	rec := postForm(t, h, url.Values{
		"Action":              {"GetIdentityMailFromDomainAttributes"},
		"Version":             {"2010-12-01"},
		"Identities.member.1": {"mf2@example.com"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "bounce.example.com")
}

func TestMailFromDomainAttributes_Unknown_Empty(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	attrs := b.GetIdentityMailFromDomainAttributes([]string{"nobody@example.com"})
	assert.Empty(t, attrs["nobody@example.com"].MailFromDomain)
	assert.Empty(t, attrs["nobody@example.com"].MailFromDomainStatus)
}

func TestSetIdentityNotificationTopic_AllTypes_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("notif@example.com"))

	for _, notifType := range []string{"Bounce", "Complaint", "Delivery"} {
		rec := postForm(t, h, url.Values{
			"Action":           {"SetIdentityNotificationTopic"},
			"Version":          {"2010-12-01"},
			"Identity":         {"notif@example.com"},
			"NotificationType": {notifType},
			"SnsTopic":         {"arn:aws:sns:us-east-1:123:" + notifType},
		}.Encode())
		assert.Equal(t, http.StatusOK, rec.Code, "type=%s", notifType)
	}

	attrs := h.Backend.GetIdentityNotificationAttributes([]string{"notif@example.com"})
	a := attrs["notif@example.com"]
	assert.Equal(t, "arn:aws:sns:us-east-1:123:Bounce", a.BounceTopic)
	assert.Equal(t, "arn:aws:sns:us-east-1:123:Complaint", a.ComplaintTopic)
	assert.Equal(t, "arn:aws:sns:us-east-1:123:Delivery", a.DeliveryTopic)
}

func TestSetIdentityFeedbackForwardingEnabled_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("fwd@example.com"))

	rec := postForm(t, h, url.Values{
		"Action":            {"SetIdentityFeedbackForwardingEnabled"},
		"Version":           {"2010-12-01"},
		"Identity":          {"fwd@example.com"},
		"ForwardingEnabled": {"false"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	attrs := h.Backend.GetIdentityNotificationAttributes([]string{"fwd@example.com"})
	assert.False(t, attrs["fwd@example.com"].ForwardingEnabled)
}

func TestSetIdentityHeadersInNotificationsEnabled_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("hdr@example.com"))

	for _, notifType := range []string{"Bounce", "Complaint", "Delivery"} {
		rec := postForm(t, h, url.Values{
			"Action":           {"SetIdentityHeadersInNotificationsEnabled"},
			"Version":          {"2010-12-01"},
			"Identity":         {"hdr@example.com"},
			"NotificationType": {notifType},
			"Enabled":          {"true"},
		}.Encode())
		assert.Equal(t, http.StatusOK, rec.Code, "type=%s", notifType)
	}

	attrs := h.Backend.GetIdentityNotificationAttributes([]string{"hdr@example.com"})
	a := attrs["hdr@example.com"]
	assert.True(t, a.HeadersInBounce)
	assert.True(t, a.HeadersInComplaint)
	assert.True(t, a.HeadersInDelivery)
}

func TestGetIdentityNotificationAttributes_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("na@example.com"))
	require.NoError(t, h.Backend.SetIdentityNotificationTopic("na@example.com", "Bounce", "arn:sns:bounce"))

	rec := postForm(t, h, url.Values{
		"Action":              {"GetIdentityNotificationAttributes"},
		"Version":             {"2010-12-01"},
		"Identities.member.1": {"na@example.com"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "GetIdentityNotificationAttributesResponse")
}

func TestNotificationAttributes_Unknown_ForwardingEnabled(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	attrs := b.GetIdentityNotificationAttributes([]string{"unknown@example.com"})
	assert.True(t, attrs["unknown@example.com"].ForwardingEnabled, "forwarding defaults true for unknown identity")
}

func TestPutIdentityPolicy_StoresDocument(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("pol@example.com"))

	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow",` +
		`"Principal":"*","Action":"ses:SendEmail","Resource":"*"}]}`
	require.NoError(t, b.PutIdentityPolicy("pol@example.com", "send-policy", doc))

	pols, err := b.GetIdentityPolicies("pol@example.com", []string{"send-policy"})
	require.NoError(t, err)
	assert.Equal(t, doc, pols["send-policy"])
}

func TestPutIdentityPolicy_OverwritesExisting(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("pol@example.com"))
	require.NoError(t, b.PutIdentityPolicy("pol@example.com", "p1", `{"v":1}`))
	require.NoError(t, b.PutIdentityPolicy("pol@example.com", "p1", `{"v":2}`))

	pols, err := b.GetIdentityPolicies("pol@example.com", []string{"p1"})
	require.NoError(t, err)
	assert.Equal(t, `{"v":2}`, pols["p1"])
}

func TestListIdentityPolicies_ReturnsSortedNames(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("pol@example.com"))
	require.NoError(t, b.PutIdentityPolicy("pol@example.com", "z-policy", `{}`))
	require.NoError(t, b.PutIdentityPolicy("pol@example.com", "a-policy", `{}`))
	require.NoError(t, b.PutIdentityPolicy("pol@example.com", "m-policy", `{}`))

	names, err := b.ListIdentityPolicies("pol@example.com")
	require.NoError(t, err)
	assert.Equal(t, []string{"a-policy", "m-policy", "z-policy"}, names)
}

func TestDeleteIdentityPolicy_RemovesFromList(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("pol@example.com"))
	require.NoError(t, b.PutIdentityPolicy("pol@example.com", "del-pol", `{}`))
	require.NoError(t, b.DeleteIdentityPolicy("pol@example.com", "del-pol"))

	names, err := b.ListIdentityPolicies("pol@example.com")
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestDeleteIdentityPolicy_NonExistent_Idempotent(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("pol@example.com"))
	require.NoError(t, b.DeleteIdentityPolicy("pol@example.com", "nonexistent"))
}

// TestGetIdentityPolicies_EmptyNamesList_IsRejected proves PolicyNames is
// enforced as required (api_op_GetIdentityPolicies.go: "This member is
// required" -- there is no real "return everything" mode).
func TestGetIdentityPolicies_EmptyNamesList_IsRejected(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("pol@example.com"))
	require.NoError(t, b.PutIdentityPolicy("pol@example.com", "p1", `{"a":1}`))

	_, err := b.GetIdentityPolicies("pol@example.com", nil)
	require.Error(t, err)
}

func TestPutIdentityPolicy_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("pol@example.com"))

	rec := postForm(t, h, url.Values{
		"Action":     {"PutIdentityPolicy"},
		"Version":    {"2010-12-01"},
		"Identity":   {"pol@example.com"},
		"PolicyName": {"my-send-policy"},
		"Policy":     {`{"Version":"2012-10-17","Statement":[]}`},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	names, err := h.Backend.ListIdentityPolicies("pol@example.com")
	require.NoError(t, err)
	assert.Contains(t, names, "my-send-policy")
}

func TestGetIdentityPolicies_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("pol@example.com"))
	require.NoError(t, h.Backend.PutIdentityPolicy("pol@example.com", "p1", `{"doc":true}`))

	rec := postForm(t, h, url.Values{
		"Action":               {"GetIdentityPolicies"},
		"Version":              {"2010-12-01"},
		"Identity":             {"pol@example.com"},
		"PolicyNames.member.1": {"p1"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "GetIdentityPoliciesResponse")
}

func TestListIdentityPolicies_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("pol@example.com"))
	require.NoError(t, h.Backend.PutIdentityPolicy("pol@example.com", "pol1", `{}`))
	require.NoError(t, h.Backend.PutIdentityPolicy("pol@example.com", "pol2", `{}`))

	rec := postForm(t, h, url.Values{
		"Action":   {"ListIdentityPolicies"},
		"Version":  {"2010-12-01"},
		"Identity": {"pol@example.com"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ListIdentityPoliciesResponse")
}

func TestDeleteIdentityPolicy_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("pol@example.com"))
	require.NoError(t, h.Backend.PutIdentityPolicy("pol@example.com", "todel", `{}`))

	rec := postForm(t, h, url.Values{
		"Action":     {"DeleteIdentityPolicy"},
		"Version":    {"2010-12-01"},
		"Identity":   {"pol@example.com"},
		"PolicyName": {"todel"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	names, err := h.Backend.ListIdentityPolicies("pol@example.com")
	require.NoError(t, err)
	assert.NotContains(t, names, "todel")
}

func TestIdentityPolicy_MissingIdentity_Error(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.Error(t, b.PutIdentityPolicy("", "p", `{}`))
	require.Error(t, b.DeleteIdentityPolicy("", "p"))
	_, err := b.GetIdentityPolicies("", nil)
	require.Error(t, err)
	_, err = b.ListIdentityPolicies("")
	require.Error(t, err)
}

func TestPolicyCount_Tracks(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, b.VerifyEmailIdentity("b@example.com"))
	assert.Equal(t, 0, b.PolicyCount())

	require.NoError(t, b.PutIdentityPolicy("a@example.com", "p1", `{}`))
	require.NoError(t, b.PutIdentityPolicy("a@example.com", "p2", `{}`))
	require.NoError(t, b.PutIdentityPolicy("b@example.com", "p1", `{}`))
	assert.Equal(t, 3, b.PolicyCount())

	require.NoError(t, b.DeleteIdentityPolicy("a@example.com", "p1"))
	assert.Equal(t, 2, b.PolicyCount())
}

func TestPutIdentityPolicy_MissingPolicyName_Error(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	assert.Error(t, b.PutIdentityPolicy("a@example.com", "", `{}`))
}

// TestPutIdentityPolicy_InvalidJSON_Error verifies malformed and empty Policy
// documents are rejected with InvalidPolicy (ErrInvalidPolicy), matching real
// AWS SES's InvalidPolicyException. This backend does not evaluate policy
// semantics, but it does require the document be well-formed JSON, matching
// the wire error code a real SDK client would see for a malformed policy.
func TestPutIdentityPolicy_InvalidJSON_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		name    string
		policy  string
	}{
		{name: "empty_policy", policy: "", wantErr: ses.ErrInvalidParameter},
		{name: "whitespace_only_policy", policy: "   ", wantErr: ses.ErrInvalidParameter},
		{name: "malformed_json", policy: `{not valid`, wantErr: ses.ErrInvalidPolicy},
		{name: "unterminated_json", policy: `{"unterminated": `, wantErr: ses.ErrInvalidPolicy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ses.NewInMemoryBackend()
			err := b.PutIdentityPolicy("a@example.com", "p1", tt.policy)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestHandler_PutIdentityPolicy_InvalidPolicy_WireError verifies the handler
// surfaces InvalidPolicy on the wire (not a generic InternalFailure/500) for a
// malformed Policy document.
func TestHandler_PutIdentityPolicy_InvalidPolicy_WireError(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, url.Values{
		"Action":     {"PutIdentityPolicy"},
		"Version":    {"2010-12-01"},
		"Identity":   {"a@example.com"},
		"PolicyName": {"p1"},
		"Policy":     {`{"unterminated": `},
	}.Encode())

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidPolicy")
}

func TestVerifyEmailIdentity_AlreadyExists_Idempotent(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, b.SetIdentityDkimEnabled("a@example.com", true))
	require.NoError(t, b.VerifyEmailIdentity("a@example.com"))

	attrs := b.GetIdentityDkimAttributes([]string{"a@example.com"})
	assert.True(t, attrs["a@example.com"].DkimEnabled, "existing attributes preserved on re-verify")
}

func TestListIdentities_IncludesBothEmailsAndDomains(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("user@example.com"))
	_, err := b.VerifyDomainIdentity("example.com")
	require.NoError(t, err)

	p := b.ListIdentities("", 100, "")
	assert.Contains(t, p.Data, "user@example.com")
	assert.Contains(t, p.Data, "example.com")
}

func TestIdentityVerification_EmailVsDomain(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("user@example.com"))
	_, err := b.VerifyDomainIdentity("example.com")
	require.NoError(t, err)

	attrs := b.GetIdentityVerificationAttributes([]string{"user@example.com", "example.com"})
	assert.Equal(t, "Success", attrs["user@example.com"])
	assert.Equal(t, "Success", attrs["example.com"])
}

func TestSetIdentityMailFromDomain_Persists(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, b.SetIdentityMailFromDomain("a@example.com", "mail.example.com", ""))

	attrs := b.GetIdentityMailFromDomainAttributes([]string{"a@example.com"})
	assert.Equal(t, "mail.example.com", attrs["a@example.com"].MailFromDomain)
	assert.Equal(t, "Success", attrs["a@example.com"].MailFromDomainStatus)
}

func TestSetIdentityMailFromDomain_Clear(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, b.SetIdentityMailFromDomain("a@example.com", "mail.example.com", ""))
	require.NoError(t, b.SetIdentityMailFromDomain("a@example.com", "", ""))

	attrs := b.GetIdentityMailFromDomainAttributes([]string{"a@example.com"})
	assert.Empty(t, attrs["a@example.com"].MailFromDomain)
	assert.Empty(t, attrs["a@example.com"].MailFromDomainStatus)
}

func TestSetIdentityNotificationTopic_Persists(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, b.SetIdentityNotificationTopic("a@example.com", "Bounce", "arn:sns:bounce"))
	require.NoError(t, b.SetIdentityNotificationTopic("a@example.com", "Complaint", "arn:sns:complaint"))
	require.NoError(t, b.SetIdentityNotificationTopic("a@example.com", "Delivery", "arn:sns:delivery"))

	attrs := b.GetIdentityNotificationAttributes([]string{"a@example.com"})
	a := attrs["a@example.com"]
	assert.Equal(t, "arn:sns:bounce", a.BounceTopic)
	assert.Equal(t, "arn:sns:complaint", a.ComplaintTopic)
	assert.Equal(t, "arn:sns:delivery", a.DeliveryTopic)
}

func TestSetIdentityFeedbackForwardingEnabled_Persists(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, b.SetIdentityFeedbackForwardingEnabled("a@example.com", false))

	attrs := b.GetIdentityNotificationAttributes([]string{"a@example.com"})
	assert.False(t, attrs["a@example.com"].ForwardingEnabled)
}

func TestSetIdentityHeadersInNotificationsEnabled_Persists(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, b.SetIdentityHeadersInNotificationsEnabled("a@example.com", "Bounce", true))
	require.NoError(t, b.SetIdentityHeadersInNotificationsEnabled("a@example.com", "Complaint", true))

	attrs := b.GetIdentityNotificationAttributes([]string{"a@example.com"})
	a := attrs["a@example.com"]
	assert.True(t, a.HeadersInBounce)
	assert.True(t, a.HeadersInComplaint)
	assert.False(t, a.HeadersInDelivery)
}

func TestIdentityRecord_SnapshotRestore(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, b.SetIdentityDkimEnabled("a@example.com", true))
	require.NoError(t, b.SetIdentityMailFromDomain("a@example.com", "mail.example.com", ""))
	require.NoError(t, b.SetIdentityNotificationTopic("a@example.com", "Bounce", "arn:topic"))

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := ses.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	dkim := fresh.GetIdentityDkimAttributes([]string{"a@example.com"})
	assert.True(t, dkim["a@example.com"].DkimEnabled)

	mail := fresh.GetIdentityMailFromDomainAttributes([]string{"a@example.com"})
	assert.Equal(t, "mail.example.com", mail["a@example.com"].MailFromDomain)

	notif := fresh.GetIdentityNotificationAttributes([]string{"a@example.com"})
	assert.Equal(t, "arn:topic", notif["a@example.com"].BounceTopic)
}

func TestListIdentities_EmptyState(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, url.Values{
		"Action":  {"ListIdentities"},
		"Version": {"2010-12-01"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "ListIdentitiesResponse",
		"response envelope must be ListIdentitiesResponse")
	assert.Contains(t, body, "Identities",
		"Identities element must be present even when empty")
	assert.NotContains(t, body, "<member>",
		"no member elements expected when no identities exist")
}

func TestListIdentities_NextTokenAbsentWhenNotTruncated(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.VerifyEmailIdentity("a@example.com"))
	require.NoError(t, h.Backend.VerifyEmailIdentity("b@example.com"))

	rec := postForm(t, h, url.Values{
		"Action":  {"ListIdentities"},
		"Version": {"2010-12-01"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	assert.NotContains(t, rec.Body.String(), "<NextToken>",
		"NextToken must be absent when all results fit on one page")
}

func TestGetIdentityNotificationAttributes_ForwardingEnabledDefaultTrue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(b *ses.InMemoryBackend) error
		name     string
		identity string
	}{
		{
			name:     "email_identity_defaults_forwarding_true",
			identity: "a@example.com",
			setup:    func(b *ses.InMemoryBackend) error { return b.VerifyEmailIdentity("a@example.com") },
		},
		{
			name:     "domain_identity_defaults_forwarding_true",
			identity: "example.com",
			setup: func(b *ses.InMemoryBackend) error {
				_, err := b.VerifyDomainIdentity("example.com")

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			require.NoError(t, tt.setup(h.Backend.(*ses.InMemoryBackend)))

			rec := postForm(t, h, url.Values{
				"Action":              {"GetIdentityNotificationAttributes"},
				"Version":             {"2010-12-01"},
				"Identities.member.1": {tt.identity},
			}.Encode())
			require.Equal(t, http.StatusOK, rec.Code)

			assert.Contains(t, rec.Body.String(), "<ForwardingEnabled>true</ForwardingEnabled>",
				"ForwardingEnabled must default to true for newly verified identity")
		})
	}
}

func TestGetIdentityVerificationAttributes_UnknownReturnsNotStarted(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, url.Values{
		"Action":              {"GetIdentityVerificationAttributes"},
		"Version":             {"2010-12-01"},
		"Identities.member.1": {"unknown@example.com"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "<VerificationStatus>NotStarted</VerificationStatus>",
		"unknown identity must return NotStarted verification status")
}

func TestSetIdentityMailFromDomain_BehaviorOnMXFailure(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		behavior      string
		wantPersisted string
		wantErr       bool
	}{
		{name: "default_when_omitted", behavior: "", wantPersisted: "UseDefaultValue"},
		{name: "use_default_value_explicit", behavior: "UseDefaultValue", wantPersisted: "UseDefaultValue"},
		{name: "reject_message", behavior: "RejectMessage", wantPersisted: "RejectMessage"},
		{name: "invalid_value_rejected", behavior: "Bogus", wantErr: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ses.NewInMemoryBackend()
			err := b.SetIdentityMailFromDomain("a@example.com", "mail.example.com", tt.behavior)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ses.ErrInvalidParameter)

				return
			}

			require.NoError(t, err)

			attrs := b.GetIdentityMailFromDomainAttributes([]string{"a@example.com"})
			assert.Equal(t, tt.wantPersisted, attrs["a@example.com"].BehaviorOnMXFailure)
		})
	}
}

func TestSetIdentityMailFromDomain_Handler_PlumbsBehaviorOnMXFailure(t *testing.T) {
	t.Parallel()

	h := newHandler()

	rec := postForm(t, h, url.Values{
		"Action":              {"SetIdentityMailFromDomain"},
		"Version":             {"2010-12-01"},
		"Identity":            {"a@example.com"},
		"MailFromDomain":      {"mail.example.com"},
		"BehaviorOnMXFailure": {"RejectMessage"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	attrs := h.Backend.GetIdentityMailFromDomainAttributes([]string{"a@example.com"})
	assert.Equal(t, "RejectMessage", attrs["a@example.com"].BehaviorOnMXFailure)
}

func TestSESHandler_ListIdentities(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Verify identities first.
	postForm(t, h, "Action=VerifyEmailIdentity&Version=2010-12-01&EmailAddress=alice@example.com")
	postForm(t, h, "Action=VerifyEmailIdentity&Version=2010-12-01&EmailAddress=bob@example.com")

	rec := postForm(t, h, "Action=ListIdentities&Version=2010-12-01")

	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "ListIdentitiesResponse")
	assert.Contains(t, body, "alice@example.com")
	assert.Contains(t, body, "bob@example.com")
}

func TestSESHandler_DeleteIdentity(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Verify an identity first.
	postForm(t, h, "Action=VerifyEmailIdentity&Version=2010-12-01&EmailAddress=del@example.com")

	// Delete it.
	rec := postForm(t, h, "Action=DeleteIdentity&Version=2010-12-01&Identity=del@example.com")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DeleteIdentityResponse")

	// Verify it's gone.
	listRec := postForm(t, h, "Action=ListIdentities&Version=2010-12-01")
	assert.NotContains(t, listRec.Body.String(), "del@example.com")

	// Deleting again is idempotent — returns success.
	rec2 := postForm(t, h, "Action=DeleteIdentity&Version=2010-12-01&Identity=del@example.com")
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "DeleteIdentityResponse")
}

func TestSESHandler_GetIdentityVerificationAttributes(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Verify an identity first.
	postForm(t, h, "Action=VerifyEmailIdentity&Version=2010-12-01&EmailAddress=verified@example.com")

	body := url.Values{
		"Action":              {"GetIdentityVerificationAttributes"},
		"Version":             {"2010-12-01"},
		"Identities.member.1": {"verified@example.com"},
		"Identities.member.2": {"unknown@example.com"},
	}

	rec := postForm(t, h, body.Encode())

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"GetIdentityVerificationAttributesResponse"`
		Result  struct {
			VerificationAttributes struct {
				Entries []struct {
					Key   string `xml:"key"`
					Value struct {
						Status string `xml:"VerificationStatus"`
					} `xml:"value"`
				} `xml:"entry"`
			} `xml:"VerificationAttributes"`
		} `xml:"GetIdentityVerificationAttributesResult"`
	}

	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

	statusByID := make(map[string]string)
	for _, e := range resp.Result.VerificationAttributes.Entries {
		statusByID[e.Key] = e.Value.Status
	}

	assert.Equal(t, "Success", statusByID["verified@example.com"])
	assert.Equal(t, "NotStarted", statusByID["unknown@example.com"])
}

func TestSESBackend_DeleteIdentityIdempotent(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()

	// Deleting a non-existent identity should not panic or error.
	b.DeleteIdentity("nonexistent@test.com")
	assert.Equal(t, 0, b.IdentityCount())

	// Add and delete.
	require.NoError(t, b.VerifyEmailIdentity("test@test.com"))
	assert.Equal(t, 1, b.IdentityCount())

	b.DeleteIdentity("test@test.com")
	assert.Equal(t, 0, b.IdentityCount())

	// Delete again — idempotent.
	b.DeleteIdentity("test@test.com")
	assert.Equal(t, 0, b.IdentityCount())
}
