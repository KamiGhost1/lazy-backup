# lazy-backup

> Terminal-based backup manager for SSH servers with a TUI and CLI interface.

`lazy-backup` lets you manage backups across multiple remote SSH servers from a single terminal application. Backups are transferred via **rsync** (with SFTP fallback), stored locally with automatic rotation, and all operations are logged in SQLite.

---

## Features

- **Multi-server management** — add any number of SSH servers (password or key auth)
- **Backup repositories** — configure multiple backup paths per server with independent retention limits
- **Smart transfer** — uses rsync when available, falls back to SFTP automatically
- **Automatic rotation** — removes oldest local and remote backups when the limit is exceeded
- **SSH key generation** — generates RSA key pairs directly from the app
- **Operation log** — every download/upload is recorded in a local SQLite database
- **Dual interface** — interactive TUI for manual work, CLI flags for cron/automation

---

## Screenshot

```
┌─ Servers ──────────┐  ┌─ Repositories ─────────────────┐  ┌─ Logs ──────────────────────┐
│ web (root@1.2.3.4) │  │ nginx-conf: /etc/nginx (max 5) │  │ [03:00:01] download ✓       │
│ db  (pg@10.0.0.5)  │  │ pgdata: /var/lib/pg (max 3)    │  │ [03:00:04] download ✓       │
└────────────────────┘  └────────────────────────────────┘  │ [03:05:12] upload   failed  │
                                                              └─────────────────────────────┘
[tab] switch panels  [a] add  [d] delete  [g] download  [u] upload  [q] quit
```

---

## Requirements

- **Go 1.21+**
- **rsync** *(optional — recommended for efficient transfers)*
- SSH access to the remote servers you want to back up

---

## Installation

### Build from source

```bash
git clone https://github.com/kamighost1/lazy-backup.git
cd lazy-backup/app
go build -o backup-manager .
```

The SQLite database `backup_manager.db` is created automatically in the working directory on first run.

### Docker (development)

```bash
docker compose up -d
docker exec -it dev-lazy-backup bash
cd /app && go build -o backup-manager && ./backup-manager
```

---

## Usage

### TUI mode (no arguments)

```bash
./backup-manager
```

| Key | Action |
|-----|--------|
| `Tab` | Switch focus between panels |
| `a` | Add server / repository |
| `d` | Delete selected item |
| `g` | Download backup from server |
| `u` | Upload latest local backup to server |
| `q` / `Ctrl+C` | Quit |
| `Esc` | Close form |

### CLI mode

Pass any flag to skip the TUI and run in headless mode — useful for cron jobs.

```bash
# List all configured servers
./backup-manager -list-servers

# List repositories for server ID 1
./backup-manager -list-repos -server=1

# Download backup for repository ID 2
./backup-manager -repo=2 -action=download

# Upload the latest local backup to the server
./backup-manager -repo=2 -action=upload

# Print the SSH public key generated for server ID 1
./backup-manager -show-pubkey=1
```

**Cron example** — nightly backup at 03:00:

```cron
0 3 * * * /opt/backup-manager -repo=1 -action=download >> /var/log/backup.log 2>&1
```

---

## Adding a server

Press `a` in the servers panel and fill in the form:

| Field | Description |
|-------|-------------|
| Name | Display alias |
| Host | IP address or hostname |
| Port | SSH port (default: `22`) |
| User | SSH login |
| Password | Leave blank when using a key |
| Key path | Path to the private key file (optional) |

If you provide a key path that does not exist yet, the app generates a new RSA key pair (`<path>` + `<path>.pub`) automatically on the first connection. Use `-show-pubkey=<server_id>` to retrieve the public key and add it to the remote server's `~/.ssh/authorized_keys`.

---

## Local backup layout

```
backups/
  repo1_20260512_030000/   ← repo ID=1, date and time
  repo1_20260513_030000/
  repo2_20260512_030000/
```

When the number of copies for a repository exceeds `max_backups`, the oldest directory is deleted automatically (both locally and on the remote server).

---

## Project structure

```
lazy-backup/
├── app/
│   ├── main.go        # Entry point — routes to TUI or CLI
│   ├── models.go      # Data structures (Server, BackupRepo, BackupLog)
│   ├── db.go          # SQLite CRUD via modernc.org/sqlite
│   ├── cli.go         # CLI flag parser
│   ├── ui.go          # TUI (tview)
│   ├── sshutils.go    # SSH, rsync, SFTP, key generation, rotation
│   ├── db_test.go
│   ├── sshutils_test.go
│   └── go.mod
├── docker/
│   └── Dockerfile
└── docker-compose.yml
```

---

## Running tests

```bash
cd app
go test ./...
```

---

## Dependencies

| Package | Purpose |
|---------|---------|
| [`github.com/rivo/tview`](https://github.com/rivo/tview) | TUI framework |
| [`github.com/gdamore/tcell/v2`](https://github.com/gdamore/tcell) | Terminal backend |
| [`github.com/pkg/sftp`](https://github.com/pkg/sftp) | SFTP client |
| [`golang.org/x/crypto/ssh`](https://pkg.go.dev/golang.org/x/crypto/ssh) | SSH client |
| [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) | SQLite — pure-Go driver, no CGO |

---

## Known limitations

- SFTP fallback transfers only a single file, not a directory tree — use rsync for directory backups.
- Host key verification is disabled (`InsecureIgnoreHostKey`); suitable for trusted private networks only.
- Server passwords are stored in plaintext in the SQLite database.

---

## License

MIT
