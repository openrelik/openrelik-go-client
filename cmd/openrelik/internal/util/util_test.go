package util

import (
	"bytes"
	"strings"
	"testing"
)

type testStruct struct {
	Name    string
	Age     int
	private string
	Ptr     *string
	NilPtr  *string
}

func TestFprintStruct(t *testing.T) {
	ptrVal := "pointer value"
	ts := testStruct{
		Name:    "Test User",
		Age:     30,
		private: "secret",
		Ptr:     &ptrVal,
		NilPtr:  nil,
	}

	tests := []struct {
		name     string
		input    interface{}
		contains []string
		excludes []string
	}{
		{
			name:  "simple struct",
			input: ts,
			contains: []string{
				"Name                : Test User",
				"Age                 : 30",
				"Ptr                 : pointer value",
				"NilPtr              : <nil>",
			},
			excludes: []string{
				"private",
				"secret",
			},
		},
		{
			name:  "pointer to struct",
			input: &ts,
			contains: []string{
				"Name                : Test User",
				"Age                 : 30",
				"Ptr                 : pointer value",
				"NilPtr              : <nil>",
			},
			excludes: []string{
				"private",
				"secret",
			},
		},
		{
			name:  "nil struct pointer",
			input: (*testStruct)(nil),
			contains: []string{
				"<nil>",
			},
		},
		{
			name:  "non-struct input",
			input: "not a struct",
			contains: []string{
				"not a struct",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			FprintStruct(&buf, tt.input)
			output := buf.String()

			for _, c := range tt.contains {
				if !strings.Contains(output, c) {
					t.Errorf("expected output to contain %q, but it was %q", c, output)
				}
			}

			for _, e := range tt.excludes {
				if strings.Contains(output, e) {
					t.Errorf("expected output to NOT contain %q, but it was %q", e, output)
				}
			}
		})
	}
}

func TestFprintJSON(t *testing.T) {
	ts := testStruct{
		Name: "Test User",
		Age:  30,
	}

	var buf bytes.Buffer
	if err := FprintJSON(&buf, ts); err != nil {
		t.Fatalf("FprintJSON failed: %v", err)
	}

	output := buf.String()
	expected := `{
  "Name": "Test User",
  "Age": 30,
  "Ptr": null,
  "NilPtr": null
}
`
	if output != expected {
		t.Errorf("expected JSON:\n%q\ngot:\n%q", expected, output)
	}
}
