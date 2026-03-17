package cli

import (
	"bufio"
	"fmt"
	"strings"
	"syscall"

	"github.com/openrelik/openrelik-go-client/cmd/cli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var passwordReader = func(fd int) ([]byte, error) {
	return term.ReadPassword(fd)
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}

	cmd.AddCommand(newLoginCmd())
	return cmd
}

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Login to OpenRelik",
		RunE: func(cmd *cobra.Command, args []string) error {
			var server, key string

			fmt.Fprint(cmd.OutOrStdout(), "OpenRelik Server URL (e.g., http://localhost:8710): ")
			scanner := bufio.NewScanner(cmd.InOrStdin())
			if scanner.Scan() {
				server = strings.TrimSpace(scanner.Text())
			}
			if server == "" {
				return fmt.Errorf("server URL is required")
			}

			fmt.Fprint(cmd.OutOrStdout(), "OpenRelik API Key (refresh token): ")
			byteKey, err := passwordReader(int(syscall.Stdin))
			fmt.Fprintln(cmd.OutOrStdout()) // Print a newline after reading the password
			if err != nil {
				return fmt.Errorf("error reading API key: %w", err)
			}
			key = string(byteKey)

			if key == "" {
				return fmt.Errorf("API key is required")
			}

			err = config.SaveSettings(&config.Settings{ServerURL: server})
			if err != nil {
				return fmt.Errorf("error saving settings: %w", err)
			}

			err = config.SaveCredentials(&config.Credentials{APIKey: key})
			if err != nil {
				return fmt.Errorf("error saving credentials: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Successfully logged in!")
			return nil
		},
	}
}

