package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/numkem/msgscript/plugins"
	"github.com/numkem/msgscript/store"
)

const (
	MAX_LUA_RUNNING_TIME = 2 * time.Minute
	EXECUTOR_LUA_NAME    = "lua"
	EXECUTOR_WASM_NAME   = "wasm"
	EXECUTOR_PODMAN_NAME = "podman"
)

type ReplyFunc func(r *Reply)

type Message struct {
	Async    bool   `json:"async"`
	Executor string `json:"executor"`
	Method   string `json:"method"`
	Payload  []byte `json:"payload"`
	Raw      bool   `json:"raw"`
	Subject  string `json:"subject"`
	URL      string `json:"url"`
}

type Reply struct {
	Results    sync.Map
	HTML       bool                     `json:"is_html"`
	Error      string                   `json:"error,omitempty"`
	AllResults map[string]*ScriptResult `json:"results,omitempty"`
}

type ScriptResult struct {
	Code    int               `json:"http_code"`
	Error   string            `json:"error"`
	Headers map[string]string `json:"http_headers"`
	IsHTML  bool              `json:"is_html"`
	Payload []byte            `json:"payload"`
}

// Bytes:  Append each of the replies to eachother and return it
func (r *Reply) Bytes() []byte {
	var replies []byte

	r.Results.Range(func(key, value any) bool {
		replies = append(replies, value.(*ScriptResult).Payload...)
		return true
	})

	return replies
}

func (r *Reply) JSON() ([]byte, error) {
	// Convert the sync.Map to a map
	r.AllResults = make(map[string]*ScriptResult)
	r.Results.Range(func(key, value any) bool {
		r.AllResults[key.(string)] = value.(*ScriptResult)
		return true
	})

	return json.Marshal(r)
}

func NewReply() *Reply {
	return &Reply{
		Results: sync.Map{},
	}
}

func NewErrReply(err error) *Reply {
	return &Reply{
		Error: err.Error(),
	}
}

type NoScriptFoundError struct{}

func (e *NoScriptFoundError) Error() string {
	return "No script found for subject"
}

type Executor interface {
	HandleMessage(context.Context, *Message, ReplyFunc)
	Stop()
}

func StartAllExecutors(ctx context.Context, scriptStore store.ScriptStore, plugins []plugins.PreloadFunc, nc *nats.Conn) map[string]Executor {
	executors := make(map[string]Executor)

	executors[EXECUTOR_LUA_NAME] = NewLuaExecutor(ctx, scriptStore, plugins, nc)
	executors[EXECUTOR_WASM_NAME] = NewWasmExecutor(ctx, scriptStore, nil, nil)

	podmanExec, err := NewPodmanExecutor(ctx, scriptStore)
	if err != nil {
		podmanExec = nil
	}
	executors[EXECUTOR_PODMAN_NAME] = podmanExec

	return executors
}

func ExecutorByName(name string, executors map[string]Executor) (Executor, error) {
	// Handle the message by invoking the corresponding Lua script
	// Check if the executor exists
	exec, found := executors[name]
	if !found {
		exec = executors[EXECUTOR_LUA_NAME]
	}

	// Check if the executor is enabled
	if exec == nil {
		return nil, fmt.Errorf("executor %s isn't enabled", name)
	}

	return exec, nil
}

func StopAllExecutors(executors map[string]Executor) {
	for _, exec := range executors {
		if exec !=  nil {
			exec.Stop()
		}
	}
}
