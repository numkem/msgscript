package main

import (
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"

	msgplugin "github.com/numkem/msgscript/plugins"
	"github.com/numkem/msgscript/script"
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
	httpPort := flag.Int("port", DEFAULT_HTTP_PORT, "HTTP port to bind to")
	pluginDir := flag.String("plugin", "", "Plugin directory")
	flag.Parse()

	// Set up logging
	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.Fatalf("Invalid log level: %v", err)
	}
	log.SetLevel(level)

	// Create the ScriptStore based on the selected backend
	scriptStore, err := msgstore.StoreByName(*backendFlag, *etcdURL)
	if err != nil {
		log.Fatalf("failed to initialize the script store: %v", err)
	}

	// Connect to NATS
	if *natsURL == "" {
		*natsURL = natsUrlByEnv()
	}
	nc, err := nats.Connect(*natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// Initialize ScriptExecutor
	var plugins []msgplugin.PreloadFunc
	if *pluginDir != "" {
		plugins, err = msgplugin.ReadPluginDir(*pluginDir)
		if err != nil {
			log.Fatalf("failed to read plugins: %v", err)
		}
	}
	scriptExecutor := script.NewScriptExecutor(scriptStore, plugins, nc)

	log.Info("Starting message watch...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up a message handler
	_, err = nc.Subscribe(">", func(msg *nats.Msg) {
		log.Debugf("Received message on subject: %s", msg.Subject)

		if strings.HasPrefix(msg.Subject, "_INBOX.") {
			log.Debugf("Ignoring reply subject %s", msg.Subject)
			return
		}

		// Handle the message by invoking the corresponding Lua script
		scriptExecutor.HandleMessage(ctx, msg.Subject, msg.Data, func(reply string) {
			// Send a reply if the message has a reply subject
			if msg.Reply != "" {
				nc.Publish(msg.Reply, []byte(reply))
			}
		})
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to NATS subjects: %v", err)
	}

	// Start HTTP Server
	go runHTTP(*httpPort, *natsURL)

	// Listen for system interrupts for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	cancel()

	log.Info("Received shutdown signal, stopping server...")
	scriptExecutor.Stop()
}
