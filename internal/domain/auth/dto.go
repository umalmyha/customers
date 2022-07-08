package auth

import (
	"errors"
	"time"
)

type Signup struct {
	Email           string
	Password        string
	ConfirmPassword string
}

func (s Signup) ValidatePasswords() error {
	if s.Password != s.ConfirmPassword {
		return errors.New("passwords don't match")
	}
	return nil
}

type Login struct {
	Email       string
	Password    string
	Fingerprint string
	At          time.Time
}

type Refresh struct {
	Token       string
	Fingerprint string
	At          time.Time
}
