--* subject: funcs.pushover
--* name: pushover
local http = require("http")
local json = require("json")

function OnMessage(_, payload)
    local p, err = json.decode(payload)
    assert(err == nil)

    local headers = {}
    headers["Content-Type"] = "application/json"

    local response, err_message = http.request("POST", "https://api.pushover.net/1/messages.json", {
        headers = headers,
        body = json.encode({
            token = "appToken",
            user = "userToken",
            message = p.message,
            title = p.title
        })
    })

    if (err_message ~= nil) then
        return "error: " .. err_message
    end

    return "HTTP " .. response.status_code .. ": " .. response.body
end
