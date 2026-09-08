package sesv2_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sesv2sdk "github.com/aws/aws-sdk-go-v2/service/sesv2"
	sesv2types "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sesv2"
)

func TestSESv2Backend_SendEmailCap(t *testing.T) {
	t.Parallel()

	b := sesv2.NewInMemoryBackend()

	_, err := b.CreateEmailIdentity("a@example.com", "", nil)
	require.NoError(t, err)

	// Send beyond 2x the cap so the amortized compaction path runs at least
	// once. After compaction the slice length must stay between
	// maxRetainedEmails and 2*maxRetainedEmails.
	total := sesv2.EmailCompactionHighWater + 5
	for i := range total {
		_, sendErr := b.SendEmail("a@example.com", []string{"b@example.com"},
			"s", "h", "t", nil)
		require.NoError(t, sendErr, "iteration %d", i)
	}

	got := sesv2.EmailCount(b)
	assert.GreaterOrEqual(t, got, sesv2.MaxRetainedEmails,
		"retain at least the cap")
	assert.LessOrEqual(t, got, sesv2.EmailCompactionHighWater,
		"never exceed the compaction high-water mark")
}

// TestSendEmailRequiresDestination verifies that SendEmail rejects requests with no
// destination addresses. Real AWS requires at least one ToAddress, CcAddress, or
// BccAddress; the emulator previously sent the email silently with an empty
// destination list.
func TestSendEmailRequiresDestination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "absent_destination_rejected",
			body: map[string]any{
				"FromEmailAddress": "sender@example.com",
				"Content": map[string]any{
					"Simple": map[string]any{
						"Subject": map[string]any{"Data": "Hello"},
						"Body":    map[string]any{"Text": map[string]any{"Data": "body"}},
					},
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "empty_destination_rejected",
			body: map[string]any{
				"FromEmailAddress": "sender@example.com",
				"Destination": map[string]any{
					"ToAddresses":  []string{},
					"CcAddresses":  []string{},
					"BccAddresses": []string{},
				},
				"Content": map[string]any{
					"Simple": map[string]any{
						"Subject": map[string]any{"Data": "Hello"},
						"Body":    map[string]any{"Text": map[string]any{"Data": "body"}},
					},
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "to_address_accepted",
			body: map[string]any{
				"FromEmailAddress": "sender@example.com",
				"Destination":      map[string]any{"ToAddresses": []string{"rcpt@example.com"}},
				"Content": map[string]any{
					"Simple": map[string]any{
						"Subject": map[string]any{"Data": "Hello"},
						"Body":    map[string]any{"Text": map[string]any{"Data": "body"}},
					},
				},
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			doRequest(t, h, http.MethodPost, "/v2/email/identities",
				map[string]any{"EmailIdentity": "sender@example.com"})
			rec := doRequest(t, h, http.MethodPost, "/v2/email/outbound-emails", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code,
				"SendEmail status for case %q", tt.name)
		})
	}
}

// TestSendBulkEmail tests the SendBulkEmail operation.
func TestSendBulkEmail(t *testing.T) {
	t.Parallel()

	h, b := newSESv2TestHandler(t)
	_, err := b.CreateEmailIdentity("bulk@example.com", "", nil)
	require.NoError(t, err)

	doReq(t, h, http.MethodPost, "/v2/email/templates", map[string]any{
		"TemplateName": "BulkTemplate",
		"TemplateContent": map[string]any{
			"Subject": "Hello {{name}}",
			"Html":    "<html>Hi {{name}}</html>",
		},
	})

	rec := doReq(t, h, http.MethodPost, "/v2/email/outbound-bulk-emails", map[string]any{
		"FromEmailAddress": "bulk@example.com",
		"DefaultContent": map[string]any{
			"Template": map[string]any{
				"TemplateName": "BulkTemplate",
				"TemplateData": `{"name":"default"}`,
			},
		},
		"BulkEmailEntries": []map[string]any{
			{
				"Destination": map[string]any{
					"ToAddresses": []string{"to1@example.com"},
				},
			},
			{
				"Destination": map[string]any{
					"ToAddresses": []string{"to2@example.com"},
				},
				"ReplacementEmailContent": map[string]any{
					"ReplacementTemplate": map[string]any{
						"ReplacementTemplateData": `{"name":"to2"}`,
					},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	emails := b.ListEmails()
	require.Len(t, emails, 2)
	assert.Equal(t, "Hello default", emails[0].Subject,
		"DefaultContent's template must actually populate the sent email")
	assert.Equal(t, "<html>Hi default</html>", emails[0].BodyHTML)
	assert.Equal(t, "Hello to2", emails[1].Subject,
		"per-entry ReplacementTemplateData must override the default vars")
}

// TestSendEmail_ContentTemplate verifies that SendEmail's Content.Template
// (api_op_SendEmail.go:24-26: "Templated -- A message that contains
// personalization tags. When you send this type of email, Amazon SES API v2
// automatically replaces the tags with values that you specify") actually
// renders the referenced/inline template. The emulator's emailContent decode
// struct previously had no Template field at all, so a templated SendEmail
// silently recorded an email with an empty subject and body.
func TestSendEmail_ContentTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body        map[string]any
		name        string
		wantSubject string
		wantHTML    string
	}{
		{
			name: "inline_template_content",
			body: map[string]any{
				"FromEmailAddress": "tmpl@example.com",
				"Destination":      map[string]any{"ToAddresses": []string{"to@example.com"}},
				"Content": map[string]any{
					"Template": map[string]any{
						"TemplateContent": map[string]any{
							"Subject": "Hello {{name}}",
							"Html":    "<html>Hi {{name}}</html>",
						},
						"TemplateData": `{"name":"inline"}`,
					},
				},
			},
			wantSubject: "Hello inline",
			wantHTML:    "<html>Hi inline</html>",
		},
		{
			name: "stored_template_by_name",
			body: map[string]any{
				"FromEmailAddress": "tmpl@example.com",
				"Destination":      map[string]any{"ToAddresses": []string{"to@example.com"}},
				"Content": map[string]any{
					"Template": map[string]any{
						"TemplateName": "StoredTemplate",
						"TemplateData": `{"name":"stored"}`,
					},
				},
			},
			wantSubject: "Hello stored",
			wantHTML:    "<html>Hi stored</html>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newSESv2TestHandler(t)
			_, err := b.CreateEmailIdentity("tmpl@example.com", "", nil)
			require.NoError(t, err)

			doReq(t, h, http.MethodPost, "/v2/email/templates", map[string]any{
				"TemplateName": "StoredTemplate",
				"TemplateContent": map[string]any{
					"Subject": "Hello {{name}}",
					"Html":    "<html>Hi {{name}}</html>",
				},
			})

			rec := doReq(t, h, http.MethodPost, "/v2/email/outbound-emails", tt.body)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			emails := b.ListEmails()
			require.Len(t, emails, 1)
			assert.Equal(t, tt.wantSubject, emails[0].Subject,
				"Content.Template must actually populate the sent email")
			assert.Equal(t, tt.wantHTML, emails[0].BodyHTML)
		})
	}
}

// TestSendBulkEmailRequiresDefaultContent verifies that SendBulkEmail rejects
// requests with no DefaultContent, since every real bulk email needs content
// to send. The emulator previously accepted the request and recorded emails
// with an empty subject and body no matter what the caller asked for.
func TestSendBulkEmailRequiresDefaultContent(t *testing.T) {
	t.Parallel()

	h, b := newSESv2TestHandler(t)
	_, err := b.CreateEmailIdentity("bulk-nocontent@example.com", "", nil)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodPost, "/v2/email/outbound-bulk-emails", map[string]any{
		"FromEmailAddress": "bulk-nocontent@example.com",
		"BulkEmailEntries": []map[string]any{
			{
				"Destination": map[string]any{
					"ToAddresses": []string{"to1@example.com"},
				},
			},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestSendBulkEmailSDKRoundTrip drives SendBulkEmail through the real
// aws-sdk-go-v2 sesv2 client, verifying the bulkEmailEntry typed request DTO
// (send_email.go) decodes multi-recipient BulkEmailEntries correctly and
// bulkEmailEntryResultOutput (wire_output.go) round-trips through the
// genuine SDK deserializer -- catching the class of dropped-field bug a
// hand-decoded map[string]any assertion would miss.
func TestSendBulkEmailSDKRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entries []sesv2types.BulkEmailEntry
	}{
		{
			name: "single_entry_single_recipient",
			entries: []sesv2types.BulkEmailEntry{
				{Destination: &sesv2types.Destination{ToAddresses: []string{"to1@example.com"}}},
			},
		},
		{
			name: "multiple_entries_multiple_recipients",
			entries: []sesv2types.BulkEmailEntry{
				{Destination: &sesv2types.Destination{
					ToAddresses: []string{"to1@example.com", "to2@example.com"},
				}},
				{Destination: &sesv2types.Destination{
					ToAddresses:  []string{"to3@example.com"},
					CcAddresses:  []string{"cc1@example.com"},
					BccAddresses: []string{"bcc1@example.com"},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			client := newSESv2SDKClient(t, h)

			_, err := client.CreateEmailIdentity(t.Context(), &sesv2sdk.CreateEmailIdentityInput{
				EmailIdentity: aws.String("bulk-sdk@example.com"),
			})
			require.NoError(t, err)

			out, err := client.SendBulkEmail(t.Context(), &sesv2sdk.SendBulkEmailInput{
				FromEmailAddress: aws.String("bulk-sdk@example.com"),
				DefaultContent: &sesv2types.BulkEmailContent{
					Template: &sesv2types.Template{
						TemplateData: aws.String(`{}`),
						TemplateContent: &sesv2types.EmailTemplateContent{
							Subject: aws.String("Hi"),
							Text:    aws.String("body"),
						},
					},
				},
				BulkEmailEntries: tt.entries,
			})
			require.NoError(t, err)
			require.Len(t, out.BulkEmailEntryResults, len(tt.entries))

			for _, r := range out.BulkEmailEntryResults {
				assert.NotEmpty(t, aws.ToString(r.MessageId))
				assert.Equal(t, sesv2types.BulkEmailStatusSuccess, r.Status)
			}

			emails := h.Backend.ListEmails()
			require.Len(t, emails, len(tt.entries))
			for _, e := range emails {
				assert.Equal(t, "Hi", e.Subject,
					"DefaultContent.Template.TemplateContent must reach the recorded email")
				assert.Equal(t, "body", e.BodyText)
			}
		})
	}
}

// TestSendEmail tests the SendEmail operation via HTTP.
func TestSendEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "sends email successfully",
			body: map[string]any{
				"FromEmailAddress": "sender@example.com",
				"Destination": map[string]any{
					"ToAddresses": []string{"recipient@example.com"},
				},
				"Content": map[string]any{
					"Simple": map[string]any{
						"Subject": map[string]any{"Data": "Hello"},
						"Body": map[string]any{
							"Text": map[string]any{"Data": "Hello World"},
							"Html": map[string]any{"Data": "<b>Hello World</b>"},
						},
					},
				},
			},
			wantCode: http.StatusOK,
		},
		{
			name: "missing from address returns bad request",
			body: map[string]any{
				"Destination": map[string]any{
					"ToAddresses": []string{"recipient@example.com"},
				},
				"Content": map[string]any{
					"Simple": map[string]any{
						"Subject": map[string]any{"Data": "Hello"},
						"Body":    map[string]any{},
					},
				},
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.wantCode == http.StatusOK {
				doRequest(t, h, http.MethodPost, "/v2/email/identities",
					map[string]any{"EmailIdentity": "sender@example.com"})
			}

			rec := doRequest(t, h, http.MethodPost, "/v2/email/outbound-emails", tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.NotEmpty(t, out["MessageId"])

				emails := h.Backend.ListEmails()
				require.Len(t, emails, 1)
				assert.Equal(t, "sender@example.com", emails[0].From)
			}
		})
	}
}

// TestSendEmail_UnverifiedIdentity drives the real SDK client and asserts the
// specific typed exception SendEmail's own deserializeOpError models for an
// unverified From identity (sesv2@v1.66.4 types/errors.go:220, "The message
// can't be sent because the sending domain isn't verified."). The emulator
// previously raised a plain BadRequestException, which a real client's
// errors.As against MailFromDomainNotVerifiedException would never match.
func TestSendEmail_UnverifiedIdentity(t *testing.T) {
	t.Parallel()

	h := newHandler()
	client := newSESv2SDKClient(t, h)

	_, err := client.SendEmail(t.Context(), &sesv2sdk.SendEmailInput{
		FromEmailAddress: aws.String("unverified@example.com"),
		Destination: &sesv2types.Destination{
			ToAddresses: []string{"recipient@example.com"},
		},
		Content: &sesv2types.EmailContent{
			Simple: &sesv2types.Message{
				Subject: &sesv2types.Content{Data: aws.String("Hello")},
				Body: &sesv2types.Body{
					Text: &sesv2types.Content{Data: aws.String("Hello World")},
				},
			},
		},
	})
	require.Error(t, err)

	var mfnv *sesv2types.MailFromDomainNotVerifiedException
	require.ErrorAs(t, err, &mfnv, "expected a real MailFromDomainNotVerifiedException from the SDK deserializer")
}

// TestSendBulkEmail_UnverifiedIdentity drives the real SDK client and asserts
// that each entry's Status is MAIL_FROM_DOMAIN_NOT_VERIFIED (types.go:305)
// for an unverified From identity, instead of the whole call succeeding. The
// emulator previously discarded SendEmail's per-entry error entirely
// (msgID, _ := b.SendEmail(...)) and always reported SUCCESS.
func TestSendBulkEmail_UnverifiedIdentity(t *testing.T) {
	t.Parallel()

	h := newHandler()
	client := newSESv2SDKClient(t, h)

	out, err := client.SendBulkEmail(t.Context(), &sesv2sdk.SendBulkEmailInput{
		FromEmailAddress: aws.String("unverified-bulk@example.com"),
		DefaultContent: &sesv2types.BulkEmailContent{
			Template: &sesv2types.Template{
				TemplateData: aws.String(`{}`),
				TemplateContent: &sesv2types.EmailTemplateContent{
					Subject: aws.String("Hi"),
					Text:    aws.String("body"),
				},
			},
		},
		BulkEmailEntries: []sesv2types.BulkEmailEntry{
			{Destination: &sesv2types.Destination{ToAddresses: []string{"to1@example.com"}}},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.BulkEmailEntryResults, 1)
	assert.Equal(t, sesv2types.BulkEmailStatusMailFromDomainNotVerified, out.BulkEmailEntryResults[0].Status)
	assert.Empty(t, aws.ToString(out.BulkEmailEntryResults[0].MessageId))
	assert.Empty(t, h.Backend.ListEmails(), "an unverified sender must not record any email")
}
