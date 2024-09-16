--* subject: funcs.nats_publish
--* name: nats_publish
local nats = require("nats")
local json = require("json")

function OnMessage(_, _)
    nats.publish("funcs.pushover", json.encode({ title = "booyah", message = "boo-YAH" }))
end
