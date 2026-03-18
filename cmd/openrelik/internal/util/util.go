package util

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
)

// PrintStruct nicely prints the fields of a struct to stdout.
func PrintStruct(s interface{}) {
	FprintStruct(os.Stdout, s)
}

// FprintStruct nicely prints the fields of a struct to the given writer.
func FprintStruct(w io.Writer, s interface{}) {
	v := reflect.ValueOf(s)

	// Handle pointer to struct
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			fmt.Fprintf(w, "<nil>\n")
			return
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		fmt.Fprintf(w, "%v\n", s)
		return
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		// Handle unexported fields
		if !field.IsExported() {
			continue
		}

		var val interface{}
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				val = "<nil>"
			} else {
				val = value.Elem().Interface()
			}
		} else {
			val = value.Interface()
		}

		fmt.Fprintf(w, "%-20s: %v\n", field.Name, val)
	}
}

// FprintJSON prints the given interface as a pretty-printed JSON string.
func FprintJSON(w io.Writer, s interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(s)
}
