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
    - [In HTTP+HTML mode](#in-httphtml-mode)
  - [Return value](#return-value)
- [HTTP handler](#http-handler)
  - [Special HTTP Endpoints](#special-http-endpoints)
- [Installation](#installation)
  - [Docker container](#docker-container)
  - [NixOS service](#nixos-service)
  - [Building it](#building-it)
    - [Worker tags](#worker-tags)
- [Clustering](#clustering)
  - [Adding Scripts](#adding-scripts)
- [Command line flags](#command-line-flags)
  - [Cli options](#cli-options)
    - [Commands](#commands)
    - [dev](#dev)
    - [devhttp](#devhttp)
    - [Command line options](#command-line-options)
- [Worker](#worker)
  - [Executors](#executors)
    - [Lua](#lua)
      - [Inputs](#inputs)
        - [Normal mode](#normal-mode)
        - [Web Mode](#web-mode)
      - [Outputs](#outputs)
        - [Normal Mode](#normal-mode)
        - [Web Mode](#web-mode-1)
      - [Plugin system](#plugin-system)
      - [Libraries](#libraries)
      - [Web "framework" library](#web-framework-library)
    - [WASM](#wasm)
      - [Inputs](#inputs-1)
      - [Outputs](#outputs-1)
    - [Podman](#podman)
      - [Inputs](#inputs-2)
      - [Outputs](#outputs-2)
- [Contributing](#contributing)
- [Support](#support)

<!-- markdown-toc end -->

TLDR: msgscript is what you could call a poor man's Lambda-like function/application server. It can run functions and even some small web based applications.

## Features

- Single binary
- Nearly no overheads
- Good enough performances (RTT of around 10ms for the hello example)
- Runs Lua functions, WASM binaries and Podman containers (no docker)
- Can use reusable libraries in Lua
- Can add Go based plugins in Lua
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
- `html`: When set (to any value), the response will contain HTML.
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
--* html: true

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

In HTTP+HTML mode, you can return 3 different values:
- The HTML as a string
- The HTTP code (200 is missing)
- The HTTP headers (empty if missing)

## HTTP handler

If you have a application that cannot reach NATS by itself (say a Web Hook), it's possible to use the included HTTP handler.

The server listens to port 7643 by default (it can be changed through the command line). You can push messages by doing a POST request to `http://serverIP:7643/<subject>` where the `<subject>` is any subjects that you have scripts registered to it.

Example using curl (if you are running locally and for the subject of the example above):

```
curl -X POST -d 'John' http://127.0.0.1:7643/http.hello
```

### Special HTTP Endpoints

Some endpoints exists to see the content of the store. These endpoints are meant to be used with a browser.

- `/_/` shows the `README.md` file converted to HTML
- `/_/list` shows a list of the current subjects
- `/_/subject/{subject}` list the scripts associated to a subject by name
- `/_/info/{subject}/{name}` shows information about a script including the source

## Installation

### Docker container

A container is avaible through the ghcr.io container registry. You can use it as so:

``` sh
docker pull ghcr.io/numkem/msgscript:latest
```

It takes either `server` or `cli` as a first parameter.

### NixOS service

You can enable it in your NixOS configuration using the provided module (once included from either the flake or importing the module using something like `niv` or manually):

Below is an example usage of the module with etcd and external NATS.
```nix
{ pkgs, lib, ... }:
let
  msgscriptFlake = builtins.getFlake "github:numkem/msgscript";
in
{
  imports = [
    msgscriptFlake.nixosModules.default
  ];

  services.msgscript = {
    enable = true;
    backend = "etcd";
    enableTelemetry = true;
    otelEndpoint = "127.0.0.1:4317";
    etcdEndpoints = [
      "http://127.0.0.1:2379"
    ];
    natsUrl = "nats://127.0.0.1:4222";
    plugins = with msgscriptFlake.packages."x86_64-linux".plugins; [
      db
      httpscrape
      gopher-lua-libs
      vault
    ];
    workerEnvironment = {
      PATH = lib.makeBinPath [
        pkgs.busybox
        pkgs.bash
        pkgs.curl
        pkgs.iputils
      ];
    };
  };
}
```

The options are defined in the [nix/modules/default.nix](nix/modules/default.nix) file.
 
### Building it
 
Being a standalone Go binary, you can build each of the binaries like so:
 ```sh
 # Clone the msgscript repository
 git clone https://github.com/numkem/msgscript.git
 cd msgscript
 go build ./cmd/cli    # Generates the CLI binary
 go build ./cmd/server # Generates the server binary
 go build ./cmd/worker # Generates the worker binary
 ```

#### Worker tags

It optionally requires the btrfs headers (podman), gpgme (podman) and wasmtime (wasm) as dependencies to build the worker code with full support for all each executor.

Example:
``` sh
go build cmd/worker -tags "wasmtime podman"
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

## Command line flags

### Cli options

The `cli` binary helps with managing the scripts in the message store and for helping with developing scripts.

#### Commands

  `add`         Add a script to the backend by reading the provided lua file
  `completion`  Generate the auto-completion script for the specified shell
  `dev`         Executes the script locally like how the server would
  `devhttp`     Starts a web server that will run only to receive request from this script
  `help`        Help about any command
  `lib`         library related commands
  `list`        list all the scripts registered in the store
  `rm`          Remove an existing script
  
The commands that manages scripts (add, list, rm) are not really useful when using the file base store.

#### dev

Useful for developing scripts that aren't HTTP based (web hooks).

First argument is the script in question. It will return the script's output.

#### devhttp

Useful for developing scripts that are HTTP based (web hooks).

First argument is the script in question. You can then reach your script at `http://localhost:7634/<subject>/`. The script is reloaded from the store on every HTTP request so you don't have to restart the command each time.

#### Command line options

| Flag        | Default          | Example                   | Description                                                              |
|-------------|------------------|---------------------------|--------------------------------------------------------------------------|
| `-backend`  | `file`           | `etcd`                    | Backend used for the script storage                                      |
| `-etcdurls` | `localhost:2379` | `etcd-master:2379`        | URL of the etcd server                                                   |
| `-help`     |                  |                           | Show help                                                                |
| `-library`  | (empty)          | `./libs`                  | Directory that contains lua Libraries. Only used with the file backend   |
| `-log`      | `info`           | `warning`                 | Log level to use (debug, info, warn, error)                              |
| `-natsurl`  | (empty)          | `nats://192.168.0.4:4222` | URL for the nats server. If not provided, a built-in one will be used.   |
| `-plugin`   | (empty)          | `./plugins`               | Lua plugin directory to load them from. Only used with the file backend  |
| `-port`     | `7643`           | `8888`                    | HTTP port to bind to                                                     |
| `-script`   | (empty)          | `./scripts`               | Directory that contains the scripts. Only used with the file backend     |
| `-wenv`     | (empty)          | `./worker.env`            | Path to a file containing environment variables for the worker. Ex: PATH |
| `-wexec`    | `./worker`       | `./worker-with-podman`    | Path to the worker executable                                            |

## Worker

Whenever a script is run, Msgscript spawns a process from `cmd/worker` which will run the script and return the result through NATS.

### Executors

Msgscript has support for multiple executors, namely what runs the code in the scripts.

* Lua is the fastest and the smallest. It's meant to be the primary choice as it doesn't need anything else to run.
* WASM requires a built binary specifically for the WASM (not WASI) runtime. The `examples/wasm` folder contains a C and Go workable example.
* Podman runs a container and cleans up afterwards.

#### Lua

This is the default executor.

When writing Lua scripts for msgscript, you have access to additional built-in modules:
- `etcd`: Read/Write/Update/Delete keys in etcd [source](lua/etcd.go)
- `http`: For making HTTP requests [source](https://github.com/cjoudrey/gluahttp)
- `json`: For JSON parsing and generation [source](https://github.com/layeh/gopher-json)
- `lfs`: LuaFilesystem implementation [source](https://layeh.com/gopher-lfs)
- `nats`: For publishing messages back to NATS [source](lua/nats.go)
- `re`: Regular expression library [source](https://github.com/yuin/gluare)

These can be included using the built-in `require()` Lua function.

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

##### Inputs

###### Normal mode

Msgscript for a function called `OnMessage`. This function doesn't have to be a `global` but it can optionally be. The function will receive 2 parameters:

| Suggested name | Description                                                                        |
|----------------|------------------------------------------------------------------------------------|
| `subject`      | Contains the actual subject as a string name on NATS. Can be ignored by using `_`. |
| `payload`      | Contains the payload from the NATS message.                                        |

The names of the variables are a suggestions and only the order matters. They can be ignored (`_`) or have any names supported by Lua.

###### Web Mode

Msgscript will call a function named after the HTTP verb used capitalized (ex: GET, POST, PUT, DELETE). This function doesn't have to be a `global` but it can optionally be. The function will receive 2 parameters:

* The request URL with the msgscript part stripped. Ex: `/foo/bar/1`.
* The payload sent in the HTTP request.

##### Outputs

###### Normal Mode

Everything returned from the function is sent back as the response.

###### Web Mode

The function called can return up to 3 variables:

| Description              | Default | Example                                  |
|--------------------------|---------|------------------------------------------|--------------------------------------------------------------|
| The body of the response | (empty) |                                          | `<html><head></head><body><h1>Hello world!</h1>/body><html>` |
| HTTP code                | `200`   | `404`, `500`, `302`...                   |
| Headers                  | `{}`    | `{ "Content-Type": "application/json" }` |

##### Plugin system

While there is already a lot of modules added to the Lua execution environment, it is possible to add more using the included plugin system.

An example [plugin](plugins/hello/main.go) is available. The plugins can be loaded using the `-plugin` flag for both the server and cli.

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

* `scrape`: An HTTP parser [source](https://github.com/felipejfc/gluahttpscrape)
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

##### Libraries

Libraries are Lua files that gets prepended to the script that needs to be run. These libraries can be used within other scripts using the `require` header like this:

``` lua
--* require: foo
```

In this case, it will load the library named `foo` and prepend it to the running script.

Some example libraries are available [here](examples/libs).

##### Web "framework" library

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
- The function handling the endpoint returns 4 values: the [mustache](https://mustache.github.io) template, the data for the template, the HTTP code and HTTP headers
- If the template is either empty (`""`) or nil, it will be assumed that it's returning a JSON document. The `Content-Type` will be set as such.

#### WASM

Binaires compiled targetting WASM can be used. Some examples are provided in the `examples/wasm` directory.

The format required in the store looks like this:

```
--* subject: funcs.wasm
--* name: wasm
--* executor: wasm
/home/numkem/src/msgscript/examples/wasm/c/c.wasm
```

The import parts are `subject`, `name` which are common with all other executors. The `executor` key needs to be set to `wasm`. The content is the path to the WASM executable.

##### Inputs

The following environment variables will be set for when the WASM binary is launched:

| Variable  | Example        | Description                                                             |
|-----------|----------------|-------------------------------------------------------------------------|
| `SUBJECT` | `app.wasmtest` | The subject of the script                                               |
| `METHOD`  | `GET`          | When called through the web interface, the HTTP method will be set here |
| `URL`     | `/foo/bar/1`   | When called through the web, remaining URL will be put here             |

stdin will contain the payload of the message.

##### Outputs

* stderr will be read for errors, if it's not empty the execution will be seen as failed
* stdout will be returned as-is in the message response's payload

#### Podman

The format for the script stored in the store is of a JSON. 

```
--* subject: funcs.hello
--* name: podman
--* executor: podman
{
    "image": "hello-world"
}
```

Like for WASM, it takes the same important keys. The content of the file can be:

| Key          | Description                                                                |
|--------------|----------------------------------------------------------------------------|
| `command`    | The command to use to start the container                                  |
| `group`      | The group running the container                                            |
| `image`      | Container image name                                                       |
| `mounts`     | List of mounts in the same format you would write them on the command line |
| `privileged` | true/false if the container should run with more permissions               |
| `user`       | The user running the container                                             |

##### Inputs

The following environment variables will be set for when the WASM binary is launched:

| Variable  | Example          | Description                                                             |
|-----------|------------------|-------------------------------------------------------------------------|
| `SUBJECT` | `app.podmantest` | The subject of the script                                               |
| `METHOD`  | `GET`            | When called through the web interface, the HTTP method will be set here |
| `URL`     | `/foo/bar/1`     | When called through the web, remaining URL will be put here             |

##### Outputs

* stderr will be read for errors, if it's not empty the execution will be seen as failed
* stdout will be returned as-is in the message response's payload

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Support

If you encounter any problems or have any questions, please open an issue on the GitHub repository.
