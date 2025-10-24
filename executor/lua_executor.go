package executor

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/cjoudrey/gluahttp"
	luajson "github.com/layeh/gopher-json"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"github.com/yuin/gluare"
	lua "github.com/yuin/gopher-lua"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	lfs "layeh.com/gopher-lfs"

	luamodules "github.com/numkem/msgscript/lua"
	msgplugins "github.com/numkem/msgscript/plugins"
	"github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

var luaTracer = otel.Tracer("msgscript.executor.lua")

// LuaExecutor defines the structure responsible for managing Lua script execution
type LuaExecutor struct {
	cancelFunc context.CancelFunc       // Context cancellation function
	ctx        context.Context          // Context for cancellation
	nc         *nats.Conn               // Connection to NATS
	store      msgstore.ScriptStore     // Interface for the script storage backend
	plugins    []msgplugins.PreloadFunc // Plugins to load before execution
}

// NewLuaExecutor creates a new ScriptExecutor using the provided ScriptStore
func NewLuaExecutor(c context.Context, store msgstore.ScriptStore, plugins []msgplugins.PreloadFunc, nc *nats.Conn) Executor {
	ctx, cancelFunc := context.WithCancel(c)

	return &LuaExecutor{
		cancelFunc: cancelFunc,
		ctx:        ctx,
		nc:         nc,
		store:      store,
		plugins:    plugins,
	}
}

// HandleMessage receives a message, matches it to a Lua script, and executes the script in a new goroutine
// Run the Lua script in a separate goroutine to handle the message for each script
func (le *LuaExecutor) HandleMessage(ctx context.Context, msg *Message, scr *script.Script) *ScriptResult {
	ctx, span := luaTracer.Start(ctx, "lua.handle_message",
		trace.WithAttributes(
			attribute.String("subject", msg.Subject),
			attribute.String("method", msg.Method),
			attribute.Int("payload_size", len(msg.Payload)),
		),
	)
	defer span.End()

	fields := log.Fields{
		"subject":  msg.Subject,
		"executor": "lua",
	}

	ss := strings.Split(scr.Name, "/")
	name := ss[len(ss)-1]
	fields["path"] = name

	// Create a child span for this script execution
	_, scriptSpan := luaTracer.Start(ctx, "lua.build_script",
		trace.WithAttributes(
			attribute.String("script.name", scr.Subject),
			attribute.String("script.path", scr.Name),
			attribute.Bool("script.is_html", scr.HTML),
			attribute.Int("script.lib_count", len(scr.LibKeys)),
		),
	)
	defer scriptSpan.End()

	res := new(ScriptResult)

	tmp, err := os.MkdirTemp(os.TempDir(), "msgscript-lua-*s")
	if err != nil {
		scriptSpan.RecordError(err)
		scriptSpan.SetStatus(codes.Error, "Failed to create temp directory")

		res.Error = fmt.Sprintf("failed to create temp directory: %w", err)
		return nil
	}
	defer os.RemoveAll(tmp)

	err = os.Chdir(tmp)
	if err != nil {
		scriptSpan.RecordError(err)
		scriptSpan.SetStatus(codes.Error, "Failed to change directory")

		res.Error = fmt.Sprintf("failed to change to temp directory %s: %w", tmp, err)
		return nil
	}
	scriptSpan.SetAttributes(attribute.String("temp_dir", tmp))

	// Load libraries
	_, libSpan := luaTracer.Start(ctx, "lua.load_libraries",
		trace.WithAttributes(
			attribute.Int("library_count", len(scr.LibKeys)),
		),
	)
	defer libSpan.End()
	libs, err := le.store.LoadLibrairies(ctx, scr.LibKeys)
	if err != nil {
		libSpan.RecordError(err)
		libSpan.SetStatus(codes.Error, "Failed to load libraries")

		scriptSpan.RecordError(err)
		scriptSpan.SetStatus(codes.Error, "Failed to load libraries")

		res.Error = fmt.Errorf("failed to read librairies: %w", err).Error()
		return res
	}
	libSpan.SetStatus(codes.Ok, "")

	// Acquire lock
	_, lockSpan := luaTracer.Start(ctx, "lua.acquire_lock",
		trace.WithAttributes(
			attribute.String("lock.name", scr.Name),
		),
	)
	defer lockSpan.End()

	locked, err := le.store.TakeLock(ctx, scr.Name)
	if err != nil {
		lockSpan.RecordError(err)
		lockSpan.SetStatus(codes.Error, "Failed to acquire lock")

		scriptSpan.RecordError(err)
		scriptSpan.SetStatus(codes.Error, "Failed to load libraries")

		log.WithFields(fields).Debugf("failed to get lock: %s", err)
		res.Error = fmt.Sprintf("failed to get lock: %s", err)
		return res
	}

	if !locked {
		lockSpan.SetStatus(codes.Error, "Lock not acquired")

		scriptSpan.SetStatus(codes.Error, "Could not acquire lock")

		log.WithFields(fields).Debug("we don't have a lock, giving up")
		res.Error = "cannot get lock"
		return res
	}
	lockSpan.SetStatus(codes.Ok, "Lock acquired")

	defer le.store.ReleaseLock(ctx, scr.Name)

	log.WithFields(fields).WithField("isHTML", scr.HTML).Debug("executing script")

	// Initialize Lua state
	_, luaInitSpan := luaTracer.Start(ctx, "lua.initialize_state")
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
			luaInitSpan.RecordError(err)
			luaInitSpan.SetStatus(codes.Error, "Failed to load plugins")

			scriptSpan.RecordError(err)
			scriptSpan.SetStatus(codes.Error, "Failed to load plugins")

			res.Error = fmt.Sprintf("failed to load plugin: %w", err)
			return res
		}
	}
	luaInitSpan.SetStatus(codes.Ok, "")
	luaInitSpan.End()

	// Build script content
	var sb strings.Builder
	for _, l := range libs {
		sb.Write(l)
		sb.WriteString("\n")
	}
	sb.Write(scr.Content)
	scriptContent := sb.String()
	scriptSpan.SetAttributes(attribute.Int("script.content_size", len(scriptContent)))
	log.WithFields(fields).Debugf("script:\n%+s\n\n", scriptContent)

	// Execute Lua script
	_, execSpan := luaTracer.Start(ctx, "lua.execute_script")
	if err := L.DoString(scriptContent); err != nil {
		scriptSpan.RecordError(err)
		scriptSpan.SetStatus(codes.Error, "Script execute error")

		msg := fmt.Sprintf("error executing Lua script: %s", err)
		log.WithFields(fields).Errorf(msg)
		res.Error = err.Error()
		return nil
	}
	execSpan.SetStatus(codes.Ok, "")
	execSpan.End()

	// Execute the appropriate message handler
	if scr.HTML {
		// If the message is set to return HTML, we pass 2 things to the fonction named after the HTTP
		// method received ex: POST(), GET()...
		// The 2 things are:
		//   - The URL part after the function name
		//   - The body of the HTTP call
		res = le.executeHTMLMessage(ctx, fields, L, msg, scr.Name)
	} else {
		// If we do not have an HTML based message, we call the function named
		// OnMessage() with 2 parameters:
		//   - The subject
		//   - The body of the message
		res = le.executeRawMessage(ctx, fields, L, msg, scr.Name)
	}

	scriptSpan.SetStatus(codes.Ok, "Script executed successfully")

	return res
}

func (*LuaExecutor) executeHTMLMessage(ctx context.Context, fields log.Fields, L *lua.LState, msg *Message, name string) *ScriptResult {
	_, span := luaTracer.Start(ctx, "lua.execute_html_message",
		trace.WithAttributes(
			attribute.String("script.name", name),
			attribute.String("http.method", msg.Method),
		),
	)
	defer span.End()

	res := new(ScriptResult)

	log.WithFields(fields).Debug("Running HTML based script")
	gMethod := L.GetGlobal(msg.Method)
	if msg.Method != "" && gMethod.Type().String() != "nil" {
		if err := L.CallByParam(lua.P{
			Fn:      gMethod,
			NRet:    3,
			Protect: true,
		}, lua.LString(msg.URL), lua.LString(string(msg.Payload))); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("Failed to call %s function", msg.Method))

			res.Error = fmt.Errorf("failed to call %s function: %w", msg.Method, err).Error()
			return res
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

	span.SetAttributes(
		attribute.Int("http.status_code", res.Code),
		attribute.Int("response.size", len(res.Payload)),
		attribute.Int("response.header_count", len(res.Headers)),
	)
	span.SetStatus(codes.Ok, "HTML message executed")

	return res
}

func (*LuaExecutor) executeRawMessage(ctx context.Context, fields log.Fields, L *lua.LState, msg *Message, name string) *ScriptResult {
	_, span := luaTracer.Start(ctx, "lua.execute_raw_message",
		trace.WithAttributes(
			attribute.String("script.name", name),
			attribute.String("subject", msg.Subject),
		),
	)
	defer span.End()

	res := new(ScriptResult)
	log.WithFields(fields).Debug("Running standard script")

	gOnMessage := L.GetGlobal("OnMessage")
	if gOnMessage.Type().String() == "nil" {
		span.SetStatus(codes.Error, "OnMessage function not found")
		res.Error = "failed to find global function named 'OnMessage'"
		return res
	}

	// Call the "OnMessage" function
	err := L.CallByParam(lua.P{
		Fn:      gOnMessage,
		NRet:    1,
		Protect: true,
	}, lua.LString(msg.Subject), lua.LString(string(msg.Payload)))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to call OnMessage")
		res.Error = fmt.Errorf("failed to call OnMessage function: %w", err).Error()
		return res
	}

	result := L.Get(-1)
	val, ok := result.(lua.LString)
	if ok {
		res.Payload = []byte(val.String())
		span.SetAttributes(attribute.Int("response.size", len(res.Payload)))
		span.SetStatus(codes.Ok, "Raw message executed")

		log.WithFields(fields).Debugf("Script output: \n%s\n", string(res.Payload))
	} else {
		span.SetStatus(codes.Error, "Script did not return a string")

		log.WithFields(fields).Debug("Script did not return a string")
	}

	return res
}

// Stop gracefully shuts down the ScriptExecutor and stops watching for messages
func (se *LuaExecutor) Stop() {
	se.cancelFunc()
	log.Debug("LuaExecutor stopped")
}
