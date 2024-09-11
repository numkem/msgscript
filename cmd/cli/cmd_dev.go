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
	devCmd.MarkFlagRequired("subject")

	devCmd.PersistentFlags().StringP("name", "n", "", "The name of the script in the backend")
	devCmd.MarkFlagRequired("name")

	devCmd.PersistentFlags().StringP("actualSubject", "x", "", "The actual subject nats would use in case the subject is a wildcard")
	devCmd.PersistentFlags().StringP("payload", "p", "", "Path to or actual payload to send to the function")
}

func devCmdRun(cmd *cobra.Command, args []string) {
	store := msgstore.NewDevStore()
	scriptExecutor := script.NewScriptExecutor(store)

	subject := cmd.Flag("subject").Value.String()
	actualSubject := cmd.Flag("actualSubject").Value.String()
	name := cmd.Flag("name").Value.String()
	if actualSubject == "" {
		actualSubject = subject
	}

	// Add the given script to the store
	scriptContent, err := os.ReadFile(args[0])
	if err != nil {
		log.Errorf("failed to read script file %s: %v", args[0], err)
		return
	}
	log.Debug("read lua file")
	store.AddScript(actualSubject, name, string(scriptContent))

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
		"subject":      actualSubject,
		"payload":      string(payload),
		"lua_filename": args[0],
	}

	var wg sync.WaitGroup
	wg.Add(1)
	log.WithFields(fields).Debug("running the function")
	scriptExecutor.HandleMessage(actualSubject, payload, func(reply string) {
		log.Debug("function executed")
		cmd.Println(reply)
		wg.Done()
	})

	wg.Wait()
	scriptExecutor.Stop()
}