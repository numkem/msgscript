package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/numkem/msgscript"
	"github.com/numkem/msgscript/executor"
	msgplugin "github.com/numkem/msgscript/plugins"
	"github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

var version = "dev"
var mainTracer = otel.Tracer("msgscript.main")

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Set up logging
	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.Fatalf("Invalid log level: %v", err)
	}
	log.SetLevel(level)

	if os.Getenv("DEBUG") != "" {
		log.SetLevel(log.DebugLevel)
	}

	if os.Getenv("TELEMETRY_TRACES") != "" {
		log.WithField("kind", "traces").Info("Starting telemetry")

		// Init traces
		otelShutdown, err := setupOTelSDK(ctx)
		if err != nil {
			log.Errorf("failed to initialize opentelemetry traces: %v", err)
			os.Exit(1)
		}
		defer func() {
			err = errors.Join(err, otelShutdown(context.Background()))
		}()
	}

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
			log.Info("Starting embeded NATS server... on 127.0.0.1:4222")
			ns, err := natsserver.NewServer(&natsserver.Options{
				Host: "127.0.0.1",
				Port: 4222,
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	executors := executor.StartAllExecutors(ctx, scriptStore, plugins, nc)

	log.Info("Starting message watch...")

	// Set up a message handler
	_, err = nc.Subscribe(">", func(msg *nats.Msg) {
		// Extract trace context from NATS message headers
		ctx := otel.GetTextMapPropagator().Extract(
			context.Background(),
			natsHeaderCarrier(msg.Header),
		)

		// Start a span for the NATS message handling
		ctx, span := mainTracer.Start(ctx, "nats.handle_message",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("nats.subject", msg.Subject),
				attribute.Int("nats.message_size", len(msg.Data)),
			),
		)
		defer span.End()

		log.Debugf("Received message on subject: %s", msg.Subject)

		if strings.HasPrefix(msg.Subject, "_INBOX.") {
			span.SetAttributes(attribute.Bool("nats.is_inbox", true))
			span.SetStatus(codes.Ok, "Ignored inbox subject")
			log.Debugf("Ignoring reply subject %s", msg.Subject)
			return
		}

		m := new(executor.Message)
		err := json.Unmarshal(msg.Data, m)
		// if the payload isn't a JSON Message, take it as a whole
		if err != nil {
			m.Subject = msg.Subject
			m.Payload = msg.Data
			m.Raw = true
		}

		fields := log.Fields{
			"subject": m.Subject,
			"raw":     m.Raw,
			"async":   m.Async,
		}

		span.SetAttributes(
			attribute.Bool("message.raw", m.Raw),
			attribute.Bool("message.async", m.Async),
		)

		// The above unmarshalling only applies to the structure of the JSON.
		// Even if you feed it another JSON where none of the keys matches,
		// it will just end up being an empty struct
		if m.Payload == nil {
			m = &executor.Message{
				Subject: msg.Subject,
				Payload: msg.Data,
			}
		}

		if m.Async {
			span.SetAttributes(attribute.String("reply.mode", "async"))
			err = nc.Publish(msg.Reply, []byte("{}"))
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "Failed to publish async reply")
				log.WithFields(fields).Errorf("failed to reply to message: %v", err)

				replyWithError(nc, fmt.Errorf("failed to reply to message: %v", err), msg.Reply)
				return
			}
		} else {
			span.SetAttributes(attribute.String("reply.mode", "sync"))
		}

		cctx, getScriptsSpan := mainTracer.Start(ctx, "nats.handle_message.get_scripts", trace.WithAttributes(
			attribute.String("script.Name", m.Subject),
			attribute.String("script.URL", m.URL),
		))

		scripts, err := scriptStore.GetScripts(cctx, m.Subject)
		if err != nil {
			log.WithError(err).WithField("subject", m.Subject).Error("failed to get scripts for subject")
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to get scripts")

			replyWithError(nc, fmt.Errorf("failed to get scripts for subject"), msg.Reply)
			return
		}
		getScriptsSpan.SetStatus(codes.Ok, fmt.Sprintf("found %d scripts", len(scripts)))
		getScriptsSpan.End()

		_, executeScriptsSpan := mainTracer.Start(ctx, "nats.handle_message.run_scripts")
		defer executeScriptsSpan.End()

		var wg sync.WaitGroup
		allResults := make(chan *executor.ScriptResult, len(scripts))
		for _, scr := range scripts {
			wg.Add(1)
			go func(ctx context.Context, msg *executor.Message, script *script.Script) {
				defer wg.Done()

				// Pass the context with trace info to the executor
				exec, err := executor.ExecutorByName(scr.Executor, executors)
				if err != nil {
					executeScriptsSpan.RecordError(err)
					executeScriptsSpan.SetStatus(codes.Error, "Failed to get executor")
					log.WithError(err).Error("failed to get executor for script")

					allResults <- &executor.ScriptResult{Error: fmt.Sprintf("failed to get executor for script: %v", err)}
					return
				}

				rep := exec.HandleMessage(ctx, m, scr)
				allResults <- rep
			}(ctx, m, scr)
		}
		wg.Wait()

		close(allResults)

		_, parseReplySpan := mainTracer.Start(ctx, "nats.handle_message.parse_replies")
		msgRep := new(Reply)
		for res := range allResults {
			if res.IsHTML {
				msgRep.HTML = true
			}

			msgRep.Results = append(msgRep.Results, res)
		}
		parseReplySpan.SetStatus(codes.Ok, "responses parsed")
		parseReplySpan.End()

		_, natsReplaySpan := mainTracer.Start(ctx, "nats.handle_message.send_reply")
		err = replyMessage(nc, m, msg.Reply, msgRep)
		if err != nil {
			natsReplaySpan.RecordError(err)
			natsReplaySpan.SetStatus(codes.Error, "Failed to send reply through NATS")

			log.WithError(err).Errorf("failed to send reply through NATS")
			return
		}

		log.WithField("subject", msg.Subject).Debugf("finished running %d scripts", len(scripts))
		span.SetStatus(codes.Ok, "Message handled")
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to NATS subjects: %v", err)
	}

	defer func() {
		log.Info("Received shutdown signal, stopping server...")
		executor.StopAllExecutors(executors)
	}()

	// Start HTTP Server
	runHTTP(*httpPort, *natsURL)
}
