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

	s := new(Script)
	err := s.Read(r)
	assert.Nil(t, err)

	assert.Equal(t, "pushover", s.Name)
	assert.Equal(t, "funcs.pushover", s.Subject)
	assert.Equal(t, header, string(s.Content))
}
