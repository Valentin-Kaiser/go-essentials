package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"

	"github.com/Valentin-Kaiser/go-essentials/apperror"
)

// AesCipher is a struct that provides methods for encrypting and decrypting data using AES.
// It uses the crypto/aes and crypto/cipher packages from the Go standard library.
type AesCipher struct {
	passphrase []byte // The key argument should be the AES key, either 16, 24, or 32 bytes to select AES-128, AES-192, or AES-256.
	Error      error
}

// NewAesCipher creates a new AesCipher instance with the specified key.
func NewAesCipher() *AesCipher {
	return &AesCipher{}
}

// WithAES128 sets the key size to 128 bits (16 bytes).
func (a *AesCipher) WithAES128() *AesCipher {
	var err error
	a.passphrase, err = GetRandomBytes(16)
	if err != nil {
		a.Error = apperror.Wrap(err)
	}
	return a
}

// WithAES192 sets the key size to 192 bits (24 bytes).
func (a *AesCipher) WithAES192() *AesCipher {
	var err error
	a.passphrase, err = GetRandomBytes(24)
	if err != nil {
		a.Error = apperror.Wrap(err)
	}
	return a
}

// WithAES256 sets the key size to 256 bits (32 bytes).
func (a *AesCipher) WithAES256() *AesCipher {
	var err error
	a.passphrase, err = GetRandomBytes(32)
	if err != nil {
		a.Error = apperror.Wrap(err)
	}
	return a
}

// WithPassphrase sets the key to the specified byte slice.
// The key must be either 16, 24, or 32 bytes to select AES-128, AES-192, or AES-256.
func (a *AesCipher) WithPassphrase(passphrase []byte) *AesCipher {
	a.passphrase = passphrase
	return a
}

// Encrypt uses the specified symmetric key to encrypt the input string using AES
func (a *AesCipher) Encrypt(plaintext string, out io.Writer) *AesCipher {
	if a.Error != nil {
		return a
	}

	plainBytes := []byte(plaintext)
	encrypter, err := aes.NewCipher(a.passphrase)
	if err != nil {
		a.Error = apperror.NewError("failed to create AES cipher").AddError(err)
		return a
	}

	gcm, err := cipher.NewGCM(encrypter)
	if err != nil {
		a.Error = apperror.NewError("failed to create GCM cipher").AddError(err)
		return a
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		a.Error = apperror.NewError("failed to read nonce").AddError(err)
		return a
	}

	outBytes := gcm.Seal(nonce, nonce, plainBytes, nil)
	encoded := base64.StdEncoding.EncodeToString(outBytes)
	_, err = out.Write([]byte(encoded))
	if err != nil {
		a.Error = apperror.NewError("failed to write encrypted data").AddError(err)
		return a
	}

	return a
}

// Decrypt uses the specified symmetric key to decrypt the input string using AES
func (a *AesCipher) Decrypt(ciphertext string, out io.Writer) *AesCipher {
	cipherBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		a.Error = apperror.NewError("failed to decode base64 string").AddError(err)
		return a
	}

	c, err := aes.NewCipher(a.passphrase)
	if err != nil {
		a.Error = apperror.NewError("failed to create AES cipher").AddError(err)
		return a
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		a.Error = apperror.NewError("failed to create GCM cipher").AddError(err)
		return a
	}

	nonceSize := gcm.NonceSize()
	if len(cipherBytes) < nonceSize {
		a.Error = apperror.NewError("ciphertext too short")
		return a
	}

	nonce, cipherBytes := cipherBytes[:nonceSize], cipherBytes[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherBytes, nil)
	if err != nil {
		a.Error = apperror.NewError("failed to decrypt data").AddError(err)
		return a
	}

	_, err = out.Write(plaintext)
	if err != nil {
		a.Error = apperror.NewError("failed to write decrypted data").AddError(err)
		return a
	}

	return a
}
