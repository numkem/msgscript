function OnMessage(subject, payload)
  local filename = "/tmp/lua_test.out"
  file = io.open(filename, "a")

  file:write("testing 123 from nats")
  file:write("\n"..payload)
  file:close()

  return "written to file "..filename
end
