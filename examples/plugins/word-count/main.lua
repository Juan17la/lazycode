local lc = require("lazycode")

lc.command("wc", function()
  local buf = lc.current_buffer()
  if not buf then
    lc.message("There's no file here.")
    return
  end
  local text = buf:text()
  local _, lines = text:gsub("\n", "\n")
  local _, words = text:gsub("%S+", "")
  lc.message(lines .. " lines, " .. words .. " words, " .. buf:len() .. " bytes")
end)
