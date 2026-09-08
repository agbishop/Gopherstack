package transfer

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"
)

// DeleteCertificate removes a certificate by ID.
func (b *InMemoryBackend) DeleteCertificate(certificateID string) error {
	b.mu.Lock("DeleteCertificate")
	defer b.mu.Unlock()

	if !b.certificates.Has(certificateID) {
		return fmt.Errorf("%w: certificate %s not found", ErrCertificateNotFound, certificateID)
	}

	b.certificates.Delete(certificateID)
	delete(b.tagsStore, certificateARN(b.accountID, b.region, certificateID))

	return nil
}

// ImportCertificateInput holds all fields for ImportCertificate.
type ImportCertificateInput struct {
	NotBefore        time.Time
	NotAfter         time.Time
	ActiveDate       time.Time
	InactiveDate     time.Time
	Tags             map[string]string
	Usage            string
	Body             string
	Description      string
	CertificateChain string
	PrivateKey       string
}

// ImportCertificate imports a certificate. NotBefore/NotAfter are optional; zero
// values use defaults (now and +1 year). If Body is a valid PEM certificate,
// NotBefore/NotAfter/Serial are extracted from it.
func (b *InMemoryBackend) ImportCertificate(
	usage, body, description string,
	notBefore, notAfter time.Time,
	tags map[string]string,
) (*Certificate, error) {
	return b.ImportCertificateFull(&ImportCertificateInput{
		Usage: usage, Body: body, Description: description,
		NotBefore: notBefore, NotAfter: notAfter, Tags: tags,
	})
}

// ImportCertificateFull imports a certificate with full configuration, including the
// real-AWS ActiveDate/InactiveDate/CertificateChain fields.
func (b *InMemoryBackend) ImportCertificateFull(in *ImportCertificateInput) (*Certificate, error) {
	b.mu.Lock("ImportCertificate")
	defer b.mu.Unlock()

	notBefore, notAfter, serial := in.NotBefore, in.NotAfter, ""

	// Try to parse PEM if provided.
	if in.Body != "" {
		block, _ := pem.Decode([]byte(in.Body))
		if block == nil {
			return nil, fmt.Errorf(
				"%w: certificate body is not a valid PEM block",
				ErrValidation,
			)
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: failed to parse certificate: %w",
				ErrValidation,
				err,
			)
		}

		// Override notBefore/notAfter from certificate.
		notBefore = cert.NotBefore
		notAfter = cert.NotAfter
		serial = cert.SerialNumber.String()
	}

	certID := "cert-" + uuid.NewString()[:20]

	now := time.Now()
	if notBefore.IsZero() {
		notBefore = now
	}

	if notAfter.IsZero() {
		notAfter = now.AddDate(1, 0, 0)
	}

	merged := make(map[string]string, len(in.Tags))
	maps.Copy(merged, in.Tags)

	c := &Certificate{
		CertificateID:    certID,
		Usage:            in.Usage,
		Body:             in.Body,
		CertificateChain: in.CertificateChain,
		Serial:           serial,
		HasPrivateKey:    in.PrivateKey != "",
		Description:      in.Description,
		NotBeforeDate:    notBefore,
		NotAfterDate:     notAfter,
		ActiveDate:       in.ActiveDate,
		InactiveDate:     in.InactiveDate,
		CreatedAt:        now,
		Tags:             merged,
		AccountID:        b.accountID,
		Region:           b.region,
	}
	c.Status = certificateStatus(c, now)
	b.certificates.Put(c)
	b.initTagsStore(certificateARN(b.accountID, b.region, certID), merged)

	cp := *c
	cp.Tags = make(map[string]string, len(merged))
	maps.Copy(cp.Tags, merged)

	return &cp, nil
}

// certificateStatus computes a certificate's ACTIVE/INACTIVE status the way real AWS
// documents it: ActiveDate/InactiveDate take precedence when set, falling back to
// NotBeforeDate/NotAfterDate (the X.509 validity window) otherwise.
func certificateStatus(c *Certificate, now time.Time) string {
	start := c.NotBeforeDate
	if !c.ActiveDate.IsZero() {
		start = c.ActiveDate
	}

	end := c.NotAfterDate
	if !c.InactiveDate.IsZero() {
		end = c.InactiveDate
	}

	if !start.IsZero() && now.Before(start) {
		return agreementStatusInactive
	}

	if !end.IsZero() && now.After(end) {
		return agreementStatusInactive
	}

	return agreementStatusActive
}

// DescribeCertificate returns a certificate by ID.
func (b *InMemoryBackend) DescribeCertificate(certificateID string) (*Certificate, error) {
	b.mu.RLock("DescribeCertificate")
	defer b.mu.RUnlock()

	c, ok := b.certificates.Get(certificateID)
	if !ok {
		return nil, fmt.Errorf(
			"%w: certificate %s not found",
			ErrCertificateNotFound,
			certificateID,
		)
	}

	cp := *c
	cp.Tags = make(map[string]string, len(c.Tags))
	maps.Copy(cp.Tags, c.Tags)

	return &cp, nil
}

// ListCertificates returns all certificates sorted by certificateID.
func (b *InMemoryBackend) ListCertificates() []*Certificate {
	b.mu.RLock("ListCertificates")
	defer b.mu.RUnlock()

	all := b.certificates.All()
	out := make([]*Certificate, 0, len(all))

	for _, c := range all {
		cp := *c
		cp.Tags = make(map[string]string, len(c.Tags))
		maps.Copy(cp.Tags, c.Tags)
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CertificateID < out[j].CertificateID
	})

	return out
}

// UpdateCertificateInput holds all optional fields for UpdateCertificate.
type UpdateCertificateInput struct {
	ActiveDate    time.Time
	InactiveDate  time.Time
	CertificateID string
	Description   string
}

// UpdateCertificate updates mutable fields on a certificate.
func (b *InMemoryBackend) UpdateCertificate(
	certificateID, description string,
) (*Certificate, error) {
	return b.UpdateCertificateFull(&UpdateCertificateInput{
		CertificateID: certificateID,
		Description:   description,
	})
}

// UpdateCertificateFull updates all mutable fields on a certificate, including the
// real-AWS ActiveDate/InactiveDate fields (recomputes Status when either changes).
func (b *InMemoryBackend) UpdateCertificateFull(in *UpdateCertificateInput) (*Certificate, error) {
	b.mu.Lock("UpdateCertificate")
	defer b.mu.Unlock()

	c, ok := b.certificates.Get(in.CertificateID)
	if !ok {
		return nil, fmt.Errorf(
			"%w: certificate %s not found",
			ErrCertificateNotFound,
			in.CertificateID,
		)
	}

	if in.Description != "" {
		c.Description = in.Description
	}

	if !in.ActiveDate.IsZero() {
		c.ActiveDate = in.ActiveDate
	}

	if !in.InactiveDate.IsZero() {
		c.InactiveDate = in.InactiveDate
	}

	c.Status = certificateStatus(c, time.Now())

	cp := *c
	cp.Tags = make(map[string]string, len(c.Tags))
	maps.Copy(cp.Tags, c.Tags)

	return &cp, nil
}

// AddCertificateInternal seeds a certificate for testing purposes.
func (b *InMemoryBackend) AddCertificateInternal(certID string) {
	b.mu.Lock("AddCertificateInternal")
	defer b.mu.Unlock()

	b.certificates.Put(&Certificate{
		CertificateID: certID,
		CreatedAt:     time.Now(),
		Tags:          make(map[string]string),
		AccountID:     b.accountID,
		Region:        b.region,
	})
}
