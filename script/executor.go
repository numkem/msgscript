package script

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/cjoudrey/gluahttp"
	luajson "github.com/layeh/gopher-json"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"github.com/yuin/gluare"
	"github.com/yuin/gopher-lua"
	lfs "layeh.com/gopher-lfs"

	luamodules "github.com/numkem/msgscript/lua"
	msgplugins "github.com/numkem/msgscript/plugins"
	msgstore "github.com/numkem/msgscript/store"
)

type Message struct {
	Subject string
	Payload []byte
	Method  string
	URL     string
}

// ScriptExecutor defines the structure responsible for managing Lua script execution
type ScriptExecutor struct {
	cancelFunc context.CancelFunc       // Context cancellation function
	ctx        context.Context          // Context for cancellation
	nc         *nats.Conn               // Connection to NATS
	store      msgstore.ScriptStore     // Interface for the script storage backend
	plugins    []msgplugins.PreloadFunc // Plugins to load before execution
}

// NewScriptExecutor creates a new ScriptExecutor using the provided ScriptStore
func NewScriptExecutor(store msgstore.ScriptStore, plugins []msgplugins.PreloadFunc, nc *nats.Conn) *ScriptExecutor {
	ctx, cancelFunc := context.WithCancel(context.Background())

	return &ScriptExecutor{
		cancelFunc: cancelFunc,
		ctx:        ctx,
		nc:         nc,
		store:      store,
		plugins:    plugins,
	}
}

// HandleMessage receives a message, matches it to a Lua script, and executes the script in a new goroutine
func (se *ScriptExecutor) HandleMessage(ctx context.Context, msg *Message, replyFunc func(string)) {
	// Look up the Lua script for the given subject
	scripts, err := se.store.GetScripts(ctx, msg.Subject)
	if err != nil {
		log.Errorf("failed to get scripts for subject %s: %v", msg.Subject, err)
		return
	}

	if len(scripts) == 0 {
		t := fmt.Sprintf("No script found for subject: %s", msg.Subject)
		log.Infof(t)
		replyFunc(t)
		return
	}

	var wg sync.WaitGroup
	resp := sync.Map{}
	// Loop through each scripts attached to the subject as there might be more than one
	for path, script := range scripts {
		wg.Add(1)

		ss := strings.Split(path, "/")
		name := ss[len(ss)-1]
		fields := log.Fields{
			"subject": msg.Subject,
			"path":    name,
		}

		tmp, err := os.MkdirTemp(os.TempDir(), fmt.Sprintf("msgscript-%s-%s", msg.Subject, name))
		if err != nil {
			log.WithFields(fields).Errorf("failed to create temp directory: %v", err)
			return
		}
		defer os.RemoveAll(tmp)

		// Run the Lua script in a separate goroutine to handle the message for each script
		go func(script string) {
			err := os.Chdir(tmp)
			if err != nil {
				log.WithFields(fields).Errorf("failed to change to temp directory %s: %v", tmp, err)
			}
			defer wg.Done()

			// Read the script to get the headers (for the libraries for example)
			sr := new(ScriptReader)
			err = sr.ReadString(script)
			if err != nil {
				log.WithFields(fields).Errorf("failed to read script: %v", err)
				return
			}

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
			L.PreloadModule("re", gluare.Loader)
			lfs.Preload(L)
			luajson.Preload(L)
			luamodules.PreloadNats(L, se.nc)
			luamodules.PreloadEtcd(L)

			// Load plugins
			if se.plugins != nil {
				err = msgplugins.LoadPlugins(L, se.plugins)
				if err != nil {
					log.Errorf("failed to load plugin: %v", err)
					return
				}
			}

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

			// If this is from an HTTP call, use the method as function to the entrypoint object
			if msg.Method != "" {
				if err := L.CallByParam(lua.P{
					Fn:      L.GetGlobal(msg.Method),
					NRet:    1,
					Protect: true,
				}, lua.LString(msg.URL), lua.LString(string(msg.Payload))); err != nil {
					msg := fmt.Sprintf("failed to call %s function: %v", msg.Method, err)
					log.WithFields(fields).Error(msg)
					resp.Store(name, fmt.Sprintf("error: %v", err))
					return
				}
			} else {
				if err := L.CallByParam(lua.P{
					Fn:      L.GetGlobal("OnMessage"),
					NRet:    1,
					Protect: true,
				}, lua.LString(msg.Subject), lua.LString(string(msg.Payload))); err != nil {
					msg := fmt.Sprintf("failed to call OnMessage function: %v", err)
					log.WithFields(fields).Error(msg)
					resp.Store(name, fmt.Sprintf("error: %v", err))
					return
				}
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
	log.WithField("subject", msg.Subject).Debugf("finished running %d scripts", len(scripts))

	// Return a JSON representation of the result of each function ran
	r := make(map[string]string)
	resp.Range(func(k, v any) bool {
		r[k.(string)] = v.(string)
		return true
	})

	// For non-http calls, we can return a map of all the reponses for each of the scripts
	if msg.Method == "" {
		j, err := json.Marshal(r)
		if err != nil {
			log.WithField("subject", msg.Subject).Error("failed to marshal response")
		}
		replyFunc(string(j))
	} else {
		// Return the first value found
		// TODO: Maybe append all of the answers together?
		for _, v := range r {
			replyFunc(v)
		}
	}
}

// Stop gracefully shuts down the ScriptExecutor and stops watching for messages
func (se *ScriptExecutor) Stop() {
	se.cancelFunc()
	log.Debug("ScriptExecutor stopped")
}
