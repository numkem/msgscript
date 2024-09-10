package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	log "github.com/sirupsen/logrus"

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

func main() {
	// Define CLI flags
	backend := flag.String("backend", "etcd", "Backend to use to store the lua script")
	etcdURL := flag.String("etcdurl", "localhost:2379", "URL of etcd server")
	logLevel := flag.String("level", "info", "Logging level (debug, info, warn, error)")
	name := flag.String("name", "", "Name of the script")
	subject := flag.String("subject", "", "Subject for which the Lua script will be triggered (required)")
	flag.Parse()

	// Set up logging
	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.Fatalf("Invalid log level: %v", err)
	}
	log.SetLevel(level)

	// Check if the subject flag is provided
	if *subject == "" {
		log.Fatal("The --subject flag is required.")
	}
	// Check that the name flag is required
	if *name == "" {
		log.Fatal("The --name flag is required")
	}

	var scriptStore msgstore.ScriptStore
	switch *backend {
	case msgstore.BACKEND_ETCD_NAME:
		scriptStore, err = msgstore.NewEtcdScriptStore(*etcdURL)
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
		log.Fatalf("Unknown backend: %s", *backend)
	}

	// The first argument after the flags is the path to the Lua script
	if len(flag.Args()) < 1 {
		log.Fatalf("Please provide the path to the Lua script file.")
	}
	luaFilePath := flag.Args()[0]

	// Load the Lua script from the provided file path
	script, err := loadLuaScript(luaFilePath)
	if err != nil {
		log.Fatalf("Error loading Lua script: %v", err)
	}

	// Add the script to etcd under the given subject
	err = scriptStore.AddScript(*subject, *name, string(script))
	if err != nil {
		log.Fatalf("Failed to add script to etcd: %v", err)
	}

	fmt.Printf("Script added successfully for subject %s named %s \n", *subject, *name)
}
