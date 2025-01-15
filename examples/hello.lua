--* subject: funcs.hello
--* name: hello

function OnMessage(subject, payload)
    local response = "Processed subject: " .. subject .. " with payload: " .. payload

    return response
end
