# SerialHub 部署指南

本文档介绍如何构建、发布和部署 SerialHub。

## 目录

- [快速开始](#快速开始)
- [构建](#构建)
- [发布](#发布)
- [部署](#部署)
- [配置](#配置)
- [服务管理](#服务管理)
- [故障排除](#故障排除)

## 快速开始

### 开发环境运行

```powershell
# Windows
go run ./cmd/serialhub -p COM9 -D

# Linux/macOS
go run ./cmd/serialhub -p /dev/ttyUSB0 -D
```

### 生产环境部署

```powershell
# Windows 一键部署
.\scripts\deploy.ps1 -SerialPort COM9 -Port 5000 -CreateService -StartAfterInstall

# Linux 一键部署
sudo ./scripts/deploy.sh --serial /dev/ttyUSB0 --port 5000 --service --start
```

## 构建

### 前置要求

- Go 1.26+
- Git
- PowerShell 7+ (Windows) 或 Bash (Linux/macOS)

### 手动构建

```bash
# 基本构建
go build -o bin/serialhub ./cmd/serialhub

# Windows
go build -o bin/serialhub.exe ./cmd/serialhub

# 带版本信息构建（版本号从 Git commit 自动获取）
go build -ldflags "-s -w" -o bin/serialhub ./cmd/serialhub
```

### 查看版本

```bash
./bin/serialhub --version
# 输出: SerialHub v5ed0fa5
```

## 发布

### Windows 发布

```powershell
# 使用发布脚本
.\scripts\build-release.ps1

# 指定版本号
.\scripts\build-release.ps1 -Version v1.0.0

# 指定输出目录
.\scripts\build-release.ps1 -OutputDir ./releases
```

输出文件：`dist/serialhub-<版本>-windows-amd64.zip`

### Linux/macOS 发布

```bash
# 使用发布脚本
./scripts/build-release.sh

# 指定版本号
./scripts/build-release.sh v1.0.0
```

输出文件：`dist/serialhub-<版本>-<os>-<arch>.tar.gz`

### 发布包内容

```
serialhub-<版本>-<平台>/
├── serialhub(.exe)    # 可执行文件
├── config.toml        # 配置文件
├── start.ps1          # Windows 启动脚本
├── start.sh           # Linux/macOS 启动脚本
├── README.md          # 项目说明
├── DEPLOY.md          # 部署文档
└── MCP.md             # MCP 使用指南
```

## 部署

### Windows 部署

#### 方式 1: 使用部署脚本（推荐）

```powershell
# 以管理员身份运行 PowerShell
.\scripts\deploy.ps1 -SerialPort COM9 -Port 5000 -CreateService -StartAfterInstall

# 参数说明
-SerialPort <端口>      # 默认串口（如 COM9）
-Port <端口>            # MCP/Web 服务端口（默认 5000）
-InstallDir <路径>      # 安装目录（默认 C:\Program Files\SerialHub）
-CreateService          # 创建 Windows 服务
-StartAfterInstall      # 安装后立即启动
```

#### 方式 2: 手动部署

```powershell
# 1. 创建目录
mkdir "C:\Program Files\SerialHub"

# 2. 复制文件
copy bin\serialhub.exe "C:\Program Files\SerialHub\"
copy config.example.toml "C:\Program Files\SerialHub\config.toml"

# 3. 编辑配置文件
notepad "C:\Program Files\SerialHub\config.toml"

# 4. 启动服务
"C:\Program Files\SerialHub\serialhub.exe" -c "C:\Program Files\SerialHub\config.toml"
```

### Linux 部署

#### 方式 1: 使用部署脚本（推荐）

```bash
# 一键部署并创建 systemd 服务
sudo ./scripts/deploy.sh --serial /dev/ttyUSB0 --port 5000 --service --start

# 参数说明
-d, --dir <路径>        # 安装目录（默认 /opt/serialhub）
-p, --port <端口>       # MCP/Web 服务端口（默认 5000）
-s, --serial <串口>     # 默认串口（如 /dev/ttyUSB0）
--service               # 创建 systemd 服务
--start                 # 安装后立即启动
```

#### 方式 2: 手动部署

```bash
# 1. 创建目录
sudo mkdir -p /opt/serialhub

# 2. 复制文件
sudo cp bin/serialhub /opt/serialhub/
sudo cp config.example.toml /opt/serialhub/config.toml

# 3. 编辑配置文件
sudo nano /opt/serialhub/config.toml

# 4. 创建 systemd 服务
sudo tee /etc/systemd/system/serialhub.service > /dev/null <<EOF
[Unit]
Description=SerialHub
After=network.target

[Service]
Type=simple
ExecStart=/opt/serialhub/serialhub -c /opt/serialhub/config.toml
Restart=always
User=root
WorkingDirectory=/opt/serialhub

[Install]
WantedBy=multi-user.target
EOF

# 5. 启动服务
sudo systemctl daemon-reload
sudo systemctl enable serialhub
sudo systemctl start serialhub
```

### Docker 部署（可选）

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -ldflags "-s -w" -o serialhub ./cmd/serialhub

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/serialhub .
COPY --from=builder /app/config.example.toml ./config.toml
EXPOSE 5000
CMD ["./serialhub", "-c", "./config.toml"]
```

构建并运行：

```bash
docker build -t serialhub .
docker run -d --name serialhub --device=/dev/ttyUSB0 -p 5000:5000 serialhub
```

## 配置

### 配置文件示例

```toml
# SerialHub 配置文件

[serial]
port = "COM9"           # 串口名（Windows: COM9, Linux: /dev/ttyUSB0）
baudRate = 115200       # 波特率
dataBits = 8            # 数据位（5/6/7/8）
parity = "none"         # 校验位（none/even/odd）
stopBits = 1            # 停止位（1/2）

[mcp]
httpPort = 5000         # MCP HTTP 服务端口

[log]
level = "info"          # 日志级别（debug/info/warn/error）
dir = "./logs"          # 日志目录
```

### 环境变量

| 变量名 | 说明 | 示例 |
|--------|------|------|
| `SERIALHUB_LOG_DIR` | 日志目录 | `D:\Logs` 或 `/var/log/serialhub` |

## 服务管理

### Windows 服务

```powershell
# 查看服务状态
Get-Service SerialHub

# 启动服务
Start-Service SerialHub

# 停止服务
Stop-Service SerialHub

# 重启服务
Restart-Service SerialHub

# 删除服务
sc delete SerialHub
```

### Linux systemd

```bash
# 查看状态
sudo systemctl status serialhub

# 启动服务
sudo systemctl start serialhub

# 停止服务
sudo systemctl stop serialhub

# 重启服务
sudo systemctl restart serialhub

# 查看日志
sudo journalctl -u serialhub -f

# 启用开机自启
sudo systemctl enable serialhub

# 禁用开机自启
sudo systemctl disable serialhub
```

## 故障排除

### 串口权限问题（Linux）

```bash
# 将用户添加到 dialout 组
sudo usermod -a -G dialout $USER

# 重新登录或执行
newgrp dialout
```

### 端口被占用

```bash
# 查看端口占用（Linux）
sudo lsof -i :5000

# 查看端口占用（Windows）
netstat -ano | findstr :5000

# 杀掉进程（Windows）
taskkill /PID <PID> /F
```

### 服务启动失败

1. 检查配置文件路径是否正确
2. 检查日志文件：`logs/serialhub.log`
3. 检查串口是否存在且有权限访问
4. 尝试前台运行查看错误信息：
   ```bash
   ./serialhub -c config.toml --no-tray
   ```

### 更新版本

```bash
# 1. 停止服务
sudo systemctl stop serialhub

# 2. 备份配置
cp /opt/serialhub/config.toml ~/config.toml.bak

# 3. 重新部署
sudo ./scripts/deploy.sh --service

# 4. 恢复配置
sudo cp ~/config.toml.bak /opt/serialhub/config.toml
sudo systemctl restart serialhub
```

## 安全建议

1. **防火墙配置**: 限制 5000 端口访问
   ```bash
   # Linux (ufw)
   sudo ufw allow from 192.168.1.0/24 to any port 5000
   
   # Windows Firewall
   New-NetFirewallRule -DisplayName "SerialHub" -Direction Inbound -LocalPort 5000 -Protocol TCP -RemoteAddress 192.168.1.0/24 -Action Allow
   ```

2. **绑定地址**: 默认只绑定 127.0.0.1，如需外部访问使用 `--host 0.0.0.0`

3. **配置文件权限**: 确保配置文件只有管理员可读写
   ```bash
   chmod 600 /opt/serialhub/config.toml
   ```

## 更多文档

- [MCP 使用指南](./MCP.md) - AI 工具集成说明
- [项目架构](./AGENTS.md) - 代码结构和开发指南
- [README](./README.md) - 项目介绍和快速开始
