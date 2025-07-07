package main

import (
	"github.com/felipejfc/gluahttpscrape"
	"github.com/yuin/gopher-lua"
)

func Preload(L *lua.LState, envs map[string]string) {
	L.PreloadModule("scrape", gluahttpscrape.NewHttpScrapeModule().Loader)
}
