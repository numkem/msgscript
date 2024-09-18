package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

func validateArgIsPath(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("a single path to a lua file is required")
	}

	if _, err := os.Stat(args[0]); err != nil {
		return fmt.Errorf("invalid filename %s: %v", args[0], err)
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
	scriptStore, err := msgstore.StoreByName(cmd.Flag("backend").Value.String(), cmd.Flag("etcdurls").Value.String())
	if err != nil {
		log.Errorf("failed to get script store: %v", err)
	}
	subject := cmd.Flag("subject").Value.String()
	name := cmd.Flag("name").Value.String()

	// Try to read the file to see if we can find headers
	r := new(script.ScriptReader)
	err = r.ReadFile(args[0])
	if err != nil {
		log.Errorf("failed to read the script file %s: %v", args[0], err)
		return
	}
	if subject == "" {
		if r.Script.Subject == "" {
			log.Errorf("subject is required")
			return
		}

		subject = r.Script.Subject
	}
	if name == "" {
		if r.Script.Name == "" {
			log.Errorf("name is required")
			return
		}

		name = r.Script.Name
	}

	// Add the script to etcd under the given subject
	err = scriptStore.AddScript(cmd.Context(), subject, name, string(r.Script.Content))
	if err != nil {
		log.Fatalf("Failed to add script to etcd: %v", err)
	}

	fmt.Printf("Script added successfully for subject %s named %s \n", subject, name)
}
