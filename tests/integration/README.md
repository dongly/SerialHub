# SerialHub Python 集成测试使用说明

## 简介

`test_serialhub.py` 是 SerialHub 的 Python 集成测试套件，使用 pytest 框架编写。测试覆盖 CLI、服务器生命周期、配置文件、MCP 工具、Telnet、日志和串口硬件等全功能。

## 前置要求

```bash
# 安装依赖
pip install pytest requests

# 确保已构建 serialhub.exe
go build -o bin/serialhub.exe ./cmd/serialhub
```

## 快速开始

### 1. 仅运行 CLI 基础测试（推荐，无需启动服务器）

```bash
pytest tests/integration/test_serialhub.py -v -k "not server"
```

测试内容：
- `test_version` — 验证 `--version` 输出
- `test_help_flags` — 验证所有命令行参数
- `test_debug_with_help` — 验证 `--debug` 与 `--help` 组合
- `test_invalid_flag` — 验证无效参数处理
- `test_baud_rate_flag` — 验证波特率参数

### 2. 运行所有测试（需启动服务器）

```bash
# Windows cmd
set SERIALHUB_INTEGRATION_TEST=1
set SERIALHUB_TEST_PORT=COM4
pytest tests/integration/test_serialhub.py -v

# Windows PowerShell
$env:SERIALHUB_INTEGRATION_TEST = "1"; $env:SERIALHUB_TEST_PORT = "COM4"; pytest tests/integration/test_serialhub.py -v

# Linux/macOS
export SERIALHUB_INTEGRATION_TEST=1
export SERIALHUB_TEST_PORT=/dev/ttyUSB0
pytest tests/integration/test_serialhub.py -v
```

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `SERIALHUB_INTEGRATION_TEST` | 启用服务器交互测试 | 未设置（跳过） |
| `SERIALHUB_TEST_PORT` | 测试用串口号 | `COM4` |
| `SERIALHUB_LOG_DIR` | 日志目录 | 临时目录 |

## 硬件测试要求

### 串口回环测试（COM4）

硬件测试需要将串口的 **TX 和 RX 短接**（回环模式）：

```
COM4 端口:  TX pin ─────┬───── RX pin
                       │
                    (短接)
```

回环模式下，发送的数据会立即被接收，用于测试数据完整性。

### 测试波特率

- 9600
- 19200
- 38400
- 57600
- 115200

## 测试分类

### TestCLI — 命令行测试
验证 serialhub 命令行参数解析和输出。

### TestServerLifecycle — 服务器生命周期
- `test_start_stop` — 启动服务器并健康检查
- `test_invalid_serial_port` — 无效串口启动测试

### TestConfigFile — 配置文件测试
- `test_toml_config` — TOML 配置文件加载
- `test_invalid_config_file` — 无效配置处理
- `test_config_save_to_toml` — **配置保存到 config.toml**
- `test_config_auto_save_on_tray_change` — **托盘菜单修改自动保存**
- `test_default_config_location` — **默认配置文件位置（可执行文件同级目录）**

### TestMCPTools — MCP 工具测试
- `test_serial_list` — 列出可用串口
- `test_serial_status_not_connected` — 未连接状态
- `test_serial_connect_invalid` — 无效串口连接
- `test_serial_disconnect_not_connected` — 未连接时断开
- `test_serial_write_not_connected` — 未连接时写入
- `test_serial_read_not_connected` — 未连接时读取
- `test_mcp_health_endpoint` — 健康检查端点
- `test_mcp_unknown_tool` — 未知工具处理

### TestTelnet — Telnet 服务测试
- `test_telnet_connect` — 连接测试
- `test_telnet_send_data` — 数据发送
- `test_telnet_multiple_clients` — 多客户端支持
- `test_telnet_loopback` — **回环测试（Telnet ↔ 串口）**
- `test_telnet_unicode_and_large_data` — **Unicode 和长数据测试**

### TestLogging — 日志测试
- `test_log_file_created` — 日志文件生成
- `test_debug_mode_logging` — 调试模式日志

### TestSerialHardware — 串口硬件测试（需真实串口，COM4 回环模式）
- `test_serial_connect_disconnect` — 连接/断开
- `test_serial_write_read` — 写入/读取
- `test_hardware_end_to_end` — **多轮回环测试**
- `test_serial_params_change` — **多波特率测试 (9600-115200)**
- `test_long_data_loopback` — **长数据测试 (1KB, 10KB)**
- `test_binary_and_unicode_data` — **Unicode/二进制数据测试**
- `test_mcp_various_data_types` — **MCP 各种数据类型回环测试**

## 常用命令

```bash
# 查看所有测试
pytest tests/integration/test_serialhub.py --collect-only

# 运行特定测试类
pytest tests/integration/test_serialhub.py -v -k "TestCLI"

# 运行特定测试方法
pytest tests/integration/test_serialhub.py -v -k "test_version"

# 显示详细输出
pytest tests/integration/test_serialhub.py -v -s

# 失败时停止
pytest tests/integration/test_serialhub.py -v -x

# 生成覆盖率报告（需 pytest-cov）
pip install pytest-cov
pytest tests/integration/test_serialhub.py --cov=.
```

## 故障排查

### UnicodeDecodeError: 'gbk' codec can't decode
已修复。测试文件使用 `encoding="utf-8"` 处理 Go 的中文输出。

### 测试跳过（skipped）
服务器交互测试需要设置环境变量：
```bash
set SERIALHUB_INTEGRATION_TEST=1
```

### 串口测试跳过
硬件测试需要真实串口设备，或串口不可用时自动跳过。

### 端口被占用
测试使用动态端口分配（`find_free_port()`），一般不会出现冲突。如发生冲突，检查是否有残留进程：
```bash
tasklist | findstr serialhub
taskkill /F /IM serialhub.exe
```

## 文件结构

```
tests/integration/
├── test_serialhub.py      # 主测试文件
├── __init__.py            # （可选）Python 包标记
└── README.md              # 本说明文件
```

## 注意事项

1. **Windows 编码**：测试已处理 Windows GBK 编码问题
2. **二进制自动构建**：如 `bin/serialhub.exe` 不存在，测试会自动构建
3. **临时目录**：测试使用 `tmp_path` 创建临时配置和日志目录
4. **进程清理**：`serialhub_server` fixture 确保进程终止和清理
5. **回环模式**：硬件测试需要 COM4 的 TX-RX 短接
6. **数据流向**：
   - MCP 发送 → 串口回环 → MCP 接收（回环数据）
   - MCP 发送 → DataBridge 转发 → Telnet 接收（转发数据）
