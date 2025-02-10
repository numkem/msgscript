--* name: web
local template = require("template")
local mustache, _ = template.choose("mustache")
local json = require("json")

Router = {}
Router.__index = Router

-- Methods to add a handler to a route
function Router:get(url, handler)
    self.routes["GET"][url] = handler
end

function Router:post(url, handler)
    self.routes["POST"][url] = handler
end

function Router:put(url, handler)
    self.routes["PUT"][url] = handler
end

function Router:patch(url, handler)
    self.routes["PATCH"][url] = handler
end

function Router:head(url, handler)
    self.routes["HEAD"][url] = handler
end

function Router:options(url, handler)
    self.routes["OPTIONS"][url] = handler
end

function Router:delete(url, handler)
    self.routes["DELETE"][url] = handler
end

local LIBWEB_HTTP_VERBS = {
    "GET", "POST", "PUT", "PATCH", "HEAD", "OPTIONS", "DELETE"
}

function Router.new(layout)
    if layout == nil or layout == "" then
        layout = [[
<html>
  <header>
    {{& header }}
  </header>
  <body>
    {{& body }}
    {{& footer }}
  </body>
</html>
]]
    end

    local router = setmetatable({
        layout = layout,
        routes = {},
    }, Router)
    for _, m in pairs(LIBWEB_HTTP_VERBS) do
        router.routes[m] = {}
    end

    -- Setup handlers for each of the HTTP methods
    for _, m in pairs(LIBWEB_HTTP_VERBS) do
        _G[m] = function (url, body)
            return router:handle(m, url, body)
        end
    end

    return router
end

function Router:handle(method, url, payload)
    -- extract the path and query from the url
    local path, query = self:parseRequest(method, url)

    -- First, check for exact route match
    if self.routes[method][path] then
        local tmpl_ret, data, code, opts = self.routes[method][path](Request.new(query), payload)
        return self:processResponse(tmpl_ret, data, code, opts)
    end

    -- If no exact match, check for pattern routes
    for pattern, handler in pairs(self.routes[method]) do
        local params = self:matchURLPattern(pattern, path)
        if next(params) ~= nil then
            -- Create a request with both query and URL params
            local queries = {}
            for k, v in pairs(query or {}) do
                queries[k] = v
            end

            local paths = {}
            for k, v in pairs(params) do
                paths[k] = v
            end

            local tmpl_ret, data, code, opts = handler(Request.new(queries, paths), payload)
            return self:processResponse(tmpl_ret, data, code, opts)
        end
    end

    -- No route found
    return string.format("URL %s doesn't exist", url), 404, {}
end

function Router:processResponse(tmpl_ret, data, code, opts)
    if data == nil then data = {} end
    if code == nil then code = 200 end
    if opts == nil then opts = { headers = {}, no_template = false } end

    -- If we have no template ("" or nil), we use the data as a JSON response
    if tmpl_ret == "" or tmpl_ret == nil then
        return json.encode(data), code, { ["Content-Type"] = "application/json" }
    end

    local handler_body = mustache:render(tmpl_ret, data)

    if not opts.no_template then
        return mustache:render(self.layout, { header = opts.header, body = handler_body, footer = opts.footer }), code, opts.headers
    else
        return handler_body, code, opts.headers
    end
end

function Router:matchURLPattern(pattern, url)
    local result = {}

    local patternParts = {}
    for part in pattern:gmatch("[^/]+") do
        table.insert(patternParts, part)
    end

    local urlParts = {}
    for part in url:gmatch("[^/]+") do
        table.insert(urlParts, part)
    end

    if #patternParts > #urlParts then
        return result
    end

    for i, patternPart in ipairs(patternParts) do
        if i > #urlParts then
            break
        end

        local urlPart = urlParts[i]

        if patternPart:match("^<.*>$") then
            local key = patternPart:sub(2, -2)
            result[key] = urlPart
        elseif patternPart ~= urlPart and not patternPart:match("^<.*>$") then
            return {}
        end
    end

    return result
end

function Router:parseRequest(method, url)
    local queryStr = ""
    local path = ""
    if method == "GET" then
        local ss = url:split("?")
        if #ss == 1 then
            if ss[1] == "" then
                table.insert(ss, 1, "/")
            end

            table.insert(ss, 2, "")
        end

        path = ss[1]
        queryStr = ss[2]
    else
        path = url
    end

    return path, self:parseQuery(queryStr)
end

function Router:parseQuery(queryStr)
    if string.find(queryStr, "&") ~= 0 then
        local queries = {}

        for _, q in ipairs(queryStr:split("&")) do
            local eq = q:split("=")
            queries[eq[1]] = eq[2]
        end

        return queries
    else
        return {}
    end
end

-- Returns a table splitting some string with a delimiter
function string:split(delimiter)
    local result = {}
    local from = 1
    local delim_from, delim_to = string.find(self, delimiter, from, true)
    while delim_from do
        if (delim_from ~= 1) then
            table.insert(result, string.sub(self, from, delim_from - 1))
        end
        from = delim_to + 1
        delim_from, delim_to = string.find(self, delimiter, from, true)
    end
    if (from <= #self) then table.insert(result, string.sub(self, from)) end
    return result
end

--
-- Request: Passed to a handler as first parameter
--
Request = {}
Request.__index = Request

function Request.new(queries, paths)
    return setmetatable({ queries = queries, paths = paths }, Request)
end

function Request:query(name)
    if name == nil then
        return self.queries
    else
        return self.queries[name]
    end
end

function Request:path(name)
    if name == nil then
        return self.paths
    else
        return self.paths[name]
    end
end
