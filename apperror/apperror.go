// The apperror package provides a custom error type that includes a stack trace and additional errors.
// It is designed to be used in Go applications to handle errors more effectively.
package apperror

import (
	"fmt"
	"runtime"

	"github.com/Valentin-Kaiser/go-essentials/flag"
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
	ErrorTraceFormat = "%s [%s]"
	// AdvancedTraceFormat is the format for displaying the error message with a stack trace and additional errors
	AdvancedTraceFormat = "%s | %s [%s]"
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
	e.Trace = trace(e, 2)
	return e
}

// NewErrorf creates a new Error instance with the formatted message
// If the error is already of type Error you should use Wrap instead
func NewErrorf(format string, a ...interface{}) Error {
	e := Error{
		Message: fmt.Sprintf(format, a...),
	}
	e.Trace = trace(e, 2)
	return e
}

// Wrap wraps an error and adds a stack trace to it
// Should be used to wrap errors that are of type Error
func Wrap(err error) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(Error); ok {
		e.Trace = trace(e, 2)
		return e
	}
	e := Error{
		Message: err.Error(),
	}
	e.Trace = trace(e, 2)
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
		return fmt.Sprintf(AdvancedTraceFormat, trace, e.Message, errors)
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

// trace generates a stack trace for the error
// It uses runtime.Caller to get the file name and line number
func trace(e Error, level int) []string {
	pc, file, line, ok := runtime.Caller(level)
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
