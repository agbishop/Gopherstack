package aadauth

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken is returned by Parse/Validate when a token is malformed,
// expired, or fails signature verification.
var ErrInvalidToken = errors.New("aadauth: invalid token")

// Parse parses and cryptographically validates tokenString against iss's
// public key, returning its claims. Used only when a caller opts into
// signature verification (e.g. services/azurearm's --azure-arm-validate-tokens);
// by default ARM accepts any bearer token (or none) without calling this.
func (iss *Issuer) Parse(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method", ErrInvalidToken)
		}

		return &iss.privateKey.PublicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("%w: token claims are not valid", ErrInvalidToken)
	}

	return claims, nil
}

// Validate parses tokenString and additionally checks its audience and
// tenant claims match wantAudience/wantTenant.
func (iss *Issuer) Validate(tokenString, wantAudience, wantTenant string) (jwt.MapClaims, error) {
	claims, err := iss.Parse(tokenString)
	if err != nil {
		return nil, err
	}

	if aud, _ := claims["aud"].(string); wantAudience != "" && aud != wantAudience {
		return nil, fmt.Errorf("%w: audience mismatch", ErrInvalidToken)
	}

	if tid, _ := claims["tid"].(string); wantTenant != "" && tid != wantTenant {
		return nil, fmt.Errorf("%w: tenant mismatch", ErrInvalidToken)
	}

	return claims, nil
}
