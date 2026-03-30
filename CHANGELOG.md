# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-03-30

### Added

- `reach_stats` MCP tool — structured CPU, memory, disk, network, top processes
- `reach_dryrun` MCP tool — command risk scoring before execution
- `/health` endpoint now returns `version` and `capabilities` for discovery
- CLI commands: `reach stats`, `reach dryrun`

### Security

- `reach_stats` excludes process command lines to prevent credential leakage
- `reach_dryrun` splits shell chains and scores each part independently

## [0.1.0] - 2026-03-29

### Added

- Agent server with self-signed TLS and Bearer Token authentication
- CLI commands: `exec`, `read`, `write`, `upload`, `download`, `info`, `health`
- Client with TOFU (Trust-On-First-Use) certificate fingerprint pinning
- MCP server for Claude Code integration (`reach mcp install`)
- Command blacklist to block dangerous commands (`rm -rf /`, `mkfs`, `dd`, etc.)
- `AUTH_FAIL` logging for fail2ban integration
- Configurable security settings in `config.yaml`
- Process group isolation with timeout enforcement
- Atomic file writes (temp → fsync → rename)
- Cross-platform support: Linux amd64, macOS arm64

### Security

- Self-signed TLS with ECDSA P-256
- 128-bit random Bearer Token
- TOFU fingerprint pinning on client side
- Configurable command blacklist with custom regex patterns
- fail2ban-compatible authentication failure logging
