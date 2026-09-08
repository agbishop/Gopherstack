package ses

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// maxRecipientsPerMessage is the AWS SES limit on the combined number of
// To, Cc and Bcc recipients in a single SendEmail/SendTemplatedEmail call.
// Exceeding it yields a MessageRejected error in real SES.
const maxRecipientsPerMessage = 50

// maxRetainedEmails is the maximum number of sent emails retained in memory.
// Oldest emails are evicted when the limit is exceeded.
const maxRetainedEmails = 10000

// AWS SES mailbox simulator addresses: the documented, deterministic way to
// trigger a bounce or complaint outcome without a real receiving mailbox.
// https://docs.aws.amazon.com/ses/latest/dg/send-an-email-from-console.html#send-email-simulator
const (
	simulatorBounceAddress       = "bounce@simulator.amazonses.com"
	simulatorSuppressionListAddr = "suppressionlist@simulator.amazonses.com"
	simulatorComplaintAddress    = "complaint@simulator.amazonses.com"
)

// classifySimulatedRecipients reports whether recipients contains one of the
// SES mailbox simulator's bounce/suppression-list or complaint addresses.
func classifySimulatedRecipients(recipients []string) (bool, bool) {
	var bounced, complained bool

	for _, r := range recipients {
		switch {
		case strings.EqualFold(r, simulatorBounceAddress), strings.EqualFold(r, simulatorSuppressionListAddr):
			bounced = true
		case strings.EqualFold(r, simulatorComplaintAddress):
			complained = true
		}
	}

	return bounced, complained
}

// allRecipients concatenates To, Cc, and Bcc into a single slice.
func allRecipients(to, cc, bcc []string) []string {
	out := make([]string, 0, len(to)+len(cc)+len(bcc))
	out = append(out, to...)
	out = append(out, cc...)
	out = append(out, bcc...)

	return out
}

// checkSendingAllowedLocked validates the account-level, quota, and
// configuration-set preconditions shared by every send operation (SendEmail,
// SendRawEmail via SendEmail, SendTemplatedEmail, SendBulkTemplatedEmail):
// sending must not be paused account-wide, matching real AWS SES's
// AccountSendingPausedException; the simulated 24-hour send quota
// (GetSendQuota's Max24HourSend) must not already be exhausted, matching
// MessageRejected; and a non-empty ConfigurationSetName must reference an
// existing configuration set, matching ConfigurationSetDoesNotExist.
//
// The caller MUST hold b.mu for writing.
func (b *InMemoryBackend) checkSendingAllowedLocked(configurationSetName string) error {
	if !b.accountSendingEnabled {
		return fmt.Errorf("%w: account-level sending is currently paused", ErrAccountSendingPaused)
	}

	if b.sentLast24HoursLocked() >= maxSendQuota24Hours {
		return fmt.Errorf(
			"%w: 24-hour sending quota of %d messages exceeded",
			ErrMessageRejected, maxSendQuota24Hours,
		)
	}

	if configurationSetName != "" {
		if !b.configSets.Has(configurationSetName) {
			return fmt.Errorf("%w: %s", ErrConfigSetNotFound, configurationSetName)
		}
	}

	return nil
}

// appendEmailLocked appends e to the slice and O(1) map, evicting the oldest
// entries when the cap is exceeded.
//
// The caller MUST hold b.mu for writing.
func (b *InMemoryBackend) appendEmailLocked(e Email) {
	b.emails = append(b.emails, e)
	ec := e
	b.emailsByID.Put(&ec)

	if len(b.emails) > maxRetainedEmails {
		evicted := b.emails[:len(b.emails)-maxRetainedEmails]
		for _, ev := range evicted {
			b.emailsByID.Delete(ev.MessageID)
		}

		b.emails = b.emails[len(b.emails)-maxRetainedEmails:]
	}
}

// SendEmail captures an outbound email and returns a message ID.
// The source address must be a verified identity or from a verified domain
// (matching real AWS SES behavior).
func (b *InMemoryBackend) SendEmail(in SendEmailInput) (string, error) {
	if in.From == "" {
		return "", fmt.Errorf("%w: Source is required", ErrInvalidParameter)
	}

	if len(in.To)+len(in.Cc)+len(in.Bcc) == 0 {
		return "", fmt.Errorf(
			"%w: Destination must contain at least one ToAddress, CcAddress, or BccAddress",
			ErrInvalidParameter,
		)
	}

	// AWS SES caps a single message at 10 MiB total (subject + body + headers).
	const maxMessageBytes = 10 * 1024 * 1024
	if len(in.Subject)+len(in.BodyHTML)+len(in.BodyText) > maxMessageBytes {
		return "", fmt.Errorf("%w: message exceeds 10 MB", ErrMessageRejected)
	}

	if total := len(in.To) + len(in.Cc) + len(in.Bcc); total > maxRecipientsPerMessage {
		return "", fmt.Errorf(
			"%w: Recipient count exceeds %d (got %d)",
			ErrMessageRejected, maxRecipientsPerMessage, total,
		)
	}

	msgID, email, targets, err := b.sendEmailLocked(in)
	if err != nil {
		return "", err
	}

	b.publishEmailNotifications(email, targets)

	return msgID, nil
}

// sendEmailLocked performs the locked portion of SendEmail -- precondition
// checks, message construction, and appending to the email store -- and
// collects the SNS notification targets for the send's outcome. The caller
// publishes to those targets after the lock is released (see
// publishEmailNotifications), matching cloudwatch's alarmActionDeps pattern
// of not holding the backend lock across an outbound SNS call.
func (b *InMemoryBackend) sendEmailLocked(in SendEmailInput) (string, Email, sesNotificationTargets, error) {
	b.mu.Lock("SendEmail")
	defer b.mu.Unlock()

	if err := b.checkSendingAllowedLocked(in.ConfigurationSetName); err != nil {
		return "", Email{}, sesNotificationTargets{}, err
	}

	if !b.isVerifiedLocked(in.From) {
		return "", Email{}, sesNotificationTargets{}, fmt.Errorf(
			"%w: Email address is not verified. The following identities failed the check in region %s: %s",
			ErrMessageRejected, strings.ToUpper(b.region), in.From,
		)
	}

	msgID := "ses-" + uuid.New().String()
	bounced, complained := classifySimulatedRecipients(allRecipients(in.To, in.Cc, in.Bcc))

	email := Email{
		MessageID:            msgID,
		From:                 in.From,
		To:                   in.To,
		Cc:                   in.Cc,
		Bcc:                  in.Bcc,
		ReplyTo:              in.ReplyTo,
		Subject:              in.Subject,
		BodyHTML:             in.BodyHTML,
		BodyText:             in.BodyText,
		ConfigurationSetName: in.ConfigurationSetName,
		Tags:                 in.Tags,
		ReturnPath:           in.ReturnPath,
		ReturnPathArn:        in.ReturnPathArn,
		SourceArn:            in.SourceArn,
		Timestamp:            time.Now(),
		Bounced:              bounced,
		Complained:           complained,
	}
	b.appendEmailLocked(email)

	targets := b.collectNotificationTargetsLocked(in.From, in.ConfigurationSetName, bounced, complained)

	return msgID, email, targets, nil
}

// SendTemplatedEmail sends an email using a stored template and returns the message ID.
// The source address must be a verified identity or from a verified domain.
// The template must already exist; ErrTemplateNotFound is returned otherwise.
func (b *InMemoryBackend) SendTemplatedEmail(in SendTemplatedEmailInput) (string, error) {
	if in.From == "" {
		return "", fmt.Errorf("%w: Source is required", ErrInvalidParameter)
	}

	// Validate template data up front so malformed JSON is rejected with
	// InvalidParameterValue regardless of verification state, matching SES.
	vars, err := parseTemplateData(in.TemplateData)
	if err != nil {
		return "", err
	}

	if total := len(in.To) + len(in.Cc) + len(in.Bcc); total > maxRecipientsPerMessage {
		return "", fmt.Errorf(
			"%w: Recipient count exceeds %d (got %d)",
			ErrMessageRejected, maxRecipientsPerMessage, total,
		)
	}

	msgID, email, targets, err := b.sendTemplatedEmailLocked(in, vars)
	if err != nil {
		return "", err
	}

	b.publishEmailNotifications(email, targets)

	return msgID, nil
}

// sendTemplatedEmailLocked is SendTemplatedEmail's locked portion -- see
// sendEmailLocked's doc comment for why notification publishing happens
// after this returns, unlocked.
func (b *InMemoryBackend) sendTemplatedEmailLocked(
	in SendTemplatedEmailInput, vars map[string]string,
) (string, Email, sesNotificationTargets, error) {
	b.mu.Lock("SendTemplatedEmail")
	defer b.mu.Unlock()

	if sendErr := b.checkSendingAllowedLocked(in.ConfigurationSetName); sendErr != nil {
		return "", Email{}, sesNotificationTargets{}, sendErr
	}

	if !b.isVerifiedLocked(in.From) {
		return "", Email{}, sesNotificationTargets{}, fmt.Errorf(
			"%w: Email address is not verified. The following identities failed the check in region %s: %s",
			ErrMessageRejected, strings.ToUpper(b.region), in.From,
		)
	}

	tmpl, ok := b.templates.Get(in.TemplateName)
	if !ok {
		return "", Email{}, sesNotificationTargets{}, fmt.Errorf("%w: %s", ErrTemplateNotFound, in.TemplateName)
	}

	msgID := "ses-" + uuid.New().String()
	bounced, complained := classifySimulatedRecipients(allRecipients(in.To, in.Cc, in.Bcc))

	email := Email{
		MessageID:            msgID,
		From:                 in.From,
		To:                   in.To,
		Cc:                   in.Cc,
		Bcc:                  in.Bcc,
		ReplyTo:              in.ReplyTo,
		Subject:              renderTemplateVars(tmpl.SubjectPart, vars),
		BodyHTML:             renderTemplateVars(tmpl.HTMLPart, vars),
		BodyText:             renderTemplateVars(tmpl.TextPart, vars),
		ConfigurationSetName: in.ConfigurationSetName,
		Tags:                 in.Tags,
		ReturnPath:           in.ReturnPath,
		ReturnPathArn:        in.ReturnPathArn,
		SourceArn:            in.SourceArn,
		Timestamp:            time.Now(),
		Bounced:              bounced,
		Complained:           complained,
	}
	b.appendEmailLocked(email)

	targets := b.collectNotificationTargetsLocked(in.From, in.ConfigurationSetName, bounced, complained)

	return msgID, email, targets, nil
}

// ListEmails returns a copy of all captured emails.
func (b *InMemoryBackend) ListEmails() []Email {
	b.mu.RLock("ListEmails")
	defer b.mu.RUnlock()

	out := make([]Email, len(b.emails))
	copy(out, b.emails)

	return out
}

// GetEmailByID returns the email with the given MessageID in O(1) time, or an error if not found.
func (b *InMemoryBackend) GetEmailByID(messageID string) (Email, error) {
	b.mu.RLock("GetEmailByID")
	defer b.mu.RUnlock()

	if e, ok := b.emailsByID.Get(messageID); ok {
		return *e, nil
	}

	return Email{}, fmt.Errorf("%w: %s", ErrEmailNotFound, messageID)
}

// sweepExpiredEmails removes emails older than emailTTL. Called by the janitor.
// The caller must NOT hold the lock.
func (b *InMemoryBackend) sweepExpiredEmails(cutoff time.Time) int {
	b.mu.Lock("sweepExpiredEmails")
	defer b.mu.Unlock()

	first := 0

	for first < len(b.emails) && b.emails[first].Timestamp.Before(cutoff) {
		b.emailsByID.Delete(b.emails[first].MessageID)
		first++
	}

	if first == 0 {
		return 0
	}

	b.emails = b.emails[first:]

	return first
}

// SendBounce generates and sends a bounce message for a previously received
// email. Real AWS SES models BounceSender and BouncedRecipientInfoList as
// required input members (SendBounceInput), so both must be supplied here;
// BounceSender must additionally be a verified identity (or a verified
// domain), matching the same sender-verification rule enforced by SendEmail.
func (b *InMemoryBackend) SendBounce(originalMsgID, bounceSender string, recipients []string) (string, error) {
	if strings.TrimSpace(originalMsgID) == "" {
		return "", fmt.Errorf("%w: OriginalMessageId is required", ErrInvalidParameter)
	}

	if strings.TrimSpace(bounceSender) == "" {
		return "", fmt.Errorf("%w: BounceSender is required", ErrInvalidParameter)
	}

	if len(recipients) == 0 {
		return "", fmt.Errorf("%w: BouncedRecipientInfoList must contain at least one entry", ErrInvalidParameter)
	}

	b.mu.Lock("SendBounce")
	defer b.mu.Unlock()

	if !b.isVerifiedLocked(bounceSender) {
		return "", fmt.Errorf(
			"%w: Email address is not verified. The following identities failed the check in region %s: %s",
			ErrMessageRejected, strings.ToUpper(b.region), bounceSender,
		)
	}

	return "ses-bounce-" + uuid.New().String(), nil
}

// SendBulkTemplatedEmail sends one email per destination and returns a message
// ID for each. Each destination is rendered with the request-level
// DefaultTemplateData merged with that destination's ReplacementTemplateData,
// matching AWS SES SendBulkTemplatedEmail semantics where replacement values
// override defaults on a per-recipient basis. ConfigurationSetName, ReplyTo,
// ReturnPath, ReturnPathArn and SourceArn mirror the corresponding
// SendBulkTemplatedEmailInput members and are threaded through to every
// generated Email record exactly as SendEmail/SendTemplatedEmail do for a
// single-destination send. Message tags follow the same per-destination
// override pattern as template data: a destination's ReplacementTags, when
// non-empty, is used in place of (not merged with) the request-level
// DefaultTags for that destination's stored Email record.
func (b *InMemoryBackend) SendBulkTemplatedEmail(in SendBulkTemplatedEmailInput) ([]string, error) {
	if strings.TrimSpace(in.Source) == "" {
		return nil, fmt.Errorf("%w: Source is required", ErrInvalidParameter)
	}

	if strings.TrimSpace(in.TemplateName) == "" {
		return nil, fmt.Errorf("%w: Template is required", ErrInvalidParameter)
	}

	// Validate the template exists before touching any destination so a missing
	// template fails fast with TemplateDoesNotExist even for an empty batch,
	// matching real SES which validates the template at request time.
	if _, err := b.GetTemplate(in.TemplateName); err != nil {
		return nil, err
	}

	// Validate the configuration set (when supplied) exists up front, matching
	// the same ConfigurationSetDoesNotExist precondition enforced by SendEmail
	// and SendTemplatedEmail.
	if in.ConfigurationSetName != "" {
		b.mu.RLock("SendBulkTemplatedEmail")
		exists := b.configSets.Has(in.ConfigurationSetName)
		b.mu.RUnlock()

		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrConfigSetNotFound, in.ConfigurationSetName)
		}
	}

	msgIDs := make([]string, 0, len(in.Destinations))

	for _, d := range in.Destinations {
		// Each destination merges its replacement data over the request default.
		// We pre-render the variables here and pass the merged JSON down so
		// SendTemplatedEmail performs the substitution against stored parts.
		merged, err := mergeTemplateData(in.DefaultTemplateData, d.ReplacementTemplateData)
		if err != nil {
			return nil, err
		}

		mergedJSON, err := json.Marshal(merged)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to encode template data", ErrInvalidParameter)
		}

		tags := in.DefaultTags
		if len(d.ReplacementTags) > 0 {
			tags = d.ReplacementTags
		}

		msgID, err := b.SendTemplatedEmail(SendTemplatedEmailInput{
			From:                 in.Source,
			To:                   d.To,
			Cc:                   d.Cc,
			Bcc:                  d.Bcc,
			ReplyTo:              in.ReplyTo,
			TemplateName:         in.TemplateName,
			TemplateData:         string(mergedJSON),
			ConfigurationSetName: in.ConfigurationSetName,
			Tags:                 tags,
			ReturnPath:           in.ReturnPath,
			ReturnPathArn:        in.ReturnPathArn,
			SourceArn:            in.SourceArn,
		})
		if err != nil {
			return nil, err
		}

		msgIDs = append(msgIDs, msgID)
	}

	return msgIDs, nil
}

// SearchEmails returns emails whose From, Subject, or To fields contain the given query string (case-insensitive).
// This provides O(n) filtered access while leveraging the existing email slice.
func (b *InMemoryBackend) SearchEmails(query string) []Email {
	b.mu.RLock("SearchEmails")
	defer b.mu.RUnlock()

	if query == "" {
		out := make([]Email, len(b.emails))
		copy(out, b.emails)

		return out
	}

	q := strings.ToLower(query)
	var out []Email

	for _, e := range b.emails {
		if strings.Contains(strings.ToLower(e.From), q) ||
			strings.Contains(strings.ToLower(e.Subject), q) ||
			containsAny(e.To, q) {
			out = append(out, e)
		}
	}

	return out
}

// containsAny reports whether any element of ss contains substr (case-insensitive).
func containsAny(ss []string, substr string) bool {
	for _, s := range ss {
		if strings.Contains(strings.ToLower(s), substr) {
			return true
		}
	}

	return false
}
