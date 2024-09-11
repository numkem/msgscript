package script

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cjoudrey/gluahttp"
	luajson "github.com/layeh/gopher-json"
	log "github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"

	msgstore "github.com/numkem/msgscript/store"
)

// ScriptExecutor defines the structure responsible for managing Lua script execution
type ScriptExecutor struct {
	store      msgstore.ScriptStore // Interface for the script storage backend
	ctx        context.Context      // Context for cancellation
	cancelFunc context.CancelFunc   // Context cancellation
}

// NewScriptExecutor creates a new ScriptExecutor using the provided ScriptStore
func NewScriptExecutor(store msgstore.ScriptStore) *ScriptExecutor {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &ScriptExecutor{
		store:      store,
		ctx:        ctx,
		cancelFunc: cancelFunc,
	}
}

// HandleMessage receives a message, matches it to a Lua script, and executes the script in a new goroutine
func (se *ScriptExecutor) HandleMessage(subject string, payload []byte, replyFunc func(string)) {
	// Look up the Lua script for the given subject
	scripts, err := se.store.GetScripts(subject)
	if err != nil {
		log.Errorf("failed to get scripts for subject %s: %v", subject, err)
	}

	if len(scripts) == 0 {
		log.Infof("No script found for subject: %s", subject)
		return

	}

	for _, script := range scripts {
		// Run the Lua script in a separate goroutine to handle the message
		go func(script string) {
			L := lua.NewState()
			L.SetContext(se.ctx)
			defer L.Close()

			fields := log.Fields{
				"subject": subject,
			}

			// Set up the Lua state with the subject and payload
			L.PreloadModule("http", gluahttp.NewHttpModule(&http.Client{}).Loader)
			luajson.Preload(L)

			if err := L.DoString(script); err != nil {
				msg := fmt.Sprintf("Error parsing Lua script: %v", err)
				log.WithFields(fields).Errorf(msg)
				replyFunc("error: " + msg)
				return
			}

			if err := L.CallByParam(lua.P{
				Fn:      L.GetGlobal("OnMessage"),
				NRet:    1,
				Protect: true,
			}, lua.LString(subject), lua.LString(string(payload))); err != nil {
				msg := fmt.Sprintf("failed to call OnMessage function: %v", err)
				log.WithFields(fields).Error(msg)
				replyFunc("error: " + msg)
				return
			}

			// Retrieve the result from the Lua state (assuming it's a string)
			result := L.Get(-1)
			if str, ok := result.(lua.LString); ok {
				replyFunc(string(str))
			} else {
				log.WithFields(fields).Warn("Script did not return a string")
			}
		}(script)
	}
}

// Stop gracefully shuts down the ScriptExecutor and stops watching for changes
func (se *ScriptExecutor) Stop() {
	se.cancelFunc()
	log.Debug("ScriptExecutor stopped")
}
