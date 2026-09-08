package sesv2

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// EmailTemplateContent holds the content for an email template.
type EmailTemplateContent struct {
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

// EmailTemplate represents a SES v2 email template.
type EmailTemplate struct {
	TemplateContent *EmailTemplateContent `json:"templateContent"`
	Tags            map[string]string     `json:"tags,omitempty"`
	CreatedAt       time.Time             `json:"createdAt"`
	TemplateName    string                `json:"templateName"`
}

// emailTemplateARN builds the ARN for an email template:
// arn:{partition}:ses:{region}:{account}:template/{name}. Confirmed against
// terraform-provider-aws's templateARN (internal/service/ses/template.go),
// which must construct this exact ARN to tag/import real templates -- also
// matches tenants.go's existing resourceTypeFromARN, which already treats
// ":template/" as EMAIL_TEMPLATE.
func (b *InMemoryBackend) emailTemplateARN(name string) string {
	return arn.Build("ses", b.region, b.accountID, "template/"+name)
}

// CreateEmailTemplate creates a new email template.
func (b *InMemoryBackend) CreateEmailTemplate(
	templateName string,
	content *EmailTemplateContent,
	tags map[string]string,
) (*EmailTemplate, error) {
	if strings.TrimSpace(templateName) == "" {
		return nil, fmt.Errorf("%w: TemplateName is required", ErrInvalidInput)
	}

	b.mu.Lock("CreateEmailTemplate")
	defer b.mu.Unlock()

	if b.emailTemplates.Has(templateName) {
		return nil, fmt.Errorf(
			"%w: email template %s already exists",
			ErrAlreadyExists,
			templateName,
		)
	}

	var contentCopy *EmailTemplateContent
	if content != nil {
		c := *content
		contentCopy = &c
	}

	t := &EmailTemplate{
		TemplateName:    templateName,
		TemplateContent: contentCopy,
		CreatedAt:       time.Now(),
		Tags:            tags,
	}
	b.emailTemplates.Put(t)

	if len(tags) > 0 {
		b.putResourceTagsLocked(b.emailTemplateARN(templateName), tags)
	}

	cp := *t

	return &cp, nil
}

// ---- email templates ----

// GetEmailTemplate retrieves a template.
func (b *InMemoryBackend) GetEmailTemplate(name string) (*EmailTemplate, error) {
	b.mu.RLock("GetEmailTemplate")
	defer b.mu.RUnlock()

	t, ok := b.emailTemplates.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: email template %s not found", ErrNotFound, name)
	}

	cp := *t
	cp.Tags = b.liveTagsLocked(b.emailTemplateARN(name))

	return &cp, nil
}

// DeleteEmailTemplate removes a template.
func (b *InMemoryBackend) DeleteEmailTemplate(name string) error {
	b.mu.Lock("DeleteEmailTemplate")
	defer b.mu.Unlock()

	if !b.emailTemplates.Has(name) {
		return fmt.Errorf("%w: email template %s not found", ErrNotFound, name)
	}

	b.emailTemplates.Delete(name)
	delete(b.resourceTags, b.emailTemplateARN(name))

	return nil
}

// UpdateEmailTemplate updates a template.
func (b *InMemoryBackend) UpdateEmailTemplate(name string, content *EmailTemplateContent) error {
	b.mu.Lock("UpdateEmailTemplate")
	defer b.mu.Unlock()

	t, ok := b.emailTemplates.Get(name)
	if !ok {
		return fmt.Errorf("%w: email template %s not found", ErrNotFound, name)
	}

	if content != nil {
		cp := *content
		t.TemplateContent = &cp
	}

	return nil
}

// ListEmailTemplates returns all email templates.
func (b *InMemoryBackend) ListEmailTemplates(
	nextToken string,
	pageSize int,
) page.Page[*EmailTemplate] {
	b.mu.RLock("ListEmailTemplates")
	defer b.mu.RUnlock()

	snap := b.emailTemplates.Snapshot()

	items := make([]*EmailTemplate, 0, len(snap))
	for _, t := range snap {
		cp := *t
		items = append(items, &cp)
	}

	return page.New(items, nextToken, pageSize, sesv2DefaultMaxItems)
}

// TestRenderEmailTemplate renders a template with merge data.
func (b *InMemoryBackend) TestRenderEmailTemplate(name, templateData string) (string, error) {
	b.mu.RLock("TestRenderEmailTemplate")
	defer b.mu.RUnlock()

	t, ok := b.emailTemplates.Get(name)
	if !ok {
		return "", fmt.Errorf("%w: email template %s not found", ErrNotFound, name)
	}

	vars, err := parseTemplateVars(templateData)
	if err != nil {
		return "", err
	}

	subject, html, text := "", "", ""
	if t.TemplateContent != nil {
		subject = renderTemplateVars(t.TemplateContent.Subject, vars)
		html = renderTemplateVars(t.TemplateContent.HTML, vars)
		text = renderTemplateVars(t.TemplateContent.Text, vars)
	}

	return strings.Join([]string{subject, html, text}, "\n---\n"), nil
}

// parseTemplateVars decodes a JSON object of template substitution variables,
// the shape SES TemplateData/ReplacementTemplateData strings carry.
func parseTemplateVars(data string) (map[string]string, error) {
	vars := map[string]string{}
	if strings.TrimSpace(data) == "" {
		return vars, nil
	}

	raw := map[string]any{}
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, fmt.Errorf("%w: TemplateData must be valid JSON", ErrInvalidInput)
	}

	for k, v := range raw {
		vars[k] = fmt.Sprintf("%v", v)
	}

	return vars, nil
}

// renderTemplateVars substitutes {{key}} placeholders in s using vars.
func renderTemplateVars(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}

	return s
}

// mergeTemplateVars layers overrides on top of defaults without mutating
// either input map.
func mergeTemplateVars(defaults, overrides map[string]string) map[string]string {
	merged := make(map[string]string, len(defaults)+len(overrides))
	maps.Copy(merged, defaults)

	maps.Copy(merged, overrides)

	return merged
}
