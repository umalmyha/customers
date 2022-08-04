package auth

import "golang.org/x/crypto/bcrypt"

// GeneratePasswordHash creates hash based on provided password
func GeneratePasswordHash(pass string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword verifies that hash is equal to the one which will be produced by password
func VerifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
