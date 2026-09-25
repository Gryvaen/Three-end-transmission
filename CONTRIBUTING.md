# Contributing to LanRoom

Thanks for your interest in improving LanRoom. Bug reports, fixes and feature ideas are all welcome.

## Reporting issues

Before opening an issue, check [Troubleshooting](docs/troubleshooting.md) and the existing issues. A useful report includes:

- how you run LanRoom (binary, Docker host or bridge network, behind Nginx)
- the devices and browsers involved
- steps to reproduce, and what you expected instead
- relevant server log lines (`docker logs lanroom --tail 50`)

Do not report security vulnerabilities in public issues; see [SECURITY.md](SECURITY.md).

## Development setup

Requirements:

- Go 1.26 or later (with Go 1.21+, `GOTOOLCHAIN=auto` downloads the right version)
- Docker, optional, for testing the container setup

```bash
git clone https://github.com/Gryvaen/Three-end-transmission.git
cd Three-end-transmission
go run .
```

The frontend in `web/` is embedded into the binary with `go:embed`, so restart the server after editing it. It is plain HTML, CSS and JavaScript, with no build step and no dependencies.

## Project layout

```
main.go              Entry point: config, embedded frontend, HTTP server
internal/config/     Environment variable parsing
internal/hub/        WebSocket hub, message protocol, device identity
internal/netutil/    LAN IP detection
internal/server/     HTTP routes, resumable uploads, PIN auth, proxy handling
web/                 Frontend
deploy/              Reverse proxy examples
docs/                User documentation (Chinese)
```

## Before you submit

```bash
gofmt -l .        # should print nothing for files you changed
go vet ./...
go test ./...
```

- Add or update tests for server-side changes. The tests in `internal/server` show how to drive the HTTP API and WebSocket with `httptest`.
- For UI changes, test on at least one phone as well as a desktop browser; many past bugs only appeared on mobile.
- Update `docs/` when you change configuration, the API or deployment steps, and add an entry under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md).

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/), with an optional scope:

```
feat: support public server deployment behind Nginx
fix(web): make device taps work on mobile
fix(hub): allow several connections per device ID
docs: document the upload API
chore: ignore .env
```

Explain *why* in the body when the change isn't obvious.

## Pull requests

- Keep each pull request focused on one change.
- Describe what changed and how you tested it.
- Make sure the checks above pass.

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
