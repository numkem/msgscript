//go:build !wasmtime

// This is so that we can build without wasm's dependancies

package executor

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"

	msgplugins "github.com/numkem/msgscript/plugins"
	"github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

type noWasmExecutor struct{}

func NewWasmExecutor(c context.Context, store msgstore.ScriptStore, plugins []msgplugins.PreloadFunc, nc *nats.Conn) Executor {
	return &noWasmExecutor{}
}

func (e *noWasmExecutor) HandleMessage(ctx context.Context, msg *Message, scr *script.Script) *ScriptResult {
	return ScriptResultWithError(fmt.Errorf("msgscript wasn't built with wasm support"))
}

func (e *noWasmExecutor) Stop() {}
