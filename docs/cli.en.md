# CLI & Execution Modes

Beyond the standard desktop graphical window, the **TelegramDL** binary can be invoked from the command line with various parameters and operational modes.

---

## :material-laptop: Execution Modes

```bash
# Windows
TelegramDL.exe [options]

# Linux / macOS
./TelegramDL [options]
```

### Supported Arguments:

| Argument | Alias | Description |
| :--- | :--- | :--- |
| *(no arguments)* | — | Starts the full desktop application with the Wails GUI window. |
| `--server` | `-s`, `/server` | **Headless Server Mode**: Starts the REST and WebSocket service without opening a window. |
| `--update` | `-u`, `/update` | Checks for newer releases on GitHub and self-updates the binary. |
| `--help` | `-h`, `/?` | Prints usage information and exit. |

---

## :material-server: Headless Server Mode (`--server`)

Headless mode is ideal for:
- Home servers or NAS appliances (Synology, TrueNAS, unRAID).
- Headless Linux virtual machines without a display server (X11/Wayland).
- Background services intended for remote management via smartphone or web browser through [Remote Access](remote-access.md).

```bash
TelegramDL.exe --server
```

When launched in this mode:
1. Acquires the single-instance lock to prevent process conflicts.
2. Initializes the MTProto session with Telegram.
3. Launches the HTTP REST and WebSocket listener on the configured host and port (default `127.0.0.1:8000` on new installations; existing ones keep their saved address).
4. Stays resident handling downloads and listener events until receiving a termination signal (`Ctrl+C` / `SIGINT`).

---

## :material-sync: Automatic Updater (`--update`)

You can trigger an update check and installation directly from your terminal:

```bash
TelegramDL.exe --update
```

The integrated updater (`pkg/updater/updater.go`):
1. Queries the GitHub Releases API for TelegramDL.
2. Compares your running version with the latest release tag.
3. Streams the matching OS/architecture binary package.
4. Performs an atomic in-place binary replacement and clean restart.

---

## :material-lock: Single-Instance Locking

TelegramDL implements single-instance mutual exclusion to ensure database and socket safety:

- Launching a second instance while one is already running triggers an alert and exits gracefully without corrupting SQLite or failing on port collisions.
- During self-updates, the spawned replacement binary waits up to 8 seconds for the parent process to relinquish the instance lock before binding.
