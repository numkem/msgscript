//go:build !wasm
// This is so that we can build without wasm's dependancies

package executor

import (
	"context"

	"github.com/nats-io/nats.go"
	msgplugins "github.com/numkem/msgscript/plugins"
	msgstore "github.com/numkem/msgscript/store"
)

type noWasmExecutor struct{}

func NewWasmExecutor(c context.Context, store msgstore.ScriptStore, plugins []msgplugins.PreloadFunc, nc *nats.Conn) Executor {
	return &noWasmExecutor{}
}

func (e *noWasmExecutor) HandleMessage(ctx context.Context, msg *Message, rf ReplyFunc) {
	r := NewReply()
	r.Error = "msgscript wasn't built with wasm support"
	rf(r)
}

func (e *noWasmExecutor) Stop() {}
