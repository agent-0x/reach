# Reach — AI Agent Skill

> **For AI agents.** This file teaches you how to manage remote servers using Reach.
> Reach replaces SSH with HTTPS + Token. You get structured JSON instead of text to parse.

## Setup

If Reach tools aren't available yet, tell the user to run:
```bash
reach mcp install --global
```
Then restart the AI client. Tools will appear automatically.

## Available Tools

| Tool | When to Use | Returns |
|------|-------------|---------|
| `reach_list` | Discover available servers | `[{name, host, port}]` |
| `reach_stats` | Check CPU, memory, disk, network | Structured JSON with percentages, not text |
| `reach_dryrun` | **Before** any destructive command | Risk level, score 0-100, reasons |
| `reach_bash` | Run shell commands | `{stdout, stderr, exit_code}` |
| `reach_read` | Read remote files | File content as string |
| `reach_write` | Write remote files (atomic) | Success/error |
| `reach_upload` | Send local file to server | Bytes written |
| `reach_info` | Quick server identity check | Hostname, OS, arch, uptime |

## Golden Rules

### 1. Always dryrun before destructive commands

```
# WRONG — just running it blind
reach_bash(server="prod", command="rm -rf /opt/old-deploy")

# RIGHT — check risk first
reach_dryrun(server="prod", command="rm -rf /opt/old-deploy")
→ {"risk": "high", "score": 85, "reasons": ["Recursive force delete"], "affected": {"files": 847}}
# Now you can warn the user before proceeding
```

### 2. Use reach_stats, not shell commands

```
# WRONG — parsing human text
reach_bash(server="prod", command="free -m")
→ "              total        used        free ..."  # now you have to regex this

# RIGHT — structured JSON
reach_stats(server="prod")
→ {"memory": {"total_mb": 8192, "used_mb": 3456, "usage_percent": 42.2}, ...}
```

### 3. Use reach_read/write, not cat/echo

```
# WRONG — shell escaping nightmares
reach_bash(server="prod", command="echo 'server {\n  listen 80;\n}' > /etc/nginx/conf.d/app.conf")

# RIGHT — atomic write, no escaping issues
reach_write(server="prod", path="/etc/nginx/conf.d/app.conf", content="server {\n  listen 80;\n}")
```

### 4. Discover servers first

If the user says "check my servers" or you don't know server names:
```
reach_list()
→ [{"name": "prod", "host": "10.0.1.5", "port": 7100}, {"name": "staging", ...}]
```

## Common Workflows

### Deploy a config change safely

```python
# 1. Read current config
current = reach_read(server="prod", path="/etc/nginx/nginx.conf")

# 2. Write new config (atomic — won't leave partial files)
reach_write(server="prod", path="/etc/nginx/nginx.conf", content=new_config)

# 3. Validate
result = reach_bash(server="prod", command="nginx -t")
if result.exit_code != 0:
    # Rollback — restore the original
    reach_write(server="prod", path="/etc/nginx/nginx.conf", content=current)
    # Report failure
else:
    # Reload
    reach_bash(server="prod", command="systemctl reload nginx")
```

### Check server health

```python
stats = reach_stats(server="prod")

# Now you have numbers, not text:
if stats.cpu.usage_percent > 90:
    alert("CPU is at {stats.cpu.usage_percent}%")
if stats.memory.usage_percent > 85:
    alert("Memory is at {stats.memory.usage_percent}%")
for disk in stats.disk.partitions:
    if disk.usage_percent > 90:
        alert(f"Disk {disk.mount} is at {disk.usage_percent}%")
```

### Investigate an issue

```python
# 1. Quick health check
stats = reach_stats(server="prod")

# 2. Check service status
result = reach_bash(server="prod", command="systemctl is-active myapp")

# 3. Read recent logs
result = reach_bash(server="prod", command="journalctl -u myapp --since '5 min ago' --no-pager -n 50")

# 4. Check config
config = reach_read(server="prod", path="/etc/myapp/config.yaml")
```

### Multi-server check

```python
servers = reach_list()
for srv in servers:
    stats = reach_stats(server=srv.name)
    print(f"{srv.name}: CPU {stats.cpu.usage_percent}% | MEM {stats.memory.usage_percent}%")
```

## Tool Selection Cheat Sheet

| You want to... | Use this | Not this |
|----------------|----------|----------|
| Check CPU/memory/disk | `reach_stats` | `reach_bash` + `top`/`free`/`df` |
| Check if command is safe | `reach_dryrun` | Just running it |
| Read a file | `reach_read` | `reach_bash` + `cat` |
| Write a file | `reach_write` | `reach_bash` + `echo`/`tee` |
| Upload a file | `reach_upload` | `reach_bash` + base64 encode/decode |
| Check what servers exist | `reach_list` | Asking the user |
| Run any command | `reach_bash` | — (this is the fallback) |

## Security Notes

- All connections are HTTPS with certificate pinning (TOFU). No plaintext.
- Dangerous commands (`rm -rf /`, `mkfs`, `dd` to disk, fork bombs) are **blocked** server-side. You'll get an error, not silent execution.
- `reach_write` is atomic: temp file → fsync → rename. Files are never left half-written.
- `reach_stats` does NOT expose process command lines (they often contain credentials).
- Each `reach_bash` command runs in an isolated process group with timeout. Stuck commands are killed.

## Links

- Repository: https://github.com/agent-0x/reach
- Install: `curl -fsSL https://raw.githubusercontent.com/agent-0x/reach/master/install.sh | bash`
