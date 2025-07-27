package executor

import (
	"encoding/json"
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
	"golang.org/x/net/context"
)

type WasmExecutor struct {
	cancelFunc context.CancelFunc
	ctx        context.Context
	store      msgstore.ScriptStore
}

func NewWasmExecutor(store msgstore.ScriptStore, plugins []msgplugins.PreloadFunc, nc *nats.Conn) Executor {
	ctx, cancelFunc := context.WithCancel(context.Background())

	return &WasmExecutor{
		cancelFunc: cancelFunc,
		ctx:        ctx,
		store:      store,
	}
}

func (we *WasmExecutor) HandleMessage(ctx context.Context, msg *Message, replyFunc func(r *Reply)) {
	// Look up the Lua script for the given subject
	scripts, err := we.store.GetScripts(ctx, msg.Subject)
	if err != nil {
		log.Errorf("failed to get scripts for subject %s: %v", msg.Subject, err)
		return
	}

	if scripts == nil {
		err := &NoScriptFoundError{}
		replyFunc(&Reply{Error: err.Error()})
		return
	}

	var wg sync.WaitGroup
	r := NewReply()
	for path, content := range scripts {
		wg.Add(1)

		ss := strings.Split(path, "/")
		name := ss[len(ss)-1]
		fields := log.Fields{
			"subject": msg.Subject,
			"path":    name,
		}

		go func(content []byte) {
			defer wg.Done()

			script, err := scriptLib.ReadString(string(content))
			if err != nil {
				log.WithFields(fields).Errorf("failed to read script: %v", err)
				return
			}

			modulePath := strings.TrimSuffix(string(script.Content), "\n")

			// Script's content is the path to the wasm script
			wasmBytes, err := os.ReadFile(modulePath)
			if err != nil {
				log.WithFields(fields).Errorf("failed to read wasm module file %s: %v", script.Content, err)
				return
			}

			tmpFile, err := os.CreateTemp(os.TempDir(), "msgscript-wasm-stdout-*")
			if err != nil {
				log.WithFields(fields).Errorf("failed to create temp file: %v", err)
				return
			}
			defer os.Remove(tmpFile.Name())
			defer tmpFile.Close()

			engine := wasmtime.NewEngine()
			module, err := wasmtime.NewModule(engine, wasmBytes)
			if err != nil {
				log.WithFields(fields).Errorf("failed to create module: %v", err)
				return
			}

			linker := wasmtime.NewLinker(engine)
			err = linker.DefineWasi()
			if err != nil {
				log.WithFields(fields).Errorf("failed to define WASI: %v", err)
				return
			}

			wasiConfig := wasmtime.NewWasiConfig()
			err = wasiConfig.SetStdoutFile(tmpFile.Name())
			if err != nil {
				log.WithFields(fields).Errorf("failed to set wasi stdout to tmpFile %s: %v", tmpFile.Name(), err)
				return
			}
			wasiConfig.SetEnv([]string{"SUBJECT", "PAYLOAD", "METHOD", "URL"}, []string{msg.Subject, string(msg.Payload), msg.Method, msg.URL})

			store := wasmtime.NewStore(engine)
			store.SetWasi(wasiConfig)

			instance, err := linker.Instantiate(store, module)
			if err != nil {
				log.WithFields(fields).Errorf("failed to instantiate: %v", err)
				return
			}

			scrRes := new(ScriptResult)
			// Execute the main function of the WASM module
			wasmFunc := instance.GetFunc(store, "_start")
			if wasmFunc == nil {
				log.WithFields(fields).Error("GET function not found")
				return
			}

			log.WithFields(fields).Debug("calling wasm function")
			_, err = wasmFunc.Call(store)
			ec, _ := err.(*wasmtime.Error).ExitStatus()
			if ec != 0 {
				if err != nil {
					log.WithFields(fields).Errorf("failed to call wasm module function: %v", err)
					return
				}
			}
			log.WithFields(fields).Debug("wasm function finished")

			log.WithFields(fields).WithField("stdout_file", tmpFile.Name()).Debug("reading wasm stdout file")
			tmpFile.Seek(0, 0)
			output, err := io.ReadAll(tmpFile)
			if err != nil {
				log.WithFields(fields).Errorf("failed to read tempFile %s after wasm execution: %v", tmpFile.Name(), err)
			}
			log.WithFields(fields).WithField("stdout_file", tmpFile.Name()).Debug("read stdout file")

			log.WithFields(fields).Debug("decoding JSON output")
			// Decode the returned reply
			err = json.Unmarshal(output, scrRes)
			if err != nil {
				// If we can't decode the return value, we take it as is
				scrRes.Payload = output
			}

			log.WithFields(fields).Debug("finished running wasm module")
			r.Results.Store(script.Name, scrRes)
			log.WithFields(fields).Debug("stored result")
		}(content)
	}

	wg.Wait()
	log.WithField("subject", msg.Subject).Debugf("finished running %d scripts", len(scripts))

	replyFunc(r)
}

func (we *WasmExecutor) Stop() {
	we.cancelFunc()
	log.Debug("WasmExecutor stopped")
}
