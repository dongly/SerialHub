# SerialHub

Bidirectional bridge between serial ports (MCU) and network connections (telnet/AI).

## Purpose

SerialHub enables AI-assisted debugging of MCU programs by:
- Forwarding MCU serial output to both telnet (for human monitoring) and AI interfaces (for programmatic analysis)
- Allowing bidirectional communication for both human operators (via MobaXterm, etc.) and AI tools
- Supporting MCU shell operations for runtime control and debugging

## Architecture

```
┌─────────────┐
│     MCU     │
└──────┬──────┘
       │ Serial Port (COM9, 115200, 8N1)
       ▼
┌─────────────────────────────────────────┐
│              SerialHub                  │
│                                         │
│  ┌─────────────┐    ┌───────────────┐  │
│  │   Serial    │    │  DataBridge   │  │
│  │   Manager   │◄──►│  (Event Bus)  │  │
│  └─────────────┘    └───────┬───────┘  │
│                             │          │
│              ┌──────────────┼────────┐ │
│              ▼              ▼        ▼ │
│       ┌───────────┐  ┌──────────┐ ... │
│       │  Telnet   │  │   MCP    │     │
│       │  Server   │  │ Service  │     │
│       │ (port 2323)│ │(stdio/HTTP)│   │
│       └───────────┘  └──────────┘     │
└─────────────────────────────────────────┘
       │                    │
       ▼                    ▼
┌─────────────┐     ┌─────────────┐
│ MobaXterm   │     │ AI Tools    │
│ (Human)     │     │ (OpenCode,  │
│             │     │  iFlow CLI) │
└─────────────┘     └─────────────┘
```

## Tech Stack

| Component | Technology |
|-----------|------------|
| Runtime | Bun |
| Language | TypeScript |
| Serial Port | @serialport |
| AI Interface | MCP (Model Context Protocol) |
| Testing | Bun test |

## Features

- **Dual-path forwarding**: Serial data forwarded to both telnet and AI interfaces simultaneously
- **Bidirectional**: Commands from telnet or AI sent to MCU
- **MCP Protocol**: Standard AI tool integration via Model Context Protocol
- **Dual transport**: stdio (for AI tool subprocess) and HTTP+SSE (for standalone server)
- **Configurable**: All ports, baud rates, timeouts configurable via JSON or CLI
- **Observable**: All data flows logged and trackable
- **Error resilient**: Network/serial failures handled gracefully

## Installation

```bash
bun install
```

## Usage

### Mode 1: stdio MCP (AI Tool Integration)

Run SerialHub as an MCP server via stdio:

```bash
serialhub mcp
```

Configure in your AI tool (e.g., OpenCode, iFlow CLI):

```json
{
  "mcpServers": {
    "serialhub": {
      "command": "serialhub",
      "args": ["mcp"]
    }
  }
}
```

### Mode 2: Server Mode (HTTP+SSE + Telnet)

Run SerialHub as a standalone server:

```bash
serialhub serve
```

Options:
```bash
serialhub serve --serial-port COM9 --baud-rate 115200
serialhub serve --telnet-port 2323 --mcp-port 3000
serialhub serve --config config.json
serialhub serve --debug
```

### Connecting via Telnet

```bash
telnet localhost 2323
```

### MCP HTTP API

```bash
# List serial ports
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_list"},"id":1}'

# Connect to port
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_connect","arguments":{"port":"COM9"}},"id":2}'

# Send command
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"serial_write","arguments":{"data":"help\n"}},"id":3}'
```

## MCP Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `serial_list` | List available serial ports | - |
| `serial_connect` | Connect to a serial port | `port`, `baudRate?` |
| `serial_disconnect` | Disconnect from serial port | - |
| `serial_write` | Send data to serial port | `data`, `encoding?` |
| `serial_read` | Read data from buffer | `timeout?` |
| `serial_status` | Get connection status | - |

## Configuration

Default configuration:

```json
{
  "serial": {
    "port": "COM9",
    "baudRate": 115200,
    "dataBits": 8,
    "parity": "none",
    "stopBits": 1
  },
  "telnet": {
    "port": 2323
  },
  "mcp": {
    "httpPort": 3000
  },
  "debug": false
}
```

## Development

```bash
# Type check
bun run typecheck

# Lint
bun run lint

# Test
bun test

# Start development server
bun run serve
```

## License

MIT