package elbv2

import (
	"fmt"
)

// validateCertificates checks every listener certificate's CertificateArn against
// the wired CertificateResolver. Callers must hold b.mu. A nil certResolver (the
// default) accepts every CertificateArn unvalidated.
func (b *InMemoryBackend) validateCertificates(certs []Certificate) error {
	if b.certResolver == nil {
		return nil
	}

	for _, c := range certs {
		if c.CertificateArn == "" {
			continue
		}

		if !b.certResolver.ResolveCertificate(c.CertificateArn) {
			return fmt.Errorf("%w: %s", ErrCertificateNotFound, c.CertificateArn)
		}
	}

	return nil
}

// markCertificatesInUse reports listenerArn's attachment to each certificate to the
// wired CertificateResolver. Callers must hold b.mu. A nil certResolver is a no-op.
func (b *InMemoryBackend) markCertificatesInUse(listenerArn string, certs []Certificate) {
	if b.certResolver == nil {
		return
	}

	for _, c := range certs {
		if c.CertificateArn != "" {
			b.certResolver.AddInUseBy(c.CertificateArn, listenerArn)
		}
	}
}

// unmarkCertificatesInUse reports listenerArn's detachment from each certificate to
// the wired CertificateResolver. Callers must hold b.mu. A nil certResolver is a no-op.
func (b *InMemoryBackend) unmarkCertificatesInUse(listenerArn string, certs []Certificate) {
	if b.certResolver == nil {
		return
	}

	for _, c := range certs {
		if c.CertificateArn != "" {
			b.certResolver.RemoveInUseBy(c.CertificateArn, listenerArn)
		}
	}
}

// unmarkCertificateArnsInUse is unmarkCertificatesInUse for bare ARNs, used by
// RemoveListenerCertificates which receives certArns rather than []Certificate.
// Callers must hold b.mu.
func (b *InMemoryBackend) unmarkCertificateArnsInUse(listenerArn string, certArns []string) {
	if b.certResolver == nil {
		return
	}

	for _, a := range certArns {
		if a != "" {
			b.certResolver.RemoveInUseBy(a, listenerArn)
		}
	}
}

// AddListenerCertificates adds certificates to a listener.
func (b *InMemoryBackend) AddListenerCertificates(listenerArn string, certs []Certificate) error {
	b.mu.Lock("AddListenerCertificates")
	defer b.mu.Unlock()

	listener, ok := b.listeners.Get(listenerArn)
	if !ok {
		return ErrListenerNotFound
	}

	if err := b.validateCertificates(certs); err != nil {
		return err
	}

	existing := make(map[string]bool, len(listener.Certificates))
	for _, c := range listener.Certificates {
		existing[c.CertificateArn] = true
	}

	for _, c := range certs {
		if !existing[c.CertificateArn] {
			listener.Certificates = append(listener.Certificates, c)
			existing[c.CertificateArn] = true
		}
	}

	b.markCertificatesInUse(listenerArn, certs)

	return nil
}

// DescribeListenerCertificates returns certificates on a listener.
func (b *InMemoryBackend) DescribeListenerCertificates(listenerArn string) ([]Certificate, error) {
	b.mu.RLock("DescribeListenerCertificates")
	defer b.mu.RUnlock()

	listener, ok := b.listeners.Get(listenerArn)
	if !ok {
		return nil, ErrListenerNotFound
	}

	result := make([]Certificate, len(listener.Certificates))
	copy(result, listener.Certificates)

	return result, nil
}

// RemoveListenerCertificates removes certificate ARNs from a listener.
func (b *InMemoryBackend) RemoveListenerCertificates(listenerArn string, certArns []string) error {
	b.mu.Lock("RemoveListenerCertificates")
	defer b.mu.Unlock()

	listener, ok := b.listeners.Get(listenerArn)
	if !ok {
		return ErrListenerNotFound
	}

	remove := make(map[string]bool, len(certArns))
	for _, c := range certArns {
		remove[c] = true
	}

	remaining := make([]Certificate, 0, len(listener.Certificates))
	for _, c := range listener.Certificates {
		if !remove[c.CertificateArn] {
			remaining = append(remaining, c)
		}
	}

	if len(remaining) == 0 && (listener.Protocol == protoHTTPS || listener.Protocol == protoTLS) {
		return fmt.Errorf(
			"%w: Cannot remove the last certificate from an HTTPS/TLS listener",
			ErrInvalidParameter,
		)
	}

	listener.Certificates = remaining
	b.unmarkCertificateArnsInUse(listenerArn, certArns)

	return nil
}
