package flag_test

import (
	"os"
	"testing"

	"github.com/Valentin-Kaiser/go-core/flag"
)

func TestDefaultFlags(t *testing.T) {
	t.Parallel()
	// Test that default flags are properly initialized
	if flag.Path != "./data" {
		t.Errorf("Expected default Path to be './data', got '%s'", flag.Path)
	}

	if flag.Help != false {
		t.Errorf("Expected default Help to be false, got %v", flag.Help)
	}

	if flag.Version != false {
		t.Errorf("Expected default Version to be false, got %v", flag.Version)
	}

	if flag.Debug != false {
		t.Errorf("Expected default Debug to be false, got %v", flag.Debug)
	}
}

func TestRegisterFlag(t *testing.T) {
	t.Parallel()
	// Test registering a string flag
	var stringFlag string
	flag.RegisterFlag("test-string", &stringFlag, "A test string flag")

	// Test registering a bool flag
	var boolFlag bool
	flag.RegisterFlag("test-bool", &boolFlag, "A test bool flag")

	// Test registering an int flag
	var intFlag int
	flag.RegisterFlag("test-int", &intFlag, "A test int flag")

	// Test registering various numeric types
	var int8Flag int8
	flag.RegisterFlag("test-int8", &int8Flag, "A test int8 flag")

	var int16Flag int16
	flag.RegisterFlag("test-int16", &int16Flag, "A test int16 flag")

	var int32Flag int32
	flag.RegisterFlag("test-int32", &int32Flag, "A test int32 flag")

	var int64Flag int64
	flag.RegisterFlag("test-int64", &int64Flag, "A test int64 flag")

	var uintFlag uint
	flag.RegisterFlag("test-uint", &uintFlag, "A test uint flag")

	var uint8Flag uint8
	flag.RegisterFlag("test-uint8", &uint8Flag, "A test uint8 flag")

	var uint16Flag uint16
	flag.RegisterFlag("test-uint16", &uint16Flag, "A test uint16 flag")

	var uint32Flag uint32
	flag.RegisterFlag("test-uint32", &uint32Flag, "A test uint32 flag")

	var uint64Flag uint64
	flag.RegisterFlag("test-uint64", &uint64Flag, "A test uint64 flag")

	var float32Flag float32
	flag.RegisterFlag("test-float32", &float32Flag, "A test float32 flag")

	var float64Flag float64
	flag.RegisterFlag("test-float64", &float64Flag, "A test float64 flag")
}

func TestRegisterFlagPanics(t *testing.T) {
	t.Parallel()
	// Test that registering a duplicate flag panics
	var testFlag string
	flag.RegisterFlag("unique-flag", &testFlag, "A unique flag")

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when registering duplicate flag")
		}
	}()
	flag.RegisterFlag("unique-flag", &testFlag, "A duplicate flag")
}

func TestRegisterFlagNonPointer(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when registering non-pointer flag")
		}
	}()
	var testFlag string
	flag.RegisterFlag("non-pointer", testFlag, "A non-pointer flag")
}

func TestRegisterFlagNilPointer(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when registering nil pointer flag")
		}
	}()
	var testFlag *string
	flag.RegisterFlag("nil-pointer", testFlag, "A nil pointer flag")
}

func TestRegisterFlagUnsupportedType(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when registering unsupported type")
		}
	}()
	var testFlag []string
	flag.RegisterFlag("unsupported", &testFlag, "An unsupported type flag")
}

func TestInit(t *testing.T) {
	t.Parallel()
	// Save original args
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Test normal initialization
	os.Args = []string{"program"}
	flag.Init()
	// Should not panic or exit

	// Test with help flag (we can't easily test the exit behavior)
	// This is mainly to ensure the function runs without error
	flag.Help = false // Reset to false
	flag.Init()
}

func TestPrint(t *testing.T) {
	t.Parallel()
	// Test that Print doesn't panic
	// We can't easily test the output, but we can ensure it doesn't crash
	flag.Print()
}

// Test flag registration with default values
func TestRegisterFlagWithDefaults(t *testing.T) {
	t.Parallel()
	var stringFlag = "default"
	flag.RegisterFlag("default-string", &stringFlag, "A string flag with default")

	var intFlag = 42
	flag.RegisterFlag("default-int", &intFlag, "An int flag with default")

	var boolFlag = true
	flag.RegisterFlag("default-bool", &boolFlag, "A bool flag with default")

	var float64Flag = 3.14
	flag.RegisterFlag("default-float64", &float64Flag, "A float64 flag with default")
}

// Test integration with actual command line parsing
func TestCommandLineIntegration(t *testing.T) {
	t.Parallel()
	// Save original args
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Test with command line arguments
	var testString string
	var testInt int
	var testBool bool

	flag.RegisterFlag("integration-string", &testString, "Integration test string")
	flag.RegisterFlag("integration-int", &testInt, "Integration test int")
	flag.RegisterFlag("integration-bool", &testBool, "Integration test bool")

	// Simulate command line arguments
	os.Args = []string{
		"program",
		"--integration-string=hello",
		"--integration-int=123",
		"--integration-bool=true",
	}

	flag.Init()

	// Note: The actual parsing depends on pflag being properly set up
	// These tests mainly ensure the registration doesn't break
}

// Test that flags are properly bound to pflag
func TestFlagBinding(t *testing.T) {
	t.Parallel()
	var testFlag string
	flag.RegisterFlag("binding-test", &testFlag, "A binding test flag")

	// This mainly tests that the function completes without error
	// Actual binding verification would require more complex setup
}
