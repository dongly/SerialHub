# SerialHub 快速开始

## 1. 下载与安装

从 [GitHub Releases](https://github.com/dongly/serialhub/releases) 下载对应平台的版本：

- **Windows**: `serialhub-x.x.x-windows-amd64.zip`
- **Linux**: `serialhub-x.x.x-linux-amd64.tar.gz`

### 解压

包内为平铺结构：

```
serialhub-x.x.x-windows-amd64/
├── serialhub.exe    # 主程序（Linux 为 serialhub）
├── config.toml      # 配置文件模板
├── start.ps1        # Windows 启动脚本（PowerShell，自动杀旧进程+最小化）
├── start.bat        # Windows 启动脚本（cmd 极简版）
├── README.md / README.en.md
├── MCP.md           # MCP 使用指南
├── QUICKSTART.md
├── VERSION          # 版本信息
└── LICENSE          # Apache-2.0
```

### 加入 PATH（可选，命令行直接敲 `serialhub` 用）

**Windows**（PowerShell，管理员）：

```powershell
# 假设解压到 D:\Tools\serialhub
[Environment]::SetEnvironmentVariable("Path", $env:Path + ";D:\Tools\serialhub", "Machine")
```

**Linux**：

```bash
# 假设解压到 ~/tools/serialhub
sudo ln -s ~/tools/serialhub/serialhub /usr/local/bin/serialhub
```

### 验证安装

```bash
serialhub --version    # 输出 SerialHub v0.5.0 形式即成功
```

### WSL 用户：把 USB 串口接入 WSL

WSL 默认看不到 Windows 宿主的 USB 串口，需要 usbipd 挂载。推荐图形工具
[wsl-usb-manager](https://github.com/nickbeth/wsl-usb-manager) 一键 attach，
或使用命令行 `usbipd`（`usbipd list` → `usbipd bind` → `usbipd attach --wsl`）。
挂载成功后 WSL 内出现 `/dev/ttyUSB*`，即可被 SerialHub 联邦模式从侧使用。

## 2. 启动 SerialHub

### Windows

```powershell
# 方式 1: 使用启动脚本
.\start.ps1

# 方式 2: 双击 start.bat（极简启动）

# 方式 3: 直接运行
.\serialhub.exe -p COM9
```

### Linux

```bash
# 直接运行
./serialhub

# 指定串口启动
./serialhub -p /dev/ttyUSB0
```

## 3. 访问 Web 终端

打开浏览器访问：
```
http://localhost:5000/terminal
```

## 4. AI 工具配置

SerialHub 是标准 MCP 服务器，OpenCode / Claude Code / Cursor / Windsurf / VS Code
等客户端均可接入。以 OpenCode 为例（`opencode.json`）：

```json
{
  "mcp": {
    "serialhub": {
      "type": "remote",
      "url": "http://127.0.0.1:5000/mcp",
      "enabled": true
    }
  }
}
```

其他客户端（Claude Code / Cursor / Windsurf / VS Code）的配置方法见
[MCP.md 的「MCP 客户端配置」](./MCP.md)。也支持 `serialhub --stdio`
本地拉起方式（客户端自动启动、退出时随之结束）。

## 5. 常用命令

| 命令 | 说明 |
|------|------|
| `serialhub -p COM9` | 指定串口启动 |
| `serialhub -p COM9 -b 9600` | 指定波特率 |
| `serialhub -D` | 调试模式 |
| `serialhub -c config.toml` | 使用配置文件 |

## 6. MCP 工具使用

AI 工具可用以下 MCP 工具与串口交互：

```python
# 1. 列出可用串口
serial_list()

# 2. 连接串口
serial_connect({"port": "COM9", "baudRate": 115200})

# 3. 发送命令
serial_write({"data": "version"})

# 4. 读取响应
serial_read({"timeout": 3000})

# 5. 断开连接
serial_disconnect()
```

## 7. 配置文件示例

编辑 `config.toml`：

```toml
# SerialHub 配置文件

# 监听地址
host = "127.0.0.1"

# 日志目录，为空则保存到可执行文件目录下的 logs/
# logDir = "D:/Logs"

# 调试模式
# debug = false

[serial]
# 串口号，为空时不自动连接（如 COM3、/dev/ttyUSB0）
port = ""
baudRate = 115200
dataBits = 8
# 校验位：none / even / odd
parity = "none"
# 停止位：1 / 1.5 / 2
stopBits = 1

[mcp]
httpPort = 5000
```

## 8. 系统托盘 (Windows)

Windows 版本默认启用系统托盘：
- **双击图标**: 显示/隐藏控制台
- **右键菜单**: 串口信息、端口信息、退出

## 9. 故障排除

| 问题 | 解决方案 |
|------|----------|
| 串口无法连接 | 检查串口号是否正确，使用 `serial_list` 查看可用串口 |
| 端口被占用 | 关闭其他串口工具或重启 SerialHub |
| Web 终端无法访问 | 检查防火墙设置，确认端口 5000 未被占用 |
| AI 工具无法连接 | 检查 MCP URL 是否正确，确认 SerialHub 已启动 |

## 10. 获取帮助

```bash
# 查看帮助
serialhub --help

# 查看版本
serialhub --version
```

更多文档：
- [完整使用指南](./README.md)
- [MCP 详细文档](./MCP.md)
- [开发指南](./AGENTS.md)
