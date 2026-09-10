# Contributing to NexGuard

Thanks for contributing. This guide should take you from `git clone` to a
passing test run without needing anything that is not written here.

## Requirements

Go 1.26 or newer. No CGO, no C toolchain, and no database is required to build
or to run the default test suite.

## Getting set up

```sh
git clone https://github.com/jbrahy/nexguard.git
cd nexguard
go build ./...
```

That builds everything. To produce the scanner binary specifically:

```sh
go build -o bin/avtool ./cmd/avtool
```

The two commands in this repository are `cmd/avtool`, the scanner CLI, and
`cmd/avtool-web`, the licensing, billing, and marketing service behind
nexguardhq.com. The README has the full repository layout.

One wrinkle worth knowing before you read any import path: the Go module is
still `github.com/jbrahy/AntiVirus`, from before the project was renamed. The
module path does not match the repository name, and that is expected.

## Running the checks

CI runs exactly four commands on every pull request. Running them locally means
no surprises:

```sh
gofmt -l ./cmd ./internal   # must print nothing
go vet ./...
go build ./...
go test -race ./...
```

`gofmt -l` prints the files that are not formatted, so an empty output is a
pass. If it lists anything, `gofmt -w` on those files fixes it.

## Tests

```sh
go test ./...
```

This passes on a clean checkout with nothing else installed. The web service
tests need a database, and they skip themselves cleanly when one is not
reachable rather than failing, so a scanner-only contributor never has to set
up MariaDB.

### Running the database-backed tests

If you are changing anything under `internal/web` or `cmd/avtool-web`, run them
for real. They expect a MariaDB or MySQL database and read its DSN from
`TEST_DB_DSN`, defaulting to `root@tcp(127.0.0.1:3306)/avtool_web_test`:

```sh
mysql -e 'CREATE DATABASE IF NOT EXISTS avtool_web_test;'
mysql avtool_web_test < database/avtool-web/001_schema.sql
mysql avtool_web_test < database/avtool-web/002_index_stripe_customer_id.sql
mysql avtool_web_test < database/avtool-web/003_phone_sms_consent.sql

TEST_DB_DSN='root@tcp(127.0.0.1:3306)/avtool_web_test' go test ./internal/web/...
```

Watch the output. A test that prints `no reachable test MariaDB ... skipping`
did not run, and a suite that skips is not a suite that passed.

### Testing philosophy

Tests here exercise real behavior rather than mocks. The database tests talk to
a real MariaDB instead of a fake, which is why they need a DSN and why they
skip instead of pretending. Prefer a test that drives the real code path, and
reach for a substitute only when the real thing cannot be reached from a test.

## Pull requests

- Keep the change focused. One concern per PR.
- Link the issue it addresses, for example `Fixes #47`.
- Include a short test plan: what you ran, and what you saw.
- Add or update tests for behavior you change.
- Match the surrounding code style rather than reformatting nearby code.

Commit messages follow the convention already in `main`: a `type(scope):`
prefix and an imperative summary, for example `fix(watch): validate paths exist
and are directories` or `docs: add security policy`. Common types here are
`feat`, `fix`, `docs`, `ci`, and `test`.

## Reporting a security issue

Do not open a public issue for a vulnerability. See
[SECURITY.md](SECURITY.md) for how to report one privately.

## Conduct

Be respectful and constructive in reviews and discussions. Assume the person on
the other side is acting in good faith.
