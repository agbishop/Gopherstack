package acm

import (
	"context"
	"fmt"
	"time"

	sdktypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
)

// autoValidate transitions a certificate from PENDING_VALIDATION to ISSUED after a
// short delay, simulating the DNS/email validation workflow.
func (b *InMemoryBackend) autoValidate(region, certARN string) {
	b.mu.Lock("autoValidate")
	defer b.mu.Unlock()

	delete(b.timersStore(region), certARN)

	c, ok := b.certs.Get(regionKey(region, certARN))
	if !ok || c.Status != statusPendingValidation {
		return
	}

	now := time.Now().UTC()
	c.Status = statusIssued
	c.IssuedAt = &now

	for i := range c.DomainValidationOptions {
		c.DomainValidationOptions[i].ValidationStatus = validationStatusSuccess
	}
}

// autoValidateRenewal transitions a certificate's RenewalSummary from PENDING_VALIDATION to SUCCESS after a
// short delay, simulating the DNS/email validation workflow for managed renewals.
func (b *InMemoryBackend) autoValidateRenewal(region, certARN string) {
	b.mu.Lock("autoValidateRenewal")
	defer b.mu.Unlock()

	delete(b.timersStore(region), certARN)

	c, ok := b.certs.Get(regionKey(region, certARN))
	if !ok || c.RenewalSummary == nil || c.RenewalSummary.RenewalStatus != renewalStatusPendingValidation {
		return
	}

	c.RenewalSummary.RenewalStatus = validationStatusSuccess
	c.RenewalSummary.UpdatedAt = time.Now().UTC()
	for i := range c.RenewalSummary.DomainValidationOptions {
		c.RenewalSummary.DomainValidationOptions[i].ValidationStatus = validationStatusSuccess
	}
}

// ResendValidationEmail re-triggers the EMAIL validation flow for a certificate
// that is still in PENDING_VALIDATION status with EMAIL validation method.
func (b *InMemoryBackend) ResendValidationEmail(ctx context.Context, certARN, domain, validationDomain string) error {
	if certARN == "" {
		return fmt.Errorf("%w: CertificateArn is required", ErrInvalidParameter)
	}

	if domain == "" {
		return fmt.Errorf("%w: Domain is required", ErrInvalidParameter)
	}

	if validationDomain == "" {
		return fmt.Errorf("%w: ValidationDomain is required", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("ResendValidationEmail")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if cert.Status != statusPendingValidation {
		return fmt.Errorf("%w: certificate is not in PENDING_VALIDATION status", ErrInvalidParameter)
	}

	if cert.ValidationMethod != validationMethodEMAIL {
		return fmt.Errorf("%w: certificate was not requested with EMAIL validation", ErrInvalidParameter)
	}

	found := false
	for i, dvo := range cert.DomainValidationOptions {
		if dvo.DomainName == domain {
			cert.DomainValidationOptions[i].ValidationStatus = statusPendingValidation
			found = true
		}
	}

	if !found {
		return fmt.Errorf("%w: domain %s not found in certificate", ErrInvalidParameter, domain)
	}

	// Reset the auto-validate timer to simulate email resend triggering re-validation.
	timers := b.timersStore(region)
	if t, exists := timers[certARN]; exists {
		t.Stop()
		delete(timers, certARN)
	}

	t := time.AfterFunc(b.getAutoValidateDelayLocked(), func() { b.autoValidate(region, certARN) })
	timers[certARN] = t

	return nil
}

// validRevocationReason derives its answer from types.RevocationReason.Values()
// so it cannot drift from the real enum -- the previous hand-copied list was
// missing SUPERCEDED, the deprecated misspelling AWS keeps accepting
// alongside SUPERSEDED.
func validRevocationReason(r string) bool {
	for _, v := range sdktypes.RevocationReason("").Values() {
		if string(v) == r {
			return true
		}
	}

	return false
}

// RevokeCertificate marks the certificate as REVOKED with the given reason.
// Returns ErrAlreadyRevoked if the certificate is already revoked.
// Only ISSUED certificates can be revoked; PENDING_VALIDATION certs return ErrConflict.
func (b *InMemoryBackend) RevokeCertificate(ctx context.Context, certARN, revocationReason string) error {
	if certARN == "" {
		return fmt.Errorf("%w: CertificateArn is required", ErrInvalidParameter)
	}

	if revocationReason == "" {
		return fmt.Errorf("%w: RevocationReason is required", ErrInvalidParameter)
	}

	if !validRevocationReason(revocationReason) {
		return fmt.Errorf("%w: invalid RevocationReason %q", ErrInvalidParameter, revocationReason)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("RevokeCertificate")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if cert.Status == statusRevoked {
		// RevokeCertificate's real error set (InvalidArnException/AccessDeniedException/
		// ConflictException/ResourceInUseException/ResourceNotFoundException/
		// ThrottlingException/ValidationException, deserializers.go, acm@v1.43.4) has no
		// InvalidStateException, and no declared code's doc text fits "already revoked"
		// (a terminal, one-time state) the way ConflictException fits PENDING_VALIDATION
		// below -- landmined rather than guessed; see gopherstack-bzyl.
		return fmt.Errorf("%w: certificate %s is already revoked", ErrAlreadyRevoked, certARN)
	}

	if cert.Status == statusPendingValidation {
		// ConflictException: "You are trying to update a resource or configuration that
		// is already being created or updated. Wait for the previous operation to finish
		// and try again." (types/errors.go, acm@v1.43.4) -- a certificate still in
		// PENDING_VALIDATION has an unfinished RequestCertificate/RenewCertificate
		// validation in progress, matching this doc word for word. RevokeCertificate does
		// not declare InvalidStateException at all; gopherstack-bzyl.
		return fmt.Errorf(
			"%w: certificate %s is in PENDING_VALIDATION and cannot be revoked",
			ErrConflict, certARN,
		)
	}

	now := time.Now().UTC()
	cert.Status = statusRevoked
	cert.RevocationReason = revocationReason
	cert.RevokedAt = &now

	// Stop any pending auto-validate timer.
	timers := b.timersStore(region)
	if t, exists := timers[certARN]; exists {
		t.Stop()
		delete(timers, certARN)
	}

	return nil
}

// validTransparencyPreference reports whether a given CertificateTransparencyLoggingPreference is valid.
func validTransparencyPreference(p string) bool {
	return p == transparencyLoggingEnabled || p == transparencyLoggingDisabled
}

// UpdateCertificateOptions sets the CertificateTransparencyLoggingPreference for
// a certificate. Only ISSUED certificates may be updated.
func (b *InMemoryBackend) UpdateCertificateOptions(ctx context.Context, certARN, transparencyLoggingPref string) error {
	if certARN == "" {
		return fmt.Errorf("%w: CertificateArn is required", ErrInvalidParameter)
	}

	if transparencyLoggingPref == "" {
		return fmt.Errorf("%w: Options.CertificateTransparencyLoggingPreference is required", ErrInvalidParameter)
	}

	if !validTransparencyPreference(transparencyLoggingPref) {
		return fmt.Errorf(
			"%w: invalid CertificateTransparencyLoggingPreference %q",
			ErrInvalidParameter,
			transparencyLoggingPref,
		)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateCertificateOptions")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if cert.Status != statusIssued {
		return fmt.Errorf("%w: only ISSUED certificates may have options updated", ErrInvalidState)
	}

	cert.CertificateTransparencyLoggingPref = transparencyLoggingPref

	return nil
}

// ExpireCertificate transitions an ISSUED certificate to EXPIRED status.
// Returns ErrCertNotFound if no such certificate exists, ErrInvalidParameter if the
// certificate is not in ISSUED status.
func (b *InMemoryBackend) ExpireCertificate(ctx context.Context, certARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("ExpireCertificate")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if cert.Status != statusIssued {
		return fmt.Errorf("%w: only ISSUED certificates can be expired, got %s", ErrInvalidParameter, cert.Status)
	}

	cert.Status = statusExpired

	return nil
}

// InactivateCertificate transitions an ISSUED certificate to INACTIVE status.
// Returns ErrCertNotFound if no such certificate exists, ErrInvalidParameter if the
// certificate is not in ISSUED status.
func (b *InMemoryBackend) InactivateCertificate(ctx context.Context, certARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("InactivateCertificate")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if cert.Status != statusIssued {
		return fmt.Errorf("%w: only ISSUED certificates can be inactivated, got %s", ErrInvalidParameter, cert.Status)
	}

	cert.Status = statusInactive

	return nil
}

// TimeoutPendingValidation transitions a PENDING_VALIDATION certificate to VALIDATION_TIMED_OUT.
// Returns ErrCertNotFound if no such certificate exists, ErrInvalidParameter if the
// certificate is not in PENDING_VALIDATION status.
func (b *InMemoryBackend) TimeoutPendingValidation(ctx context.Context, certARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("TimeoutPendingValidation")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if cert.Status != statusPendingValidation {
		return fmt.Errorf(
			"%w: only PENDING_VALIDATION certificates can time out, got %s",
			ErrInvalidParameter, cert.Status,
		)
	}

	// Stop any pending auto-validate timer.
	timers := b.timersStore(region)
	if t, exists := timers[certARN]; exists {
		t.Stop()
		delete(timers, certARN)
	}

	cert.Status = statusValidationTimedOut

	return nil
}

// FailCertificate transitions a PENDING_VALIDATION certificate to FAILED status with
// the given failure reason.
// Returns ErrCertNotFound if no such certificate exists, ErrInvalidParameter if the
// certificate is not in PENDING_VALIDATION status.
func (b *InMemoryBackend) FailCertificate(ctx context.Context, certARN, reason string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("FailCertificate")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if cert.Status != statusPendingValidation {
		return fmt.Errorf(
			"%w: only PENDING_VALIDATION certificates can be failed, got %s",
			ErrInvalidParameter, cert.Status,
		)
	}

	// Stop any pending auto-validate timer.
	timers := b.timersStore(region)
	if t, exists := timers[certARN]; exists {
		t.Stop()
		delete(timers, certARN)
	}

	cert.Status = statusFailed
	cert.FailureReason = reason

	return nil
}
