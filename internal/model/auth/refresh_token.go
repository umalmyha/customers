package auth

import (
	"errors"
	"time"
)

type RefreshToken struct {
	Id          string
	UserId      string
	Fingerprint string
	ExpiresIn   int
	CreatedAt   time.Time
}

func (r *RefreshToken) Verify(fingerprint string, now time.Time) error {
	if r.Fingerprint != fingerprint {
		return errors.New("invalid refresh token provided")
	}

	if r.CreatedAt.Add(time.Duration(r.ExpiresIn) * time.Second).Before(now) {
		return errors.New("refresh token already expired")
	}
	return nil
}
