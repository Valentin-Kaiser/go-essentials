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
// Example:
//
//	package main
//
//	import (
//		"github.com/Valentin-Kaiser/go-core/interruption"
//		"github.com/rs/zerolog/log"
//	)
//
//	func main() {
//		defer interruption.Handle()
//		log.Info().Msg("Application started")
//		// Your application logic here
//
//		ctx := interruption.OnSignal([]func() error{
//			func() error {
//				log.Info().Msg("Received interrupt signal, shutting down gracefully")
//				return nil
//			},
//		}, os.Interrupt, syscall.SIGTERM)
//
//		<-ctx.Done() // Wait for the signal handler to complete
//	}
//
// In debug mode, a full stack trace is logged to aid in debugging.
// In production mode, only the panic message and caller information are logged
// to avoid cluttering logs with excessive detail.
package interruption

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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

// OnSignal registers a handler function to be called when the specified signals are received.
// It allows graceful shutdown or cleanup operations when the application receives termination signals.
// The returned context is canceled when the handler function returns, allowing the application to wait for cleanup operations to complete.
func OnSignal(handlers []func() error, signals ...os.Signal) context.Context {
	ctx, stop := signal.NotifyContext(context.Background(), signals...)
	go func() {
		for _, handler := range handlers {
			if err := handler(); err != nil {
				log.Error().Err(err).Msgf("[Signal] handler failed: %v", err)
			}
		}
		stop()
	}()
	return ctx
}
