package auth

import (
	"crypto"
	"errors"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"time"
)

type JwtClaims struct {
	jwt.RegisteredClaims
}

type Jwt struct {
	Signed    string
	ExpiresAt int64
}

type JwtIssuer struct {
	issuer     string
	method     jwt.SigningMethod
	timeToLive time.Duration
	privateKey crypto.PrivateKey
}

func NewJwtIssuer(issuer string, method jwt.SigningMethod, ttl time.Duration, key crypto.PrivateKey) *JwtIssuer {
	return &JwtIssuer{
		issuer:     issuer,
		method:     method,
		timeToLive: ttl,
		privateKey: key,
	}
}

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

type JwtValidator struct {
	method    jwt.SigningMethod
	publicKey crypto.PublicKey
}

func NewJwtValidator(method jwt.SigningMethod, key crypto.PublicKey) *JwtValidator {
	return &JwtValidator{publicKey: key, method: method}
}

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
