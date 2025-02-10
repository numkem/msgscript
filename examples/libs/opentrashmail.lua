--* name: opentrashmail
local http = require("http")
local json = require("json")

OpenTrashMail = {
    hostname = ""
}

function OpenTrashMail:new(hostname)
    self.hostname = hostname
end

function OpenTrashMail:rawEmailById(address, id)
    local resp, err = http.get(string.format("%s/api/raw/%s/%s", self.hostname, address, id))
    assert(err == nil)

    return resp.body
end

function OpenTrashMail:emailsForAddress(address)
    local resp, err = http.get(string.format("%s/json/%s", self.hostname, address))
    assert(err == nil)

    return json.decode(resp.body)
end

function OpenTrashMail:bodyForEmail(address, id)
    local resp, err = http.get(string.format("%s/json/%s/%s", self.hostname, address, id))
    assert(err == nil)

    json.decode(resp.body)
end

function OpenTrashMail:ListAccounts()
    local resp, err = http.get(string.format("%s/json/listaccounts", self.hostname))
    assert(err == nil)

    json.decode(resp.body)
end
