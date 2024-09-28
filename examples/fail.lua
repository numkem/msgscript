-- This script is meant to fail as a example
--* subject: funcs.fail
--* name: fail

function OnMessage(_, _)
    someUndefinedFunction("fail")
end
