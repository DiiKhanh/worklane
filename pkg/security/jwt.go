package security

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the decoded, transport-free view the app layer consumes.
type Claims struct {
	UserID   string
	TenantID string
	Email    string
}

// jwtClaims is the on-the-wire shape (custom fields + standard registered claims).
type jwtClaims struct {
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

// Issuer signs tokens with an Ed25519 private key. Only auth-svc holds one.
type Issuer struct {
	key ed25519.PrivateKey
	ttl time.Duration
}

// NewIssuer parses a PKCS#8 PEM private key.
func NewIssuer(privPEM string, ttl time.Duration) (*Issuer, error) {
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		return nil, errors.New("security: invalid private key PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("security: parse private key: %w", err)
	}
	priv, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("security: private key is not ed25519")
	}
	return &Issuer{key: priv, ttl: ttl}, nil
}

// Issue returns a signed token and its expiry.
func (i *Issuer) Issue(userID, tenantID, email string, now time.Time) (string, time.Time, error) {
	exp := now.Add(i.ttl)
	claims := jwtClaims{
		TenantID: tenantID,
		Email:    email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(i.key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("security: sign token: %w", err)
	}
	return tok, exp, nil
}

// Verifier checks signatures with an Ed25519 public key. Resource services hold only this.
type Verifier struct{ key ed25519.PublicKey }

// NewVerifier parses a PKIX PEM public key.
func NewVerifier(pubPEM string) (*Verifier, error) {
	block, _ := pem.Decode([]byte(pubPEM))
	if block == nil {
		return nil, errors.New("security: invalid public key PEM")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("security: parse public key: %w", err)
	}
	pub, ok := k.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("security: public key is not ed25519")
	}
	return &Verifier{key: pub}, nil
}

// Parse validates the signature + expiry and returns the claims. It pins the algorithm to
// EdDSA so an attacker cannot swap in "alg: none" or a weaker method.
func (v *Verifier) Parse(token string) (Claims, error) {
	var c jwtClaims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("security: unexpected signing method %v", t.Header["alg"])
		}
		return v.key, nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("security: parse token: %w", err)
	}
	return Claims{UserID: c.Subject, TenantID: c.TenantID, Email: c.Email}, nil
}
