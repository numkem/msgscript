package main

import (
	"os"

	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	msgplugin "github.com/numkem/msgscript/plugins"
	"github.com/numkem/msgscript/script"
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

func natsUrlByEnv() string {
	if url := os.Getenv("NATS_URL"); url != "" {
		return url
	} else {
		return nats.DefaultURL
	}
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

	scriptExecutor := script.NewScriptExecutor(store, plugins, nil)

	subject := cmd.Flag("subject").Value.String()
	name := cmd.Flag("name").Value.String()

	// Try to read the file to see if we can find headers
	s := new(script.Script)
	err = s.ReadFile(args[0])
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
	err = store.AddScript(cmd.Context(), subject, name, string(s.Content))
	if err != nil {
		cmd.PrintErrf("failed to add script to store: %v\n")
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

	m := &script.Message{
		Payload: payload,
		Subject: subject,
	}
	scriptExecutor.HandleMessage(cmd.Context(), m, func(r *script.Reply) {
		fields := log.Fields{"subject": subject}
		log.WithFields(fields).Debug("script replied")

		j, err := r.JSON()
		if err != nil {
			log.WithFields(fields).Errorf("failed to Unmarshal reply: %v", err)
		}

		cmd.Printf("Result: %v\n", string(j))
		stopChan <- struct{}{}
	})

	<-stopChan
	scriptExecutor.Stop()
}
