package config

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/golang-jwt/jwt/v4"
)

const jwtSigningAlgorithmEd25519 = "EdDSA"

// JwtCfg contains config for jwt
type JwtCfg struct {
	SigningMethod jwt.SigningMethod
	Issuer        string             `env:"AUTH_JWT_ISSUER" envDefault:"customers-api"`
	TimeToLive    time.Duration      `env:"AUTH_JWT_TIME_TO_LIVE" envDefault:"10m"`
	PrivateKey    ed25519.PrivateKey `env:"AUTH_JWT_PRIVATE_KEY_FILE"`
	PublicKey     ed25519.PublicKey  `env:"AUTH_JWT_PUBLIC_KEY_FILE"`
}

// RefreshTokenCfg contains config for refresh token
type RefreshTokenCfg struct {
	MaxCount   int           `env:"AUTH_REFRESH_TOKEN_MAX_COUNT" envDefault:"5"`
	TimeToLive time.Duration `env:"AUTH_REFRESH_TOKEN_TIME_TO_LIVE" envDefault:"720h"`
}

// RedisCfg contains config for redis
type RedisCfg struct {
	Addr       string `env:"REDIS_ADDR"`
	Password   string `env:"REDIS_PASSWORD"`
	DB         int    `env:"REDIS_DB" envDefault:"0"`
	MaxRetries int    `env:"REDIS_MAX_RETRIES" envDefault:"3"`
	PoolSize   int    `env:"REDIS_POOL_SIZE" envDefault:"50"`
}

// Config contains necessary application configuration
type Config struct {
	PostgresConnString string `env:"POSTGRES_URL"`
	MongoConnString    string `env:"MONGO_URL"`
	RedisCfg           RedisCfg
	JwtCfg             JwtCfg
	RefreshTokenCfg    RefreshTokenCfg
}

// Build constructs new Config based on environment variables
func Build() (Config, error) {
	var cfg Config
	cfg.JwtCfg.SigningMethod = jwt.GetSigningMethod(jwtSigningAlgorithmEd25519)

	opts := env.Options{RequiredIfNoDef: true}
	parsers := map[reflect.Type]env.ParserFunc{
		reflect.TypeOf(cfg.JwtCfg.PrivateKey): privateKeyFromFileParser,
		reflect.TypeOf(cfg.JwtCfg.PublicKey):  publicKeyFromFileParser,
	}

	if err := env.ParseWithFuncs(&cfg, parsers, opts); err != nil {
		return cfg, fmt.Errorf("failed to parse environment variables - %w", err)
	}

	return cfg, nil
}

func privateKeyFromFileParser(v string) (any, error) {
	path := filepath.Clean(v)

	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file for auth - %w", err)
	}

	privateKey, err := jwt.ParseEdPrivateKeyFromPEM(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key for auth - %w", err)
	}
	return privateKey, nil
}

func publicKeyFromFileParser(v string) (any, error) {
	path := filepath.Clean(v)

	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file for auth - %w", err)
	}

	publicKey, err := jwt.ParseEdPublicKeyFromPEM(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key for auth - %w", err)
	}
	return publicKey, nil
}
