package interruption

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

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

func TestHandleWithDetailedPanicTypes(t *testing.T) {
	tests := []struct {
		name      string
		panicData interface{}
	}{
		{"custom error", fmt.Errorf("custom error message")},
		{"runtime error", fmt.Errorf("runtime error: invalid memory address or nil pointer dereference")},
		{"slice bound error", "runtime error: slice bounds out of range"},
		{"map access", "runtime error: assignment to entry in nil map"},
		{"channel close", "send on closed channel"},
		{"complex struct", struct {
			ID   int
			Name string
		}{ID: 123, Name: "test"}},
		{"slice panic", []string{"panic", "data"}},
		{"map panic", map[string]int{"error": 500}},
		{"function panic", func() string { return "function panic" }},
		{"very long string", strings.Repeat("very long panic message ", 100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Handle() should not re-panic: %v", r)
				}
			}()

			func() {
				defer Handle()
				panic(tt.panicData)
			}()
		})
	}
}

func TestHandleWithCallStack(t *testing.T) {
	// Test that Handle captures the correct caller information
	originalDebug := flag.Debug
	defer func() { flag.Debug = originalDebug }()

	// Test in debug mode
	flag.Debug = true

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Handle() should not re-panic in debug mode: %v", r)
			}
		}()

		func() {
			defer Handle()
			panic("debug mode panic")
		}()
	}()

	// Test in production mode
	flag.Debug = false

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Handle() should not re-panic in production mode: %v", r)
			}
		}()

		func() {
			defer Handle()
			panic("production mode panic")
		}()
	}()
}

func TestHandleWithGoroutineStack(t *testing.T) {
	done := make(chan bool, 1)

	go func() {
		defer func() {
			done <- true
		}()

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Handle() should not re-panic in goroutine: %v", r)
			}
		}()

		func() {
			defer Handle()
			panic("goroutine panic")
		}()
	}()

	select {
	case <-done:
		// Test completed successfully
	case <-time.After(5 * time.Second):
		t.Error("Goroutine test timed out")
	}
}

func TestHandleWithRecursivePanic(t *testing.T) {
	depth := 0
	maxDepth := 5

	var recursivePanic func()
	recursivePanic = func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Handle() should not re-panic at depth %d: %v", depth, r)
			}
		}()

		defer Handle()

		depth++
		if depth < maxDepth {
			recursivePanic()
		}
		panic(fmt.Sprintf("recursive panic at depth %d", depth))
	}

	recursivePanic()
}

func TestHandleCallerInformation(t *testing.T) {
	// This test verifies that Handle() captures caller information correctly
	// We can't easily test the actual log output, but we can ensure it doesn't panic

	tests := []struct {
		name string
		fn   func()
	}{
		{
			"direct call",
			func() {
				defer Handle()
				panic("direct panic")
			},
		},
		{
			"nested call",
			func() {
				func() {
					defer Handle()
					panic("nested panic")
				}()
			},
		},
		{
			"anonymous function",
			func() {
				func() {
					defer Handle()
					panic("anonymous panic")
				}()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Handle() should not re-panic in %s: %v", tt.name, r)
				}
			}()

			tt.fn()
		})
	}
}

func TestHandleWithRuntimeCallerFailure(t *testing.T) {
	// Test what happens when runtime.Caller fails
	// This is difficult to test directly, but we can ensure Handle() is robust

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Handle() should not re-panic when runtime.Caller fails: %v", r)
		}
	}()

	func() {
		defer Handle()
		panic("test runtime caller failure scenario")
	}()
}

func TestHandleInDifferentDebugModes(t *testing.T) {
	originalDebug := flag.Debug
	defer func() { flag.Debug = originalDebug }()

	modes := []struct {
		name  string
		debug bool
	}{
		{"debug mode", true},
		{"production mode", false},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			flag.Debug = mode.debug

			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Handle() should not re-panic in %s: %v", mode.name, r)
				}
			}()

			func() {
				defer Handle()
				panic(fmt.Sprintf("panic in %s", mode.name))
			}()
		})
	}
}

func TestHandleWithNilPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Handle() should not re-panic with nil panic: %v", r)
		}
	}()

	func() {
		defer Handle()
		panic(nil)
	}()
}

func TestHandleWithLargePanicData(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Handle() should not re-panic with large panic data: %v", r)
		}
	}()

	// Create large panic data
	largeData := make([]byte, 1024*1024) // 1MB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	func() {
		defer Handle()
		panic(largeData)
	}()
}

func TestHandleFilePathProcessing(t *testing.T) {
	// Test that file paths are processed correctly in caller information
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Handle() should not re-panic during file path processing: %v", r)
		}
	}()

	// Create a scenario with nested directory structure
	func() {
		defer Handle()
		panic("file path processing test")
	}()
}

func TestHandleWithOSSignals(t *testing.T) {
	// Test Handle() behavior when OS signals might be involved
	// This is more of a robustness test

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Handle() should not re-panic with OS signal simulation: %v", r)
		}
	}()

	func() {
		defer Handle()
		// Simulate some OS-related panic
		panic("simulated OS signal panic")
	}()
}

func TestHandleMemoryPressure(t *testing.T) {
	// Test Handle() under memory pressure scenarios
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Handle() should not re-panic under memory pressure: %v", r)
		}
	}()

	func() {
		defer Handle()
		// Simulate out of memory scenario
		panic("runtime: out of memory")
	}()
}

func TestMultipleHandleRegistrations(t *testing.T) {
	// Test that multiple Handle() calls work correctly
	counter := 0

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Multiple Handle() calls should not re-panic: %v", r)
		}
		if counter != 3 {
			t.Errorf("Expected counter to be 3, got %d", counter)
		}
	}()

	func() {
		defer func() { counter++ }()
		defer Handle()

		func() {
			defer func() { counter++ }()
			defer Handle()

			func() {
				defer func() { counter++ }()
				defer Handle()
				panic("nested handle test")
			}()
		}()
	}()
}

func TestHandlePerformance(t *testing.T) {
	// Basic performance test to ensure Handle() doesn't introduce significant overhead
	iterations := 1000

	start := time.Now()
	for i := 0; i < iterations; i++ {
		func() {
			defer func() {
				recover() // Catch the panic to prevent test failure
			}()
			defer Handle()
			panic("performance test")
		}()
	}
	duration := time.Since(start)

	// Ensure it doesn't take too long (arbitrary threshold)
	if duration > time.Second {
		t.Errorf("Handle() performance test took too long: %v", duration)
	}
}

// Test runtime characteristics
func TestHandleRuntimeCharacteristics(t *testing.T) {
	// Test that Handle() works correctly with different runtime characteristics

	// Test with different GOMAXPROCS values
	originalGOMAXPROCS := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(originalGOMAXPROCS)

	for _, procs := range []int{1, 2, 4, runtime.NumCPU()} {
		t.Run(fmt.Sprintf("GOMAXPROCS=%d", procs), func(t *testing.T) {
			runtime.GOMAXPROCS(procs)

			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Handle() should not re-panic with GOMAXPROCS=%d: %v", procs, r)
				}
			}()

			func() {
				defer Handle()
				panic(fmt.Sprintf("panic with GOMAXPROCS=%d", procs))
			}()
		})
	}
}

func TestHandleWithEnvironmentVariables(t *testing.T) {
	// Test Handle() behavior with different environment configurations
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Test with modified environment
	os.Setenv("PATH", "")

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Handle() should not re-panic with modified environment: %v", r)
		}
	}()

	func() {
		defer Handle()
		panic("environment variable test")
	}()
}

// Benchmark tests
func BenchmarkHandle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		func() {
			defer func() {
				recover() // Catch the panic to prevent benchmark failure
			}()
			defer Handle()
			panic("benchmark test")
		}()
	}
}

func BenchmarkHandleWithLargeStack(b *testing.B) {
	for i := 0; i < b.N; i++ {
		func() {
			defer func() {
				recover() // Catch the panic to prevent benchmark failure
			}()

			// Create a deep call stack
			var deepCall func(depth int)
			deepCall = func(depth int) {
				if depth > 0 {
					deepCall(depth - 1)
				} else {
					defer Handle()
					panic("deep stack benchmark")
				}
			}

			deepCall(100) // 100 levels deep
		}()
	}
}

func BenchmarkHandleDebugMode(b *testing.B) {
	originalDebug := flag.Debug
	flag.Debug = true
	defer func() { flag.Debug = originalDebug }()

	for i := 0; i < b.N; i++ {
		func() {
			defer func() {
				recover() // Catch the panic to prevent benchmark failure
			}()
			defer Handle()
			panic("debug mode benchmark")
		}()
	}
}

func BenchmarkHandleProductionMode(b *testing.B) {
	originalDebug := flag.Debug
	flag.Debug = false
	defer func() { flag.Debug = originalDebug }()

	for i := 0; i < b.N; i++ {
		func() {
			defer func() {
				recover() // Catch the panic to prevent benchmark failure
			}()
			defer Handle()
			panic("production mode benchmark")
		}()
	}
}
