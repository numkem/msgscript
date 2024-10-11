package lua

import (
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/yuin/gopher-lua"
)

var nc *nats.Conn

// Preload adds the NATS module to the given Lua state.
func PreloadNats(L *lua.LState, conn *nats.Conn) {
	L.PreloadModule("nats", natsLoader)
	nc = conn
}

func natsLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"publish": natsPublish,
	})
	L.Push(mod)
	return 1
}

func natsPublish(L *lua.LState) int {
	if nc == nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("Not connected to NATS"))
		return 2
	}

	subject := L.ToString(1)
	message := L.ToString(2)

	err := nc.Publish(subject, []byte(message))
	if err != nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(fmt.Sprintf("Failed to publish message: %v", err)))
		return 2
	}

	L.Push(lua.LBool(true))
	return 1
}
