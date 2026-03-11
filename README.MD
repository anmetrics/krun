# krun

Minimal process manager for Linux via systemd user services.

## Install

```bash
# build from source
CGO_ENABLED=0 go build -ldflags="-s -w" -o krun .

# copy to PATH
sudo cp krun /usr/local/bin/
```

## Usage

```bash
# start an app
krun start api --cmd "node dist/main.js" --cwd /srv/api

# stop
krun stop api

# restart
krun restart api

# view logs (follows)
krun logs api

# list all apps
krun list

# remove app and its service file
krun remove api
```

## How it works

krun generates systemd user service files at `~/.config/systemd/user/krun-<name>.service`.

- Auto restart on crash (`Restart=always`, 3s delay)
- Auto start on boot via `loginctl enable-linger`
- Logs accessible through `journalctl`

## Requirements

- Linux with systemd
- `systemctl`, `journalctl`, `loginctl`
