package config

import (
	"crypto"
	"fmt"
	"github.com/caarlos0/env/v6"
	"github.com/golang-jwt/jwt/v4"
	"io/ioutil"
	"os"
	"time"
)

const jwtSigningAlgorithmEd25519 = "EdDSA"

type JwtCfg struct {
	Issuer        string        `env:"AUTH_JWT_ISSUER" envDefault:"customers-api"`
	TimeToLive    time.Duration `env:"AUTH_JWT_TIME_TO_LIVE" envDefault:"10m"`
	SigningMethod jwt.SigningMethod
	PrivateKey    crypto.PrivateKey
	PublicKey     crypto.PublicKey
}

type RefreshTokenCfg struct {
	MaxCount   int           `env:"AUTH_REFRESH_TOKEN_MAX_COUNT" envDefault:"5"`
	TimeToLive time.Duration `env:"AUTH_REFRESH_TOKEN_TIME_TO_LIVE" envDefault:"720h"`
}

type AuthCfg struct {
	JwtCfg          JwtCfg
	RefreshTokenCfg RefreshTokenCfg
}

type RedisCfg struct {
	Addr       string `env:"REDIS_ADDR"`
	Password   string `env:"REDIS_PASSWORD"`
	Db         int    `env:"REDIS_DB" envDefault:"0"`
	MaxRetries int    `env:"REDIS_MAX_RETRIES" envDefault:"3"`
	PoolSize   int    `env:"REDIS_POOL_SIZE" envDefault:"50"`
}

type Config struct {
	PostgresConnString string `env:"POSTGRES_URL"`
	MongoConnString    string `env:"MONGO_URL"`
	RedisCfg           RedisCfg
	AuthCfg            AuthCfg
}

func Build() (Config, error) {
	var cfg Config
	opts := env.Options{RequiredIfNoDef: true}

	if err := env.Parse(&cfg, opts); err != nil {
		return cfg, fmt.Errorf("failed to parse environment variables - %w", err)
	}

	cfg.AuthCfg.JwtCfg.SigningMethod = jwt.GetSigningMethod(jwtSigningAlgorithmEd25519)

	jwtPrivateKeyFile := os.Getenv("AUTH_JWT_PRIVATE_KEY_FILE")
	jwtPrivateKeyBytes, err := ioutil.ReadFile(jwtPrivateKeyFile)
	if err != nil {
		return cfg, fmt.Errorf("failed to read private key file for jwt - %w", err)
	}

	jwtPrivateKey, err := jwt.ParseEdPrivateKeyFromPEM(jwtPrivateKeyBytes)
	if err != nil {
		return cfg, fmt.Errorf("failed to parse private key for jwt - %w", err)
	}
	cfg.AuthCfg.JwtCfg.PrivateKey = jwtPrivateKey

	jwtPublicKeyFile := os.Getenv("AUTH_JWT_PUBLIC_KEY_FILE")
	jwtPublicKeyBytes, err := ioutil.ReadFile(jwtPublicKeyFile)
	if err != nil {
		return cfg, fmt.Errorf("failed to read public key file for jwt - %w", err)
	}

	jwtPublicKey, err := jwt.ParseEdPublicKeyFromPEM(jwtPublicKeyBytes)
	if err != nil {
		return cfg, fmt.Errorf("failed to parse public key for jwt - %w", err)
	}
	cfg.AuthCfg.JwtCfg.PublicKey = jwtPublicKey

	return cfg, nil
}
