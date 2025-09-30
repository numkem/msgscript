//go:build wasm

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/bytecodealliance/wasmtime-go/v35"
	"github.com/nats-io/nats.go"
	msgplugins "github.com/numkem/msgscript/plugins"
	scriptLib "github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
	log "github.com/sirupsen/logrus"
)

type WasmExecutor struct {
	cancelFunc context.CancelFunc
	ctx        context.Context
	store      msgstore.ScriptStore
}

func NewWasmExecutor(c context.Context, store msgstore.ScriptStore, plugins []msgplugins.PreloadFunc, nc *nats.Conn) Executor {
	ctx, cancelFunc := context.WithCancel(c)

	return &WasmExecutor{
		cancelFunc: cancelFunc,
		ctx:        ctx,
		store:      store,
	}
}

func (we *WasmExecutor) HandleMessage(ctx context.Context, msg *Message, rf ReplyFunc) {
	// Look up the Lua script for the given subject
	scripts, err := we.store.GetScripts(ctx, msg.Subject)
	if err != nil {
		log.Errorf("failed to get scripts for subject %s: %v", msg.Subject, err)
		return
	}

	if scripts == nil {
		err := &NoScriptFoundError{}
		rf(&Reply{Error: err.Error()})
		return
	}

	errs := make(chan error, len(scripts))
	var wg sync.WaitGroup
	r := NewReply()
	for path, content := range scripts {
		wg.Add(1)

		ss := strings.Split(path, "/")
		name := ss[len(ss)-1]
		fields := log.Fields{
			"subject":  msg.Subject,
			"path":     name,
			"executor": "wasm",
		}

		go func(content []byte) {
			defer wg.Done()

			script, err := scriptLib.ReadString(string(content))
			if err != nil {
				errs <- fmt.Errorf("failed to read script: %w", err)
				return
			}

			modulePath := strings.TrimSuffix(string(script.Content), "\n")

			// Script's content is the path to the wasm script
			wasmBytes, err := os.ReadFile(modulePath)
			if err != nil {
				errs <- fmt.Errorf("failed to read wasm module file %s: %w", script.Content, err)
				return
			}

			stdoutFile, err := createTempFile("msgscript-wasm-stdout-*")
			if err != nil {
				errs <- fmt.Errorf("failed to create stdout temp file: %w", err)
				return
			}
			defer os.Remove(stdoutFile.Name())
			defer stdoutFile.Close()

			stderrFile, err := createTempFile("msgscript-wasm-stderr-*")
			if err != nil {
				errs <- fmt.Errorf("failed to create stderr temp file: %w", err)
				return
			}
			defer os.Remove(stderrFile.Name())
			defer stderrFile.Close()

			engine := wasmtime.NewEngine()
			module, err := wasmtime.NewModule(engine, wasmBytes)
			if err != nil {
				errs <- fmt.Errorf("failed to create module: %w", err)
				return
			}

			linker := wasmtime.NewLinker(engine)
			err = linker.DefineWasi()
			if err != nil {
				errs <- fmt.Errorf("failed to define WASI: %w", err)
				return
			}

			wasiConfig := wasmtime.NewWasiConfig()
			wasiConfig.SetStdoutFile(stdoutFile.Name())
			wasiConfig.SetStderrFile(stderrFile.Name())
			wasiConfig.SetEnv([]string{"SUBJECT", "PAYLOAD", "METHOD", "URL"}, []string{msg.Subject, string(msg.Payload), msg.Method, msg.URL})

			store := wasmtime.NewStore(engine)
			store.SetWasi(wasiConfig)

			instance, err := linker.Instantiate(store, module)
			if err != nil {
				errs <- fmt.Errorf("failed to instantiate: %w", err)
				return
			}

			// Standard WASI, will take what's in stdout as return value
			log.WithFields(fields).Debug("running wasm module")
			// Execute the main function of the WASM module
			scrRes, err := we.executeRawMessage(instance, store, stdoutFile)
			if err != nil {
				errs <- fmt.Errorf("failed to execute WASM module: %w", err)
				return
			}
			log.WithFields(fields).Debug("finished running wasm module")

			// Check stderr file. If there is something, we consider it as an error.
			_, err = stderrFile.Seek(0, 0)
			if err != nil {
				errs <- fmt.Errorf("failed to seek in stderr temp file: %w", err)
				return
			}

			b, err := io.ReadAll(stderrFile)
			if err != nil {
				errs <- fmt.Errorf("failed to read stderr temp file: %w", err)
				return
			}
			if len(b) > 0 {
				scrRes.Error = string(b)
			}

			r.Results.Store(script.Name, scrRes)
			log.WithFields(fields).Debug("stored result")
		}(content)
	}
	wg.Wait()
	log.WithField("subject", msg.Subject).Debugf("finished running %d scripts", len(scripts))

	close(errs)
	for e := range errs {
		r.Error = r.Error + " " + e.Error()
	}

	rf(r)
}

func (*WasmExecutor) executeRawMessage(instance *wasmtime.Instance, store *wasmtime.Store, tmpFile *os.File) (*ScriptResult, error) {
	wasmFunc := instance.GetFunc(store, "_start")
	if wasmFunc == nil {
		return nil, fmt.Errorf("GET function not found")
	}

	_, err := wasmFunc.Call(store)
	ec, _ := err.(*wasmtime.Error).ExitStatus()
	if ec != 0 {
		if err != nil {
			return nil, fmt.Errorf("failed to call wasm module function: %w", err)
		}
	}

	tmpFile.Seek(0, 0)
	output, err := io.ReadAll(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read tempFile %s after wasm execution: %w", tmpFile.Name(), err)
	}

	scrRes := new(ScriptResult)
	// Decode the returned reply
	err = json.Unmarshal(output, scrRes)
	if err != nil {
		// If we can't decode the return value, we take it as is
		scrRes.Payload = output
	}

	return scrRes, nil
}

func (we *WasmExecutor) Stop() {
	we.cancelFunc()
	log.Debug("WasmExecutor stopped")
}
