# msgscript

<img src="https://raw.github.com/numkem/msgscript/logo.webp" width="75%" height="75%">

<!-- markdown-toc start - Don't edit this section. Run M-x markdown-toc-refresh-toc -->
**Table of Contents**

- [msgscript](#msgscript)
    - [Features](#features)
    - [Installation](#installation)
    - [Dependencies](#dependencies)
    - [Usage](#usage)
        - [Adding Scripts](#adding-scripts)
        - [Writing Lua Scripts](#writing-lua-scripts)
    - [Contributing](#contributing)
    - [Support](#support)

<!-- markdown-toc end -->


msgscript is primarily a Go server that runs Lua functions internally based on NATS subjects.

## Features

- Single binary
- Nearly no overheads
- Good enough performances (rtt of around 316ms for the pushover example)
- Runs Lua functions based on NATS subjects
- Integrates with etcd for script storage

## Installation

msgscript is primarily designed to be used with Nix and NixOS. You can enable it in your NixOS configuration using the provided module:

```nix
services.msgscript.enable = true;
```

## Dependencies

The server requires:
- etcd
- NATS

Ensure these services are running and accessible to the msgscript server.

## Usage

### Adding Scripts

You can add Lua scripts to etcd using the `msgscriptcli` command. Here's an example:

```bash
msgscriptcli -subject funcs.pushover -name pushover ./examples/pushover.lua
```

This command adds the `pushover.lua` script from the `examples` directory, associating it with the subject `funcs.pushover` and the name `pushover`.

### Writing Lua Scripts

When writing Lua scripts for msgscript, you have access to additional modules:

- `json`: For JSON parsing and generation
- `http`: For making HTTP requests

These modules are provided by the `gopher-lua` libraries.

Some examples scripts are provided in the `examples` folder.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Support

If you encounter any problems or have any questions, please open an issue on the GitHub repository.
