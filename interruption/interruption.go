// Package interruption provides a simple mechanism for recovering from panics
// in both the main application and concurrently running goroutines.
//
// It ensures that unexpected runtime panics do not crash the application silently,
// by logging detailed error messages and stack traces.
//
// Usage:
// Call `defer interruption.Handle()` at the beginning of the `main` function
// and in every goroutine to catch and log panics.
//
// In debug mode, a full stack trace is logged to aid in debugging.
// In production mode, only the panic message and caller information are logged
// to avoid cluttering logs with excessive detail.
package interruption

import (
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/Valentin-Kaiser/go-core/flag"
	"github.com/rs/zerolog/log"
)

// Handle is a function that handles panics in the application
// It recovers from the panic and logs the error message along with the stack trace
func Handle() {
	if err := recover(); err != nil {
		caller := "unknown"
		line := 0
		_, file, line, ok := runtime.Caller(2)
		if ok {
			caller = fmt.Sprintf("%s/%s", filepath.Base(filepath.Dir(file)), strings.Trim(filepath.Base(file), filepath.Ext(file)))
		}

		if flag.Debug {
			log.Error().Msgf("[Interrupt] %v code: %v => %v \n %v", caller, line, err, string(debug.Stack()))
			return
		}
		log.Error().Msgf("[Interrupt] %v code: %v => %v", caller, line, err)
	}
}
