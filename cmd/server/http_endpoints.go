package main

import (
	"net/http"

	"github.com/numkem/msgscript"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

func logo(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "image/webp")
	w.Write(msgscript.LOGO)
}

func index(w http.ResponseWriter, r *http.Request) {
	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	md := p.Parse(msgscript.README)

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	w.WriteHeader(http.StatusOK)
	w.Write(markdown.Render(md, renderer))
}
