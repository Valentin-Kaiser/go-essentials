// The flag package provides a simple way to define and parse command-line flags
// for a Go application. It uses the pflag library to handle the flags and provides
// a simple API to register and parse flags.
//
// The package defines a set of default flags, including:
// - `Path`: the default path for the application
// - `Help`: a flag to print the help page
// - `Version`: a flag to print the software version
// - `Debug`: a flag to enable debug mode
//
// The `Init` function initializes the flags and parses them. It should be called
// in the main package of the application.
//
// To register a new flag, use the `RegisterFlag` function. This function takes
// the name of the flag, a pointer to the value to be set, and a usage string.
//
// Example usage:
//
// ```go
//
//		package main
//
//		import (
//			"github.com/Valentin-Kaiser/go-essentials/flag"
//			"fmt"
//		)
//
//	 	var (
//			// Define a new flag
//			CustomFlag string
//		)
//
//		func main() {
//			// Register the new flag
//			flag.RegisterFlag("custom", &CustomFlag, "A custom flag for demonstration")
//
//			// Initialize the flags
//			flag.Init()
//
//			// Print the value of the custom flag
//			fmt.Println("Custom Flag Value:", CustomFlag)
//		}
//
// ´´´
package flag

import (
	"fmt"
	"reflect"

	"github.com/spf13/pflag"
)

var (
	Path    string
	Help    bool
	Version bool
	Debug   bool
)

func init() {
	pflag.StringVar(&Path, "path", "./data", "Sets the application default path")
	pflag.BoolVar(&Help, "help", false, "Prints the help page")
	pflag.BoolVar(&Version, "version", false, "Prints the software version")
	pflag.BoolVar(&Debug, "debug", false, "Enables debug mode")
}

// Init initializes the flags and parses them
// It should be called in the main package of the application
func Init() {
	pflag.Parse()

	if Help {
		Print()
		return
	}
}

// Print prints the help page and the default values of the flags
func Print() {
	fmt.Println("Usage:")
	pflag.PrintDefaults()
}

// RegisterFlag registers a new flag with the given name, value and usage
// It panics if the flag is already registered or if the value is not a pointer
func RegisterFlag(name string, value interface{}, usage string) {
	if pflag.Lookup(name) != nil {
		panic(fmt.Sprintf("flag %s already registered", name))
	}

	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("flag %s must be a pointer", name))
	}

	if val.IsNil() {
		panic(fmt.Sprintf("flag %s must not be nil", name))
	}

	switch v := value.(type) {
	case *string:
		pflag.StringVar(v, name, *v, usage)
	case *bool:
		pflag.BoolVar(v, name, *v, usage)
	case *int:
		pflag.IntVar(v, name, *v, usage)
	case *int8:
		pflag.Int8Var(v, name, *v, usage)
	case *int16:
		pflag.Int16Var(v, name, *v, usage)
	case *int32:
		pflag.Int32Var(v, name, *v, usage)
	case *int64:
		pflag.Int64Var(v, name, *v, usage)
	case *uint:
		pflag.UintVar(v, name, *v, usage)
	case *uint8:
		pflag.Uint8Var(v, name, *v, usage)
	case *uint16:
		pflag.Uint16Var(v, name, *v, usage)
	case *uint32:
		pflag.Uint32Var(v, name, *v, usage)
	case *uint64:
		pflag.Uint64Var(v, name, *v, usage)
	case *float32:
		pflag.Float32Var(v, name, *v, usage)
	case *float64:
		pflag.Float64Var(v, name, *v, usage)
	default:
		panic(fmt.Sprintf("unsupported type %T", v))
	}
}
