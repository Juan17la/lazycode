# Reviewing a plugin

CI checks that the entry is well formed, and that the pinned commit matches the checksum, name and version. It never runs the plugin. A person checks everything else, by reading all the code at the pinned commit, not the author's latest:

- [ ] `does` in `plugin.toml` is true and complete. It's what users see when asked to accept.
- [ ] Nothing reads or sends files, keys, tokens or the environment beyond what `does` says.
- [ ] Every `os.execute`, `io.popen`, `os.remove` and `loadstring` has a clear reason. Nothing is downloaded and then run.
- [ ] No obfuscated code, and no compiled files.
- [ ] A `language.toml` server command is the well-known program for that language.
- [ ] For an update, read the diff from the last accepted commit.
- [ ] The name doesn't imitate another plugin.

Decline with a reason if anything is unclear. To remove a plugin that turns out to be harmful, delete its entry and open an advisory: removal stops new installs, but doesn't remove it from machines that already have it.
