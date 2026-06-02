package web

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash for storage.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}
