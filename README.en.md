# SerialHub

English | [简体中文](./README.md)

A bidirectional bridge between a serial port (MCU) and network clients (web terminal / AI).

**📚 Docs**: [MCP Guide](./MCP.md) | [Project Architecture](./AGENTS.md) | [Integration Tests](./tests/integration/README.md)

## Overview

SerialHub bridges a single MCU UART to both humans and AI agents:

- **Humans** get a live xterm.js web terminal in the browser
- **AI agents** get a native MCP server (Streamable HTTP + stdio) with 7 tools: `serial_list` / `serial_connect` / `serial_write` / `serial_read` / `serial_clear` / `serial_disconnect` / `serial_status`
- Both channels share **one serial connection and one data buffer** — humans and AI literally watch the same bytes
- **Federation mode** links a Windows master with a WSL worker; serial ports from both sides are aggregated as `side:port` (e.g. `windows:COM3`, `wsl:/dev/ttyUSB0`), and the worker auto-promotes to master if the master goes down
- Single Go binary with the web frontend embedded; runs on Windows (system tray) / Linux / macOS

## Architecture

```
┌─────────────┐
│     MCU     │
└──────┬──────┘
       │ UART (COM9, 115200, 8N1)
       ▼
┌─────────────────────────────────────────┐
│              SerialHub                  │
│                                         │
│  ┌─────────────┐    ┌───────────────┐  │
│  │   Serial    │◄──►│   DataBridge  │  │
│  │   Manager   │    │  (event bus)  │  │
│  └─────────────┘    └───────┬───────┘  │
│                             │          │
│              ┌──────────────┼────────┐ │
│              ▼              ▼        ▼ │
│       ┌───────────┐  ┌──────────┐ ... │
│       │   Web     │  │   MCP    │     │
│       │ Terminal  │  │  Server  │     │
│       │ (port 5000)│ │  (HTTP)  │     │
│       └───────────┘  └──────────┘     │
└─────────────────────────────────────────┘
       │                    │
       ▼                    ▼
┌─────────────┐     ┌─────────────┐
│   Browser   │     │  AI tools   │
│   (human)   │     │ (OpenCode,  │
│             │     │  iFlow CLI) │
└─────────────┘     └─────────────┘
```

## Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.26+ |
| Serial I/O | go.bug.st/serial |
| AI interface | MCP (Model Context Protocol) / go-sdk |
| Web terminal | WebSocket / xterm.js |
| CLI | spf13/cobra |
| Config | spf13/viper |
| System tray | getlantern/systray |
| Logging | sirupsen/logrus |

## Features

- **Dual forwarding**: serial data is forwarded to the web terminal and the AI interface simultaneously
- **Bidirectional**: commands from the web terminal or AI both reach the MCU
- **Web terminal**: browser terminal over WebSocket with xterm.js; auto-opens in the browser on start (WSL pops the Windows browser too)
- **MCP protocol**: standard HTTP JSON-RPC (MCP Streamable HTTP transport, non-streaming JSON responses); also `--stdio` for local launch (OpenCode local mode, transparently proxies to a running instance)
- **Federation mode**: run on Windows and WSL at the same time — the later instance joins automatically, `serial_list` aggregates ports from both sides; the worker auto-promotes when the master is lost
- **Configurable**: all ports, baud rates, and timeouts via TOML or CLI
- **Observable**: all data flows can be logged and traced
- **Error recovery**: network/serial faults are handled gracefully

## Build

```bash
go build -o bin/serialhub.exe ./cmd/serialhub
```

## Command Reference

### `serialhub` (default: serve mode)

Starts the HTTP + web terminal server:

```bash
serialhub                                    # start with defaults
serialhub -p COM8                            # specify serial port
serialhub -p COM8 -b 9600 --parity even      # full serial options
serialhub -m 8080                            # use port 8080
serialhub --host 0.0.0.0                     # listen on all interfaces (needed by the Windows master in federation mode)
serialhub --stdio                            # stdio mode (launched by an MCP client)
serialhub -c config.toml                     # use a config file
serialhub -D                                 # debug mode
```

Full options:

| Option | Short | Description | Default |
|--------|-------|-------------|---------|
| `--serial-port <port>` | `-p` | Serial port name | config file or empty |
| `--baud-rate <rate>` | `-b` | Baud rate | 115200 |
| `--data-bits <bits>` | `-d` | Data bits (5/6/7/8) | 8 |
| `--parity <type>` | - | Parity (none/even/odd) | none |
| `--stop-bits <bits>` | `-s` | Stop bits (1/2) | 1 |
| `--mcp-port <port>` | `-m` | MCP HTTP port | 5000 |
| `--host <host>` | - | Listen address | 127.0.0.1 |
| `--config <path>` | `-c` | Config file path | - |
| `--debug` | `-D` | Debug mode | false |
| `--stdio` | - | stdio mode: launched by an MCP client (transparent proxy if a master exists) | false |
| `--minimized` | - | Script launch: skip auto-opening the browser (cross-platform; also hides the console on Windows) | false |

### System Tray (Windows)

On Windows, `serialhub` starts a system tray icon by default and hides the console window.

**Tray icon states:**
- Gray — no serial port connected
- Green — serial port connected
- Red — connection error

**Context menu:**

| Menu item | Function |
|-----------|----------|
| Serial info | Click to connect/disconnect |
| Port info | Shows the MCP port (not clickable) |
| Show/hide console | Toggles the console window |
| Exit | Quits SerialHub |

**Interaction:**
- Double-click the tray icon: toggle the console window
- Right-click the tray icon: open the menu

### Quick Start

**Scenario: human + AI debugging together**

1. Start SerialHub:

```bash
serialhub -p COM9 --host 0.0.0.0 -D
```

2. Human monitors via the web terminal:

Open `http://localhost:5000/terminal` in a browser.

3. AI tool connects via HTTP MCP:

```json
{
  "mcp": {
    "serialhub": {
      "type": "remote",
      "url": "http://localhost:5000/mcp",
      "enabled": true
    }
  }
}
```

4. Serial data flows to both the web terminal and the AI interface; either side can send commands.

### Web Terminal

SerialHub embeds a WebSocket terminal built on xterm.js.

**URL**: `http://localhost:5000/terminal`

**Features:**
- Live serial output
- Keyboard input forwarded to the serial port
- Control characters (Ctrl+C, Ctrl+D, …)
- Auto-reconnect

### MCP HTTP API

Once the server is running, call MCP tools via JSON-RPC:

```bash
# Health check
curl http://localhost:5000/health

# List serial ports
curl -X POST http://localhost:5000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_list"},"id":1}'

# Connect a serial port
curl -X POST http://localhost:5000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_connect","arguments":{"port":"COM9"}},"id":2}'

# Send a command (newline appended automatically)
curl -X POST http://localhost:5000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_write","arguments":{"data":"help"}},"id":3}'

# Read response (blocking; timeout=0 waits forever)
curl -X POST http://localhost:5000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_read","arguments":{"timeout":5000}},"id":4}'
```

## MCP Tools

| Tool | Description | Arguments |
|------|-------------|-----------|
| `serial_list` | List all available serial ports | - |
| `serial_connect` | Connect to a serial port | `port` (required), `baudRate?` (default 115200) |
| `serial_disconnect` | Disconnect the current port | - |
| `serial_write` | Send data to the serial port | `data` (required), `addNewline?` (default true) |
| `serial_read` | Blocking read; returns when data arrives | `timeout?` (default 1000ms, 0=wait forever), `maxSize?` (default 4096 bytes) |
| `serial_clear` | Clear the read buffer, drop unread data | - |
| `serial_status` | Query connection status | - |

### AI Tool Usage Guide

#### Standard workflow

```
serial_list → identify target → serial_connect → serial_write → serial_read
```

#### When to use which tool

| Scenario | Recommended | Notes |
|----------|-------------|-------|
| Don't know the port name | `serial_list` | Pick by vendorId/productId or vendor name |
| Before debugging | `serial_connect` | Must connect first |
| Send a shell command | `serial_write` + `serial_read` | Write then immediately read, e.g. `help`, `version`, `reboot` |
| Send a control command | `serial_write` | Control or config commands to the MCU |
| Get command output | `serial_read` | timeout=0 for unknown response times |
| Check connection | `serial_status` | Confirm before acting, or after failures |
| Switch device | `serial_disconnect` → `serial_connect` | Disconnect then connect the new one |
| End session | `serial_disconnect` | Release the serial port |

#### Examples

**1. First connection**

```
serial_list()
// returns: { ports: [{ path: "COM6", vendorId: "0D28", productId: "0202" }, ...] }

serial_connect({ port: "COM6", baudRate: 115200 })
// returns: { success: true, port: "COM6", baudRate: 115200 }
```

**2. Send a command and get the response**

```
serial_write({ data: "version" })       // newline appended automatically
// returns: { success: true, bytesWritten: 8 }

serial_read({ timeout: 2000 })
// returns: { data: "MCU v1.2.3\nBuild: 2024-01-15\n", timedOut: false, bytes: 28 }
```

**3. Wait for an unknown-duration response**

```
serial_write({ data: "flash_verify" })  // long operation
serial_read({ timeout: 0 })             // wait until the device replies
```

**4. Switch to another device**

```
serial_disconnect()
serial_list()
serial_connect({ port: "COM7" })
```

**5. Check status**

```
serial_status()
// connected: { connected: true, port: "COM6", baudRate: 115200 }
// idle:      { connected: false }
```

#### Error Handling

| Error | Cause | Fix |
|-------|-------|-----|
| serial_write returns `serial port not connected` | Not connected yet or dropped | Call serial_connect first |
| serial_read returns `timedOut: true` | No data within timeout | Increase timeout or check the device |
| serial_connect returns `success: false` | Port missing, permission, or busy | Check serial_list output and baud rate |
| Partial output | Long output, single read incomplete | Loop serial_read until timedOut=true |

#### Best Practices

1. **Check status first** for complex operations
2. **Match the baud rate** — common values 115200, 9600
3. **Set timeouts sensibly** — 1–5s for regular commands, 0 for long operations
4. **Read right after writing** to avoid data piling up
5. **Expect newlines** (`\n`) in most shell responses

## Configuration

Priority: **CLI flags > config file > defaults**

TOML config with `#` comments:

```toml
# Log directory; empty = ./logs/ next to the executable
# logDir = "D:/Logs"

[serial]
port = ""           # serial port; empty = no auto-connect
baudRate = 115200
dataBits = 8
parity = "none"     # none / even / odd
stopBits = 1

[mcp]
httpPort = 5000
```

| Key | Default | Description |
|-----|---------|-------------|
| `serial.port` | `""` | Serial port; empty = no auto-connect |
| `serial.baudRate` | `115200` | Baud rate |
| `serial.dataBits` | `8` | Data bits (5/6/7/8) |
| `serial.parity` | `"none"` | Parity (none/even/odd) |
| `serial.stopBits` | `1` | Stop bits (1/2) |
| `mcp.httpPort` | `5000` | MCP HTTP port (also serves the web terminal) |
| `logDir` | `""` | Log directory; empty = `logs/` next to the executable |
| `debug` | `false` | Debug mode |

## Development

```bash
# Run
go run ./cmd/serialhub

# Build
go build -o bin/serialhub.exe ./cmd/serialhub

# Test
go test ./...

# Static analysis
go vet ./...

# Tidy dependencies
go mod tidy
```

## License

This project is licensed under the [Apache License 2.0](./LICENSE) (Copyright 2026 dongly).
