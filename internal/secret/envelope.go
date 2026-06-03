package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const envelopePrefix = "enc:v1:"

func SealString(keyMaterial, plaintext string) (string, error) {
	if strings.TrimSpace(plaintext) == "" {
		return "", nil
	}
	keyMaterial = strings.TrimSpace(keyMaterial)
	if keyMaterial == "" {
		return "", errors.New("secret key is required")
	}

	aead, err := aeadFromKeyMaterial(keyMaterial)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nil, nonce, []byte(plaintext), nil)
	out := append(nonce, sealed...)
	return envelopePrefix + base64.RawURLEncoding.EncodeToString(out), nil
}

func OpenString(keyMaterial, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, envelopePrefix) {
		return value, nil
	}
	keyMaterial = strings.TrimSpace(keyMaterial)
	if keyMaterial == "" {
		return "", errors.New("secret key is required")
	}

	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, envelopePrefix))
	if err != nil {
		return "", err
	}
	aead, err := aeadFromKeyMaterial(keyMaterial)
	if err != nil {
		return "", err
	}
	if len(raw) < aead.NonceSize() {
		return "", errors.New("encrypted secret is invalid")
	}
	nonce := raw[:aead.NonceSize()]
	ciphertext := raw[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func aeadFromKeyMaterial(keyMaterial string) (cipher.AEAD, error) {
	sum := sha256.Sum256([]byte(keyMaterial))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
