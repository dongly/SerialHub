# SerialHub Python 集成测试使用说明

## 简介

`tests/integration/` 是 SerialHub 的 Python 集成测试套件（pytest），覆盖 CLI、
服务器生命周期、配置文件、MCP 工具、WebSocket、Web 终端、日志和串口硬件。
浏览器 UI 测试在 `tests/integration/playwright/` 与 `tests/e2e/`。

## 前置要求

```bash
pip install pytest requests

# 确保已构建 serialhub（缺失时测试会自动构建到 bin/）
go build -o bin/serialhub ./cmd/serialhub
```

## 快速开始

```bash
# 仅 CLI 基础测试（无需服务器）
pytest tests/integration/ -v -k "not server"

# 全量测试（需启动服务器）
# Windows PowerShell
$env:SERIALHUB_INTEGRATION_TEST = "1"; $env:SERIALHUB_TEST_PORT = "COM4"; pytest tests/integration/ -v
# Linux/macOS
export SERIALHUB_INTEGRATION_TEST=1 SERIALHUB_TEST_PORT=/dev/ttyUSB0
pytest tests/integration/ -v
```

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `SERIALHUB_INTEGRATION_TEST` | 启用服务器交互测试 | 未设置（跳过） |
| `SERIALHUB_TEST_PORT` | 测试用串口号 | `COM4` |
| `SERIALHUB_LOG_DIR` | 日志目录 | 临时目录 |

## 硬件测试要求

串口回环测试需将 **TX 和 RX 短接**（发送的数据立即被接收，验证数据完整性），
覆盖波特率 9600–115200、1KB/10KB 长数据、Unicode/二进制数据。

## 测试文件

```
tests/integration/
├── conftest.py               # 共享 fixtures 和辅助函数
├── test_cli.py               # CLI 命令行测试
├── test_config.py            # 配置文件测试
├── test_logging.py           # 日志测试
├── test_mcp_tools.py         # MCP 工具测试
├── test_serial_hardware.py   # 串口硬件测试（需回环串口）
├── test_server_lifecycle.py  # 服务器生命周期测试
├── test_websocket.py         # WebSocket 测试
├── test_tray.py              # 系统托盘测试（Windows）
├── test_web_terminal.py      # Web 终端测试
├── playwright/               # 浏览器 UI 测试（Playwright）
└── README.md                 # 本说明文件
```

E2E 测试（需真实串口设备与浏览器）见 `tests/e2e/`。

## 故障排查

- **测试跳过（skipped）**：服务器交互测试需设置 `SERIALHUB_INTEGRATION_TEST=1`；
  串口测试需真实设备，不可用时自动跳过。
- **端口被占用**：测试使用动态端口分配，如冲突检查残留进程
  （Windows: `tasklist | findstr serialhub` → `taskkill /F /IM serialhub.exe`）。
- **Windows 编码**：测试文件使用 `encoding="utf-8"` 处理 Go 的中文输出。

## 注意事项

1. 二进制缺失时测试自动构建 `bin/serialhub`
2. 临时目录使用 `tmp_path` fixture 创建
3. `serialhub_server` fixture 确保进程终止和清理
