--* subject: funcs.hello_plugin
--* name: hello
local hello = require("hello")

function OnMessage(_, _)
    return hello.print()
end
