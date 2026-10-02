# SerialHub

English | [简体中文](./README.md)

A two-way bridge between serial ports (MCU) and network clients (Web terminal / AI tools): it bridges a single MCU serial port to both humans and AI, as one Go binary (frontend embedded), for Windows (system tray) / Linux / macOS.

**📚 Docs**: [Quick Start](./QUICKSTART.md) (中文) | [MCP Guide](./MCP.md) (中文) | [Config & Architecture](./AGENTS.md) | [Release Process](./RELEASING.md) | [Integration Tests](./tests/integration/README.md)

## Overview

- **Humans**: an xterm.js Web terminal in the browser for live viewing and input (opens automatically on startup; on WSL it opens the Windows host browser)
- **AI**: a native MCP server (Streamable HTTP + stdio) exposing 8 tools (`serial_list` / `serial_connect` / `serial_write` / `serial_read` / `serial_clear` / `serial_disconnect` / `serial_status` / `serial_script`)
- Both channels share **the same serial connection and data buffer** — humans and AI see the same bytes
- Everything configurable via TOML or CLI flags, with data logging on demand (`--log-data`)

## Architecture

```mermaid
flowchart TB
    MCU["MCU"] <-->|"Serial (COM9, 115200, 8N1)"| Serial["Serial Manager"]

    subgraph SerialHub["SerialHub (single binary)"]
        Serial <-->|"Event bus"| Bridge["DataBridge"]
        Bridge <-->|"read / write"| Buffer["DataBuffer<br/>shared buffer"]
        Bridge <-->|"WebSocket (port 5050)"| Web["Web Terminal<br/>xterm.js"]
        Bridge <-->|"JSON-RPC (HTTP)"| MCP["MCP Server<br/>8 tools"]
    end

    Web <-->|"WebSocket"| Browser["Browser<br/>human"]
    MCP <-->|"MCP protocol"| AI["AI tools<br/>OpenCode / VS Code etc."]
```

## Installation

**One-line install** (finds the latest release, downloads and verifies; set `SERIALHUB_GITHUB_API` to use a mirror):

```bash
# Linux (installs to ~/.local/bin)
curl -fsSL https://raw.githubusercontent.com/dongly/serialhub/main/install.sh | bash
```

```powershell
# Windows: run in PowerShell (installs to %LOCALAPPDATA%\Programs\serialhub and adds to user PATH)
irm https://raw.githubusercontent.com/dongly/serialhub/main/install.ps1 | iex
```

- Manual download, PATH setup, install verification, WSL USB serial attach and more: [QUICKSTART.md](./QUICKSTART.md) (中文).
- Build from source (the only way on macOS; requires Go 1.26+): `git clone https://github.com/dongly/serialhub && cd serialhub && go build -o serialhub ./cmd/serialhub`
- Upgrade & uninstall: `serialhub upgrade` (download, verify, atomically replace itself) / `serialhub uninstall` (dry-run list, then clean MCP entries / config / logs / binary); proxies honor `HTTPS_PROXY`.

## Quick Start

**Scenario: human + AI debugging together**

1. Start SerialHub:

```bash
serialhub -p COM9 --host 0.0.0.0 -D
```

On Windows prefer the launcher script in the install directory, `.\sr.ps1 [args]` (e.g. `.\sr.ps1 -p COM9 -D`):

- It first force-kills any still-running serialhub process, so restarting needs no manual cleanup;
- It starts SerialHub minimized (adds `--minimized` automatically) and prints the PID, the log path (`logs\serialhub.log` next to the exe) and how to stop it;
- Arguments `-p / -b / -c / -D / -m` match the same-named arguments of `serialhub.exe` directly (the script's `-listen` maps to the exe's `--host`); you can also double-click `sr.bat` (which forwards to `sr.ps1`). The script is named `sr` so it never shadows `serialhub.exe` on PATH.

2. Human watches via the Web terminal: open `http://localhost:5050/terminal`. The "Exit" button remotely shuts down SerialHub (graceful shutdown; beware when exposed to LAN via `--host 0.0.0.0` — anyone who can open the page can stop the service).
3. AI connects: run `serialhub setup` to write the MCP client configuration in one command (stdio local mode by default; supports OpenCode / Claude Code / Cursor / Windsurf / VS Code / Codex); manual setup see [MCP.md](./MCP.md).
4. Serial data is forwarded to both the Web terminal and the AI interface; each can send commands independently.

## Commands & Configuration

All options: `serialhub --help` and the CLI flag table in [AGENTS.md](./AGENTS.md). Config precedence is **CLI flags > config file > defaults**; lookup order, persist timing and log directory are documented in [AGENTS.md](./AGENTS.md), full example in [config.example.toml](./config.example.toml). On Windows the system tray starts by default (connect / disconnect / quit from the menu) — see [QUICKSTART.md](./QUICKSTART.md#windows-系统托盘) (中文).

## Development

Development workflow and common commands: [AGENTS.md](./AGENTS.md) (中文); testing Windows builds from WSL (tray, single-instance lock, PowerShell scripts): [docs/wsl-windows-testing.md](./docs/wsl-windows-testing.md) (中文).

## License

Released under the [Apache License 2.0](./LICENSE) (Copyright 2026 dongly).
