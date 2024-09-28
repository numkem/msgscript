package script

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/cjoudrey/gluahttp"
	luajson "github.com/layeh/gopher-json"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"github.com/tengattack/gluasql"
	mysql "github.com/tengattack/gluasql/mysql"
	sqlite3 "github.com/tengattack/gluasql/sqlite3"
	"github.com/vadv/gopher-lua-libs/cmd"
	"github.com/vadv/gopher-lua-libs/filepath"
	"github.com/vadv/gopher-lua-libs/inspect"
	"github.com/vadv/gopher-lua-libs/ioutil"
	"github.com/vadv/gopher-lua-libs/runtime"
	luastrings "github.com/vadv/gopher-lua-libs/strings"
	"github.com/vadv/gopher-lua-libs/time"
	"github.com/yuin/gluare"
	lua "github.com/yuin/gopher-lua"
	lfs "layeh.com/gopher-lfs"

	luamodules "github.com/numkem/msgscript/lua"
	msgstore "github.com/numkem/msgscript/store"
)

// ScriptExecutor defines the structure responsible for managing Lua script execution
type ScriptExecutor struct {
	store      msgstore.ScriptStore // Interface for the script storage backend
	ctx        context.Context      // Context for cancellation
	cancelFunc context.CancelFunc   // Context cancellation
	nc         *nats.Conn
}

// NewScriptExecutor creates a new ScriptExecutor using the provided ScriptStore
func NewScriptExecutor(store msgstore.ScriptStore, nc *nats.Conn) *ScriptExecutor {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &ScriptExecutor{
		store:      store,
		ctx:        ctx,
		cancelFunc: cancelFunc,
		nc:         nc,
	}
}

// HandleMessage receives a message, matches it to a Lua script, and executes the script in a new goroutine
func (se *ScriptExecutor) HandleMessage(ctx context.Context, subject string, payload []byte, replyFunc func(string)) {
	// Look up the Lua script for the given subject
	scripts, err := se.store.GetScripts(ctx, subject)
	if err != nil {
		log.Errorf("failed to get scripts for subject %s: %v", subject, err)
		return
	}

	if len(scripts) == 0 {
		log.Infof("No script found for subject: %s", subject)
		return
	}

	var wg sync.WaitGroup
	resp := sync.Map{}
	// Loop through each scripts attached to the subject as there might be more than one
	for name, script := range scripts {
		wg.Add(1)
		// Run the Lua script in a separate goroutine to handle the message for each script
		go func(script string) {
			defer wg.Done()
			fields := log.Fields{
				"subject": subject,
				"path":    name,
			}

			// Read the script to get the headers (for the libraries for example)
			sr := new(ScriptReader)
			sr.ReadString(script)

			libs, err := se.store.LoadLibrairies(ctx, sr.Script.LibKeys)
			if err != nil {
				log.WithFields(fields).Errorf("failed to read librairies: %v", err)
				return
			}

			locked, err := se.store.TakeLock(ctx, name)
			if err != nil {
				log.WithFields(fields).Debugf("failed to get lock: %v", err)
				log.WithFields(fields).Debug("bailing out")
				return
			}

			if !locked {
				log.WithFields(fields).Debug("We don't have a lock, giving up")
				return
			}
			defer se.store.ReleaseLock(ctx, name)

			log.WithFields(fields).Debug("executing script")

			L := lua.NewState()
			L.SetContext(se.ctx)
			defer L.Close()

			// Set up the Lua state with the subject and payload
			L.PreloadModule("http", gluahttp.NewHttpModule(&http.Client{}).Loader)
			L.PreloadModule("mysql", mysql.Loader)
			L.PreloadModule("re", gluare.Loader)
			L.PreloadModule("sqlite3", sqlite3.Loader)
			cmd.Preload(L)
			filepath.Preload(L)
			gluasql.Preload(L)
			inspect.Preload(L)
			ioutil.Preload(L)
			lfs.Preload(L)
			luajson.Preload(L)
			luamodules.PreloadNats(L, se.nc)
			runtime.Preload(L)
			luastrings.Preload(L)
			time.Preload(L)

			var sb strings.Builder
			for _, l := range libs {
				sb.WriteString(l + "\n")
			}
			sb.WriteString(script)
			log.WithFields(fields).Debugf("script:\n%+s\n\n", sb.String())

			if err := L.DoString(sb.String()); err != nil {
				msg := fmt.Sprintf("error parsing Lua script: %v", err)
				log.WithFields(fields).Errorf(msg)
				resp.Store(name, fmt.Sprintf("error: %v", err))
				return
			}

			if err := L.CallByParam(lua.P{
				Fn:      L.GetGlobal("OnMessage"),
				NRet:    1,
				Protect: true,
			}, lua.LString(subject), lua.LString(string(payload))); err != nil {
				msg := fmt.Sprintf("failed to call OnMessage function: %v", err)
				log.WithFields(fields).Error(msg)
				resp.Store(name, fmt.Sprintf("error: %v", err))
				return
			}

			// Retrieve the result from the Lua state (assuming it's a string)
			result := L.Get(-1)
			if str, ok := result.(lua.LString); ok {
				resp.Store(name, string(str))
			} else {
				log.WithFields(fields).Warn("Script did not return a string")
			}
		}(script)
	}

	wg.Wait()
	log.WithField("subject", subject).Debugf("finished running %d scripts", len(scripts))

	// Return a JSON representation of the result of each function ran
	r := make(map[string]string)
	resp.Range(func(k, v any) bool {
		r[k.(string)] = v.(string)
		return true
	})

	j, err := json.Marshal(r)
	if err != nil {
		log.WithField("subject", subject).Error("failed to marshal response")
	}
	replyFunc(string(j))
}

// Stop gracefully shuts down the ScriptExecutor and stops watching for messages
func (se *ScriptExecutor) Stop() {
	se.cancelFunc()
	log.Debug("ScriptExecutor stopped")
}
