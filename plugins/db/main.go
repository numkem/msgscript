package main

import (
	"github.com/tengattack/gluasql"
	mysql "github.com/tengattack/gluasql/mysql"
	sqlite3 "github.com/tengattack/gluasql/sqlite3"
	"github.com/yuin/gopher-lua"
)

func Preload(L *lua.LState) {
	L.PreloadModule("mysql", mysql.Loader)
	L.PreloadModule("sqlite3", sqlite3.Loader)
	gluasql.Preload(L)
}
