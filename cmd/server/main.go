package main

import (
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"

	msgstore "github.com/numkem/msgscript/store"
)

func natsUrlByEnv() string {
	if url := os.Getenv("NATS_URL"); url != "" {
		return url
	} else {
		return nats.DefaultURL
	}
}

func main() {
	// Parse command-line flags
	backendFlag := flag.String("backend", msgstore.BACKEND_ETCD_NAME, "Storage backend to use (etcd, sqlite, flatfile)")
	etcdURL := flag.String("etcdurl", "localhost:2379", "URL of etcd server")
	natsURL := flag.String("natsurl", "", "URL of NATS server")
	logLevel := flag.String("log", "info", "Logging level (debug, info, warn, error)")
	flag.Parse()

	// Set up logging
	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.Fatalf("Invalid log level: %v", err)
	}
	log.SetLevel(level)

	// Create the ScriptStore based on the selected backend
	var scriptStore msgstore.ScriptStore
	switch *backendFlag {
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
		log.Fatalf("Unknown backend: %s", *backendFlag)
	}

	// Initialize ScriptExecutor
	scriptExecutor := NewScriptExecutor(scriptStore)

	// Connect to NATS
	if *natsURL == "" {
		*natsURL = natsUrlByEnv()
	}
	nc, err := nats.Connect(*natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	log.Info("Starging message watch...")

	// Set up a message handler
	_, err = nc.Subscribe(">", func(msg *nats.Msg) {
		log.Debugf("Received message on subject: %s", msg.Subject)

		if strings.HasPrefix(msg.Subject, "_INBOX.") {
			log.Debugf("Ignoring reply subject %s", msg.Subject)
			return
		}

		// Handle the message by invoking the corresponding Lua script
		scriptExecutor.HandleMessage(msg.Subject, msg.Data, func(reply string) {
			// Send a reply if the message has a reply subject
			if msg.Reply != "" {
				nc.Publish(msg.Reply, []byte(reply))
			}
		})
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to NATS subjects: %v", err)
	}

	// Listen for system interrupts for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info("Received shutdown signal, stopping server...")
	scriptExecutor.Stop()
}
