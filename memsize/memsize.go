package memsize

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"reflect"

	"github.com/Valentin-Kaiser/go-essentials/apperror"
)

// SizeOf returns the number of bytes that 'value' takes up in memory, without using reflect or unsafe
// this functions has minor overhead for the encoding process.
func SizeOf(value interface{}) (int64, error) {
	if interfaceIsNullPointer(value) {
		return 0, nil
	}
	buff := new(bytes.Buffer)
	err := gob.NewEncoder(buff).Encode(value)
	if err != nil {
		return 0, apperror.Wrap(err)
	}
	return int64(buff.Len()), nil
}

// ToByteString returns the number of bytes as a string with a unit
func ToByteString(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

// interfaceIsNullPointer returns true if the interface is nil or if the interface is a pointer, map, array, chan, or slice and is nil
func interfaceIsNullPointer(i interface{}) bool {
	if i == nil {
		return true
	}
	switch reflect.TypeOf(i).Kind() {
	case reflect.Ptr, reflect.Map, reflect.Array, reflect.Chan, reflect.Slice:
		return reflect.ValueOf(i).IsNil()
	}
	return false
}
