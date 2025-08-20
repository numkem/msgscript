package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	scriptLib "github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

func validateArgIsPath(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("a single path to a lua file is required")
	}

	if _, err := os.Stat(args[0]); err != nil {
		return fmt.Errorf("invalid filename %s: %w", args[0], err)
	}

	return nil
}

var addCmd = &cobra.Command{
	Use:   "add",
	Args:  validateArgIsPath,
	Short: "Add a script to the backend by reading the provided lua file",
	Run:   addCmdRun,
}

func init() {
	rootCmd.AddCommand(addCmd)

	addCmd.PersistentFlags().StringP("subject", "s", "", "The NATS subject to respond to")
	addCmd.PersistentFlags().StringP("name", "n", "", "The name of the script in the backend")
}

func addCmdRun(cmd *cobra.Command, args []string) {
	scriptStore, err := msgstore.StoreByName(cmd.Flag("backend").Value.String(), cmd.Flag("etcdurls").Value.String(), "", "")
	if err != nil {
		cmd.PrintErrf("failed to get script store: %w", err)
		return
	}
	subject := cmd.Flag("subject").Value.String()
	name := cmd.Flag("name").Value.String()

	// Try to read the file to see if we can find headers
	s, err := scriptLib.ReadFile(args[0])
	if err != nil {
		cmd.PrintErrf("failed to read the script file %s: %w", args[0], err)
		return
	}
	if subject == "" {
		if s.Subject == "" {
			cmd.PrintErrf("subject is required")
			return
		}

		subject = s.Subject
	}
	if name == "" {
		if s.Name == "" {
			cmd.PrintErrf("name is required")
			return
		}

		name = s.Name
	}

	// Add the script to etcd under the given subject
	err = scriptStore.AddScript(cmd.Context(), subject, name, s.Content)
	if err != nil {
		cmd.PrintErrf("Failed to add script to etcd: %w", err)
		return
	}

	cmd.Printf("Script added successfully for subject %s named %s \n", subject, name)
}
