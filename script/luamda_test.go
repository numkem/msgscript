package script

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLuamdReaderReadString(t *testing.T) {
	str := `
--* GET: /foo/bar
--* POST: /foo/bar
--* PUT: /foo/bar
--* DELETE: /foo/bar
--* PATCH: /foo/bar
--* OPTIONS: /foo/bar
--* HEAD: /foo/bar
`

	r := &LuamdaReader{}
	err := r.ReadString(str)
	assert.Nil(t, err)

	assert.Len(t, r.Endpoints.GET, 1)
	assert.Len(t, r.Endpoints.POST, 1)
	assert.Len(t, r.Endpoints.PUT, 1)
	assert.Len(t, r.Endpoints.DELETE, 1)
	assert.Len(t, r.Endpoints.PATCH, 1)
	assert.Len(t, r.Endpoints.OPTIONS, 1)
	assert.Len(t, r.Endpoints.HEAD, 1)
}
