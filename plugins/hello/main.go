package main

import (
	"fmt"

	"github.com/yuin/gopher-lua"
)

func Preload(L *lua.LState) {
	L.PreloadModule("hello", func(L *lua.LState) int {
		mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
			"print": print,
		})

		L.Push(mod)
		return 1
	})
}

func print(L *lua.LState) int {
	msg := "Hello from a Go plugin!"
	fmt.Println(msg)
	L.Push(lua.LString(msg))
	return 1
}
