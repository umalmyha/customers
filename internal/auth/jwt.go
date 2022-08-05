package auth

import (
	"crypto"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

// JwtClaims represents JWT claims
type JwtClaims struct {
	jwt.RegisteredClaims
}

// Jwt represents signed jwt and unix expires at
type Jwt struct {
	Signed    string
	ExpiresAt int64
}

// JwtIssuer issues jwt according to config
type JwtIssuer struct {
	issuer     string
	method     jwt.SigningMethod
	timeToLive time.Duration
	privateKey crypto.PrivateKey
}

// NewJwtIssuer builds JwtIssuer
func NewJwtIssuer(issuer string, method jwt.SigningMethod, ttl time.Duration, key crypto.PrivateKey) *JwtIssuer {
	return &JwtIssuer{
		issuer:     issuer,
		method:     method,
		timeToLive: ttl,
		privateKey: key,
	}
}

// Sign issues new jwt
func (j *JwtIssuer) Sign(subj string, issuedAt time.Time) (*Jwt, error) {
	expiresAt := issuedAt.Add(j.timeToLive)

	claims := JwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			Issuer:    j.issuer,
			Subject:   subj,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
		},
	}

	token := jwt.NewWithClaims(j.method, claims)

	signed, err := token.SignedString(j.privateKey)
	if err != nil {
		return nil, err
	}

	return &Jwt{Signed: signed, ExpiresAt: expiresAt.Unix()}, nil
}

// JwtValidator verifies jwt according to config
type JwtValidator struct {
	method    jwt.SigningMethod
	publicKey crypto.PublicKey
}

// NewJwtValidator builds new JwtValidator
func NewJwtValidator(method jwt.SigningMethod, key crypto.PublicKey) *JwtValidator {
	return &JwtValidator{publicKey: key, method: method}
}

// Verify checks if jwt valid
func (j *JwtValidator) Verify(rawToken string) (JwtClaims, error) {
	var claims JwtClaims
	if _, err := jwt.ParseWithClaims(rawToken, &claims, j.keyFunc); err != nil {
		return JwtClaims{}, err
	}
	return claims, nil
}

func (j *JwtValidator) keyFunc(token *jwt.Token) (any, error) {
	if token.Method.Alg() != j.method.Alg() {
		return nil, errors.New("failed to verify signing algorithm")
	}
	return j.publicKey, nil
}
