package auth

import (
	"errors"
	"github.com/google/uuid"
	"time"
)

var ErrInvalidFingerprint = errors.New("invalid fingerprint for refresh token provided")
var ErrRefreshTokenExpired = errors.New("refresh token expired")

type RefreshTokenIssuer struct {
	maxCount          int
	timeToLiveSeconds int
}

func NewRefreshTokenIssuer(maxCount int, ttl time.Duration) *RefreshTokenIssuer {
	return &RefreshTokenIssuer{
		maxCount:          maxCount,
		timeToLiveSeconds: int(ttl.Seconds()),
	}
}

func (r *RefreshTokenIssuer) Sign(userId string, fingerprint string, at time.Time) RefreshToken {
	return RefreshToken{
		Id:          uuid.NewString(),
		UserId:      userId,
		Fingerprint: fingerprint,
		ExpiresIn:   r.timeToLiveSeconds,
		CreatedAt:   at,
	}
}

func (r *RefreshTokenIssuer) TokensMaxCount() int {
	return r.maxCount
}

type RefreshToken struct {
	Id          string
	UserId      string
	Fingerprint string
	ExpiresIn   int
	CreatedAt   time.Time
}

func (r RefreshToken) Verify(fingerprint string, now time.Time) error {
	if r.Fingerprint != fingerprint {
		return ErrInvalidFingerprint
	}

	if r.CreatedAt.Add(time.Duration(r.ExpiresIn) * time.Second).Before(now) {
		return ErrRefreshTokenExpired
	}
	return nil
}
