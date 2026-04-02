package cli

import (
	"github.com/spf13/cobra"
)

func newUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage users",
	}

	cmd.AddCommand(newMeCmd())
	return cmd
}

func newMeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Get current user profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient()
			if err != nil {
				return err
			}

			user, _, err := client.Users().GetMe(cmd.Context())
			if err != nil {
				return err
			}

			return formatAndPrint(cmd, user)
		},
	}
}
