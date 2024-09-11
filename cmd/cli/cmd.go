package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "msgscript",
	Short: "msgscript CLI",
	Long:  `msgscript is a command line interface for managing scripts for the msgscript-server`,
}

func init() {
	if os.Getenv("DEBUG") != "" {
		log.SetLevel(log.DebugLevel)
	}

	rootCmd.PersistentFlags().StringP("log-level", "l", "info", "set the logger to this log level")
	rootCmd.PersistentFlags().StringP("etcdurls", "e", "localhost:2379", "Endpoints to connect to etcd")
	rootCmd.PersistentFlags().StringP("natsurl", "u", "nats://localhost:4222", "NATS url to reach")
	rootCmd.PersistentFlags().StringP("backend", "b", "etcd", "The name of the backend to use to manipulate the scripts")
}

func Execute() error {
	return rootCmd.Execute()
}
