package script

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScriptReaderRead(t *testing.T) {
	header := `--* subject: funcs.pushover
--* name: pushover
local http = require("http")
local json = require("json")

function OnMessage(_, payload)
end`

	r := strings.NewReader(header)

	reader := new(ScriptReader)
	err := reader.Read(r)
	assert.Nil(t, err)

	assert.Equal(t, "pushover", reader.Script.Name)
	assert.Equal(t, "funcs.pushover", reader.Script.Subject)
	assert.Equal(t, header, string(reader.Script.Content))
}
