package interruption

import (
	"errors"
	"testing"

	"github.com/Valentin-Kaiser/go-core/flag"
)

func TestHandle(t *testing.T) {
	// Test that Handle recovers from panic
	defer func() {
		if r := recover(); r != nil {
			t.Error("Handle should recover from panic, but panic was not handled")
		}
	}()

	// Test normal execution (no panic)
	func() {
		defer Handle()
		// Normal code that doesn't panic
	}()
}

func TestHandleWithPanic(t *testing.T) {
	// Test that Handle actually recovers from panic
	var handled bool

	func() {
		defer func() {
			if r := recover(); r != nil {
				Handle()
			}
			handled = true
		}()
		panic("test panic")
	}()

	if !handled {
		t.Error("Handle should have been called after panic recovery")
	}
}

func TestHandleWithDifferentPanicTypes(t *testing.T) {
	testCases := []struct {
		name  string
		panic interface{}
	}{
		{"string panic", "string panic message"},
		{"int panic", 42},
		{"error panic", errors.New("test error")},
		{"nil panic", nil},
		{"struct panic", struct{ msg string }{msg: "struct panic"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var handled bool

			func() {
				defer func() {
					if r := recover(); r != nil {
						Handle()
					}
					handled = true
				}()
				panic(tc.panic)
			}()

			if !handled {
				t.Errorf("Handle should have recovered from %s", tc.name)
			}
		})
	}
}

func TestHandleInDebugMode(t *testing.T) {
	// Save original debug state
	originalDebug := flag.Debug
	defer func() { flag.Debug = originalDebug }()

	// Test with debug mode enabled
	flag.Debug = true

	var handled bool

	func() {
		defer func() {
			if r := recover(); r != nil {
				Handle()
			}
			handled = true
		}()
		panic("debug mode panic")
	}()

	if !handled {
		t.Error("Handle should have recovered from panic in debug mode")
	}
}

func TestHandleInProductionMode(t *testing.T) {
	// Save original debug state
	originalDebug := flag.Debug
	defer func() { flag.Debug = originalDebug }()

	// Test with debug mode disabled
	flag.Debug = false

	var handled bool

	func() {
		defer func() {
			if r := recover(); r != nil {
				Handle()
			}
			handled = true
		}()
		panic("production mode panic")
	}()

	if !handled {
		t.Error("Handle should have recovered from panic in production mode")
	}
}

func TestHandleNested(t *testing.T) {
	// Test nested panic handling
	var outerHandled, innerHandled bool

	func() {
		defer func() {
			if r := recover(); r != nil {
				Handle()
			}
			outerHandled = true
		}()

		func() {
			defer func() {
				if r := recover(); r != nil {
					Handle()
				}
				innerHandled = true
			}()
			panic("inner panic")
		}()

		panic("outer panic")
	}()

	if !innerHandled {
		t.Error("Inner Handle should have been called")
	}
	if !outerHandled {
		t.Error("Outer Handle should have been called")
	}
}

func TestHandleMultiple(t *testing.T) {
	// Test multiple separate panic recoveries
	for i := 0; i < 3; i++ {
		var handled bool

		func() {
			defer func() {
				if r := recover(); r != nil {
					Handle()
				}
				handled = true
			}()
			panic("multiple panic test")
		}()

		if !handled {
			t.Errorf("Handle should have recovered from panic %d", i)
		}
	}
}

func TestHandleWithoutPanic(t *testing.T) {
	// Test that Handle doesn't interfere with normal execution
	var normalExecution bool

	func() {
		defer Handle()
		normalExecution = true
	}()

	if !normalExecution {
		t.Error("Handle should not interfere with normal execution")
	}
}

// Test that shows Handle can be used in goroutines
func TestHandleInGoroutine(t *testing.T) {
	done := make(chan bool)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				Handle()
			}
			done <- true
		}()
		panic("goroutine panic")
	}()

	select {
	case <-done:
		// Success - panic was handled
	case <-make(chan bool):
		t.Error("Handle should have recovered from panic in goroutine")
	}
}

// Benchmark to test performance impact
func BenchmarkHandle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		func() {
			defer Handle()
			// Normal execution without panic
		}()
	}
}

func BenchmarkHandleWithPanic(b *testing.B) {
	for i := 0; i < b.N; i++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					Handle()
				}
			}()
			panic("benchmark panic")
		}()
	}
}
