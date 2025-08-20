package executor

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/cjoudrey/gluahttp"
	luajson "github.com/layeh/gopher-json"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"github.com/yuin/gluare"
	lua "github.com/yuin/gopher-lua"
	lfs "layeh.com/gopher-lfs"

	luamodules "github.com/numkem/msgscript/lua"
	msgplugins "github.com/numkem/msgscript/plugins"
	scriptLib "github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

// LuaExecutor defines the structure responsible for managing Lua script execution
type LuaExecutor struct {
	cancelFunc context.CancelFunc       // Context cancellation function
	ctx        context.Context          // Context for cancellation
	nc         *nats.Conn               // Connection to NATS
	store      msgstore.ScriptStore     // Interface for the script storage backend
	plugins    []msgplugins.PreloadFunc // Plugins to load before execution
}

// NewLuaExecutor creates a new ScriptExecutor using the provided ScriptStore
func NewLuaExecutor(store msgstore.ScriptStore, plugins []msgplugins.PreloadFunc, nc *nats.Conn) Executor {
	ctx, cancelFunc := context.WithCancel(context.Background())

	return &LuaExecutor{
		cancelFunc: cancelFunc,
		ctx:        ctx,
		nc:         nc,
		store:      store,
		plugins:    plugins,
	}
}

func replyWithError(fields log.Fields, replyFunc func(r *Reply), msg string, a ...any) {
	e := fmt.Errorf(msg, a...)
	log.WithFields(fields).Error(e)
	replyFunc(&Reply{Error: e.Error()})
}

// HandleMessage receives a message, matches it to a Lua script, and executes the script in a new goroutine
func (le *LuaExecutor) HandleMessage(ctx context.Context, msg *Message, replyFunc func(r *Reply)) {
	fields := log.Fields{
		"subject": msg.Subject,
	}

	// Look up the Lua script for the given subject
	scripts, err := le.store.GetScripts(ctx, msg.Subject)
	if err != nil || scripts == nil {
		replyWithError(fields, replyFunc, "failed to get scripts for subject %s: %v", msg.Subject, err)
		return
	}

	errs := make(chan error, len(scripts))
	var wg sync.WaitGroup
	r := NewReply()
	// Loop through each scripts attached to the subject as there might be more than one
	for path, script := range scripts {
		wg.Add(1)

		ss := strings.Split(path, "/")
		name := ss[len(ss)-1]
		fields["path"] = name

		// Run the Lua script in a separate goroutine to handle the message for each script
		go func(content []byte) {
			defer wg.Done()

			tmp, err := os.MkdirTemp(os.TempDir(), "msgscript-lua-*s")
			if err != nil {
				errs <- fmt.Errorf("failed to create temp directory: %w", err)
				return
			}
			defer os.RemoveAll(tmp)

			err = os.Chdir(tmp)
			if err != nil {
				errs <- fmt.Errorf("failed to change to temp directory %s: %w", tmp, err)
				return
			}

			// Read the script to get the headers (for the libraries for example)
			s, err := scriptLib.ReadString(string(content))
			if err != nil {
				errs <- fmt.Errorf("failed to read script: %w", err)
				return
			}

			libs, err := le.store.LoadLibrairies(ctx, s.LibKeys)
			if err != nil {
				errs <- fmt.Errorf("failed to read librairies: %w", err)
				return
			}

			locked, err := le.store.TakeLock(ctx, name)
			if err != nil {
				log.WithFields(fields).Debugf("failed to get lock: %w", err)
				return
			}

			if !locked {
				log.WithFields(fields).Debug("we don't have a lock, giving up")
				return
			}
			defer le.store.ReleaseLock(ctx, name)

			log.WithFields(fields).WithField("isHTML", s.HTML).Debug("executing script")

			L := lua.NewState()
			tctx, tcan := context.WithTimeout(le.ctx, MAX_LUA_RUNNING_TIME)
			defer tcan()
			L.SetContext(tctx)
			defer L.Close()

			// Set up the Lua state with the subject and payload
			L.PreloadModule("http", gluahttp.NewHttpModule(&http.Client{}).Loader)
			L.PreloadModule("re", gluare.Loader)
			lfs.Preload(L)
			luajson.Preload(L)
			luamodules.PreloadEtcd(L)
			luamodules.PreloadNats(L, le.nc)

			// Load plugins
			if le.plugins != nil {
				err = msgplugins.LoadPlugins(L, le.plugins)
				if err != nil {
					errs <- fmt.Errorf("failed to load plugin: %w", err)
					return
				}
			}

			var sb strings.Builder
			for _, l := range libs {
				sb.Write(l)
				sb.WriteString("\n")
			}
			sb.Write(content)
			log.WithFields(fields).Debugf("script:\n%+s\n\n", sb.String())

			res := &ScriptResult{
				IsHTML:  s.HTML,
				Headers: make(map[string]string),
			}
			if err := L.DoString(sb.String()); err != nil {
				msg := fmt.Sprintf("error parsing Lua script: %w", err)
				log.WithFields(fields).Errorf(msg)
				res.Error = err.Error()
				r.Results.Store(name, res)
				return
			}

			// Retrieve the result from the Lua state (assuming it's a string)
			if s.HTML {
				// If the message is set to return HTML, we pass 2 things to the fonction named after the HTTP
				// method received ex: POST(), GET()...
				// The 2 things are:
				//   - The URL part after the function name
				//   - The body of the HTTP call
				le.executeHTMLMessage(fields, L, msg, r, res, name)
			} else {
				// If we do not have an HTML based message, we call the function named
				// OnMessage() with 2 parameters:
				//   - The subject
				//   - The body of the message
				le.executeRawMessage(fields, L, r, msg, res, name)
			}
		}(script)
	}
	wg.Wait()
	log.WithField("subject", msg.Subject).Debugf("finished running %d scripts", len(scripts))

	close(errs)
	for e := range errs {
		r.Error = r.Error + " " + e.Error()
	}

	replyFunc(r)
}

func (*LuaExecutor) executeHTMLMessage(fields log.Fields, L *lua.LState, msg *Message, reply *Reply, res *ScriptResult, name string) {
	log.WithFields(fields).Debug("Running HTML based script")
	gMethod := L.GetGlobal(msg.Method)
	if msg.Method != "" && gMethod.Type().String() != "nil" {
		if err := L.CallByParam(lua.P{
			Fn:      gMethod,
			NRet:    3,
			Protect: true,
		}, lua.LString(msg.URL), lua.LString(string(msg.Payload))); err != nil {
			reply.Error = fmt.Errorf("failed to call %s function: %w", msg.Method, err).Error()
			return
		}
	}

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

	reply.Results.Store(name, res)
}

func (*LuaExecutor) executeRawMessage(fields log.Fields, L *lua.LState, reply *Reply, msg *Message, res *ScriptResult, name string) {
	log.WithFields(fields).Debug("Running standard script")

	gOnMessage := L.GetGlobal("OnMessage")
	if gOnMessage.Type().String() == "nil" {
		reply.Error = "failed to find global function named 'OnMessage'"
		return
	}

	// Call the "OnMessage" function
	err := L.CallByParam(lua.P{
		Fn:      gOnMessage,
		NRet:    1,
		Protect: true,
	}, lua.LString(msg.Subject), lua.LString(string(msg.Payload)))
	if err != nil {
		reply.Error = fmt.Errorf("failed to call OnMessage function: %w", err).Error()
		return
	}

	result := L.Get(-1)
	if val, ok := result.(lua.LString); ok {
		res.Payload = []byte(val.String())
		reply.Results.Store(name, res)
		log.WithFields(fields).Debugf("Script output: \n%s\n", string(res.Payload))
	} else {
		log.WithFields(fields).Debug("Script did not return a string")
	}
}

// Stop gracefully shuts down the ScriptExecutor and stops watching for messages
func (se *LuaExecutor) Stop() {
	se.cancelFunc()
	log.Debug("LuaExecutor stopped")
}
