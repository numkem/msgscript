//go:build wasmtime

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bytecodealliance/wasmtime-go/v37"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	msgplugins "github.com/numkem/msgscript/plugins"
	"github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

var wasmTracer = otel.Tracer("msgscript.executor.wasm")

type WasmExecutor struct {
	cancelFunc context.CancelFunc
	ctx        context.Context
	store      msgstore.ScriptStore
}

func NewWasmExecutor(c context.Context, store msgstore.ScriptStore, plugins []msgplugins.PreloadFunc, nc *nats.Conn) Executor {
	ctx, cancelFunc := context.WithCancel(c)

	log.Info("WASM executor initialized")

	return &WasmExecutor{
		cancelFunc: cancelFunc,
		ctx:        ctx,
		store:      store,
	}
}

func (we *WasmExecutor) HandleMessage(ctx context.Context, msg *Message, scr *script.Script) *ScriptResult {
	ctx, span := wasmTracer.Start(ctx, "wasm.handle_message", trace.WithAttributes(
		attribute.String("subject", scr.Subject),
		attribute.String("script.name", scr.Name),
		attribute.String("method", msg.Method),
		attribute.Int("payload_size", len(msg.Payload)),
	))
	defer span.End()

	res := new(ScriptResult)

	fields := log.Fields{
		"subject":  scr.Subject,
		"path":     scr.Name,
		"executor": "wasm",
	}

	modulePath := strings.TrimSuffix(string(scr.Content), "\n")
	span.SetAttributes(attribute.String("wasm.module_path", modulePath))

	// Script's content is the path to the wasm script
	_, readSpan := wasmTracer.Start(ctx, "wasm.read_module", trace.WithAttributes(
		attribute.String("module_path", modulePath),
	))
	wasmBytes, err := os.ReadFile(modulePath)
	if err != nil {
		readSpan.RecordError(err)
		readSpan.SetStatus(codes.Error, "Failed to read WASM module")
		readSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to read WASM module")
		res.Error = fmt.Sprintf("failed to read wasm module file %s: %w", scr.Content, err)
		return res
	}
	readSpan.SetAttributes(attribute.Int("wasm.module_size", len(wasmBytes)))
	readSpan.SetStatus(codes.Ok, "")
	readSpan.End()

	// Create temp files for stdout/stderr
	_, stdoutSpan := wasmTracer.Start(ctx, "wasm.create_stdout_file")
	stdoutFile, err := createTempFile("msgscript-wasm-stdout-*")
	if err != nil {
		stdoutSpan.RecordError(err)
		stdoutSpan.SetStatus(codes.Error, "Failed to create stdout file")
		stdoutSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create stdout file")
		return ScriptResultWithError(fmt.Errorf("failed to create stdout temp file: %w", err))
	}
	stdoutSpan.SetAttributes(attribute.String("stdout_file", stdoutFile.Name()))
	stdoutSpan.SetStatus(codes.Ok, "")
	stdoutSpan.End()
	defer os.Remove(stdoutFile.Name())
	defer stdoutFile.Close()

	_, stderrSpan := wasmTracer.Start(ctx, "wasm.create_stderr_file")
	stderrFile, err := createTempFile("msgscript-wasm-stderr-*")
	if err != nil {
		stderrSpan.RecordError(err)
		stderrSpan.SetStatus(codes.Error, "Failed to create stderr file")
		stderrSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create stderr file")
		return ScriptResultWithError(fmt.Errorf("failed to create stderr temp file: %w", err))
	}
	stderrSpan.SetAttributes(attribute.String("stderr_file", stderrFile.Name()))
	stderrSpan.SetStatus(codes.Ok, "")
	stderrSpan.End()
	defer os.Remove(stderrFile.Name())
	defer stderrFile.Close()

	// Initialize WASM runtime
	_, initSpan := wasmTracer.Start(ctx, "wasm.initialize_runtime")
	engine := wasmtime.NewEngine()
	module, err := wasmtime.NewModule(engine, wasmBytes)
	if err != nil {
		initSpan.RecordError(err)
		initSpan.SetStatus(codes.Error, "Failed to create WASM module")
		initSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create WASM module")
		return ScriptResultWithError(fmt.Errorf("failed to create module: %w", err))
	}

	linker := wasmtime.NewLinker(engine)
	err = linker.DefineWasi()
	if err != nil {
		initSpan.RecordError(err)
		initSpan.SetStatus(codes.Error, "Failed to define WASI")
		initSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to define WASI")
		return ScriptResultWithError(fmt.Errorf("failed to define WASI: %w", err))
	}

	wasiConfig := wasmtime.NewWasiConfig()
	wasiConfig.SetStdoutFile(stdoutFile.Name())
	wasiConfig.SetStderrFile(stderrFile.Name())
	wasiConfig.SetEnv([]string{"SUBJECT", "PAYLOAD", "METHOD", "URL"}, []string{msg.Subject, string(msg.Payload), msg.Method, msg.URL})
	span.SetAttributes(
		attribute.String("wasm.env.subject", msg.Subject),
		attribute.String("wasm.env.method", msg.Method),
		attribute.String("wasm.env.url", msg.URL),
	)

	store := wasmtime.NewStore(engine)
	store.SetWasi(wasiConfig)

	instance, err := linker.Instantiate(store, module)
	if err != nil {
		initSpan.RecordError(err)
		initSpan.SetStatus(codes.Error, "Failed to instantiate WASM module")
		initSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to instantiate WASM module")
		return ScriptResultWithError(fmt.Errorf("failed to instantiate: %w", err))
	}
	initSpan.SetStatus(codes.Ok, "")
	initSpan.End()

	// Execute WASM module
	_, execSpan := wasmTracer.Start(ctx, "wasm.execute_module", trace.WithAttributes(
		attribute.String("wasm.function", "_start"),
	))
	log.WithFields(fields).Debug("running wasm module")

	// Execute the main function of the WASM module
	wasmFunc := instance.GetFunc(store, "_start")
	if wasmFunc == nil {
		execSpan.SetStatus(codes.Error, "_start function not found")
		execSpan.End()
		span.SetStatus(codes.Error, "_start function not found")
		return ScriptResultWithError(fmt.Errorf("GET function not found"))
	}

	_, err = wasmFunc.Call(store)
	if err != nil {
		ec, ok := err.(*wasmtime.Error).ExitStatus()
		execSpan.SetAttributes(attribute.Int("wasm.exit_code", int(ec)))
		if ok && ec != 0 {
			execSpan.RecordError(err)
			execSpan.SetStatus(codes.Error, fmt.Sprintf("WASM exit code: %d", ec))
			execSpan.End()
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("WASM exit code: %d", ec))
			return ScriptResultWithError(fmt.Errorf("failed to call wasm module function: %w", err))
		}
	}
	execSpan.SetStatus(codes.Ok, "")
	execSpan.End()

	// Read stdout
	_, stdoutReadSpan := wasmTracer.Start(ctx, "wasm.read_stdout")
	payload, err := readTempFile(stdoutFile)
	if err != nil {
		stdoutReadSpan.RecordError(err)
		stdoutReadSpan.SetStatus(codes.Error, "Failed to read stdout")
		stdoutReadSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to read stdout")
		return ScriptResultWithError(fmt.Errorf("failed to read stdout temp file: %v", err))
	}
	stdoutReadSpan.SetAttributes(attribute.Int("stdout_size", len(payload)))
	stdoutReadSpan.SetStatus(codes.Ok, "")
	stdoutReadSpan.End()

	// Parse result
	_, parseSpan := wasmTracer.Start(ctx, "wasm.parse_result")
	err = json.Unmarshal(payload, res)
	if err != nil {
		// If we can't find that struct in the result, we just take the return data as is
		parseSpan.SetAttributes(attribute.Bool("result.is_json", false))
		res.Payload = payload
	} else {
		parseSpan.SetAttributes(attribute.Bool("result.is_json", true))
	}
	parseSpan.SetStatus(codes.Ok, "")
	parseSpan.End()

	span.SetAttributes(attribute.Int("result.payload_size", len(res.Payload)))

	// Check stderr file
	_, stderrReadSpan := wasmTracer.Start(ctx, "wasm.read_stderr")
	errB, err := readTempFile(stderrFile)
	if err != nil {
		stderrReadSpan.RecordError(err)
		stderrReadSpan.SetStatus(codes.Error, "Failed to read stderr")
		stderrReadSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to read stderr")
		return ScriptResultWithError(fmt.Errorf("failed to read stderr temp file: %v", err))
	}

	if len(errB) > 0 {
		res.Error = string(errB)
		stderrReadSpan.SetAttributes(
			attribute.Int("stderr_size", len(errB)),
			attribute.Bool("has_error", true),
		)
		span.SetAttributes(attribute.String("wasm.stderr", res.Error))
		span.SetStatus(codes.Error, "WASM module wrote to stderr")
	} else {
		stderrReadSpan.SetAttributes(attribute.Bool("has_error", false))
		span.SetStatus(codes.Ok, "WASM module executed successfully")
	}
	stderrReadSpan.SetStatus(codes.Ok, "")
	stderrReadSpan.End()

	return res
}

func readTempFile(f *os.File) ([]byte, error) {
	// Rewind the file
	_, err := f.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	// Read all of it than assign
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (we *WasmExecutor) Stop() {
	we.cancelFunc()
	log.Debug("WasmExecutor stopped")
}
