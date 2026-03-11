# krun

Minimal process manager for Linux — a lightweight PM2 alternative using systemd user services.

3MB static binary. Zero dependencies. No daemon.

## Install

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o krun .
sudo cp krun /usr/local/bin/
```

## Commands

### Process Management

```bash
# Start an app
krun start api --cmd "node dist/main.js" --cwd /srv/api

# Start with options
krun start worker --cmd "python worker.py" \
  --cwd /srv/worker \
  --env NODE_ENV=production \
  --env PORT=3000 \
  --max-memory 512M \
  --instances 3 \
  --log-file /var/log/worker.log \
  --error-file /var/log/worker.err \
  --interpreter python3 \
  --cron-restart daily \
  --no-autorestart

# Start from ecosystem config
krun start ecosystem.json

# Stop / restart / reload
krun stop api
krun restart api
krun reload api          # graceful reload (SIGHUP then restart)

# Stop/restart all
krun stop all
krun restart all

# Remove app (stop + delete service)
krun remove api
krun remove all
```

### Monitoring

```bash
# List all apps (with PID, CPU, memory, uptime, restarts)
krun list

# Detailed info
krun info api

# Real-time dashboard (refreshes every 2s)
krun monit

# Show environment variables
krun env api

# Show app config as JSON
krun config api
```

### Logs

```bash
# Follow logs (like tail -f)
krun logs api

# Show last 100 lines without following
krun logs api --lines 100 --nostream

# Flush logs
krun flush api
krun flush all
```

### Save & Restore

```bash
# Save current running apps config
krun save

# Restore saved apps after reboot
krun resurrect
```

### Startup

```bash
# Enable auto-start on boot (loginctl enable-linger)
krun startup

# Disable auto-start
krun unstartup
```

### Misc

```bash
krun reset api           # Reset restart counter
krun version
krun help
```

## Ecosystem Config

Create `ecosystem.json`:

```json
{
  "apps": [
    {
      "name": "api",
      "cmd": "node dist/main.js",
      "cwd": "/srv/api",
      "env": {
        "NODE_ENV": "production",
        "PORT": "3000"
      },
      "max_memory": "512M",
      "instances": 1
    },
    {
      "name": "worker",
      "cmd": "python worker.py",
      "cwd": "/srv/worker",
      "interpreter": "python3",
      "auto_restart": true
    }
  ]
}
```

Then: `krun start ecosystem.json`

## Command Aliases

| Command   | Aliases          |
|-----------|-----------------|
| list      | ls, status       |
| info      | describe, show   |
| logs      | log              |
| monit     | monitor, dash    |
| remove    | delete           |

## How It Works

- Generates systemd user service files at `~/.config/systemd/user/krun-<name>.service`
- `Restart=always` with 3s delay for crash recovery
- `loginctl enable-linger` for boot persistence
- Logs via `journalctl` (no custom log daemon)
- App configs saved at `~/.config/krun/apps.json`
- Memory limits via systemd `MemoryMax`
- Cron restart via systemd timers

## Requirements

- Linux with systemd (Debian, Ubuntu, Arch, etc.)
- `systemctl`, `journalctl`, `loginctl`
