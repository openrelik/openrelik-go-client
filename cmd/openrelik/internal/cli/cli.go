package cli

import (
	"fmt"
	"os"

	"github.com/openrelik/openrelik-go-client"
	"github.com/openrelik/openrelik-go-client/cmd/cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	serverURL string
	apiKey    string
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "openrelik",
		Short: "OpenRelik CLI client",
		Long:  `A command line tool to interact with the OpenRelik API`,
	}

	cmd.PersistentFlags().StringVarP(&serverURL, "server", "s", "", "OpenRelik server URL (e.g. http://localhost:8710)")

	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newUsersCmd())

	return cmd
}

func newClient() (*openrelik.Client, error) {
	s := serverURL
	if s == "" {
		s = os.Getenv("OPENRELIK_SERVER_URL")
	}
	if s == "" {
		if settings, err := config.LoadSettings(); err == nil {
			s = settings.ServerURL
		}
	}

	k := apiKey
	if k == "" {
		k = os.Getenv("OPENRELIK_API_KEY")
	}
	if k == "" {
		if creds, err := config.LoadCredentials(); err == nil {
			k = creds.APIKey
		}
	}

	if s == "" {
		return nil, fmt.Errorf("server URL is required (use --server, OPENRELIK_SERVER_URL env var, or run 'openrelik auth login')")
	}
	if k == "" {
		return nil, fmt.Errorf("API key is required (use OPENRELIK_API_KEY env var, or run 'openrelik auth login')")
	}

	return openrelik.NewClient(s, k)
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

