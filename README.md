# NexGuard

A hash-based antivirus CLI and background watcher for macOS, Linux, and Windows,
plus the licensing and billing service that sells the premium tier.

[![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Platform: macOS · Linux · Windows](https://img.shields.io/badge/platform-macOS%20·%20Linux%20·%20Windows-lightgrey)](#platform)
[![Release](https://img.shields.io/github/v/release/jbrahy/nexguard?sort=semver)](https://github.com/jbrahy/nexguard/releases)
[![Tests](https://img.shields.io/badge/tests-120%20passing-brightgreen)](#tests)

The scanner is free and open source (MIT), and ships as a CLI named `avtool`.
**[NexGuard](https://nexguardhq.com)** is the paid tier on top of the same
scanner: a premium threat feed and priority support. Both the scanner and the
service that sells it live in this repository.

## The scanner

`avtool` detects known-malicious files by comparing file hashes against a local
database of known-bad SHA256 hashes, sourced from a manually-maintained list
plus a periodic sync from a public threat intel feed (MalwareBazaar). It has two
modes:

- **`avtool scan <path>`** — on-demand, interactive scan. Walks a path, hashes
  files, and on a match prompts you to quarantine, delete, ignore, or just
  report it.
- **`avtool watch`** — a background daemon that watches user-specified
  directories in real time and queues any matches for later review via
  `avtool review` (it can't prompt interactively since it runs headless — on
  macOS this is typically run under launchd). Desktop match notifications are
  macOS-only for now; on Linux and Windows, matches are still queued and
  visible via `avtool review`.

Full command set: `scan`, `watch`, `review`, `sync`, `hashes`, and
`quarantine`, all implemented and shipping in v1.0.0.

### What it does not do

Detection is intentionally hash-based only in this first version — no
heuristics, no signature/pattern matching (YARA/ClamAV), no behavioral
analysis. It will not catch anything that isn't byte-identical to a known-bad
sample. That constraint is a deliberate scope decision, not an oversight; the
reasoning is written up in the
[scanner design spec](docs/superpowers/specs/2026-08-18-antivirus-tool-design.md).

## The service

`cmd/avtool-web` is the Go service behind [nexguardhq.com](https://nexguardhq.com):
the marketing site, accounts, Stripe checkout, and the license endpoint the CLI
calls to validate a premium key. Built on `chi`, backed by MySQL, templates
rendered server-side. Its design is written up in the
[service design spec](docs/superpowers/specs/2026-08-20-avtool-web-foundation-design.md).

| Package | Responsibility |
|---|---|
| `internal/web/auth` | Session auth, `RequireAuth` and `OptionalAuth` middleware |
| `internal/web/billing` | Stripe checkout and subscription webhooks |
| `internal/web/license` | Premium key issuance, validation, and revocation |
| `internal/web/ratelimit` | Per-IP limiters on the two credential-guessing surfaces |
| `internal/web/handlers` | Landing, dashboard, articles, and static pages |
| `internal/web/db` | Parameterized MySQL access |

Login and license validation are rate limited separately and on purpose: both
are credential-guessing surfaces, but license validation is a machine client
polling periodically rather than a human filling in a form, so it gets a higher
ceiling. Client IPs are taken from the rightmost `X-Forwarded-For` entry, since
the leftmost is attacker-controlled.

## Install

Download a prebuilt binary from the
[Releases page](https://github.com/jbrahy/nexguard/releases) for macOS (Intel or
Apple Silicon), Linux (amd64 or arm64), or Windows (amd64). Every release ships
a `checksums.txt`.

Or build from source (Go 1.26+, no CGO required):

```sh
go build -o bin/avtool ./cmd/avtool
```

### Where it keeps its data

The database, quarantine directory, and detection log live under an `avtool`
directory inside the OS config directory that Go's `os.UserConfigDir` reports:

| OS | Directory |
|---|---|
| macOS | `~/Library/Application Support/avtool/` |
| Linux | `~/.config/avtool/` (or `$XDG_CONFIG_HOME/avtool/`) |
| Windows | `%AppData%\avtool\` |

`--db-path` and `--quarantine-dir` override the first two.

## Tests

```sh
go test ./...
```

120 tests across 20 packages, covering the scanner and hash database, the feed
sync, quarantine and review flows, the filesystem watcher, and the full web
service including auth, billing, licensing, and rate limiting.

## Platform

macOS, Linux, and Windows.

## Repository layout

```
cmd/avtool/       the scanner CLI
cmd/avtool-web/   the licensing, billing, and marketing service
internal/         scanner internals (hashdb, feed, scanner, watcher,
                  quarantine, notify, store) and the web service packages
database/         numbered SQL migrations for the web service
web/              templates and static assets
docs/             design specs and compliance notes
```

The Go module path is still `github.com/jbrahy/AntiVirus`, from before the
project was renamed. Import paths reflect that; the repository and product do
not.

## License

MIT — see [LICENSE](LICENSE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
