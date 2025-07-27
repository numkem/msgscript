package script

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScriptReaderLuaRead(t *testing.T) {
	content := `local http = require("http")
local json = require("json")

function OnMessage(_, payload)
end`

	headers := `--* subject: funcs.foobar
--* name: foo
--* html: true
--* require: web
`
	s, err := ReadString(headers + content)
	assert.Nil(t, err)

	assert.Equal(t, "foo", s.Name)
	assert.Equal(t, "funcs.foobar", s.Subject)
	assert.Equal(t, content, string(s.Content))
	assert.Equal(t, true, s.HTML)
	assert.Equal(t, 1, len(s.LibKeys))
	assert.Equal(t, "web", s.LibKeys[0])
}

func TestScriptReaderWasmRead(t *testing.T) {
	content := `--* subject: funcs.foobar
--* name: foo
--* html: true
--* executor: wasm
/some/path/to/wasm/module.wasm
`
	s, err := ReadString(content)
	assert.Nil(t, err)

	assert.Equal(t, "foo", s.Name)
	assert.Equal(t, "funcs.foobar", s.Subject)
	assert.Equal(t, "wasm", s.Executor)
	assert.Equal(t, "/some/path/to/wasm/module.wasm\n", string(s.Content))
}

func TestScriptFileReaderWasmFileRead(t *testing.T) {
	s, err := ReadFile("../examples/wasm/http/wasm.lua")
	assert.Nil(t, err)

	assert.Equal(t, "wasm", s.Name)
	assert.Equal(t, "funcs.wasm", s.Subject)
	assert.Equal(t, "/home/numkem/src/msgscript/examples/wasm/http/http.wasm\n", string(s.Content))
}
