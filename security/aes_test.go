package security

import (
	"bytes"
	"errors"
	"testing"
)

func TestNewAesCipher(t *testing.T) {
	cipher := NewAesCipher()
	if cipher == nil {
		t.Error("NewAesCipher() returned nil")
	}

	if cipher.Error != nil {
		t.Errorf("NewAesCipher() created cipher with error: %v", cipher.Error)
	}

	if cipher.passphrase != nil {
		t.Error("NewAesCipher() should not have a passphrase initially")
	}
}

func TestAesCipherWithAES128(t *testing.T) {
	cipher := NewAesCipher().WithAES128()

	if cipher.Error != nil {
		t.Errorf("WithAES128() returned error: %v", cipher.Error)
	}

	if len(cipher.passphrase) != 16 {
		t.Errorf("WithAES128() passphrase length is %d, expected 16", len(cipher.passphrase))
	}
}

func TestAesCipherWithAES192(t *testing.T) {
	cipher := NewAesCipher().WithAES192()

	if cipher.Error != nil {
		t.Errorf("WithAES192() returned error: %v", cipher.Error)
	}

	if len(cipher.passphrase) != 24 {
		t.Errorf("WithAES192() passphrase length is %d, expected 24", len(cipher.passphrase))
	}
}

func TestAesCipherWithAES256(t *testing.T) {
	cipher := NewAesCipher().WithAES256()

	if cipher.Error != nil {
		t.Errorf("WithAES256() returned error: %v", cipher.Error)
	}

	if len(cipher.passphrase) != 32 {
		t.Errorf("WithAES256() passphrase length is %d, expected 32", len(cipher.passphrase))
	}
}

func TestAesCipherWithPassphrase(t *testing.T) {
	testCases := []struct {
		name       string
		passphrase []byte
		shouldWork bool
	}{
		{"16 bytes", make([]byte, 16), true},
		{"24 bytes", make([]byte, 24), true},
		{"32 bytes", make([]byte, 32), true},
		{"8 bytes", make([]byte, 8), false},   // Too short
		{"20 bytes", make([]byte, 20), false}, // Invalid length
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cipher := NewAesCipher().WithPassphrase(tc.passphrase)

			if !bytes.Equal(cipher.passphrase, tc.passphrase) {
				t.Errorf("WithPassphrase() passphrase mismatch")
			}

			// Test encryption to see if passphrase is valid
			var output bytes.Buffer
			cipher.Encrypt("test", &output)

			if tc.shouldWork && cipher.Error != nil {
				t.Errorf("WithPassphrase() with %s should work, but got error: %v", tc.name, cipher.Error)
			}
			if !tc.shouldWork && cipher.Error == nil {
				t.Errorf("WithPassphrase() with %s should fail, but succeeded", tc.name)
			}
		})
	}
}

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

			cipher := NewAesCipher().WithPassphrase(key)

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
			cipher2 := NewAesCipher().WithPassphrase(key)
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
	cipher := NewAesCipher()
	var output bytes.Buffer
	cipher.Encrypt("test", &output)

	if cipher.Error == nil {
		t.Error("Encrypt() without passphrase should return error")
	}
}

func TestAesCipherDecryptWithoutPassphrase(t *testing.T) {
	cipher := NewAesCipher()
	var output bytes.Buffer
	cipher.Decrypt("test", &output)

	if cipher.Error == nil {
		t.Error("Decrypt() without passphrase should return error")
	}
}

func TestAesCipherDecryptInvalidBase64(t *testing.T) {
	cipher := NewAesCipher().WithAES256()
	var output bytes.Buffer
	cipher.Decrypt("invalid base64 !!!", &output)

	if cipher.Error == nil {
		t.Error("Decrypt() with invalid base64 should return error")
	}
}

func TestAesCipherDecryptTooShort(t *testing.T) {
	cipher := NewAesCipher().WithAES256()
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

	cipher1 := NewAesCipher().WithPassphrase(key1)
	var encrypted bytes.Buffer
	cipher1.Encrypt("test", &encrypted)

	if cipher1.Error != nil {
		t.Fatalf("Encrypt() returned error: %v", cipher1.Error)
	}

	// Try to decrypt with different key
	key2 := make([]byte, 32)
	copy(key2, "key2key2key2key2key2key2key2key2")

	cipher2 := NewAesCipher().WithPassphrase(key2)
	var decrypted bytes.Buffer
	cipher2.Decrypt(encrypted.String(), &decrypted)

	if cipher2.Error == nil {
		t.Error("Decrypt() with wrong key should return error")
	}
}

func TestAesCipherChaining(t *testing.T) {
	// Test method chaining
	cipher := NewAesCipher().WithAES256()

	var encrypted bytes.Buffer
	cipher.Encrypt("test", &encrypted)

	if cipher.Error != nil {
		t.Errorf("Chained operations returned error: %v", cipher.Error)
	}

	// Test that error propagates through chain
	cipher2 := NewAesCipher().WithPassphrase([]byte("short")) // Invalid key size
	var output bytes.Buffer
	cipher2.Encrypt("test", &output)

	if cipher2.Error == nil {
		t.Error("Chained operations should propagate errors")
	}
}

func TestAesCipherErrorPropagation(t *testing.T) {
	// Start with error condition
	cipher := NewAesCipher()
	cipher.Error = errors.New("initial error") // Set an error

	// All subsequent operations should be no-ops
	var output bytes.Buffer
	result := cipher.WithAES256().Encrypt("test", &output)

	if result.Error == nil {
		t.Error("Operations on cipher with existing error should preserve error")
	}
}

// Test multiple encrypt/decrypt operations
func TestAesCipherMultipleOperations(t *testing.T) {
	cipher := NewAesCipher().WithAES256()

	testData := []string{"test1", "test2", "test3"}
	encrypted := make([]string, len(testData))

	// Encrypt multiple times
	for i, data := range testData {
		var output bytes.Buffer
		cipher.Encrypt(data, &output)
		if cipher.Error != nil {
			t.Errorf("Encrypt %d returned error: %v", i, cipher.Error)
			return
		}
		encrypted[i] = output.String()
	}

	// Decrypt and verify
	for i, data := range testData {
		cipher2 := NewAesCipher().WithPassphrase(cipher.passphrase)
		var output bytes.Buffer
		cipher2.Decrypt(encrypted[i], &output)
		if cipher2.Error != nil {
			t.Errorf("Decrypt %d returned error: %v", i, cipher2.Error)
			return
		}

		if output.String() != data {
			t.Errorf("Decrypt %d returned %q, expected %q", i, output.String(), data)
		}
	}
}

// Benchmark tests
func BenchmarkAesCipherEncrypt(b *testing.B) {
	cipher := NewAesCipher().WithAES256()
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

func BenchmarkAesCipherDecrypt(b *testing.B) {
	cipher := NewAesCipher().WithAES256()
	text := "benchmark test data for decryption"

	// Pre-encrypt the text
	var encrypted bytes.Buffer
	cipher.Encrypt(text, &encrypted)
	if cipher.Error != nil {
		b.Fatalf("Setup encrypt error: %v", cipher.Error)
	}

	encryptedStr := encrypted.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cipher2 := NewAesCipher().WithPassphrase(cipher.passphrase)
		var output bytes.Buffer
		cipher2.Decrypt(encryptedStr, &output)
		if cipher2.Error != nil {
			b.Fatalf("Decrypt error: %v", cipher2.Error)
		}
	}
}
