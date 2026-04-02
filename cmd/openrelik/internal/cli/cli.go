package cli

import (
	"fmt"
	"os"

	"github.com/openrelik/openrelik-go-client"
	"github.com/openrelik/openrelik-go-client/cmd/cli/internal/config"
	"github.com/openrelik/openrelik-go-client/cmd/cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	serverURL    string
	apiKey       string
	outputFormat string
	quiet        bool
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:              "openrelik",
		Short:            "OpenRelik CLI client",
		Long:             `A command line tool to interact with the OpenRelik API`,
		TraverseChildren: true,
		SilenceErrors:    true,
		SilenceUsage:     true,
	}

	cmd.CompletionOptions.DisableDefaultCmd = true

	cmd.Flags().StringVarP(&outputFormat, "format", "f", "text", "Output format (text, json)")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress all output")

	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newUsersCmd())
	cmd.AddCommand(newFoldersCmd())
	cmd.AddCommand(newFilesCmd())
	cmd.AddCommand(newWorkersCmd())
	cmd.AddCommand(newWorkflowsCmd())
	cmd.AddCommand(newRunCmd())

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
		return nil, fmt.Errorf("server URL is required (use OPENRELIK_SERVER_URL env var, or run 'openrelik auth login')")
	}
	if k == "" {
		return nil, fmt.Errorf("API key is required (use OPENRELIK_API_KEY env var, or run 'openrelik auth login')")
	}

	return openrelik.NewClient(s, k)
}

// formatAndPrint outputs the result in the requested format.
func formatAndPrint(cmd *cobra.Command, result interface{}) error {
	if quiet {
		return nil
	}
	switch outputFormat {
	case "json":
		return util.FprintJSON(cmd.OutOrStdout(), result)
	default:
		util.FprintStruct(cmd.OutOrStdout(), result)
		return nil
	}
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
