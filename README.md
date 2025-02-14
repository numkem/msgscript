<p align="center">
  <img src="logo.webp" width="50%" height="50%">
</p>

<!-- markdown-toc start - Don't edit this section. Run M-x markdown-toc-refresh-toc -->
**Table of Contents**

- [Features](#features)
- [How it works](#how-it-works)
  - [Headers](#headers)
  - [The Function](#the-function)
    - [In Normal mode](#in-normal-mode)
    - [In HTTP mode](#in-http-mode)
    - [Return value](#return-value)
  - [HTTP handler](#http-handler)
- [Installation](#installation)
  - [Single binary](#single-binary)
  - [NixOS service](#nixos-service)
  - [Building it](#building-it)
- [Clustering](#clustering)
  - [Adding Scripts](#adding-scripts)
- [Writing Lua Scripts](#writing-lua-scripts)
  - [Plugin system](#plugin-system)
  - [Libraries](#libraries)
  - [Web "framework" library](#web-framework-library)
- [Contributing](#contributing)
- [Support](#support)

<!-- markdown-toc end -->

TLDR: msgscript is what you could call a poor man's Lambda-like function/application server. It can run functions and even some small web based applications.

## Features

- Single binary
- Nearly no overheads
- Good enough performances (RTT of around 10ms for the hello example)
- Runs Lua functions
- Can use reusable libraries
- Can add Go based plugins
- HTTP handler
- Script storage either as flat files or in etcd
- Can be scaled up with multiples instances through locking (requires etcd)

## How it works

Let's start with an example script:
``` lua
--* subject: funcs.hello
--* name: hello

function OnMessage(subject, payload)
    local response = "Processed subject: " .. subject .. " with payload: " .. payload

    return response
end
```

For more technical details, a [flow chart](how_it_works.png) explains what happens when a message is sent and the server sees a match and than executes a script.

### Headers

The headers are formed with the pattern of `--* <header>: <value>`. There are multiple possible headers:
- `subject`: The subject the script is associated with
- `name`: The name of the script. Multiple scripts can be associated with the same subject
- `http`: Used to return HTML responses
- `require`: Used to load a library script. It comes from the library "repository" of scripts and is prepended to the script that will be executed.

Each script is a Lua file that gets executed when the server receives a message that matches a pattern. The pattern is defined in the `subject` field. The files also contains a `name` field. Multiple scripts can be associated with the same subject.

### The Function

#### In Normal mode

Normal mode is the default mode. The server executes the script when the message matches the `subject` and `name` fields. The function called is named `OnMessage()`. That function is fed 2 arguments, which can be named whatever you want. The first is the subject and the second is the payload. The payload is what the originating NATS message contains.

It's possible to call this mode both through NATS or with the HTTP handler.

#### In HTTP mode

Example, for a GET request:

``` lua
--* subject: http.hello
--* name: http_get
--* http: true

function GET(url, body)
    return "Hello, " .. body .. "!", 200, { ["Content-Type"] = "text/plain" }
end
```

The function executed will have the same name as the HTTP verb of the originating HTTP request.

This mode is only available with the HTTP handler.

#### In HTTP+HTML mode

If you want to use the HTTP handler and want to return HTML, you can do so by setting the `http` header to `true`.

Just like in HTTP mode, the function executed will have the same name as the HTTP verb of the originating HTTP request.

### Return value

The function is expected to return a string. If it does not, the server will log a warning: `Script returned no response`. 

In **NATS** mode: it will return the string as-is.

In **HTTP** mode: it will return the following JSON document:
``` json
{"calculate":{"http_code":0,"error":"","http_headers":{},"is_html":false,"payload":"SGkhIEknbSBlbmNvZGVkIQo="}}
```
Each keys at the root level is named after the `name` field of the script. The value is a table with the following keys:
- `http_code`: The HTTP status code. It defaults to 200
- `error`: If the script returns an error, it will be set here
- `http_headers`: A map of HTTP headers
- `is_html`: Whether the response is HTML
- `payload`: The payload of the message. It is base64 encoded

In **HTTP+HTML** mode: you can return 3 different values:
- The HTML as a string
- The HTTP code (200 is missing)
- The HTTP headers (empty if missing)

## HTTP handler

If you have a application that cannot reach nats by itself (say a webhook), it's possible to use the included HTTP handler.

The server listens to port 7643 by default (it can be changed through the command line). You can push messages by doing a POST request to `http://serverIP:7643/<subject>` where the `<subject>` is any subjects that you have scripts registered to it.

Example using curl (if you are running locally and for the subject of the example above):

```
curl -X POST -d 'John' http://127.0.0.1:7643/http.hello
```

## Installation

### Single binary

You can download the binary from the release page. There are 2 binaries available: `server` and `cli`. The server is what most people will want. The `cli` is only useful when paired with the `etcd` backend.

The server has the following options:
- `-backend`: The backend to use. Currently supports `etcd` or `file`. `file` is the default.
- `-etcdurl`: The URL of the etcd server. It can be multiple through a comma separated list.
- `-library`: The path to a library directory. It has no defaults. It can be an absolute path or a relative path.
- `-log`: The log level to use. The options are: `debug`, `info`, `warn`, `error`. It defaults to `info`. 
- `-natsurl`: The URL of the NATS server.
- `-plugin`: The path to the plugin directory. It has no defaults. It can be an absolute path or a relative path.
- `-port`: The port to listen on. It defaults to 7643.
- `-script`: The path to a script directory. It defaults to the current working directory. It can be an absolute path or a relative path.

### NixOS service

You can enable it in your NixOS configuration using the provided module (once included from either the flake or importing the module using something like `niv` or manually):

```nix
services.msgscript.enable = true;
```

 The options are defined in the [nix/modules/default.nix](nix/modules/default.nix) file.
 
### Building it
 
Being a standalone Go binary, you can build each of the binaries like so:
 ```sh
 # Clone the msgscript repository
 git clone https://github.com/numkem/msgscript.git
 cd msgscript
 go build ./cmd/server # Generates the server binary
 go build ./cmd/cli    # Generates the CLI binary
 ```

## Clustering

When msgscript is running in cluster mode, it's possible to use it with etcd. You can use the `etcd` backend to do that.

The `cli` binary provides some additional commands to manage the scripts stored inside etcd.

### Adding Scripts

You can add Lua scripts to etcd using the `msgscriptcli` command. Here's an example:

```sh
msgscriptcli add -subject funcs.pushover -name pushover ./examples/pushover.lua
```

This command adds the `pushover.lua` script from the `examples` directory, associating it with the subject `funcs.pushover` and the name `pushover`.

The `-subject` and `-name` flags are optional. If they are not provided, they will be read through the headers contained in the file.

## Writing Lua Scripts

When writing Lua scripts for msgscript, you have access to additional built-in modules:

- `etcd`: Read/Write/Update/Delete keys in etcd [source](lua/etcd.go)
- `http`: For making HTTP requests [source](https://github.com/cjoudrey/gluahttp)
- `json`: For JSON parsing and generation [source](https://github.com/layeh/gopher-json)
- `lfs`: LuaFilesystem implementation [source](https://layeh.com/gopher-lfs)
- `nats`: For publishing messages back to NATS [source](lua/nats.go)
- `re`: Regular expression library [source](https://github.com/yuin/gluare)

these can be included using the built-in `require()` Lua function.

The following example shows how to deserialize a JSON payload:
``` lua
--* subject: example.json
--* name: json
local json = require("json")

-- Assuming the payload contains: 
-- {"name": "John"}
function OnMessage(_, payload)
    local data = json.decode(payload)
    
    return "Hello, " .. data.name .. "!"
end
```

Some examples scripts are provided in the `examples` folder.

### Plugin system

While there is already a lot of modules added to the Lua execution environment, it is possible to add more using the included plugin system.

An example [plugin](plugins/hello/main.go) is available. The plugins can be loaded using the `--plugin` flag for both the server and cli.

Example using the hello plugin:

``` lua
--* subject: example.plugins.hello
--* name: hello
local hello = require("hello")

function OnMessage(_, _)
    return hello.print()
end
```

Plugins currently included in this repository:

* `scrape`: An http parser [source](https://github.com/felipejfc/gluahttpscrape)
* `db`: SQL access to MySQL, SQLite and PostgreSQL [source](https://github.com/tengattack/gluasql)
* `gopher-lua-libs`: Various modules from the [gopher-lua-libs](https://github.com/vadv/gopher-lua-libs):
    * `cmd`
    * `filepath`
    * `inspect`
    * `ioutil`
    * `runtime`
    * `strings`
    * `time`
    
**NOTE:** The plugin file needs to have the `.so` extension.

### Libraries

Libraries are Lua files that gets prepended to the script that needs to be run. These libraries can be used within other scripts using the `require` header like this:

``` lua
--* require: foo
```

In this case, it will load the library named `foo` and prepend it to the running script.

Some example libraries are available [here](examples/libs).

### Web "framework" library

The [web.lua](examples/libs/web.lua) library contains a very simple web framework. It's used in the example below:

``` lua
--* subject: example.libs.web
--* name: web
--* require: web
local json = require("json")

local router = Router.new()

router:get("/plain", function(req, _)
    return "Hello, World!", {}, 200, { ["Content-Type"] = "text/plain" }
end)

router:get("/json", function(req, _)
    return nil, { name = "John" }, 200
end)

router:get("/path/<foo>", function(req, _)
    return [[
    <p>{{ param }}</p>
    ]], { param = req.params.foo }, 200
end)

router:post("/post", function(req, _)
    doc = json.decode(req.body)
    return [[ Hello {{ name }}! ]], { name = doc.name }, 200
end)
```

While not extensive, these examples shows how to use the library.

Namely:
- The function handling the endpoint returns 4 values: the mustache template, the data for the template, the HTTP code and HTTP headers
- If the template is either empty (`""`) or nil, it will be assumed that it's returning a JSON document. The `Content-Type` will be set as such.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Support

If you encounter any problems or have any questions, please open an issue on the GitHub repository.
