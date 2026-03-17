package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMeCmd(t *testing.T) {
	// Mock API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/users/me/" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"id": 1, "username": "testuser", "display_name": "Test User", "is_admin": true}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Setup command
	root := NewRootCmd()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)

	// Set API key via environment variable since --key was removed
	os.Setenv("OPENRELIK_API_KEY", "test-key")
	defer os.Unsetenv("OPENRELIK_API_KEY")

	// Set flags to point to mock server
	root.SetArgs([]string{"users", "me", "--server", server.URL})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	output := buf.String()
	expectedFields := []string{
		"ID                  : 1",
		"Username            : testuser",
		"DisplayName         : Test User",
		"IsAdmin             : true",
	}

	for _, field := range expectedFields {
		if !strings.Contains(output, field) {
			t.Errorf("expected output to contain %q, but it was %q", field, output)
		}
	}
}
