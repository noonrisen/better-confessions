package utils

import (
	"crypto/rand"
	"errors"
	"log"
	"net/url"
	"strings"
)

func GenerateRandomSalt() []byte {
	salt := make([]byte, 16)
	_, err := rand.Read(salt)
	if err != nil {
		log.Fatalf("Failed to generate salt: %v\n", err)
	}
	return salt
}

func Url(str string) (*string, error) {
	u, err := url.Parse(strings.TrimSpace(str))
	if err != nil {
		return nil, err
	}

	if u.Scheme == "" || u.Host == "" {
		return nil, errors.New("scheme or host empty")
	}

	retStr := u.String()
	return &retStr, nil
}
