package main

import (
	"github.com/spf13/cobra"

	msgstore "github.com/numkem/msgscript/store"
)

var rmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove an existing script",
	Run:   rmCmdRun,
}

func init() {
	rootCmd.AddCommand(rmCmd)

	rmCmd.PersistentFlags().StringP("subject", "s", "", "The NATS subject to respond to")
	rmCmd.PersistentFlags().StringP("name", "n", "", "The name of the script in the backend")
	rmCmd.MarkFlagRequired("subject")
	rmCmd.MarkFlagRequired("name")
}

func rmCmdRun(cmd *cobra.Command, args []string) {
	scriptStore, err := msgstore.StoreByName(cmd.Flag("backend").Value.String(), cmd.Flag("etcdurls").Value.String())
	if err != nil {
		cmd.PrintErrf("failed to get script store: %v", err)
		return
	}
	subject := cmd.Flag("subject").Value.String()
	name := cmd.Flag("name").Value.String()

	err = scriptStore.DeleteScript(cmd.Context(), subject, name)
	if err != nil {
		cmd.PrintErrf("failed to remove script: %v", err)
	}

	cmd.Printf("Script removed\n")
}
