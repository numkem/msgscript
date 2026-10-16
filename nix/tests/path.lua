--* subject: funcs.path
--* name: path
local cmd = require("cmd")

function OnMessage(_, _)
    local result, err = cmd.exec("hello")
    if err ~= nil then
        return string.format("error: %s", err)
    end

    return result.stdout
end
