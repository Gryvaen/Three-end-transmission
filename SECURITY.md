# Security Policy

## Supported versions

LanRoom has no tagged releases yet. Security fixes are made on the `main` branch; please make sure you are running the latest `main` before reporting.

## Reporting a vulnerability

Please **do not** open a public issue for security problems.

Report privately through GitHub: open the repository's **Security** tab and choose **Report a vulnerability**. Include:

- the affected version or commit
- how LanRoom is deployed (LAN or public mode, reverse proxy, Docker)
- steps to reproduce and the impact

You should receive a response within 7 days. Once a fix is available, the issue will be disclosed in the [changelog](CHANGELOG.md).

## Security model

Knowing what LanRoom is designed to protect helps decide whether something is a vulnerability.

- **Access control** is a single PIN shared by the whole room; there are no user accounts. Anyone with the PIN is a trusted room member.
- **Direct messages** are delivered only to their recipients. Device IDs are derived from a secret key stored in each browser, so a member cannot impersonate another device.
- **Files** are identified by random 128-bit IDs and kept only for the retention period. Messages, file indexes and sessions are held in memory and cleared on restart.
- **Public deployments** must use HTTPS through a reverse proxy, with `LANROOM_TRUSTED_PROXIES` set. The server refuses to start in public mode without a PIN.
- **PIN guessing** is limited to 5 wrong attempts per client IP per minute.

## Out of scope

- LAN mode uses plain HTTP by design. It assumes a trusted local network; enable a PIN if that network is shared.
- Actions by someone who knows the room PIN, such as uploading large files or joining the room.
- Public deployments without HTTPS, or with forwarded headers trusted from untrusted sources.
- Denial of service through resource exhaustion, such as filling the disk. There is currently no total storage quota.
