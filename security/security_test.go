package security

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetRandomBytes(t *testing.T) {
	testCases := []uint{1, 16, 32, 64, 128}
	
	for _, size := range testCases {
		bytes, err := GetRandomBytes(size)
		if err != nil {
			t.Errorf("GetRandomBytes(%d) returned error: %v", size, err)
			continue
		}
		
		if len(bytes) != int(size) {
			t.Errorf("GetRandomBytes(%d) returned %d bytes, expected %d", size, len(bytes), size)
		}
		
		// Check that bytes are not all zeros (very unlikely with crypto/rand)
		allZeros := true
		for _, b := range bytes {
			if b != 0 {
				allZeros = false
				break
			}
		}
		if allZeros && size > 0 {
			t.Errorf("GetRandomBytes(%d) returned all zeros, unlikely for crypto/rand", size)
		}
	}
}

func TestGetRandomBytesZero(t *testing.T) {
	bytes, err := GetRandomBytes(0)
	if err != nil {
		t.Errorf("GetRandomBytes(0) returned error: %v", err)
	}
	if len(bytes) != 0 {
		t.Errorf("GetRandomBytes(0) returned %d bytes, expected 0", len(bytes))
	}
}

func TestSHA256(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"hello", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
		{"Hello, World!", "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f"},
		{"The quick brown fox jumps over the lazy dog", "d7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592"},
	}
	
	for _, tc := range testCases {
		result := SHA256([]byte(tc.input))
		if result != tc.expected {
			t.Errorf("SHA256(%q) = %s, expected %s", tc.input, result, tc.expected)
		}
	}
}

func TestXX(t *testing.T) {
	testCases := []struct {
		input string
	}{
		{""},
		{"hello"},
		{"Hello, World!"},
		{"The quick brown fox jumps over the lazy dog"},
	}
	
	for _, tc := range testCases {
		result := XX([]byte(tc.input))
		// We can't predict the exact hash, but it should be consistent
		result2 := XX([]byte(tc.input))
		if result != result2 {
			t.Errorf("XX(%q) not consistent: %d != %d", tc.input, result, result2)
		}
	}
}

func TestMd5(t *testing.T) {
	testCases := []struct {
		input    interface{}
		expected string
	}{
		{"", "d41d8cd98f00b204e9800998ecf8427e"},
		{"hello", "5d41402abc4b2a76b9719d911017c592"},
		{"Hello, World!", "65a8e27d8879283831b664bd8b7f0ad4"},
		{42, "a1d0c6e83f027327d8461063f4ac58a6"},
		{true, "b326b5062b2f0e69046810717534cb09"},
	}
	
	for _, tc := range testCases {
		result := Md5(tc.input)
		if result != tc.expected {
			t.Errorf("Md5(%v) = %s, expected %s", tc.input, result, tc.expected)
		}
	}
}

func TestStringToKey32Bytes(t *testing.T) {
	testCases := []string{
		"",
		"password",
		"secret",
		"very long passphrase with many characters",
	}
	
	for _, tc := range testCases {
		key := StringToKey32Bytes(tc)
		if len(key) != 32 {
			t.Errorf("StringToKey32Bytes(%q) returned %d bytes, expected 32", tc, len(key))
		}
		
		// Test consistency
		key2 := StringToKey32Bytes(tc)
		if !bytes.Equal(key, key2) {
			t.Errorf("StringToKey32Bytes(%q) not consistent", tc)
		}
	}
}

func TestStringToKey32(t *testing.T) {
	testCases := []string{
		"",
		"password",
		"secret",
		"very long passphrase with many characters",
	}
	
	for _, tc := range testCases {
		key := StringToKey32(tc)
		if len(key) != 32 {
			t.Errorf("StringToKey32(%q) returned %d characters, expected 32", tc, len(key))
		}
		
		// Test consistency
		key2 := StringToKey32(tc)
		if key != key2 {
			t.Errorf("StringToKey32(%q) not consistent", tc)
		}
		
		// Test that it's hex encoded
		for _, c := range key {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("StringToKey32(%q) contains non-hex character: %c", tc, c)
				break
			}
		}
	}
}

func TestGetRandomBytesBase64(t *testing.T) {
	testCases := []uint{1, 16, 32, 64}
	
	for _, size := range testCases {
		encoded, err := GetRandomBytesBase64(size)
		if err != nil {
			t.Errorf("GetRandomBytesBase64(%d) returned error: %v", size, err)
			continue
		}
		
		// Base64 encoding should produce a string
		if len(encoded) == 0 && size > 0 {
			t.Errorf("GetRandomBytesBase64(%d) returned empty string", size)
		}
		
		// Test that it's valid base64
		if size > 0 {
			for _, c := range encoded {
				if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || 
					 (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=') {
					t.Errorf("GetRandomBytesBase64(%d) contains invalid base64 character: %c", size, c)
					break
				}
			}
		}
	}
}

func TestGenerateRandomPassword(t *testing.T) {
	testCases := []int{1, 8, 16, 32, 64}
	
	for _, length := range testCases {
		password, err := GenerateRandomPassword(length)
		if err != nil {
			t.Errorf("GenerateRandomPassword(%d) returned error: %v", length, err)
			continue
		}
		
		if len(password) != length {
			t.Errorf("GenerateRandomPassword(%d) returned password of length %d, expected %d", length, len(password), length)
		}
		
		// Test that password contains only valid characters
		validChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-#!+/"
		for _, c := range password {
			if !strings.ContainsRune(validChars, c) {
				t.Errorf("GenerateRandomPassword(%d) contains invalid character: %c", length, c)
				break
			}
		}
	}
}

func TestGenerateRandomPasswordZero(t *testing.T) {
	password, err := GenerateRandomPassword(0)
	if err != nil {
		t.Errorf("GenerateRandomPassword(0) returned error: %v", err)
	}
	if len(password) != 0 {
		t.Errorf("GenerateRandomPassword(0) returned password of length %d, expected 0", len(password))
	}
}

func TestReadOrSavePassphrase(t *testing.T) {
	// Create a temporary directory for test
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_passphrase.txt")
	
	// Test creating new passphrase
	passphrase1, err := ReadOrSavePassphrase(testFile, 32)
	if err != nil {
		t.Errorf("ReadOrSavePassphrase() returned error: %v", err)
	}
	
	if len(passphrase1) != 32 {
		t.Errorf("ReadOrSavePassphrase() returned passphrase of length %d, expected 32", len(passphrase1))
	}
	
	// Test reading existing passphrase
	passphrase2, err := ReadOrSavePassphrase(testFile, 32)
	if err != nil {
		t.Errorf("ReadOrSavePassphrase() returned error when reading existing file: %v", err)
	}
	
	if !bytes.Equal(passphrase1, passphrase2) {
		t.Error("ReadOrSavePassphrase() returned different passphrase when reading existing file")
	}
	
	// Test that file exists and has correct content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Errorf("Failed to read test file: %v", err)
	}
	
	if !bytes.Equal(content[:32], passphrase1) {
		t.Error("File content doesn't match returned passphrase")
	}
}

func TestReadOrSavePassphraseTooShort(t *testing.T) {
	// Create a temporary file with short content
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "short_passphrase.txt")
	
	err := os.WriteFile(testFile, []byte("short"), 0600)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	// Test reading passphrase that's too short
	_, err = ReadOrSavePassphrase(testFile, 32)
	if err == nil {
		t.Error("ReadOrSavePassphrase() should return error for too short passphrase")
	}
}

func TestReadOrSavePassphraseFilePermissions(t *testing.T) {
	// Create a temporary directory for test
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "perm_test.txt")
	
	// Create passphrase file
	_, err := ReadOrSavePassphrase(testFile, 16)
	if err != nil {
		t.Errorf("ReadOrSavePassphrase() returned error: %v", err)
	}
	
	// Check file permissions
	info, err := os.Stat(testFile)
	if err != nil {
		t.Errorf("Failed to stat test file: %v", err)
	}
	
	mode := info.Mode()
	if mode.Perm() != 0600 {
		t.Errorf("File permissions are %o, expected 0600", mode.Perm())
	}
}

// Benchmark tests
func BenchmarkGetRandomBytes(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := GetRandomBytes(32)
		if err != nil {
			b.Fatalf("GetRandomBytes error: %v", err)
		}
	}
}

func BenchmarkSHA256(b *testing.B) {
	data := []byte("benchmark test data")
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		SHA256(data)
	}
}

func BenchmarkXX(b *testing.B) {
	data := []byte("benchmark test data")
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		XX(data)
	}
}

func BenchmarkMd5(b *testing.B) {
	data := "benchmark test data"
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		Md5(data)
	}
}

func BenchmarkGenerateRandomPassword(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := GenerateRandomPassword(32)
		if err != nil {
			b.Fatalf("GenerateRandomPassword error: %v", err)
		}
	}
}