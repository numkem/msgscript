--* subject: http.website
--* name: demo
--* http: true
local template = require("template")
local mustache, _ = template.choose("mustache")

local function renderPage(header, body)
    return string.format([[
<html>
  <header>
    %s
  </header>
  <body>
    %s
  </body>
</html>
]], header, body)
end

local website = {
    ["GET"] = {
        ["/foobar"] = function (_, _)
            return renderPage("", "<h1>foobar yo</h1>")
        end
    }
}

function GET(url, body)
    return website["GET"][url](url, body)
end
