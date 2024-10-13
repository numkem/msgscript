package main

import (
	"github.com/vadv/gopher-lua-libs/cmd"
	"github.com/vadv/gopher-lua-libs/filepath"
	"github.com/vadv/gopher-lua-libs/inspect"
	"github.com/vadv/gopher-lua-libs/ioutil"
	"github.com/vadv/gopher-lua-libs/runtime"
	"github.com/vadv/gopher-lua-libs/strings"
	"github.com/vadv/gopher-lua-libs/time"
	"github.com/yuin/gopher-lua"
)

func Preload(L *lua.LState) {
	cmd.Preload(L)
	filepath.Preload(L)
	inspect.Preload(L)
	ioutil.Preload(L)
	runtime.Preload(L)
	strings.Preload(L)
	time.Preload(L)
}
