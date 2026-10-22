# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Releases will follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html) once the first version is tagged.

## [Unreleased]

### Added

- Public server mode. `LANROOM_PUBLIC_URL` makes the join URL and QR code use a public address, and the server refuses to start without `LANROOM_PIN`.
- `LANROOM_LISTEN` and the `-addr` flag set the listen address, for example `127.0.0.1:8787` behind a reverse proxy.
- `LANROOM_TRUSTED_PROXIES`: `X-Forwarded-*` headers are honoured only from the listed proxies.
- Nginx example config (`deploy/nginx/lanroom.conf`) and `docker-compose.server.yml` for public deployments.
- Deployment guide for Tailscale (`tailscale serve` + HTTPS), reusing public mode, and `docker-compose.tailscale.yml`, which runs alongside the LAN instance.
- Resumable chunked uploads (`/api/uploads`, 8 MiB chunks), with progress, speed, cancel and automatic retry in the UI.
- Direct messages to one or more devices; history is filtered per device.
- Optional room PIN (`LANROOM_PIN`) with cookie sessions. The QR code of a logged-in device signs new devices in automatically.
- Configurable retention for messages and files (`LANROOM_RETENTION`, default 1 hour).
- Drag-and-drop uploads and a full-screen image viewer.
- Reply privately by tapping a sender's name; a badge on the devices button shows how many peers are online.

### Changed

- Chinese README is now the default entry point, with an English overview and a complete usage guide. Deployment, Android, configuration and troubleshooting guides now cover discovery, persistence limits and build issues.

- `/api/upload` streams to disk instead of buffering the whole form.
- Several tabs in the same browser can stay connected as one device, instead of disconnecting each other.
- Static files are served with `Cache-Control: no-cache`, so phones load the new frontend after a deploy.
- Documentation is split into a short README and detailed guides under `docs/`.

### Fixed

- Mobile connections: brief send-queue backlogs no longer drop clients; reconnects use exponential backoff and resume as soon as the page is visible again.
- Tapping a device in the mobile drawer closed the drawer instead of selecting the device.
- The join dialog could show two addresses after a DHCP change in Docker.

### Security

- Device IDs are now derived on the server from a secret key kept in the browser. Previously anyone in the room could reuse another device's broadcast ID to receive its direct messages.
- The session cookie is marked `Secure` when served over HTTPS.
- PIN attempts are rate-limited per real client IP, and forwarded headers can no longer be spoofed.
- WebSocket connections from other origins are rejected.
- Downloads are sent with `X-Content-Type-Options: nosniff`.
- `.env` (which may contain the PIN) is excluded from git and the Docker build context.

## Initial development (2026-05 – 2026-06)

- LAN hub with WebSocket group chat, online device list and file sharing.
- QR code join by LAN IP, with Docker host and bridge network support.
- Per-platform UI for Android, iOS, Windows, macOS and Linux.
- Copy chat messages, paste images, configurable upload size limit.
- mDNS discovery and a command-line client were tried, then removed in favour of joining by IP.

[Unreleased]: https://github.com/Gryvaen/Three-end-transmission/commits/main
