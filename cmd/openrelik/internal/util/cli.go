package util

import "fmt"

// SliceArgs splits a slice of arguments by the first found delimiter.
// It returns a slice of argument segments and the delimiter that was used.
// Mixing delimiters is not allowed.
func SliceArgs(firstArg string, args []string, delimiters ...string) ([][]string, string, error) {
	var segments [][]string
	var current []string
	var delimiter string

	current = append(current, firstArg)
	for _, arg := range args {
		isDelimiter := false
		for _, d := range delimiters {
			if arg == d {
				isDelimiter = true
				if delimiter != "" && delimiter != arg {
					return nil, "", fmt.Errorf("cannot mix %s and %s in the same command", delimiter, arg)
				}
				delimiter = arg
				break
			}
		}

		if isDelimiter {
			if len(current) > 0 {
				segments = append(segments, current)
			}
			current = []string{}
		} else {
			current = append(current, arg)
		}
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}

	return segments, delimiter, nil
}
