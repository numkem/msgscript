package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"

	"github.com/numkem/msgscript"
	msgplugin "github.com/numkem/msgscript/plugins"
	"github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

func main() {
	// Parse command-line flags
	backendName := flag.String("backend", msgstore.BACKEND_FILE_NAME, "Storage backend to use (etcd, sqlite, flatfile)")
	etcdURL := flag.String("etcdurl", "localhost:2379", "URL of etcd server")
	natsURL := flag.String("natsurl", "", "URL of NATS server")
	logLevel := flag.String("log", "info", "Logging level (debug, info, warn, error)")
	httpPort := flag.Int("port", DEFAULT_HTTP_PORT, "HTTP port to bind to")
	pluginDir := flag.String("plugin", "", "Plugin directory")
	libraryDir := flag.String("library", "", "Library directory")
	scriptDir := flag.String("script", ".", "Script directory")
	flag.Parse()

	// Set up logging
	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.Fatalf("Invalid log level: %v", err)
	}
	log.SetLevel(level)

	// Create the ScriptStore based on the selected backend
	scriptStore, err := msgstore.StoreByName(*backendName, *etcdURL, *scriptDir, *libraryDir)
	if err != nil {
		log.Fatalf("failed to initialize the script store: %v", err)
	}
	log.Infof("Starting %s backend", *backendName)

	if *natsURL == "" {
		*natsURL = msgscript.NatsUrlByEnv()

		if *natsURL == "" {
			// nats isn't provided, we can start an embeded one
			log.Info("Starting embeded NATS server...")
			ns, err := natsserver.NewServer(&natsserver.Options{
				Host: "127.0.0.1",
				Port: 9222,
			})
			if err != nil {
				log.Fatalf("failed to start embeded NATS server: %v", err)
			}

			go ns.Start()
			*natsURL = ns.ClientURL()

			for {
				if ns.ReadyForConnections(1 * time.Second) {
					log.Info("NATS server started")
					break
				}

				log.Info("Waiting for embeded NATS server to start...")
				time.Sleep(1 * time.Second)
			}
		}
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

		m := new(script.Message)
		err = json.Unmarshal(msg.Data, m)
		// if the payload isn't a JSON Message, take it as a whole
		if err != nil {
			log.Errorf("failed to parse message: %v", err)
		}

		// The above unmarshalling only applies to the structure of the JSON.
		// Even if you feed it another JSON where none of the keys matches,
		// it will just end up being an empty struct
		if m.Payload == nil {
			m = &script.Message{
				Subject: msg.Subject,
				Payload: msg.Data,
			}
		}

		// Handle the message by invoking the corresponding Lua script
		scriptExecutor.HandleMessage(ctx, m, func(r *script.Reply) {
			fields := log.Fields{
				"Subject": m.Subject,
				"URL":     m.URL,
				"Method":  m.Method,
			}

			if r.Error != "" {
				log.WithFields(fields).Errorf("error while running script: %v", err)
			}

			// Send a reply if the message has a reply subject
			if msg.Reply == "" {
				return
			}

			reply, err := r.JSON()
			if err != nil {
				log.WithFields(fields).Errorf("failed to serialize script reply to JSON: %v", err)
			}

			log.WithFields(fields).Debugf("sent reply: %s", reply)

			err = nc.Publish(msg.Reply, reply)
			if err != nil {
				log.WithFields(fields).Errorf("failed to publish reply after running script: %v", err)
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
