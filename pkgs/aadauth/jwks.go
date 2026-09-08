package aadauth

import (
	"encoding/base64"
	"math/big"
)

// JWKSResponse is the JSON Web Key Set document shape served from
// /{tenant}/discovery/v2.0/keys.
type JWKSResponse struct {
	Keys []JWK `json:"keys,omitempty"`
}

// JWK represents a single JSON Web Key.
type JWK struct {
	Kty string `json:"kty,omitempty"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
	Kid string `json:"kid,omitempty"`
	Use string `json:"use,omitempty"`
	Alg string `json:"alg,omitempty"`
}

// JWKS returns the JSON Web Key Set for this issuer.
func (iss *Issuer) JWKS() JWKSResponse {
	pub := &iss.privateKey.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	e := base64.RawURLEncoding.EncodeToString(eBytes)

	return JWKSResponse{
		Keys: []JWK{{
			Kty: "RSA",
			N:   n,
			E:   e,
			Kid: iss.keyID,
			Use: "sig",
			Alg: "RS256",
		}},
	}
}
