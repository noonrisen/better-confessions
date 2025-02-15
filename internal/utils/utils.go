package utils

import (
	"crypto/rand"
	"log"
)

func GenerateRandomSalt() []byte {
	salt := make([]byte, 16)
	_, err := rand.Read(salt)
	if err != nil {
		log.Fatalf("Failed to generate salt: %v\n", err)
	}
	return salt
}
