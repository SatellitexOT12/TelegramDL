---
title: CLI & Execution Modes
description: Console parameters guide, direct URL downloads, pure CLI mode, and headless server mode.
---

Beyond the standard desktop graphical window, the **TelegramDL** binary can be invoked from the command line with various parameters and operational modes.

---

## Execution Modes

```bash
# Windows
TelegramDL.exe [options|URL]

# Linux / macOS
./TelegramDL [options|URL]
```

### Supported Arguments

| Argument | Alias | Description |
| :--- | :--- | :--- |
| *(no arguments)* | — | Starts the full desktop application with the Wails GUI window. |
| `<URL>` | — | **Direct Download**: Pass a Telegram link directly. If the app is open, it enqueues to the active instance; if closed, it launches the app and queues the download. |
| `--download <URL>` | `-d` | **Pure CLI Download Mode**: Downloads the link directly in terminal text mode without launching a GUI or web server. |
| `--server` | `-s`, `/server` | **Headless Server Mode**: Starts the REST and WebSocket service without opening a window. |
| `--update` | `-u`, `/update` | Checks for newer releases on GitHub and self-updates the binary. |
| `--help` | `-h`, `/?` | Prints usage information and exit. |

---

## Direct Download via URL Argument (`<URL>`)

You can pass a Telegram URL directly after the binary to automatically enqueue or process a download:

```bash
# Example with a public channel or username
TelegramDL.exe https://t.me/channel_name/123

# Example with a private channel and message range
TelegramDL.exe https://t.me/c/1794593738/30502-30505
```

### Smart Instance Behavior

- **If the application IS ALREADY open**: The console process detects the running instance via the IPC lock system and transmits the link seamlessly over a local loopback channel (`/api/ipc/download`). The open GUI updates its queue in real time. If no active downloads are running, processing starts immediately; otherwise, it takes its place in order.
- **If the application IS CLOSED**: The process launches TelegramDL in desktop mode, initializes the interface, and enqueues the download in the background. If the queue was empty, the download is processed as soon as the app starts up.

---

## Pure Console Download Mode (`--download <URL>`)

If you want to perform a one-off download without launching the graphical window or maintaining a background web server, use the `--download` option:

```bash
TelegramDL.exe --download https://t.me/channel_name/123
```

In this mode:
1. Connects directly to the Telegram MTProto API using credentials stored in your local database.
2. Displays real-time download progress directly as text in the console terminal.
3. Automatically closes and returns control to the terminal upon completion.

---

## Headless Server Mode (`--server`)

Headless mode is ideal for:

- Home servers or NAS appliances (Synology, TrueNAS, unRAID).
- Headless Linux virtual machines without a display server (X11/Wayland).
- Background services intended for remote management via smartphone or web browser through [Remote Access](/TelegramDL/en/remote-access/).

```bash
TelegramDL.exe --server
```

When launched in this mode:

1. Acquires the single-instance lock to prevent process conflicts.
2. Initializes the MTProto session with Telegram.
3. Launches the HTTP REST and WebSocket listener on the configured host and port (default `127.0.0.1:8000` on new installations; existing ones keep their saved address).
4. Stays resident handling downloads and listener events until receiving a termination signal (`Ctrl+C` / `SIGINT`).

---

## Automatic Updater (`--update`)

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

## Single-Instance Locking & IPC Communication

TelegramDL implements single-instance mutual exclusion and inter-process communication (IPC) to prevent duplicates and route tasks:

- Launching a second instance without arguments detects the active application and exits gracefully.
- Passing a URL argument while the app is open sends the download payload to the active instance via local HTTP IPC and exits after confirming success in the console.
- During self-updates, the spawned replacement binary waits up to 8 seconds for the parent process to relinquish the instance lock before binding.
