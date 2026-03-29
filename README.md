# Reach

Control any server from any AI. A lightweight agent for remote server management.

- **One binary, one token** — install in 30 seconds
- **Works with Claude Code, GPT, Gemini, or any AI** via MCP
- **HTTPS + Token auth**, no SSH dependency

---

## Quick Start

**Server side** (on your remote machine):

```bash
reach agent init --dir /etc/reach-agent
reach agent serve --config /etc/reach-agent/config.yaml
# Copy the token shown during init
```

**Client side** (on your local machine):

```bash
reach add myserver --host <server-ip> --token <token>
reach exec myserver "uname -a"
reach read myserver /etc/hostname
```

---

## Claude Code Integration

```bash
reach mcp install
# Restart Claude Code — reach tools are now available
```

Then just ask:

```
You: "Check the nginx status on myserver"
AI: [calls reach_bash("myserver", "systemctl status nginx")]
```

Use `--global` to install for all projects:

```bash
reach mcp install --global
```

---

## MCP Tools

| Tool | Description |
|------|-------------|
| `reach_bash` | Execute a shell command |
| `reach_read` | Read a remote file |
| `reach_write` | Write a file (atomic: temp + fsync + rename) |
| `reach_upload` | Upload a local file to the server |
| `reach_info` | Get system info (CPU, memory, disk, uptime) |
| `reach_list` | List all configured servers |

---

## CLI Reference

| Command | Description |
|---------|-------------|
| `reach agent init [--dir]` | Generate TLS cert + token, write config |
| `reach agent serve [--config]` | Start the HTTPS agent server |
| `reach add <name> --host --token [--port]` | Add a server (TOFU fingerprint pinning) |
| `reach remove <name>` | Remove a server from local config |
| `reach list` | List all configured servers |
| `reach exec <server> <cmd> [-t timeout]` | Run a command remotely |
| `reach read <server> <path>` | Read a remote file |
| `reach write <server> <path>` | Write stdin to a remote file |
| `reach upload <server> <local> <remote>` | Upload a local file |
| `reach download <server> <remote> <local>` | Download a remote file |
| `reach info <server>` | Show system information |
| `reach health <server>` | Check server health |
| `reach mcp install [--global]` | Register reach as an MCP server in Claude Code |
| `reach mcp serve` | Start the MCP stdio server (used internally by Claude Code) |

---

## Security Model

- **Self-signed TLS with TOFU** — certificate fingerprint is fetched once on `reach add` and pinned locally; subsequent connections verify against it
- **128-bit random Bearer Token** — generated at `agent init`, never transmitted in plaintext
- **Process group isolation** — each command runs in its own process group; cleanup is guaranteed on timeout
- **Atomic file writes** — write to a temp file, `fsync`, then rename; no partial writes

---

## License

MIT
