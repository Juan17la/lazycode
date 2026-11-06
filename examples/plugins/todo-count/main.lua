local lc = require("lazycode")

lc.command("todos", function()
  local buf = lc.current_buffer()
  if not buf then
    lc.message("There's no file here.")
    return
  end
  local text = buf:text()
  local _, todos = text:gsub("TODO", "")
  local _, fixmes = text:gsub("FIXME", "")
  lc.message(todos .. " TODO, " .. fixmes .. " FIXME")
end)

lc.key("ctrl+alt+t", "todos")
