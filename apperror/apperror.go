// Package apperror provides a custom error type that enhances standard Go errors
// with stack traces and support for additional nested errors.
//
// It is designed to improve error handling in Go applications by offering contextual
// information such as call location and related errors, especially useful for debugging
// and logging in production environments.
//
// Features:
//   - Attaches a lightweight stack trace to each error
//   - Supports wrapping and chaining of multiple related errors
//   - Automatically includes detailed trace and error info when debug mode is enabled
//   - Implements the standard error interface
//
// Usage:
//
//	// Create a new application error
//	err := apperror.NewError("something went wrong")
//
//	// Wrap an existing error to capture a new stack trace point
//	err = apperror.Wrap(err)
//
//	// Add related errors for context
//	err = err.(apperror.Error).AddError(io.EOF)
//
//	// Print with trace and nested errors if debug mode is enabled
//	fmt.Println(err)
//
// To enable debug output (stack traces), set `flag.Debug = true` before printing errors.
//
// Note: If you're wrapping errors that are already of type `apperror.Error`,
// prefer `Wrap` over creating a new instance to preserve the trace history.
package apperror

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/Valentin-Kaiser/go-core/flag"
)

var (
	// TraceDelimiter is used to separate trace entries
	TraceDelimiter = " -> "
	// ErrorDelimiter is used to separate multiple errors
	ErrorDelimiter = " => "
	// TraceFormat is the format for displaying trace entries
	TraceFormat = "%v+%v"
	// ErrorFormat is the format for displaying the error message and additional errors
	ErrorFormat = "%s [%s]"
	// ErrorTraceFormat is the format for displaying the error message with a stack trace
	ErrorTraceFormat = "%s | %s"
	// FullFormat is the format for displaying the error message with a stack trace and additional errors
	FullFormat = "%s | %s [%s]"

	// ErrorHandler is a function that handles deferred error checks
	ErrorHandler = func(err error, msg string) {
		if err == nil {
			return
		}
		if flag.Debug {
			panic(fmt.Sprintf(FullFormat, strings.Join(trace(Error{Message: msg}), TraceDelimiter), msg, err.Error()))
		}
		panic(fmt.Sprintf(ErrorFormat, msg, err.Error()))
	}
)

// Error represents an application error with a stack trace and additional errors
// It implements the error interface and can be used to wrap other errors
type Error struct {
	Trace   []string
	Errors  []error
	Message string
}

// NewError creates a new Error instance with the given message
// If the error is already of type Error you should use Wrap instead
func NewError(msg string) Error {
	e := Error{
		Message: msg,
	}
	e.Trace = trace(e)
	return e
}

// NewErrorf creates a new Error instance with the formatted message
// If the error is already of type Error you should use Wrap instead
func NewErrorf(format string, a ...interface{}) Error {
	e := Error{
		Message: fmt.Sprintf(format, a...),
	}
	e.Trace = trace(e)
	return e
}

// Wrap wraps an error and adds a stack trace to it
// Should be used to wrap errors that are of type Error
func Wrap(err error) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(Error); ok {
		e.Trace = trace(e)
		return e
	}
	e := Error{
		Message: err.Error(),
	}
	e.Trace = trace(e)
	return e
}

// AddError adds an additional error to the Error instance context
func (e Error) AddError(err error) Error {
	if getErrors(err) != nil {
		e.Errors = append(e.Errors, getErrors(err)...)
		return e
	}
	e.Errors = append(e.Errors, err)
	return e
}

// AddErrors adds multiple additional errors to the Error instance context
func (e Error) AddErrors(errs []error) Error {
	for _, err := range errs {
		if getErrors(err) != nil {
			e.Errors = append(e.Errors, getErrors(err)...)
			continue
		}
		e.Errors = append(e.Errors, err)
	}
	return e
}

// Error implements the error interface and returns the error message
// If debug mode is enabled, it includes the stack trace and additional errors
func (e Error) Error() string {
	if flag.Debug && len(e.Trace) > 0 {
		trace := ""
		for i := len(e.Trace) - 1; i >= 0; i-- {
			trace += e.Trace[i]
			if i > 0 {
				trace += TraceDelimiter
			}
		}

		errors := ""
		for _, d := range e.Errors {
			if errors != "" {
				errors += ErrorDelimiter
			}
			errors += d.Error()
		}

		if errors == "" {
			return fmt.Sprintf(ErrorTraceFormat, trace, e.Message)
		}
		return fmt.Sprintf(FullFormat, trace, e.Message, errors)
	}

	errors := ""
	for _, d := range e.Errors {
		if errors != "" {
			errors += ErrorDelimiter
		}
		errors += d.Error()
	}

	if errors == "" {
		return e.Message
	}
	return fmt.Sprintf(ErrorFormat, e.Message, errors)
}

// Split separates the error into its components: message, trace, and additional errors
// It returns the message, a slice of trace strings, and a slice of additional errors
func Split(err error) (string, []string, []error) {
	aerr, ok := err.(Error)
	if !ok {
		return err.Error(), nil, nil
	}

	if aerr.Message == "" && len(aerr.Trace) == 0 && len(aerr.Errors) == 0 {
		return "", nil, nil
	}

	return aerr.Message, aerr.Trace, aerr.Errors
}

// Parse takes a string representation of an error and returns an Error instance
// The string should be formatted with TraceDelimiter to separate the trace entries
func Parse(str string) Error {
	parts := strings.Split(str, TraceDelimiter)
	if len(parts) < 2 {
		return NewError(str)
	}

	t := parts[:len(parts)-1]
	message := parts[len(parts)-1]

	e := Error{
		Message: message,
		Trace:   t,
	}

	if len(parts) > 2 {
		errors := parts[1 : len(parts)-1]
		for _, errStr := range errors {
			e.Errors = append(e.Errors, NewError(errStr))
		}
	}

	e.Trace = trace(e)
	return e
}

// Handle is a utility function to handle error checks
// It takes an error and a message, and if the error is not nil,
// it formats the message and panics with the error details.
func Handle(err error, msg string) {
	if ErrorHandler != nil {
		ErrorHandler(err, msg)
	}
}

// HandleCustom is a utility function to handle error checks with a custom handler
// It takes an error, a message, and a custom handler.
func HandleCustom(err error, msg string, handler func(error, string)) {
	if handler != nil {
		handler(err, msg)
		return
	}
	if ErrorHandler != nil {
		ErrorHandler(err, msg)
	}
}

// Catch is a utility function to handle error checks for example in deferred functions
// It takes an error and a message, and if the error is not nil,
// it formats the message and panics with the error details.
// defer apperror.Catch(funcWithError(), "an error occurred")
func Catch(f func() error, msg string) {
	if ErrorHandler != nil {
		ErrorHandler(f(), msg)
	}
}

// CatchCustom is a utility function to handle deferred error checks with a custom handler
// It takes a function that returns an error, a message, and a custom handler.
func CatchCustom(f func() error, msg string, handler func(error, string)) {
	if handler != nil {
		handler(f(), msg)
		return
	}
	if ErrorHandler != nil {
		ErrorHandler(f(), msg)
	}
}

// trace generates a stack trace for the error
// It uses runtime.Caller to get the file name and line number
func trace(e Error) []string {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		return e.Trace
	}

	if f := runtime.FuncForPC(pc); f != nil {
		e.Trace = append(e.Trace, fmt.Sprintf("%v+%v", f.Name(), line))
		return e.Trace
	}

	e.Trace = append(e.Trace, fmt.Sprintf("%s+%d", file, line))
	return e.Trace
}

// getErrors checks if the error is of type Error and returns the additional errors
// If the error is not of type Error, it returns nil
func getErrors(err error) []error {
	if e, ok := err.(Error); ok {
		return e.Errors
	}
	return nil
}

// Where returns the trace location of the caller at the specified level
// The level parameter indicates how many stack frames to skip
func Where(level int) string {
	pc := make([]uintptr, 32)
	n := runtime.Callers(level, pc)
	if n == 0 {
		return "unknown"
	}

	pc = pc[:n]
	frames := runtime.CallersFrames(pc)

	var sb strings.Builder
	for {
		frame, more := frames.Next()
		// Format: file:line (func)
		fmt.Fprintf(&sb, "%s:%d (%s)\n", frame.File, frame.Line, frame.Function)

		if !more {
			break
		}
	}
	return sb.String()
}
