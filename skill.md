---
name: reach
description: Manage remote servers via Reach — AI-native alternative to SSH. Use when user asks to check servers, deploy, run remote commands, read/write remote files, or monitor system health.
---

# Reach — Remote Server Management

You have access to remote servers via Reach MCP tools. Reach replaces SSH with HTTPS + Token — you get structured JSON, not text to parse.

## Setup

If reach tools aren't available, tell the user:
```bash
reach mcp install --global  # then restart your AI client
```

## Tools

| Tool | Use When |
|------|----------|
| `reach_dryrun` | **ALWAYS before destructive commands** (rm, mv, kill, restart) |
| `reach_stats` | Need CPU/memory/disk/network stats — NOT `reach_bash` + `top`/`free` |
| `reach_read` | Read remote files — NOT `reach_bash` + `cat` |
| `reach_write` | Write remote files (atomic) — NOT `reach_bash` + `echo >` |
| `reach_upload` | Send local file to server |
| `reach_bash` | Run any shell command (fallback when no specialized tool fits) |
| `reach_list` | Discover available servers |
| `reach_info` | Quick server identity (hostname, OS, arch) |

## Rules

1. **Dryrun before destroy.** Before any `reach_bash` that modifies state, call `reach_dryrun` first. If risk is `high` or `blocked`, warn the user.

2. **Structured over shell.** Use `reach_stats` instead of parsing `top`/`free`/`df` output. Use `reach_read`/`reach_write` instead of `cat`/`echo`.

3. **List first.** If you don't know server names, call `reach_list`.

## Patterns

**Safe config change:**
```
old = reach_read(server, path)        # backup
reach_write(server, path, new_config) # atomic write
result = reach_bash(server, "nginx -t") # validate
if failed: reach_write(server, path, old) # rollback
else: reach_bash(server, "systemctl reload nginx")
```

**Health check:**
```
stats = reach_stats(server)
# stats.cpu.usage_percent, stats.memory.usage_percent, stats.disk.partitions[].usage_percent
# All numbers, no parsing needed
```

**Before dangerous command:**
```
check = reach_dryrun(server, "rm -rf /opt/old")
# check.risk = "high", check.score = 85, check.affected.files = 847
# Warn user, then proceed only if confirmed
```
