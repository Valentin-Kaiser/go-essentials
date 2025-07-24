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

// Catch is a function that handles panics in the application
// It recovers from the panic and logs the error message along with the stack trace
func Catch() {
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
	ctx, cancel := context.WithCancel(context.Background())

	// Create a channel to receive signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, signals...)

	go func() {
		defer func() {
			signal.Stop(sigChan)
			cancel()
		}()

		if len(handlers) == 0 {
			return
		}

		// Wait for signal
		<-sigChan

		// Execute all handlers with panic recovery
		for _, handler := range handlers {
			func() {
				defer Catch()
				if err := handler(); err != nil {
					log.Error().Err(err).Msgf("[Signal] handler failed: %v", err)
				}
			}()
		}
	}()
	return ctx
}

// WaitForShutdown waits for the provided context to be done, enabling graceful shutdown.
// This function is designed to be used with defer to ensure the application waits for
// signal handlers to complete before exiting.
//
// Example usage:
//
//	func main() {
//		defer interruption.Handle()
//
//		ctx := interruption.OnSignal([]func() error{
//			func() error {
//				log.Info().Msg("Shutting down gracefully...")
//				return nil
//			},
//		}, os.Interrupt, syscall.SIGTERM)
//
//		defer interruption.WaitForShutdown(ctx)
//
//		// Your application logic here
//		log.Info().Msg("Application running...")
//
//		// Application will wait for signal and graceful shutdown when function exits
//	}
func WaitForShutdown(ctx context.Context) {
	if ctx == nil {
		return
	}
	<-ctx.Done()
}

// SetupGracefulShutdown is a convenience function that combines OnSignal and WaitForShutdown.
// It sets up signal handlers and returns a function that should be called with defer
// to wait for graceful shutdown. This is the recommended way to add graceful shutdown to your application.
//
// Example usage:
//
//	func main() {
//		defer interruption.Handle()
//
//		defer interruption.SetupGracefulShutdown([]func() error{
//			func() error {
//				log.Info().Msg("Database disconnecting...")
//				// database.Disconnect()
//				return nil
//			},
//			func() error {
//				log.Info().Msg("Web server stopping...")
//				// web.Instance().Stop()
//				return nil
//			},
//		}, os.Interrupt, syscall.SIGTERM)
//
//		// Your application logic here
//		log.Info().Msg("Application running...")
//
//		// Application will automatically wait for graceful shutdown when function exits
//	}
func SetupGracefulShutdown(handlers []func() error, signals ...os.Signal) func() {
	ctx := OnSignal(handlers, signals...)
	return func() {
		WaitForShutdown(ctx)
	}
}
