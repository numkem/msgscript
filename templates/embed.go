package templates

import (
	_ "embed"
)

//go:embed list.tmpl
var LIST []byte

//go:embed names.tmpl
var NAMES []byte

//go:embed info.tmpl
var INFO []byte
