package util

import (
	"bytes"
	"strings"
	"testing"
	"time"
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

func TestProgressTracker(t *testing.T) {
	var buf bytes.Buffer
	totalBytes := int64(100)
	pt := NewProgressTracker(&buf, totalBytes, "Downloading")

	// Test initial display
	pt.display()
	output := buf.String()
	if !strings.Contains(output, "Downloading") {
		t.Errorf("expected output to contain 'Downloading', got %q", output)
	}
	if !strings.Contains(output, "0%") {
		t.Errorf("expected output to contain '0%%', got %q", output)
	}

	// Test update
	buf.Reset()
	pt.Update(50)
	output = buf.String()
	if !strings.Contains(output, "50%") {
		t.Errorf("expected output to contain '50%%', got %q", output)
	}

	// Test ETA calculation (mocking samples and speed)
	now := time.Now()
	pt.currentBytes = 50
	pt.samples = []progressSample{
		{timestamp: now.Add(-10 * time.Second), bytes: 0},
		{timestamp: now, bytes: 50},
	}
	pt.lastETACalc = now.Add(-2 * time.Second) // Force recalculation
	// Speed will be 50 / 10 = 5 bytes/s
	// Remaining is 50 bytes, so ETA should be 10s

	buf.Reset()
	pt.display()
	output = buf.String()
	if !strings.Contains(output, "ETA 10s") {
		t.Errorf("expected output to contain 'ETA 10s', got %q", output)
	}

	// Test finish
	buf.Reset()
	pt.Finish()
	output = buf.String()
	if output != "\n" {
		t.Errorf("expected newline on finish, got %q", output)
	}
}
