# SerialHub Python 集成测试使用说明

## 简介

`tests/integration/` 是 SerialHub 的 Python 集成测试套件（pytest），覆盖 CLI、
服务器生命周期、配置文件、MCP 工具、WebSocket、Web 终端、日志和串口硬件。
浏览器 UI 测试在 `tests/integration/playwright/`。

## 前置要求

```bash
pip install pytest requests
# Windows 用 com0com 虚拟串口对自动回显时还需：
pip install pyserial

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
| `SERIALHUB_TEST_PORT` | 测试用串口号 | POSIX 自动 pty / Windows 自动 com0com / 否则 `COM4` |
| `SERIALHUB_TEST_PORT_PEER` | com0com 对端口（测试框架自动回显） | 未设置 |
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
├── test_serial_hardware.py   # 串口测试（伪设备即可跑，含启动自动连接 AutoConnect）
├── test_server_lifecycle.py  # 服务器生命周期测试
├── test_websocket.py         # WebSocket 测试
├── test_tray.py              # 系统托盘测试（Windows）
├── test_web_terminal.py      # Web 终端测试
├── playwright/               # 浏览器 UI 测试（Playwright）
└── README.md                 # 本说明文件
```

## 故障排查

- **测试跳过（skipped）**：服务器交互测试需设置 `SERIALHUB_INTEGRATION_TEST=1`；
  串口测试在无硬件时自动使用伪设备（POSIX pty / Windows com0com），见下文「伪设备」节。
- **端口被占用**：测试使用动态端口分配，如冲突检查残留进程
  （Windows: `tasklist | findstr serialhub` → `taskkill /F /IM serialhub.exe`）。
- **Windows 编码**：测试文件使用 `encoding="utf-8"` 处理 Go 的中文输出。

## 注意事项

1. 二进制缺失时测试自动构建 `bin/serialhub`
2. 临时目录使用 `tmp_path` fixture 创建
3. `serialhub_server` fixture 确保进程终止和清理

## 伪设备（无硬件测试）

集成测试默认在无串口硬件时使用伪设备，无需 `SERIALHUB_HARDWARE_TEST`：

- **POSIX（Linux/macOS）**：自动用 `pty` 创建伪串口并在对端回显（模拟 TX-RX 短接），
  无需任何配置；也可用 `SERIALHUB_TEST_PORT` 指定真实设备。
- **Windows**：自动检测 [com0com](https://sourceforge.net/projects/com0com/) 虚拟串口对
  （读取注册表 `HARDWARE\DEVICEMAP\SERIALCOMM` 里的 `\Device\com0com*`，如 COM22 ↔ COM23），
  自动选一端作测试串口、另一端起回显线程（需 `pyserial`；本机已安装 com0com 并建好
  COM22 ↔ COM23）。也可显式覆盖：

  ```powershell
  $env:SERIALHUB_TEST_PORT = "COM22"       # SerialHub 使用的一端
  $env:SERIALHUB_TEST_PORT_PEER = "COM23"  # 对端，测试框架自动回显（需 pyserial）
  ```

  仅设置 `SERIALHUB_TEST_PORT` 而不设 `_PEER` 时，需自备回环（真实设备 TX-RX 短接）。

  Go 侧等价入口：`tools/test-windows.sh -hw [TEST:PEER]`（WSL 一条命令交叉编译并经
  真实 Windows 实跑，自动探测 com0com 串口对并自动起对端回显，用 .NET SerialPort
  实现、无需 pyserial）。

- 若都没有可用端口，Windows 回退默认 `COM4`。

相关用例：`test_serial_hardware.py`——连接/断开、回环读写、参数切换、长数据与 Unicode
（POSIX 自动用 pty、Windows 自动用 com0com，均无需真实硬件），以及伪设备上按 `--config`
启动的启动自动连接用例（`TestAutoConnect`，仅 POSIX）。
