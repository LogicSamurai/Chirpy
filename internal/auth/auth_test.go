package auth

import (
	"testing"
)

func TestPasswordHashing(t *testing.T) {
	password := "Guru"
	hash, err := HashPassword(password)
	if err != nil {
		t.Errorf("Error occurred while comparing password and hash: %v", err)
	}
	
	if ok, err := CheckPasswordHash(password, hash) ; !ok {
		t.Errorf("Password not same: %v", err)
	}
}