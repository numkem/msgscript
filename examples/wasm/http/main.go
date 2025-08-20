package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type ScriptResult struct {
	Code    int               `json:"http_code"`
	Error   string            `json:"error"`
	Headers map[string]string `json:"http_headers"`
	IsHTML  bool              `json:"is_html"`
	Payload []byte            `json:"payload"`
}

func main() {
	html := `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Wasm Hello World</title>
</head>
<body>
  <h1>Hello from Wasm!</h1>
</body>
</html>
`
	result := ScriptResult{
		Code:    200,
		IsHTML:  true,
		Payload: []byte(html),
	}

	err := json.NewEncoder(os.Stdout).Encode(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode result: %w", err)
	}

	os.Exit(0)
}
