package config

import (
	"crypto"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"io/ioutil"
	"os"
	"strconv"
	"time"
)

const jwtSigningAlgorithmEd25519 = "EdDSA"
const defaultRefreshTokenMaxCount = 5

type MongoCfg struct {
	User        string
	Password    string
	MaxPoolSize int
}

type PostgresCfg struct {
	User        string
	Password    string
	Database    string
	SslMode     string
	PoolMaxConn int
}

type JwtCfg struct {
	Issuer        string
	SigningMethod jwt.SigningMethod
	TimeToLive    time.Duration
	PrivateKey    crypto.PrivateKey
	PublicKey     crypto.PublicKey
}

type RefreshTokenCfg struct {
	MaxCount   int
	TimeToLive time.Duration
}

type AuthCfg struct {
	JwtCfg          JwtCfg
	RefreshTokenCfg RefreshTokenCfg
}

type Config struct {
	MongoCfg    MongoCfg
	PostgresCfg PostgresCfg
	AuthCfg     AuthCfg
}

func Build() (Config, error) {
	// TODO: will be shorten on step 6 - env package

	// mongodb env variables
	mongoUser := os.Getenv("MONGO_USER")
	mongoPassword := os.Getenv("MONGO_PASSWORD")
	mongoMaxPoolSize, err := strconv.Atoi(os.Getenv("MONGO_MAX_POOL_SIZE"))
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse mongo max pool size varaible - %w", err)
	}

	// postgresql env variables
	postgresUser := os.Getenv("POSTGRES_USER")
	postgresPassword := os.Getenv("POSTGRES_PASSWORD")
	postgresDb := os.Getenv("POSTGRES_DB")
	postgresSslMode := os.Getenv("POSTGRES_SLL_MODE")
	postgresPoolMaxConn, err := strconv.Atoi(os.Getenv("POSTGRES_POOL_MAX_CONN"))
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse postgresql max pool connections varaible - %w", err)
	}

	// JWT env variables
	jwtIssuer := os.Getenv("AUTH_JWT_ISSUER")

	jwtTimeToLive, err := time.ParseDuration(os.Getenv("AUTH_JWT_TIME_TO_LIVE"))
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse JWT time to live - %w", err)
	}

	jwtPrivateKeyFile := os.Getenv("AUTH_JWT_PRIVATE_KEY_FILE")
	jwtPrivateKeyBytes, err := ioutil.ReadFile(jwtPrivateKeyFile)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read private key file for jwt - %w", err)
	}

	jwtPrivateKey, err := jwt.ParseEdPrivateKeyFromPEM(jwtPrivateKeyBytes)
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse private key for jwt - %w", err)
	}

	jwtPublicKeyFile := os.Getenv("AUTH_JWT_PUBLIC_KEY_FILE")
	jwtPublicKeyBytes, err := ioutil.ReadFile(jwtPublicKeyFile)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read public key file for jwt - %w", err)
	}

	jwtPublicKey, err := jwt.ParseEdPublicKeyFromPEM(jwtPublicKeyBytes)
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse public key for jwt - %w", err)
	}

	jwtSigningMethod := jwt.GetSigningMethod(jwtSigningAlgorithmEd25519)

	// refresh token env variables
	maxRfrTokensCount := defaultRefreshTokenMaxCount
	if os.Getenv("AUTH_REFRESH_TOKEN_MAX_COUNT") != "" {
		maxRfrTokensCount, err = strconv.Atoi(os.Getenv("AUTH_REFRESH_TOKEN_MAX_COUNT"))
		if err != nil {
			return Config{}, fmt.Errorf("failed to parse max refresh tokens count - %w", err)
		}
	}

	rfrTokenTimeToLive, err := time.ParseDuration(os.Getenv("AUTH_REFRESH_TOKEN_TIME_TO_LIVE"))
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse refresh token time to live - %w", err)
	}

	return Config{
		MongoCfg: MongoCfg{
			User:        mongoUser,
			Password:    mongoPassword,
			MaxPoolSize: mongoMaxPoolSize,
		},
		PostgresCfg: PostgresCfg{
			User:        postgresUser,
			Password:    postgresPassword,
			Database:    postgresDb,
			SslMode:     postgresSslMode,
			PoolMaxConn: postgresPoolMaxConn,
		},
		AuthCfg: AuthCfg{
			JwtCfg: JwtCfg{
				Issuer:        jwtIssuer,
				SigningMethod: jwtSigningMethod,
				TimeToLive:    jwtTimeToLive,
				PrivateKey:    jwtPrivateKey,
				PublicKey:     jwtPublicKey,
			},
			RefreshTokenCfg: RefreshTokenCfg{
				MaxCount:   maxRfrTokensCount,
				TimeToLive: rfrTokenTimeToLive,
			},
		},
	}, nil
}
