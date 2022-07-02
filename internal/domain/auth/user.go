package auth

import (
	"errors"
	"golang.org/x/crypto/bcrypt"
)

var ErrWrongPassword = errors.New("password is incorrect")

type User struct {
	Id           string `json:"id"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
}

func (u User) VerifyPassword(password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return ErrWrongPassword
	}
	return nil
}

func GeneratePasswordHash(pass string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
