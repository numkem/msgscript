package main

import (
	"github.com/vadv/gopher-lua-libs/cmd"
	"github.com/vadv/gopher-lua-libs/crypto"
	"github.com/vadv/gopher-lua-libs/filepath"
	"github.com/vadv/gopher-lua-libs/inspect"
	"github.com/vadv/gopher-lua-libs/ioutil"
	"github.com/vadv/gopher-lua-libs/log"
	"github.com/vadv/gopher-lua-libs/runtime"
	"github.com/vadv/gopher-lua-libs/storage"
	"github.com/vadv/gopher-lua-libs/strings"
	"github.com/vadv/gopher-lua-libs/tac"
	"github.com/vadv/gopher-lua-libs/tcp"
	"github.com/vadv/gopher-lua-libs/template"
	"github.com/vadv/gopher-lua-libs/time"
	"github.com/vadv/gopher-lua-libs/yaml"
	"github.com/yuin/gopher-lua"
)

func Preload(L *lua.LState) {
	cmd.Preload(L)
	crypto.Preload(L)
	filepath.Preload(L)
	inspect.Preload(L)
	ioutil.Preload(L)
	log.Preload(L)
	runtime.Preload(L)
	storage.Preload(L)
	strings.Preload(L)
	tac.Preload(L)
	tcp.Preload(L)
	template.Preload(L)
	time.Preload(L)
	yaml.Preload(L)
}
