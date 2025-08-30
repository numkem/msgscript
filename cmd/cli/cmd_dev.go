package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/numkem/msgscript/executor"
	msgplugin "github.com/numkem/msgscript/plugins"
	scriptLib "github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Args:  validateArgIsPath,
	Short: "Executes the script locally like how the server would",
	Run:   devCmdRun,
}

func init() {
	rootCmd.AddCommand(devCmd)

	devCmd.PersistentFlags().StringP("subject", "s", "", "The NATS subject to respond to")
	devCmd.PersistentFlags().StringP("name", "n", "", "The name of the script in the backend")
	devCmd.PersistentFlags().StringP("input", "i", "", "Path to or actual payload to send to the function")
	devCmd.PersistentFlags().StringP("library", "l", "", "Path to a folder containing libraries to load for the function")
	devCmd.PersistentFlags().StringP("pluginDir", "p", "", "Path to a folder with plugins")

	devCmd.MarkFlagRequired("subject")
	devCmd.MarkFlagRequired("name")
	devCmd.MarkFlagRequired("input")
}

func devCmdRun(cmd *cobra.Command, args []string) {
	store, err := msgstore.NewDevStore(cmd.Flag("library").Value.String())
	if err != nil {
		cmd.PrintErrf("failed to create store: %v\n", err)
		return
	}

	var plugins []msgplugin.PreloadFunc
	if path := cmd.Flag("pluginDir").Value.String(); path != "" {
		plugins, err = msgplugin.ReadPluginDir(path)
		if err != nil {
			cmd.PrintErrf("failed to read plugins: %v\n", err)
			return
		}
	}

	subject := cmd.Flag("subject").Value.String()
	name := cmd.Flag("name").Value.String()

	// Try to read the file to see if we can find headers
	s, err := scriptLib.ReadFile(args[0])
	if err != nil {
		log.Errorf("failed to read the script file %s: %v\n", args[0], err)
		return
	}

	if subject == "" {
		if s.Subject == "" {
			cmd.PrintErrf("subject is required\n")
			return
		}

		subject = s.Subject
	}
	if name == "" {
		if s.Name == "" {
			cmd.PrintErrf("name is required\n")
			return
		}

		name = s.Name
	}

	// Add the given script to the store
	err = store.AddScript(cmd.Context(), subject, name, s.Content)
	if err != nil {
		cmd.PrintErrf("failed to add script to store: %v\n", err)
		return
	}

	payloadFlag := cmd.Flag("input").Value.String()
	var payload []byte
	// Check if the payload is a path to a file
	if _, err := os.Stat(payloadFlag); err == nil {
		content, err := os.ReadFile(payloadFlag)
		if err != nil {
			cmd.PrintErrf("failed to read payload file %s: %v\n", payloadFlag, err)
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

	stopChan := make(chan struct{}, 1)
	log.WithFields(fields).Debug("running the function")

	m := &executor.Message{
		Payload:  payload,
		Subject:  subject,
		Executor: cmd.Flag("executor").Value.String(),
	}

	replyFunc := func(r *executor.Reply) {
		fields := log.Fields{"subject": subject}
		log.WithFields(fields).Debug("script replied")

		j, err := r.JSON()
		if err != nil {
			log.WithFields(fields).Errorf("failed to Unmarshal reply: %v", err)
		}

		cmd.Printf("Result: %s\n", string(j))
		stopChan <- struct{}{}
	}

	executors := executor.StartAllExecutors(cmd.Context(), store, plugins, nil)
	exec, err := executor.ExecutorByName(m.Executor, executors)
	if err != nil {
		cmd.PrintErrf("failed to get executor for message: %v", err)
		stopChan <- struct{}{}
		return
	}

	exec.HandleMessage(cmd.Context(), m, replyFunc)

	<-stopChan

	executor.StopAllExecutors(executors)
}
