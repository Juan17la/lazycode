# Security

## Reporting a vulnerability

Please report it privately, not in a public issue:

1. Go to the [Security tab](https://github.com/Juan17la/lazycode/security) of this repository.
2. Choose **Report a vulnerability**.

Say what you found, how to reproduce it, and which version and operating system. LazyCode is a free-time project, so there is no guaranteed response time. Data loss and security reports are looked at first.

## Supported versions

Only the latest release.

## What to know about releases

- Releases are not signed. The install scripts check each download's SHA-256 against `checksums.txt`, which comes from the same release, so that catches a corrupted download but not a compromised release.
- You can read the install scripts before running them: [install.sh](install.sh) and [install.ps1](install.ps1).
- LazyCode has no telemetry. The one automatic connection is a once-a-day check of GitHub's latest release, and `[updates] check = false` in `:config` turns it off. See [docs/11-security-and-privacy.md](docs/11-security-and-privacy.md).
