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
	Subject string `json:"subject"`
	Payload []byte `json:"payload"`
	Method  string `json:"method"`
	URL     string `json:"url"`
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

func (r *Reply) JSON() ([]byte, error) {
	// Convert the sync.Map to a map
	r.AllResults = make(map[string]*ScriptResult)
	r.Results.Range(func(key, value interface{}) bool {
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
func (se *ScriptExecutor) HandleMessage(ctx context.Context, msg *Message, replyFunc func(r *Reply)) {
	// Look up the Lua script for the given subject
	scripts, err := se.store.GetScripts(ctx, msg.Subject)
	if err != nil {
		log.Errorf("failed to get scripts for subject %s: %v", msg.Subject, err)
		return
	}

	if len(scripts) == 0 {
		err := &NoScriptFoundError{}
		log.WithField("subject", msg.Subject).Infof(err.Error())
		replyFunc(&Reply{Error: err.Error()})
		return
	}

	var wg sync.WaitGroup
	r := NewReply()
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
			s := new(Script)
			err = s.ReadString(script)
			if err != nil {
				log.WithFields(fields).Errorf("failed to read script: %v", err)
				return
			}

			libs, err := se.store.LoadLibrairies(ctx, s.LibKeys)
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

			log.WithFields(fields).WithField("isHTML", s.HTML).Debug("executing script")

			L := lua.NewState()
			L.SetContext(se.ctx)
			defer L.Close()

			// Set up the Lua state with the subject and payload
			L.PreloadModule("http", gluahttp.NewHttpModule(&http.Client{}).Loader)
			L.PreloadModule("re", gluare.Loader)
			lfs.Preload(L)
			luajson.Preload(L)
			luamodules.PreloadEtcd(L)
			luamodules.PreloadNats(L, se.nc)

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

			res := &ScriptResult{
				IsHTML:  s.HTML,
				Headers: make(map[string]string),
			}
			if err := L.DoString(sb.String()); err != nil {
				msg := fmt.Sprintf("error parsing Lua script: %v", err)
				log.WithFields(fields).Errorf(msg)
				res.Error = err.Error()
				r.Results.Store(name, res)
				return
			}

			// If the message is set to return HTML, we pass 2 things to the fonction named after the HTTP
			// method received ex: POST(), GET()...
			// The 2 things are:
			//   - The URL part after the function name
			//   - The body of the HTTP call
			if s.HTML {
				log.WithFields(fields).Debug("Running HTML based script")
				gMethod := L.GetGlobal(msg.Method)
				if msg.Method != "" && gMethod.Type().String() != "nil" {
					if err := L.CallByParam(lua.P{
						Fn:      gMethod,
						NRet:    3,
						Protect: true,
					}, lua.LString(msg.URL), lua.LString(string(msg.Payload))); err != nil {
						r.Error = fmt.Errorf("failed to call %s function: %v", msg.Method, err).Error()
						return
					}
				}
			} else {
				// If we do not have an HTML based message, we call the function named
				// OnMessage() with 2 parameters:
				//   - The subject
				//   - The body of the message
				log.WithFields(fields).Debug("Running standard script")
				gOnMessage := L.GetGlobal("OnMessage")
				if gOnMessage.Type().String() != "nil" {
					if err := L.CallByParam(lua.P{
						Fn:      gOnMessage,
						NRet:    1,
						Protect: true,
					}, lua.LString(msg.Subject), lua.LString(string(msg.Payload))); err != nil {
						r.Error = fmt.Errorf("failed to call OnMessage function: %v", err).Error()
						return
					}
				}
			}

			// Retrieve the result from the Lua state (assuming it's a string)
			if s.HTML {
				// Expected return value from lua would look like this (super minimal):
				// return "<html></html>", 200, {}
				// Only the first parameter is really necessary, the others are optional.
				// If they are not defined, they will be set to default values:
				// Return code will be a 200 (HTTP OK)
				// Headers will be empty ({})
				res.Payload = []byte(lua.LVAsString(L.Get(1)))
				res.Code = int(lua.LVAsNumber(L.Get(2)))

				if res.Code == 0 {
					res.Code = http.StatusOK
				}

				lheaders := L.Get(3)
				if ltable, ok := lheaders.(*lua.LTable); ok {
					if ltable != nil {
						ltable.ForEach(func(k, v lua.LValue) {
							res.Headers[lua.LVAsString(k)] = lua.LVAsString(v)
						})
					}
				}

				r.Results.Store(name, res)
			} else {
				result := L.Get(-1)
				if val, ok := result.(lua.LString); ok {
					res.Payload = []byte(val.String())
					r.Results.Store(name, res)
					log.WithFields(fields).Debugf("Script output: \n%s\n", string(res.Payload))
				} else {
					log.WithFields(fields).Debug("Script did not return a string")
				}
			}
		}(script)
	}

	wg.Wait()
	log.WithField("subject", msg.Subject).Debugf("finished running %d scripts", len(scripts))

	if msg.Method == "" {
		replyFunc(r)
	} else {
		// Return the first value found
		// TODO: Maybe append all of the answers together?
		replyFunc(r)
	}
}

// Stop gracefully shuts down the ScriptExecutor and stops watching for messages
func (se *ScriptExecutor) Stop() {
	se.cancelFunc()
	log.Debug("ScriptExecutor stopped")
}
