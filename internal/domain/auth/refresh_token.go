package auth

import (
	"errors"
	"time"
)

var ErrRefreshTokenExpired = errors.New("refresh token expired")
var ErrInvalidRefreshToken = errors.New("invalid refresh token provided")

type RefreshTokenOptions struct {
	maxCount   int
	ttlSeconds int
}

func NewRefreshTokenOptions(maxCount int, ttl time.Duration) RefreshTokenOptions {
	return RefreshTokenOptions{
		maxCount:   maxCount,
		ttlSeconds: int(ttl.Seconds()),
	}
}

func (r *RefreshTokenOptions) MaxCount() int {
	return r.maxCount
}

func (r *RefreshTokenOptions) TimeToLive() int {
	return r.ttlSeconds
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
		return ErrInvalidRefreshToken
	}

	if r.CreatedAt.Add(time.Duration(r.ExpiresIn) * time.Second).Before(now) {
		return ErrRefreshTokenExpired
	}
	return nil
}
