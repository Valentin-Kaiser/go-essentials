package security_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Valentin-Kaiser/go-core/security"
)

func TestAesCipherEncryptDecrypt(t *testing.T) {
	testCases := []struct {
		name      string
		plaintext string
		keySize   int
	}{
		{"AES128 short", "hello", 16},
		{"AES192 short", "hello", 24},
		{"AES256 short", "hello", 32},
		{"AES128 long", "This is a longer text that should be encrypted and decrypted correctly", 16},
		{"AES256 long", "This is a longer text that should be encrypted and decrypted correctly", 32},
		{"AES128 empty", "", 16},
		{"AES256 unicode", "Hello, 世界! 🌍", 32},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate a key of the specified size
			key := make([]byte, tc.keySize)
			copy(key, "testkeyfortesting123456789012345")

			cipher := security.NewAesCipher().WithPassphrase(key)

			// Encrypt
			var encrypted bytes.Buffer
			cipher.Encrypt(tc.plaintext, &encrypted)

			if cipher.Error != nil {
				t.Errorf("Encrypt() returned error: %v", cipher.Error)
				return
			}

			encryptedStr := encrypted.String()
			if len(encryptedStr) == 0 && len(tc.plaintext) > 0 {
				t.Error("Encrypt() returned empty string for non-empty input")
				return
			}

			// Decrypt
			cipher2 := security.NewAesCipher().WithPassphrase(key)
			var decrypted bytes.Buffer
			cipher2.Decrypt(encryptedStr, &decrypted)

			if cipher2.Error != nil {
				t.Errorf("Decrypt() returned error: %v", cipher2.Error)
				return
			}

			decryptedStr := decrypted.String()
			if decryptedStr != tc.plaintext {
				t.Errorf("Decrypt() returned %q, expected %q", decryptedStr, tc.plaintext)
			}
		})
	}
}

func TestAesCipherEncryptWithoutPassphrase(t *testing.T) {
	cipher := security.NewAesCipher()
	var output bytes.Buffer
	cipher.Encrypt("test", &output)

	if cipher.Error == nil {
		t.Error("Encrypt() without passphrase should return error")
	}
}

func TestAesCipherDecryptWithoutPassphrase(t *testing.T) {
	cipher := security.NewAesCipher()
	var output bytes.Buffer
	cipher.Decrypt("test", &output)

	if cipher.Error == nil {
		t.Error("Decrypt() without passphrase should return error")
	}
}

func TestAesCipherDecryptInvalidBase64(t *testing.T) {
	cipher := security.NewAesCipher().WithAES256()
	var output bytes.Buffer
	cipher.Decrypt("invalid base64 !!!", &output)

	if cipher.Error == nil {
		t.Error("Decrypt() with invalid base64 should return error")
	}
}

func TestAesCipherDecryptTooShort(t *testing.T) {
	cipher := security.NewAesCipher().WithAES256()
	var output bytes.Buffer
	cipher.Decrypt("dGVzdA==", &output) // "test" in base64, too short for nonce

	if cipher.Error == nil {
		t.Error("Decrypt() with too short ciphertext should return error")
	}
}

func TestAesCipherDecryptWrongKey(t *testing.T) {
	// Encrypt with one key
	key1 := make([]byte, 32)
	copy(key1, "key1key1key1key1key1key1key1key1")

	cipher1 := security.NewAesCipher().WithPassphrase(key1)
	var encrypted bytes.Buffer
	cipher1.Encrypt("test", &encrypted)

	if cipher1.Error != nil {
		t.Fatalf("Encrypt() returned error: %v", cipher1.Error)
	}

	// Try to decrypt with different key
	key2 := make([]byte, 32)
	copy(key2, "key2key2key2key2key2key2key2key2")

	cipher2 := security.NewAesCipher().WithPassphrase(key2)
	var decrypted bytes.Buffer
	cipher2.Decrypt(encrypted.String(), &decrypted)

	if cipher2.Error == nil {
		t.Error("Decrypt() with wrong key should return error")
	}
}

func TestAesCipherChaining(t *testing.T) {
	// Test method chaining
	cipher := security.NewAesCipher().WithAES256()

	var encrypted bytes.Buffer
	cipher.Encrypt("test", &encrypted)

	if cipher.Error != nil {
		t.Errorf("Chained operations returned error: %v", cipher.Error)
	}

	// Test that error propagates through chain
	cipher2 := security.NewAesCipher().WithPassphrase([]byte("short")) // Invalid key size
	var output bytes.Buffer
	cipher2.Encrypt("test", &output)

	if cipher2.Error == nil {
		t.Error("Chained operations should propagate errors")
	}
}

func TestAesCipherErrorPropagation(t *testing.T) {
	// Start with error condition
	cipher := security.NewAesCipher()
	cipher.Error = errors.New("initial error") // Set an error

	// All subsequent operations should be no-ops
	var output bytes.Buffer
	result := cipher.WithAES256().Encrypt("test", &output)

	if result.Error == nil {
		t.Error("Operations on cipher with existing error should preserve error")
	}
}

// Benchmark tests
func BenchmarkAesCipherEncrypt(b *testing.B) {
	cipher := security.NewAesCipher().WithAES256()
	text := "benchmark test data for encryption"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var output bytes.Buffer
		cipher.Encrypt(text, &output)
		if cipher.Error != nil {
			b.Fatalf("Encrypt error: %v", cipher.Error)
		}
	}
}
