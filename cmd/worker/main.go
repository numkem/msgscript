package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/numkem/msgscript/executor"
	"github.com/numkem/msgscript/script"
	"github.com/numkem/msgscript/store"
	"github.com/numkem/msgscript/plugins"
)

var mainTracer = otel.Tracer("msgscript.worker.main")

func main() {
	debug := flag.Bool("v", false, "Enable debug logging")
	flag.Parse()

	if *debug || os.Getenv("DEBUG") != "" {
		log.SetLevel(log.DebugLevel)
	}

	notifyContext, stop := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	// Start a span for the worker itself
	ctx, span := mainTracer.Start(notifyContext, "worker",
		trace.WithSpanKind(trace.SpanKindServer),
	)
	defer span.End()

	input, err := parseInput(ctx)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		replyWithError(fmt.Errorf("failed to parse input: %v", err))
		os.Exit(1)
	}

	// Init ScriptStore
	scriptStore, err := store.StoreByName(input.ScriptStore.BackendName, input.ScriptStore.EtcdURL, input.ScriptStore.ScriptDirectory, input.ScriptStore.LibraryDirectory)
	if err != nil {
		replyWithError(fmt.Errorf("failed to initialize script store: %v", err))
		os.Exit(10)
	}

	_, natsConnectSpan := mainTracer.Start(ctx, "worker.nats_connect", trace.WithAttributes(
		attribute.String("nats.URL", input.NatsURL),
	))

	// Connect to NATS
	nc, err := nats.Connect(input.NatsURL)
	if err != nil {
		replyWithError(fmt.Errorf("Failed to connect to NATS: %v", err), natsConnectSpan)
		os.Exit(20)
	}
	defer nc.Close()

	natsConnectSpan.SetStatus(codes.Ok, "connected to NATS")
	natsConnectSpan.End()

	_, luaPluginLoadSpan := mainTracer.Start(ctx, "worker.load_lua_plugins")

	// Init executors
	luaPlugins, err := plugins.ReadPluginDir(input.LuaExecutor.PluginDir)
	if err != nil {
		replyWithError(fmt.Errorf("failed to read lua plugin directory %s: %v", input.LuaExecutor.PluginDir, err), luaPluginLoadSpan)
		os.Exit(30)
	}

	luaPluginLoadSpan.SetStatus(codes.Ok, "plugins loaded")
	luaPluginLoadSpan.End()

	executors := executor.StartAllExecutors(ctx, scriptStore, luaPlugins, nc)

	cctx, getScriptsSpan := mainTracer.Start(ctx, "worker.get_scripts", trace.WithAttributes(
		attribute.String("script.Name", input.Message.Subject),
		attribute.String("script.URL", input.Message.URL),
	))

	scripts, err := scriptStore.GetScripts(cctx, input.Message.Subject)
	if err != nil {
		replyWithError(fmt.Errorf("failed to get scripts for subject %s: %v", input.Message.Subject, err))
		os.Exit(355)
	}

	getScriptsSpan.SetStatus(codes.Ok, fmt.Sprintf("found %d scripts", len(scripts)))
	getScriptsSpan.End()

	ectx, executeScriptsSpan := mainTracer.Start(ctx, "worker.run_scripts")

	var wg sync.WaitGroup
	allResults := make(chan *executor.ScriptResult, len(scripts))
	for name, scr := range scripts {
		wg.Add(1)

		go func(ctx context.Context, msg *executor.Message, script *script.Script) {
			defer wg.Done()

			_, sspan := mainTracer.Start(ectx, fmt.Sprintf("worker.run_script.%s", name), trace.WithAttributes(
				attribute.String("Name", scr.Name),
				attribute.String("Subject", scr.Subject),
				attribute.Bool("isHTML", scr.HTML),
				attribute.Int("Nb libraries", len(scr.LibKeys)),
			))
			defer sspan.End()

			if err != nil {
				sspan.RecordError(err)
				sspan.SetStatus(codes.Error, "Failed to get executor")
				log.WithError(err).Error("failed to get executor for script")

				allResults <- &executor.ScriptResult{Error: fmt.Sprintf("failed to get executor for script: %v", err)}
				return
			}

			// Executor field is optional but it defaults to Lua
			if scr.Executor == "" {
				scr.Executor = executor.EXECUTOR_LUA_NAME
			}

			exec, ok := executors[scr.Executor]
			if !ok {
				allResults <- &executor.ScriptResult{
					Error: fmt.Sprintf("executor named %s doesn't exists", scr.Executor),
				}
				return
			}
			res := exec.HandleMessage(ctx, input.Message, scr)
			if log.GetLevel() == log.DebugLevel {
				log.Errorf("script: %+v", res)
			}

			sspan.SetStatus(codes.Ok, "script executed")

			allResults <- res
		}(ctx, input.Message, scr)
	}
	wg.Wait()

	close(allResults)

	executeScriptsSpan.SetStatus(codes.Ok, "script executed")
	executeScriptsSpan.SetAttributes(attribute.Int("Nb script executed", len(allResults)))
	executeScriptsSpan.End()

	_, parseReplySpan := mainTracer.Start(ctx, "worker.parse_replies")
	msgRep := new(executor.Reply)
	for res := range allResults {
		if res.IsHTML {
			msgRep.HTML = true
		}

		msgRep.Results = append(msgRep.Results, res)
	}
	parseReplySpan.SetAttributes(attribute.Int("responses", len(msgRep.Results)))
	parseReplySpan.SetStatus(codes.Ok, "responses parsed")
	parseReplySpan.End()

	// Write to stderr since stdout is dedicated to be read
	if log.GetLevel() == log.DebugLevel {
		log.Errorf("WorkerOutput: %+v", msgRep)
	}

	// Processsing done, write the Reply to stdout
	err = json.NewEncoder(os.Stdout).Encode(&executor.WorkerOutput{
		Reply: msgRep,
	})
	if err != nil {
		replyWithError(err)
	}

	span.SetStatus(codes.Ok, "Worker is done")
}

func parseInput(ctx context.Context) (*executor.WorkerInput, error) {
	_, inputParseSpan := mainTracer.Start(ctx, "worker.parse_input")
	defer inputParseSpan.End()

	// Read input data from stdin
	input := &executor.WorkerInput{}

	err := json.NewDecoder(os.Stdin).Decode(input)
	if err != nil {
		inputParseSpan.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to parse input from stdin: %v", err)
	}

	inputParseSpan.SetStatus(codes.Ok, "input parsed")
	return input, nil
}

func replyWithError(err error, spans ...trace.Span) {
	log.Error(err)

	for _, span := range spans {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed")
		span.End()
	}

	e := json.NewEncoder(os.Stdout).Encode(&executor.WorkerOutput{
		Error: err,
	})
	if e != nil {
		log.WithError(e).Error("failed write error reply")
	}
}
