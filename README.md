<div align="center">

# LanRoom

**Share text and files between any devices, straight from the browser.**

[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)](docs/deployment.md)

English | [简体中文](README.zh-CN.md)

</div>

LanRoom is a single Go binary that serves a chat-style web page for sending messages and files between phones and computers. Android, iOS, Windows, macOS and Linux devices join by opening a URL or scanning a QR code. Nothing needs to be installed on them.

Run it on a machine in your home or office network, or deploy it to a public server behind Nginx and HTTPS.

## Features

- **Group chat and direct messages**: send to everyone, or tap a device to send to it alone.
- **Large files with resume**: chunked uploads that pick up where they left off after a dropped connection or a page reload.
- **QR code join**: devices on the network scan the code shown on the page.
- **Room PIN**: optional on a LAN, required on a public server.
- **Two deployment modes**: zero-config LAN mode, or public mode with a domain and HTTPS.
- **Self-cleaning**: messages and files expire after a configurable retention period (1 hour by default).

## Quick start

Requires Go 1.26 or later.

```bash
git clone https://github.com/Gryvaen/Three-end-transmission.git
cd Three-end-transmission
go run .
```

Open <http://127.0.0.1:8787>, enter a name, then scan the QR code under 「连接信息」 (Connection info) from other devices.

With Docker on Linux:

```bash
docker compose -f docker-compose.host.yml up -d --build
```

## Deployment

- **Local network**: run the binary or the Docker image on any always-on machine.
- **Public server**: put LanRoom behind Nginx with HTTPS and set `LANROOM_PUBLIC_URL` and `LANROOM_PIN`.

Step-by-step instructions are in the [deployment guide](docs/deployment.md).

## Documentation

The detailed guides are written in Chinese.

| Guide | Contents |
|-------|----------|
| [Deployment](docs/deployment.md) | Binary, Docker, public server with Nginx + HTTPS |
| [Configuration](docs/configuration.md) | Flags, environment variables, limits, security notes |
| [API](docs/api.md) | WebSocket protocol, HTTP endpoints, curl examples |
| [Troubleshooting](docs/troubleshooting.md) | Common problems and fixes |

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup and guidelines.

## Security

Please report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © 2026 Gryvaen
