package main

import (
	"os"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Args:  validateArgIsPath,
	Short: "Executes the script locally like how the server would do",
	Run:   devCmdRun,
}

func init() {
	rootCmd.AddCommand(devCmd)

	devCmd.PersistentFlags().StringP("subject", "s", "", "The NATS subject to respond to")
	devCmd.PersistentFlags().StringP("name", "n", "", "The name of the script in the backend")
	devCmd.PersistentFlags().StringP("payload", "p", "", "Path to or actual payload to send to the function")
}

func devCmdRun(cmd *cobra.Command, args []string) {
	store := msgstore.NewDevStore()
	scriptExecutor := script.NewScriptExecutor(store)

	subject := cmd.Flag("subject").Value.String()
	name := cmd.Flag("name").Value.String()

	// Try to read the file to see if we can find headers
	r := new(script.ScriptReader)
	err := r.ReadFile(args[0])
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

	// Add the given script to the store
	store.AddScript(cmd.Context(), subject, name, string(r.Script.Content))

	payloadFlag := cmd.Flag("payload").Value.String()
	var payload []byte
	// Check if the payload is a path to a file
	if _, err := os.Stat(payloadFlag); err == nil {
		content, err := os.ReadFile(payloadFlag)
		if err != nil {
			log.Errorf("failed to read payload file %s: %v", payloadFlag, err)
			return
		}

		payload = content
	} else {
		payload = []byte(payloadFlag)
	}
	log.Debug("loaded payload")

	fields := log.Fields{
		"subject":      subject,
		"payload":      string(payload),
		"lua_filename": args[0],
	}

	var wg sync.WaitGroup
	wg.Add(1)
	log.WithFields(fields).Debug("running the function")
	scriptExecutor.HandleMessage(cmd.Context(), subject, payload, func(reply string) {
		log.Debug("function executed")
		cmd.Println(reply)
		wg.Done()
	})

	wg.Wait()
	scriptExecutor.Stop()
}
