package identity

import "golang.org/x/crypto/bcrypt"

func hashPassword(password []byte) ([]byte, error) {
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return hash, nil
}
