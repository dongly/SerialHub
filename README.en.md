# SerialHub

English | [简体中文](./README.md)

A two-way bridge between serial ports (MCU) and network clients (Web terminal / AI tools).

**📚 Docs**: [Quick Start](./QUICKSTART.md) (中文) | [MCP Guide](./MCP.md) (中文) | [Config & Architecture](./AGENTS.md) | [Release Process](./RELEASING.md) | [Integration Tests](./tests/integration/README.md)

## Overview

SerialHub bridges a single MCU serial port to both humans and AI:

- **Humans**: an xterm.js Web terminal in the browser for live viewing and input (opens automatically on startup; on WSL it opens the Windows host browser)
- **AI**: a native MCP server (Streamable HTTP + stdio) exposing 7 tools — `serial_list` / `serial_connect` / `serial_write` / `serial_read` / `serial_clear` / `serial_disconnect` / `serial_status`
- Both channels share **the same serial connection and data buffer** — humans and AI see the same bytes
- **Single-instance lock**: one master instance per scope (Windows: one lock per user at `%LOCALAPPDATA%\serialhub\`; Linux/macOS: per user-config directory) enforced by an OS file lock; a duplicate launch transparently proxies to the running instance instead of failing; if the port is occupied (e.g. Windows/WSL localhost-forwarding conflict) it auto-increments
- A single Go binary (frontend embedded); supports Windows (system tray) / Linux / macOS; everything configurable via TOML or CLI flags, data logging on demand (`--log-data`)

## Architecture

```mermaid
flowchart TB
    MCU["MCU"] <-->|"Serial (COM9, 115200, 8N1)"| Serial["Serial Manager"]

    subgraph SerialHub["SerialHub (single binary)"]
        Serial <-->|"Event bus"| Bridge["DataBridge"]
        Bridge <-->|"read / write"| Buffer["DataBuffer<br/>shared buffer"]
        Bridge <-->|"WebSocket (port 5050)"| Web["Web Terminal<br/>xterm.js"]
        Bridge <-->|"JSON-RPC (HTTP)"| MCP["MCP Server<br/>7 tools"]
    end

    Web <-->|"WebSocket"| Browser["Browser<br/>human"]
    MCP <-->|"MCP protocol"| AI["AI tools<br/>OpenCode / iFlow CLI etc."]
```

## Installation

Download the archive for your platform from
[GitHub Releases](https://github.com/dongly/serialhub/releases) and extract it
(the Windows archive ships `serialhub.ps1` / `serialhub.bat` launcher scripts).

Self-upgrade from an installed older version (config and logs are preserved):

```bash
serialhub upgrade          # fetch latest release, verify checksum, atomically replace itself
serialhub uninstall        # uninstall: dry-run list, then clean MCP entries / config / logs / binary
```

Proxies honor `HTTPS_PROXY` / `HTTP_PROXY`; a private mirror can be set via
`SERIALHUB_GITHUB_API` (default `https://api.github.com`).
Full steps (PATH setup, install verification, WSL USB serial attach) see
[QUICKSTART.md](./QUICKSTART.md) (Chinese).

## Quick Start

**Scenario: human + AI debugging together**

1. Start SerialHub:

```bash
serialhub -p COM9 --host 0.0.0.0 -D
```

2. Human watches via the Web terminal: open `http://localhost:5050/terminal`

3. AI tools connect via HTTP MCP:

```json
{
  "mcp": {
    "servers": {
      "serialhub": {
        "type": "remote",
        "url": "http://localhost:5050/mcp",
        "oauth": false
      }
    }
  }
}
```

4. Serial data is forwarded to both the Web terminal and the AI interface; each can send commands independently.

> One-command MCP client setup: `serialhub setup` (stdio local mode by default; supports OpenCode / Claude Code / Cursor / Windsurf / VS Code / Codex).

## Command Reference

### `serialhub` (default: serve mode)

```bash
serialhub                                    # start with defaults
serialhub -p COM8                            # specify serial port
serialhub -p COM8 -b 9600 --parity even      # full serial parameters
serialhub -m 8080                            # use port 8080
serialhub --host 0.0.0.0                     # listen on all interfaces (LAN access)
serialhub --stdio                            # stdio mode (spawned by MCP clients)
serialhub -c config.toml                     # use a config file
serialhub -D                                 # debug mode
```

| Option | Short | Description | Default |
|--------|-------|-------------|---------|
| `--serial-port <port>` | `-p` | Serial port name | config file or empty |
| `--baud-rate <rate>` | `-b` | Baud rate | 115200 |
| `--data-bits <bits>` | `-d` | Data bits (5/6/7/8) | 8 |
| `--parity <type>` | - | Parity (none/even/odd) | none |
| `--stop-bits <bits>` | `-s` | Stop bits (1/2) | 1 |
| `--mcp-port <port>` | `-m` | MCP HTTP service port (auto-increments by 1 when occupied, up to 10 tries) | 5050 |
| `--host <host>` | - | Listen address | 127.0.0.1 |
| `--config <path>` | `-c` | Config file path | - |
| `--debug` | `-D` | Enable debug mode | false |
| `--log-data` | - | Emit data-content logs (500ms window aggregation, 512-byte display truncation; also `SERIALHUB_LOG_DATA=1`, explicit `--log-data=false` wins; neither is persisted to the config file) | false |
| `--stdio` | - | stdio mode: spawned by MCP clients (transparently proxies when a master instance exists) | false |
| `--minimized` | - | Launched by scripts; minimize window (also hides console on Windows); browser still opens by default | false |
| `--no-browser` | - | Skip auto-opening the browser (log still prints the Web terminal URL) | false |

On Windows the system tray starts by default (icon color reflects serial state; right-click menu to connect/disconnect, show console, quit) — see [QUICKSTART.md](./QUICKSTART.md#windows-系统托盘) (Chinese).

### Web Terminal

`http://localhost:5050/terminal`: live serial output, keyboard input forwarded to the serial port (Ctrl+C etc. supported), auto-reconnect.

### MCP Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `serial_list` | List all available serial ports | - |
| `serial_connect` | Connect to a serial port | `port` (required), `baudRate?` (default 115200) |
| `serial_disconnect` | Disconnect the current port | - |
| `serial_write` | Send data to the serial port | `data` (required), `addNewline?` (default true; set false to disable) |
| `serial_read` | Blocking read, returns when data arrives | `timeout?` (default 1000ms, 0 = wait forever), `maxSize?` (default 4096 bytes) |
| `serial_clear` | Clear the read buffer, discard unread data | - |
| `serial_status` | Get serial connection status | - |

Standard workflow: `serial_list` → `serial_connect` → `serial_write` → `serial_read` → `serial_disconnect`.
cURL/Python examples, typical workflows, error handling, when-to-use-which and best practices see [MCP.md](./MCP.md) (Chinese).

## Configuration

Precedence: **CLI flags > config file > defaults**

Config file lookup order (without `-c`):

- **Linux/macOS**: `./config.toml` (CWD) > `~/.config/serialhub/config.toml` (`XDG_CONFIG_HOME`); a legacy `config.toml` next to the binary is auto-migrated (moved) to the user config dir on first start if the user dir has none; if none exists anywhere, a new one is created in the user config dir.
- **Windows**: `config.toml` next to the executable (same as previous versions).

Persist timing: the merged config is written back **only after the single-instance lock is acquired** (i.e. becoming the master instance); a duplicate launch that proxies to the running master never modifies the config file (prevents `-m`-style flags from polluting the on-disk config).

Default log directory: `~/.config/serialhub/logs/` on Linux/macOS (legacy `logs/` history is not migrated), `logs/` next to the executable on Windows; overridable via `logDir` or `SERIALHUB_LOG_DIR`.

TOML format with `#` comments; full example in [config.example.toml](./config.example.toml):

```toml
[serial]
port = ""           # serial port; empty = no auto-connect
baudRate = 115200
dataBits = 8
parity = "none"     # none / even / odd
stopBits = 1

[mcp]
httpPort = 5050
```

## Development

```bash
go run ./cmd/serialhub          # dev run
go build -o bin/serialhub ./cmd/serialhub
go test ./...                   # tests (mock serial/conns in internal/testutil)
go vet ./...
```

- Testing Windows builds from WSL (tray, single-instance lock, PowerShell scripts): [docs/wsl-windows-testing.md](./docs/wsl-windows-testing.md) (Chinese).
- Hardware-in-the-loop tests are gated by `SERIALHUB_HARDWARE_TEST=1` with `SERIALHUB_TEST_PORT`.
- Release process: [RELEASING.md](./RELEASING.md).

## License

Released under the [Apache License 2.0](./LICENSE) (Copyright 2026 dongly).
