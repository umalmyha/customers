package infra

import (
	"crypto"
	"github.com/golang-jwt/jwt/v4"
	"io/ioutil"
	"os"
	"strconv"
	"time"
)

const AlgorithmEd25519 = "EdDSA"
const DefaultRefreshTokenCookieName = "refresh-token"
const DefaultRefreshTokenMaxCount = 5

type jwtCfg struct {
	Issuer        string
	SigningMethod jwt.SigningMethod
	TimeToLive    time.Duration
	PrivateKey    crypto.PrivateKey
	PublicKey     crypto.PublicKey
}

type refreshTokenCfg struct {
	CookieName string
	MaxCount   int
	TimeToLive time.Duration
}

type AuthCfg struct {
	Https           bool
	JwtCfg          jwtCfg
	RefreshTokenCfg refreshTokenCfg
}

func AuthConfig() (AuthCfg, error) {
	var cfg AuthCfg

	if os.Getenv("AUTH_HTTPS") == "true" {
		cfg.Https = true
	}

	jwtCfg, err := buildJwtConfig()
	if err != nil {
		return AuthCfg{}, err
	}
	cfg.JwtCfg = jwtCfg

	rfrCfg, err := buildRefreshTokenConfig()
	if err != nil {
		return AuthCfg{}, err
	}
	cfg.RefreshTokenCfg = rfrCfg

	return cfg, nil
}

func buildJwtConfig() (jwtCfg, error) {
	cfg := jwtCfg{
		Issuer:        os.Getenv("AUTH_JWT_ISSUER"),
		SigningMethod: jwt.GetSigningMethod(AlgorithmEd25519),
	}

	ttl, err := time.ParseDuration(os.Getenv("AUTH_JWT_TIME_TO_LIVE"))
	if err != nil {
		return jwtCfg{}, err
	}
	cfg.TimeToLive = ttl

	privKeyFile := os.Getenv("AUTH_JWT_PRIVATE_KEY_FILE")
	privKeyBytes, err := ioutil.ReadFile(privKeyFile)
	if err != nil {
		return jwtCfg{}, err
	}

	privateKey, err := jwt.ParseEdPrivateKeyFromPEM(privKeyBytes)
	if err != nil {
		return jwtCfg{}, err
	}
	cfg.PrivateKey = privateKey

	pubKeyFile := os.Getenv("AUTH_JWT_PUBLIC_KEY_FILE")
	pubKeyBytes, err := ioutil.ReadFile(pubKeyFile)
	if err != nil {
		return jwtCfg{}, err
	}

	publicKey, err := jwt.ParseEdPublicKeyFromPEM(pubKeyBytes)
	if err != nil {
		return jwtCfg{}, err
	}
	cfg.PublicKey = publicKey

	return cfg, nil
}

func buildRefreshTokenConfig() (refreshTokenCfg, error) {
	var cfg refreshTokenCfg

	cookieName := os.Getenv("AUTH_REFRESH_TOKEN_COOKIE_NAME")
	if cookieName == "" {
		cookieName = DefaultRefreshTokenCookieName
	}
	cfg.CookieName = cookieName

	countStr := os.Getenv("AUTH_REFRESH_TOKEN_MAX_COUNT")
	if countStr == "" {
		cfg.MaxCount = DefaultRefreshTokenMaxCount
	} else {
		maxCount, err := strconv.Atoi(countStr)
		if err != nil {
			return refreshTokenCfg{}, err
		}
		cfg.MaxCount = maxCount
	}

	ttl, err := time.ParseDuration(os.Getenv("AUTH_REFRESH_TOKEN_TIME_TO_LIVE"))
	if err != nil {
		return refreshTokenCfg{}, err
	}
	cfg.TimeToLive = ttl

	return cfg, nil
}
