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
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/Valentin-Kaiser/go-core/flag"
	"github.com/rs/zerolog/log"
)

var wg = sync.WaitGroup{}

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

// OnSignal registers a handler function to be called when the specified signals are received.
// It allows graceful shutdown or cleanup operations when the application receives termination signals.
// The application termination is blocked until the handler function returns.
func OnSignal(handler func() error, signals ...os.Signal) error {
	if len(signals) == 0 {
		return fmt.Errorf("no signals provided")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, signals...)

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-sigChan
		err := handler()
		if err != nil {
			log.Error().Err(err).Msg("Error handling signal")
		}
	}()

	return nil
}

func Wait() {
	wg.Wait()
}
