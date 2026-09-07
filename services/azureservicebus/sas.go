package azureservicebus

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultNamespace is gopherstack's fixed, Azurite-style default Service Bus
// namespace name. Real Service Bus namespaces are addressed as
// "<namespace>.servicebus.windows.net"; gopherstack has no DNS story for
// that, so this constant exists purely so a connection string built from it
// (Endpoint=sb://<DefaultNamespace>.servicebus.windows.net/;SharedAccessKeyName=...)
// has a plausible, stable shape for documentation/examples. The actual
// listener is addressed by host:port (see settings.go's DefaultPort), not by
// this name.
const DefaultNamespace = "sbemulatorns"

// DefaultKeyName is gopherstack's fixed root shared-access-policy key name,
// mirroring the real Service Bus default policy name created on every new
// namespace.
const DefaultKeyName = "RootManageSharedAccessKey"

// DefaultKeyValue is gopherstack's fixed, publicly published development SAS
// key (base64-encoded), analogous to pkgs/azureauth.DefaultAccountKey /
// Azurite's devstoreaccount1 key: a fixed dev secret so azure-sdk-for-go's
// default connection-string shape works unmodified out of the box, and so
// WithSASValidation has something deterministic to check against by default.
const DefaultKeyValue = "2R1W2VORtFi9HrmRQ1Gxp7xbySq7W0FAs2BvTZDdXeo="

// Authorization holds the structurally-parsed fields of a Service Bus
// "Authorization: SharedAccessSignature sr=<resource>&sig=<hmac>&se=<expiry>&skn=<keyname>"
// header. Parsing never fails on a bad/missing signature -- only on a header
// that isn't shaped like a SAS token at all -- mirroring pkgs/azureauth's
// "structural parse always succeeds, verification is opt-in" philosophy (see
// services/s3's PresignSecret and Blob/Queue/Table's WithSharedKeyValidation).
type Authorization struct {
	Resource  string // sr: URL-encoded resource URI the token authorizes
	Signature string // sig: base64 HMAC-SHA256
	KeyName   string // skn: shared-access-policy key name
	Expiry    int64  // se: unix seconds the token expires at
}

// ssasPrefix is the SAS Authorization scheme prefix.
const sasPrefix = "SharedAccessSignature "

// ParseSASAuthorization structurally parses a Service Bus SAS Authorization
// header value. It always extracts whatever key=value pairs are present;
// ok is false only when the header is missing the scheme prefix entirely.
func ParseSASAuthorization(header string) (Authorization, bool) {
	if !strings.HasPrefix(header, sasPrefix) {
		return Authorization{}, false
	}

	raw := strings.TrimPrefix(header, sasPrefix)

	var auth Authorization

	for pair := range strings.SplitSeq(raw, "&") {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}

		switch key {
		case "sr":
			if decoded, err := url.QueryUnescape(value); err == nil {
				auth.Resource = decoded
			} else {
				auth.Resource = value
			}
		case "sig":
			if decoded, err := url.QueryUnescape(value); err == nil {
				auth.Signature = decoded
			} else {
				auth.Signature = value
			}
		case "skn":
			auth.KeyName = value
		case "se":
			if expiry, err := strconv.ParseInt(value, 10, 64); err == nil {
				auth.Expiry = expiry
			}
		}
	}

	return auth, true
}

// SignSAS computes the Service Bus SAS signature for resource+expiry using
// keyValue (base64-encoded), matching the real algorithm: HMAC-SHA256 over
// "<url-encoded resource>\n<expiry>", base64-encoded.
func SignSAS(resource string, expiry int64, keyValue string) string {
	stringToSign := url.QueryEscape(resource) + "\n" + strconv.FormatInt(expiry, 10)

	mac := hmac.New(sha256.New, []byte(keyValue))
	mac.Write([]byte(stringToSign))

	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// VerifySAS cryptographically verifies auth against keyValue, additionally
// rejecting an expired token (se in the past relative to now). Callers only
// invoke this when opting into validation (see Handler.checkAuth /
// WithSASValidation) -- structural parsing always accepts the header
// regardless of what this returns.
func VerifySAS(auth Authorization, keyValue string, now time.Time) bool {
	if auth.Expiry != 0 && now.Unix() > auth.Expiry {
		return false
	}

	expected := SignSAS(auth.Resource, auth.Expiry, keyValue)

	return hmac.Equal([]byte(expected), []byte(auth.Signature))
}

// checkAuth structurally parses the Authorization header (recording the key
// name/resource scope for future use) and, when h.ValidateSAS is set,
// cryptographically verifies the signature -- mirroring services/s3's
// PresignSecret opt-in pattern. An absent header, a malformed header, or (when
// validation is off) a wrong signature are all accepted: this milestone's
// auth stance is permissive by default, matching every other Azure service in
// this repo. Returns false only when validation is enabled and the signature
// fails, in which case the caller rejects the request.
func (h *Handler) checkAuth(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if header == "" {
		return true // anonymous; accepted by design at this milestone
	}

	auth, ok := ParseSASAuthorization(header)
	if !ok {
		return true // structurally malformed; still accepted
	}

	if !h.ValidateSAS {
		return true
	}

	key := h.SASKeyValue
	if key == "" {
		key = DefaultKeyValue
	}

	return VerifySAS(auth, key, time.Now())
}
