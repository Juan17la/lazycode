local lc = require("lazycode")

lc.on("buffer_save", function(buf)
  local text = buf:text()
  local trimmed = text:gsub("[ \t]+(\r?\n)", "%1"):gsub("[ \t]+$", "")
  if trimmed ~= text then
    buf:replace(1, #text, trimmed)
  end
end)
