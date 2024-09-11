package main

import (
	"fmt"
	"io"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	msgstore "github.com/numkem/msgscript/store"
)

func loadLuaScript(filename string) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open lua script %s: %v", filename, err)
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read content of lua script %s: %v", filename, err)
	}

	return content, nil
}

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
	addCmd.MarkFlagRequired("subject")

	addCmd.PersistentFlags().StringP("name", "n", "", "The name of the script in the backend")
	addCmd.MarkFlagRequired("name")
}

func addCmdRun(cmd *cobra.Command, args []string) {
	var scriptStore msgstore.ScriptStore
	var err error

	backend := cmd.Flag("backend").Value.String()
	switch backend {
	case msgstore.BACKEND_ETCD_NAME:
		scriptStore, err = msgstore.NewEtcdScriptStore(cmd.Flag("etcdurls").Value.String())
		if err != nil {
			log.Fatalf("Failed to initialize etcd store: %v", err)
		}
	// case BACKEND_SQLITE_NAME:
	// 	// Initialize SQLite backend (placeholder for now)
	// 	scriptStore, err = msgstore.NewSqliteScriptStore("path/to/db.sqlite") // implement this
	// 	if err != nil {
	// 		log.Fatalf("Failed to initialize SQLite store: %v", err)
	// 	}
	// case BACKEND_FILE_NAME:
	// 	// Initialize flat file backend (placeholder for now)
	// 	scriptStore, err = msgstore.NewFileScriptStore("path/to/scripts") // implement this
	// 	if err != nil {
	// 		log.Fatalf("Failed to initialize file store: %v", err)
	// 	}
	default:
		log.Fatalf("Unknown backend: %s", backend)
	}

	luaFilePath := args[0]

	// Load the Lua script from the provided file path
	script, err := loadLuaScript(luaFilePath)
	if err != nil {
		log.Fatalf("Error loading Lua script: %v", err)
	}

	// Add the script to etcd under the given subject
	subject := cmd.Flag("subject").Value.String()
	name := cmd.Flag("name").Value.String()
	err = scriptStore.AddScript(subject, name, string(script))
	if err != nil {
		log.Fatalf("Failed to add script to etcd: %v", err)
	}

	fmt.Printf("Script added successfully for subject %s named %s \n", subject, name)
}
