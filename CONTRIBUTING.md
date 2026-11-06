# Contributing

Thanks for helping. This is a free-time project, so replies can take a while, and there is no guaranteed response time.

## Setup

You need Go (the version in `go.mod`) and a C compiler, because syntax highlighting uses tree-sitter's C library.

```bash
go test -race ./...
go vet ./...
test -z "$(gofmt -l .)"
```

CI also runs `staticcheck` and `govulncheck`. See [docs/13-testing-strategy.md](docs/13-testing-strategy.md) for how tests are organised.

## Pull requests

- CI must pass on Linux, macOS and Windows.
- Changes to the editor core need tests.
- A new dependency needs an explanation in the pull request: what it does, why a few lines of our own won't do, and its licence (it must be MIT-compatible). Then run `scripts/third-party-licenses.sh`.
- A performance regression needs a good reason. The targets are in [docs/12-performance-budget.md](docs/12-performance-budget.md).
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/), such as `fix: keep the cursor after undo`.
- Add a line to the Unreleased section of [CHANGELOG.md](CHANGELOG.md) for anything a user would notice.

## Issues

Say your operating system, terminal and `lazycode --version`. Issues labelled `good first issue` are a good place to start. Security problems go through [SECURITY.md](SECURITY.md), not a public issue.

By taking part you agree to the [Code of Conduct](CODE_OF_CONDUCT.md). Contributions are under the [MIT licence](LICENSE).
