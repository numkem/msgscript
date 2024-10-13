--* subject: funcs.localio
--* name: write

function OnMessage(_, payload)
    local filename = "lua_test.out"
    local file = io.open(filename, "a")
    assert(file ~= nil)

    file:write("testing 123 from nats")
    file:write("\n" .. payload)
    file:close()

    return "written to file " .. filename
end
