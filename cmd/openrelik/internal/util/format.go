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

// FprintStruct nicely prints the fields of a struct or a slice of structs to the given writer.
func FprintStruct(w io.Writer, s interface{}) {
	v := reflect.ValueOf(s)

	// Handle pointer
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			fmt.Fprintf(w, "<nil>\n")
			return
		}
		v = v.Elem()
	}

	// Handle slice
	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			FprintStruct(w, v.Index(i).Interface())
			if i < v.Len()-1 {
				fmt.Fprintln(w, "---")
			}
		}
		return
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

func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// FprintJSON prints the given interface as a pretty-printed JSON string.
func FprintJSON(w io.Writer, s interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(s)
}
