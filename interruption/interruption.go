// Interruption package provides a function to handle panics in the application.
// defer Handle() should be called at the beginning of the main function or each goroutine
// to ensure that any panics are caught and logged.
package interruption

import (
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/Valentin-Kaiser/go-essentials/flag"
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
