--* name: vaulttest
--* subject: app.vaulttest
--* html: true

local vault = require("vault")

function GET(_, _)
    local v, err = vault.new("", "", true)
    if err ~= nil then
        return "conn: " .. err
    end

    err = v:write("/foo", { foo = "bar" }, "secrets")
    if err ~= nil then
        return "write: " .. err
    end

    local val
    val, err = v:read("/foo", "secrets")
    if err ~= nil then
        return "read: " .. err
    end

    if val['foo'] ~= "bar" then
        return "wrong data returned"
    end

    local list
    list, err = v:list("/", "secrets")
    if err ~= nil then
        return "list: " .. err
    end

    if #list ~= 1 then
        return "list returned wrong number of keys, expected 1 got " .. #list
    end

    err = v:delete("/foo", "secrets")
    if err ~= nil then
        return "delete: " .. err
    end

    return "passed!"
end
