# avtool

A hash-based antivirus CLI and background watcher for macOS, Linux, and Windows.

Free and open source (MIT). Sold as **[NexGuard](https://nexguardhq.com)** for
those who want a premium threat feed and priority support on top of the same
scanner — see [nexguardhq.com](https://nexguardhq.com) for details.

`avtool` detects known-malicious files by comparing file hashes
against a local database of known-bad SHA256 hashes, sourced from a
manually-maintained list plus a periodic sync from a public threat
intel feed (MalwareBazaar). It has two modes:

- **`avtool scan <path>`** — on-demand, interactive scan. Walks a
  path, hashes files, and on a match prompts you to quarantine,
  delete, ignore, or just report it.
- **`avtool watch`** — a background daemon that watches
  user-specified directories in real time and queues any matches for
  later review via `avtool review` (it can't prompt interactively
  since it runs headless — on macOS this is typically run under
  launchd). Desktop match notifications are macOS-only for now; on
  Linux and Windows, matches are still queued and visible via
  `avtool review`.

Detection is intentionally hash-based only in this first version — no
heuristics, no signature/pattern matching (YARA/ClamAV), no behavioral
analysis. It will not catch anything that isn't byte-identical to a
known-bad sample. See the full design rationale and scope in
[`docs/superpowers/specs/2026-08-18-antivirus-tool-design.md`](docs/superpowers/specs/2026-08-18-antivirus-tool-design.md).

## Status

v1 implemented: `scan`, `watch`, `review`, `sync`, `hashes`, and `quarantine` are all working. Build with `go build -o bin/avtool ./cmd/avtool`.

## Install

Download a prebuilt binary from the [Releases page](https://github.com/jbrahy/AntiVirus/releases) for macOS (Intel or Apple Silicon), Linux (amd64 or arm64), or Windows (amd64).

Or build from source: `go build -o bin/avtool ./cmd/avtool` (Go 1.26+, no CGO required).

## Platform

macOS, Linux, and Windows.

## License

MIT — see [LICENSE](LICENSE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
