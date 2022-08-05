package model

import "time"

// RefreshToken is refresh token model entity
type RefreshToken struct {
	ID          string
	UserID      string
	Fingerprint string
	ExpiresIn   int
	CreatedAt   time.Time
}
