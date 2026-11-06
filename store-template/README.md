# LazyCode plugin store

The list of plugins LazyCode shows in `:store`. Only what's in [index.toml](index.toml) appears there, and each entry pins an exact commit, so a plugin can't change after review. Plugins aren't stored here.

## Submit a plugin

1. Put your plugin at the top of a public GitHub repository. See the [author guide](https://github.com/Juan17la/lazycode/blob/main/docs/guide/plugins.md).
2. Get its checksum from a clean checkout of the commit you're submitting:

   ```sh
   go run github.com/Juan17la/lazycode/cmd/storecheck@main checksum path/to/plugin
   ```

3. Open a pull request adding this to `index.toml`:

   ```toml
   [[plugin]]
   name = "my-plugin"        # The same as in plugin.toml.
   version = "1.0.0"         # The same as in plugin.toml.
   description = "Trims trailing spaces on save"
   repository = "https://github.com/you/my-plugin"
   commit = "<full commit hash>"
   checksum = "<from step 2>"
   ```

An update is a new pull request changing `version`, `commit` and `checksum`. People who installed it are asked again before the new version runs.

A language pack is a plugin with a `language.toml`, and it adds a language server. It can't add syntax colours yet.

## Review

See [REVIEWING.md](REVIEWING.md). Plugins have the same power as the person running them, so review lowers the risk without removing it.
