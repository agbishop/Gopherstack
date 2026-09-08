package acm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// certMetadata holds extracted metadata from a parsed X.509 certificate.
type certMetadata struct {
	serial  string
	subject string
	issuer  string
	// subjectCommonName/issuerCommonName are the CN RDN component alone
	// (see Certificate.SubjectCommonName's doc comment, models.go) --
	// captured directly from pkix.Name.CommonName, not derived from the
	// flattened subject/issuer strings above.
	subjectCommonName  string
	issuerCommonName   string
	signatureAlgorithm string
	keyAlgorithm       string
	keyUsage           []string
	extKeyUsage        []string
}

// deriveKeyAlgorithm maps a parsed certificate's public key to the AWS ACM
// KeyAlgorithm enum value it actually represents (types.KeyAlgorithm,
// enums.go:416-427, v1.43.4: RSA_1024/2048/3072/4096, EC_prime256v1/
// secp384r1/secp521r1) -- CertificateDetail.KeyAlgorithm is documented as
// "The algorithm that was used to generate the public-private key pair", so
// an imported certificate's real key must be reflected here, not a fixed
// default. Non-standard RSA bit lengths round up to the nearest supported
// bucket; unrecognized key types fall back to the RSA_2048 default rather
// than silently claiming an EC key.
func deriveKeyAlgorithm(pub any) string {
	switch k := pub.(type) {
	case *rsa.PublicKey:
		return rsaKeyAlgorithm(k.N.BitLen())
	case *ecdsa.PublicKey:
		switch k.Curve {
		case elliptic.P256():
			return keyAlgorithmEC
		case elliptic.P384():
			return keyAlgorithmECSecp384r1
		case elliptic.P521():
			return keyAlgorithmECSecp521r1
		default:
			return keyAlgorithmEC
		}
	default:
		return keyAlgorithmRSA2048
	}
}

// rsaKeyAlgorithm maps an RSA key's bit length to the nearest AWS
// KeyAlgorithm bucket, rounding up for non-standard sizes.
func rsaKeyAlgorithm(bits int) string {
	switch {
	case bits <= 1024: //nolint:mnd // RSA key-size buckets, not magic numbers
		return keyAlgorithmRSA1024
	case bits <= 2048: //nolint:mnd // RSA key-size bucket boundary, not a magic number
		return keyAlgorithmRSA2048
	case bits <= 3072: //nolint:mnd // RSA key-size bucket boundary, not a magic number
		return keyAlgorithmRSA3072
	default:
		return keyAlgorithmRSA4096
	}
}

// generateSelfSignedCert generates a self-signed ECDSA P-256 certificate for
// the given domain (and optional SANs) and returns PEM-encoded certificate,
// private key, extracted metadata, and the certificate's NotBefore/NotAfter validity times.
func generateSelfSignedCert(
	domainName string,
	sans []string,
	keyAlgorithm string,
) (string, string, certMetadata, time.Time, time.Time, error) {
	priv, pub, sigAlgo, err := generateKey(keyAlgorithm)
	if err != nil {
		return "", "", certMetadata{}, time.Time{}, time.Time{}, fmt.Errorf("generate key: %w", err)
	}

	serialBytes := make([]byte, 16) //nolint:mnd // 128-bit random serial
	if _, err = cryptorand.Read(serialBytes); err != nil {
		return "", "", certMetadata{}, time.Time{}, time.Time{}, fmt.Errorf("generate serial: %w", err)
	}

	serial := new(big.Int).SetBytes(serialBytes)

	dnsNames := append([]string{domainName}, sans...)

	notBefore := time.Now().UTC().Truncate(time.Second)
	notAfter := notBefore.Add(certValidityDuration)

	subjectName := pkix.Name{
		Organization:       []string{"Amazon"},
		OrganizationalUnit: []string{"Server CA 1B"},
		Country:            []string{"US"},
		CommonName:         domainName,
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      subjectName,
		DNSNames:     dnsNames,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(cryptorand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		return "", "", certMetadata{}, time.Time{}, time.Time{}, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", "", certMetadata{}, time.Time{}, time.Time{}, fmt.Errorf("marshal key: %w", err)
	}

	var keyType string
	switch priv.(type) {
	case *rsa.PrivateKey:
		keyType = "RSA PRIVATE KEY"
	case *ecdsa.PrivateKey:
		keyType = "EC PRIVATE KEY"
	}

	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: keyType, Bytes: keyDER}))

	meta := certMetadata{
		serial:             formatSerialHex(serial),
		subject:            subjectName.String(),
		issuer:             subjectName.String(), // self-signed: issuer == subject
		subjectCommonName:  subjectName.CommonName,
		issuerCommonName:   subjectName.CommonName, // self-signed: issuer == subject
		signatureAlgorithm: sigAlgo,
	}

	return certPEM, keyPEM, meta, notBefore, notAfter, nil
}

// extractCertMetadataFull parses a PEM-encoded certificate and returns the primary
// domain name (first SAN or CN), extracted metadata, NotBefore, and NotAfter.
func extractCertMetadataFull(certPEM string) (string, certMetadata, time.Time, time.Time, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return "", certMetadata{}, time.Time{}, time.Time{}, errInvalidPEM
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", certMetadata{}, time.Time{}, time.Time{}, fmt.Errorf("parse certificate: %w", err)
	}

	domainName := cert.Subject.CommonName
	if len(cert.DNSNames) > 0 {
		domainName = cert.DNSNames[0]
	}

	meta := certMetadata{
		serial:             formatSerialHex(cert.SerialNumber),
		subject:            cert.Subject.String(),
		issuer:             cert.Issuer.String(),
		subjectCommonName:  cert.Subject.CommonName,
		issuerCommonName:   cert.Issuer.CommonName,
		signatureAlgorithm: cert.SignatureAlgorithm.String(),
		keyAlgorithm:       deriveKeyAlgorithm(cert.PublicKey),
		keyUsage:           x509KeyUsageToAWS(cert.KeyUsage),
		extKeyUsage:        x509ExtKeyUsageToAWS(cert.ExtKeyUsage),
	}

	return domainName, meta, cert.NotBefore.UTC(), cert.NotAfter.UTC(), nil
}

// x509KeyUsageToAWS converts x509.KeyUsage bitmask to a slice of AWS key usage strings.
func x509KeyUsageToAWS(ku x509.KeyUsage) []string {
	mapping := []struct {
		name string
		bit  x509.KeyUsage
	}{
		{"DIGITAL_SIGNATURE", x509.KeyUsageDigitalSignature},
		{"NON_REPUDIATION", x509.KeyUsageContentCommitment},
		{"KEY_ENCIPHERMENT", x509.KeyUsageKeyEncipherment},
		{"DATA_ENCIPHERMENT", x509.KeyUsageDataEncipherment},
		{"KEY_AGREEMENT", x509.KeyUsageKeyAgreement},
		{"CERTIFICATE_SIGNING", x509.KeyUsageCertSign},
		{"CRL_SIGNING", x509.KeyUsageCRLSign},
		{"ENCIPHER_ONLY", x509.KeyUsageEncipherOnly},
		{"DECIPHER_ONLY", x509.KeyUsageDecipherOnly},
	}

	var result []string
	for _, m := range mapping {
		if ku&m.bit != 0 {
			result = append(result, m.name)
		}
	}

	return result
}

// x509ExtKeyUsageToAWS converts a slice of x509.ExtKeyUsage to AWS extended key usage strings.
func x509ExtKeyUsageToAWS(ekus []x509.ExtKeyUsage) []string {
	var result []string

	for _, eku := range ekus {
		switch eku {
		case x509.ExtKeyUsageAny:
			result = append(result, "ANY")
		case x509.ExtKeyUsageServerAuth:
			result = append(result, "TLS_WEB_SERVER_AUTHENTICATION")
		case x509.ExtKeyUsageClientAuth:
			result = append(result, "TLS_WEB_CLIENT_AUTHENTICATION")
		case x509.ExtKeyUsageCodeSigning:
			result = append(result, "CODE_SIGNING")
		case x509.ExtKeyUsageEmailProtection:
			result = append(result, "EMAIL_PROTECTION")
		case x509.ExtKeyUsageIPSECEndSystem:
			result = append(result, "IPSEC_END_SYSTEM")
		case x509.ExtKeyUsageIPSECTunnel:
			result = append(result, "IPSEC_TUNNEL")
		case x509.ExtKeyUsageIPSECUser:
			result = append(result, "IPSEC_USER")
		case x509.ExtKeyUsageTimeStamping:
			result = append(result, "TIME_STAMPING")
		case x509.ExtKeyUsageOCSPSigning:
			result = append(result, "OCSP_SIGNING")
		case x509.ExtKeyUsageMicrosoftServerGatedCrypto,
			x509.ExtKeyUsageNetscapeServerGatedCrypto,
			x509.ExtKeyUsageMicrosoftCommercialCodeSigning,
			x509.ExtKeyUsageMicrosoftKernelCodeSigning:
			result = append(result, "CUSTOM")
		}
	}

	return result
}

// buildSANList returns a deduplicated list with domainName first, followed by sans.
// Real AWS ACM always includes the primary domain as the first SubjectAlternativeName entry.
func buildSANList(domainName string, sans []string) []string {
	result := []string{domainName}
	seen := map[string]bool{domainName: true}

	for _, s := range sans {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}

// formatSerialHex formats a certificate serial number as colon-separated hex pairs,
// matching the AWS ACM serial number wire format (e.g. "1a:2b:3c:4d").
func formatSerialHex(serial *big.Int) string {
	raw := serial.Bytes()
	pairs := make([]string, len(raw))

	for i, b := range raw {
		pairs[i] = fmt.Sprintf("%02x", b)
	}

	return strings.Join(pairs, ":")
}

// encryptPrivateKeyPEM encrypts a PEM-encoded private key with the given passphrase,
// returning a PEM block with type "ENCRYPTED PRIVATE KEY".
func encryptPrivateKeyPEM(privateKeyPEM string, passphrase []byte) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", errInvalidPEM
	}

	//nolint:staticcheck // EncryptPEMBlock is deprecated but functional for ACM passphrase simulation
	encBlock, err := x509.EncryptPEMBlock(
		cryptorand.Reader,
		"ENCRYPTED PRIVATE KEY",
		block.Bytes,
		passphrase,
		x509.PEMCipherAES256,
	)
	if err != nil {
		return "", fmt.Errorf("encrypt private key: %w", err)
	}

	return string(pem.EncodeToMemory(encBlock)), nil
}

func generateKey(keyAlgorithm string) (any, any, string, error) {
	const sigAlgoSHA256WithRSA = "SHA256WITHRSA"
	const rsa2048 = 2048
	const rsa3072 = 3072
	const rsa4096 = 4096

	switch keyAlgorithm {
	case keyAlgorithmRSA1024:
		// RSA_1024 is a valid KeyAlgorithm enum value on the wire (imported
		// certificates only) but is rejected here as a client input error
		// (ValidationException/400), not surfaced as an unwrapped internal
		// error (which would previously escape handleOpError's known-error
		// switch and be reported as a 500 InternalFailure).
		return nil, nil, "", fmt.Errorf("%w: %w", ErrInvalidParameter, errWeakKey)
	case keyAlgorithmRSA2048:
		privRSA, rsaErr := rsa.GenerateKey(cryptorand.Reader, rsa2048)
		if rsaErr != nil {
			return nil, nil, "", rsaErr
		}

		return privRSA, &privRSA.PublicKey, sigAlgoSHA256WithRSA, nil
	case keyAlgorithmRSA3072:
		privRSA, rsaErr := rsa.GenerateKey(cryptorand.Reader, rsa3072)
		if rsaErr != nil {
			return nil, nil, "", rsaErr
		}

		return privRSA, &privRSA.PublicKey, sigAlgoSHA256WithRSA, nil
	case keyAlgorithmRSA4096:
		privRSA, rsaErr := rsa.GenerateKey(cryptorand.Reader, rsa4096)
		if rsaErr != nil {
			return nil, nil, "", rsaErr
		}

		return privRSA, &privRSA.PublicKey, sigAlgoSHA256WithRSA, nil
	case keyAlgorithmECSecp384r1:
		privEC, ecErr := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
		if ecErr != nil {
			return nil, nil, "", ecErr
		}

		return privEC, &privEC.PublicKey, "SHA384WITHECDSA", nil
	case keyAlgorithmEC, "":
		privEC, ecErr := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
		if ecErr != nil {
			return nil, nil, "", ecErr
		}

		return privEC, &privEC.PublicKey, "SHA256WITHECDSA", nil
	default:
		return nil, nil, "", fmt.Errorf("%w: unsupported key algorithm %s", ErrInvalidParameter, keyAlgorithm)
	}
}
