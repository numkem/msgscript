local cmd = require("cmd")

function Nixshell(command, packages)
    local full_command = string.format("nix-shell -p %s --run '%s'", table.concat(packages, " "), command)
    local result, err = cmd.exec(full_command)
    assert(err == nil)

    return result.stdout, result.stderr
end
