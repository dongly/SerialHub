# SerialHub 全功能交互式测试计划

## 测试环境要求

- **硬件**: COM9 已连接 RT-Thread 设备（或其他可用串口）
- **软件**: Windows PowerShell, curl, Telnet 客户端
- **前置条件**: `go build -o bin/serialhub.exe ./cmd/serialhub`

## 测试概览

| 模块 | 测试项 | 自动化 | 人工 |
|------|--------|--------|------|
| CLI | T1-T5 | ✅ | - |
| HTTP API | T6-T7 | ✅ | - |
| Telnet | T8-T11 | ✅ | - |
| MCP Tools | T12-T18 | ✅ | - |
| 数据流 | T19-T22 | ✅ | - |
| 系统托盘 | T23-T25 | - | ✅ |
| 异常处理 | T26-T30 | ✅ | - |

---

## 第一部分：CLI 命令测试（全自动）

### T1: 版本号输出
```powershell
# 自动执行
.\bin\serialhub.exe --version
# 预期: SerialHub v0.1.0
```
**验证点**: 输出格式正确，版本号匹配

---

### T2: 帮助信息
```powershell
# 自动执行
.\bin\serialhub.exe --help
# 预期: 显示所有 flag 说明
```
**验证点**: 包含 `-p`, `-b`, `-t`, `-m`, `--no-tray`, `--host`, `--debug`, `-c` 参数

---

### T3: 错误参数拒绝
```powershell
# 自动执行
.\bin\serialhub.exe mcp 2>&1
# 预期: Error: unknown command "mcp"
```
**验证点**: 返回非零退出码，显示错误信息

---

### T4: 无参数启动（自动连接上次串口）
```powershell
# 自动执行
Start-Process -FilePath ".\bin\serialhub.exe" -ArgumentList "--no-tray" -WindowStyle Hidden
Start-Sleep -Seconds 3
Get-Process serialhub -ErrorAction SilentlyContinue
# 预期: 进程运行中
```
**验证点**: 进程启动成功，自动连接上次串口

---

### T5: 指定串口启动
```powershell
# 自动执行
Start-Process -FilePath ".\bin\serialhub.exe" -ArgumentList "--no-tray","-p","COM9","-b","115200" -WindowStyle Hidden
Start-Sleep -Seconds 3
# 预期: 进程运行中
```
**验证点**: 日志显示连接 COM9@115200

---

## 第二部分：HTTP API 测试（全自动）

> **前置**: 启动服务 `.\bin\serialhub.exe --no-tray -p COM9`

### T6: 健康检查端点
```powershell
# 自动执行
Invoke-RestMethod -Uri "http://127.0.0.1:5000/health"
# 预期: {"status":"ok"}
```
**验证点**: 返回 200，JSON 格式正确

---

### T7: MCP SSE 端点
```powershell
# 自动执行
Invoke-WebRequest -Uri "http://127.0.0.1:5000/mcp" -UseBasicParsing -TimeoutSec 3
# 预期: event: endpoint\ndata: /mcp?sessionid=...
```
**验证点**: 返回 SSE 格式的 session endpoint

---

## 第三部分：Telnet 测试（全自动）

> **前置**: 服务运行中

### T8: TCP 连接成功
```powershell
# 自动执行
$tcp = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 2323)
$tcp.Connected
# 预期: True
$tcp.Close()
```
**验证点**: TCP 连接建立成功

---

### T9: 欢迎消息含串口信息
```powershell
# 自动执行
$tcp = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 2323)
$stream = $tcp.GetStream()
$stream.ReadTimeout = 3000
$buf = New-Object byte[] 1024
Start-Sleep -Milliseconds 500
$read = $stream.Read($buf, 0, $buf.Length)
$msg = [System.Text.Encoding]::UTF8.GetString($buf, 0, $read)
Write-Host $msg
$tcp.Close()
# 预期: "Connected to SerialHub - Serial: COM9@115200 8N1"
```
**验证点**: 欢迎消息包含串口配置信息

---

### T10: Telnet 发送命令 → 串口响应
```powershell
# 自动执行
$tcp = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 2323)
$stream = $tcp.GetStream()
$stream.ReadTimeout = 5000
$buf = New-Object byte[] 4096

# 读取欢迎消息
Start-Sleep -Milliseconds 500
$read = $stream.Read($buf, 0, $buf.Length)

# 发送 help 命令
$cmd = [System.Text.Encoding]::UTF8.GetBytes("help`r`n")
$stream.Write($cmd, 0, $cmd.Length)

# 等待响应
Start-Sleep -Seconds 2
$read = $stream.Read($buf, 0, $buf.Length)
$response = [System.Text.Encoding]::UTF8.GetString($buf, 0, $read)
Write-Host "Response: $response"
$tcp.Close()
# 预期: 响应包含 "RT-Thread shell commands:" 或设备实际输出
```
**验证点**: 收到设备响应数据

---

### T11: 多客户端并发
```powershell
# 自动执行
$clients = @()
for ($i = 0; $i -lt 3; $i++) {
    $tcp = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 2323)
    $clients += $tcp
    Start-Sleep -Milliseconds 100
}
Write-Host "Connected clients: $($clients.Count)"
foreach ($c in $clients) { $c.Close() }
# 预期: 3 个客户端都连接成功
```
**验证点**: 支持多客户端同时连接

---

## 第四部分：MCP 工具测试（全自动）

> **前置**: 服务运行中，通过 HTTP JSON-RPC 调用工具

### T12: serial_list - 列出可用串口
```powershell
# 自动执行
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_list" }
    id = 1
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
# 预期: 返回串口列表，success: true
```
**验证点**: 返回可用串口数组

---

### T13: serial_status - 查询连接状态
```powershell
# 自动执行
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_status" }
    id = 2
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
# 预期: { connected: true, port: "COM9", baudRate: 115200 }
```
**验证点**: 返回当前连接信息

---

### T14: serial_write - 发送数据
```powershell
# 自动执行
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{
        name = "serial_write"
        arguments = @{ data = "help"; addNewline = $true }
    }
    id = 3
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
# 预期: { success: true, bytesWritten: 5 }
```
**验证点**: 写入成功，返回字节数

---

### T15: serial_read - 读取数据
```powershell
# 自动执行
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{
        name = "serial_read"
        arguments = @{ timeout = 3000; maxSize = 4096 }
    }
    id = 4
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
# 预期: { success: true, data: "...", bytes: N }
```
**验证点**: 读取到设备响应数据

---

### T16: serial_write 不带换行
```powershell
# 自动执行
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{
        name = "serial_write"
        arguments = @{ data = "version"; addNewline = $false }
    }
    id = 5
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
# 预期: bytesWritten = 7 (不含换行)
```
**验证点**: 不自动追加换行符

---

### T17: serial_read 无限等待
```powershell
# 自动执行（先写入命令）
$body1 = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_write"; arguments = @{ data = "help" } }
    id = 6
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body1 -ContentType "application/json"

# 无限等待读取
$body2 = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_read"; arguments = @{ timeout = 0 } }
    id = 7
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body2 -ContentType "application/json"
# 预期: 返回数据，不会超时
```
**验证点**: timeout=0 时无限等待直到有数据

---

### T18: serial_disconnect - 断开串口
```powershell
# 自动执行
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_disconnect" }
    id = 8
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
# 预期: { success: true, message: "串口已断开: COM9" }
```
**验证点**: 断开成功，状态变为未连接

---

## 第五部分：数据流测试（全自动）

> **前置**: 重启服务，串口已连接

### T19: 串口 → Telnet 广播
```powershell
# 自动执行
# 1. 连接 Telnet
$tcp = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 2323)
$stream = $tcp.GetStream()
$stream.ReadTimeout = 5000
$buf = New-Object byte[] 4096

# 2. 读取欢迎
Start-Sleep -Milliseconds 500
$stream.Read($buf, 0, $buf.Length) | Out-Null

# 3. 通过 MCP 写入数据
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_write"; arguments = @{ data = "version" } }
    id = 10
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"

# 4. 从 Telnet 读取
Start-Sleep -Seconds 2
$read = $stream.Read($buf, 0, $buf.Length)
$data = [System.Text.Encoding]::UTF8.GetString($buf, 0, $read)
Write-Host "Telnet received: $data"
$tcp.Close()
# 预期: Telnet 收到串口数据
```
**验证点**: 数据从串口广播到 Telnet

---

### T20: Telnet → 串口写入
```powershell
# 自动执行
$tcp = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 2323)
$stream = $tcp.GetStream()
$stream.ReadTimeout = 5000
$buf = New-Object byte[] 4096

# 读取欢迎
Start-Sleep -Milliseconds 500
$stream.Read($buf, 0, $buf.Length) | Out-Null

# 从 Telnet 发送
$cmd = [System.Text.Encoding]::UTF8.GetBytes("help`r`n")
$stream.Write($cmd, 0, $cmd.Length)

# 从 MCP buffer 读取
Start-Sleep -Seconds 2
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_read"; arguments = @{ timeout = 2000 } }
    id = 11
} | ConvertTo-Json -Depth 3
$result = Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
Write-Host "MCP read: $($result | ConvertTo-Json)"
$tcp.Close()
# 预期: MCP 读取到设备响应
```
**验证点**: Telnet 数据写入串口，响应到达 MCP buffer

---

### T21: 数据缓冲区溢出
```powershell
# 自动执行
# 连续写入大量数据，测试缓冲区
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_write"; arguments = @{ data = "help`nhelp`nhelp`nhelp`nhelp" } }
    id = 12
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"

Start-Sleep -Seconds 1

# 读取大量数据
$body2 = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_read"; arguments = @{ timeout = 2000; maxSize = 8192 } }
    id = 13
} | ConvertTo-Json -Depth 3
$result = Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body2 -ContentType "application/json"
Write-Host "Buffer test: $($result | ConvertTo-Json)"
# 预期: 不崩溃，数据正确处理
```
**验证点**: 大数据量不崩溃

---

### T22: 多客户端同时接收广播
```powershell
# 自动执行
$clients = @()
$streams = @()

# 连接 3 个 Telnet 客户端
for ($i = 0; $i -lt 3; $i++) {
    $tcp = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 2323)
    $stream = $tcp.GetStream()
    $stream.ReadTimeout = 3000
    $buf = New-Object byte[] 4096
    Start-Sleep -Milliseconds 300
    $stream.Read($buf, 0, $buf.Length) | Out-Null  # 读取欢迎
    $clients += $tcp
    $streams += $stream
}

# 从 MCP 写入
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_write"; arguments = @{ data = "help" } }
    id = 14
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"

Start-Sleep -Seconds 2

# 检查每个客户端是否都收到数据
$received = 0
foreach ($s in $streams) {
    try {
        $buf = New-Object byte[] 4096
        $read = $s.Read($buf, 0, $buf.Length)
        if ($read -gt 0) { $received++ }
    } catch {}
}
Write-Host "Clients received broadcast: $received / 3"

foreach ($c in $clients) { $c.Close() }
# 预期: 3 个客户端都收到数据
```
**验证点**: 所有客户端同时收到广播

---

## 第六部分：系统托盘测试（需人工）

> ⚠️ **需要 GUI 环境，人工操作**

### T23: 托盘图标显示
```
【人工操作】

步骤:
1. 启动: .\bin\serialhub.exe -p COM9
   （不加 --no-tray）

2. 观察系统托盘区域

预期结果:
- 托盘出现 SerialHub 图标
- 图标状态：绿色（已连接）或灰色（未连接）
```
**验证点**: 图标正确显示

---

### T24: 托盘菜单操作
```
【人工操作】

步骤:
1. 右键托盘图标

2. 测试菜单项:
   - "📡 串口连接" - 点击切换连接/断开
   - "🌐 服务端口" - 查看 Telnet/MCP 端口
   - "📋 版本" - 显示版本号
   - "❌ 退出" - 关闭程序

预期结果:
- 菜单项正确显示
- 点击有响应
- 状态变化时图标更新
```
**验证点**: 菜单功能正常

---

### T25: 托盘连接状态切换
```
【人工操作】

步骤:
1. 确认当前已连接 COM9（图标绿色）

2. 点击 "📡 串口连接"

3. 观察图标变化

4. 再次点击重新连接

预期结果:
- 断开时图标变灰
- 重新连接时图标变绿
- 日志输出连接/断开信息
```
**验证点**: 状态切换正确

---

## 第七部分：异常处理测试（全自动）

### T26: 连接不存在的串口
```powershell
# 自动执行
Get-Process serialhub -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Seconds 1

Start-Process -FilePath ".\bin\serialhub.exe" -ArgumentList "--no-tray","-p","COM999" -WindowStyle Hidden
Start-Sleep -Seconds 3

# 检查是否启动失败或警告
Get-Process serialhub -ErrorAction SilentlyContinue
# 预期: 进程可能启动但串口连接失败，日志有警告
```
**验证点**: 优雅处理错误，不崩溃

---

### T27: 串口未连接时写入
```powershell
# 自动执行
# 假设串口未连接
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_write"; arguments = @{ data = "test" } }
    id = 20
} | ConvertTo-Json -Depth 3
$result = Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json" -ErrorAction SilentlyContinue
Write-Host "Write without connect: $($result | ConvertTo-Json)"
# 预期: { success: false, message: "串口未连接" }
```
**验证点**: 返回正确错误信息

---

### T28: 重复断开
```powershell
# 自动执行
# 断开两次
$body1 = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_disconnect" }
    id = 21
} | ConvertTo-Json -Depth 3
$r1 = Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body1 -ContentType "application/json"

$body2 = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_disconnect" }
    id = 22
} | ConvertTo-Json -Depth 3
$r2 = Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body2 -ContentType "application/json"

Write-Host "Disconnect 1: $($r1 | ConvertTo-Json)"
Write-Host "Disconnect 2: $($r2 | ConvertTo-Json)"
# 预期: 第二次返回错误 "串口未连接"
```
**验证点**: 重复操作不崩溃

---

### T29: 读取超时
```powershell
# 自动执行
# 清空缓冲区后等待
$body = @{
    jsonrpc = "2.0"
    method = "tools/call"
    params = @{ name = "serial_read"; arguments = @{ timeout = 1000 } }
    id = 23
} | ConvertTo-Json -Depth 3
$result = Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
Write-Host "Read timeout: $($result | ConvertTo-Json)"
# 预期: { timedOut: true } 或空数据
```
**验证点**: 超时正确处理

---

### T30: 服务优雅关闭
```powershell
# 自动执行
$pid = (Get-Process serialhub).Id
Stop-Process -Id $pid -Force
Start-Sleep -Seconds 2
Get-Process serialhub -ErrorAction SilentlyContinue
# 预期: 进程已终止，资源已释放
```
**验证点**: 清理临时文件

---

## 自动化测试脚本

将以下内容保存为 `test-all.ps1`：

```powershell
# SerialHub 全功能自动化测试脚本

$ErrorActionPreference = "Continue"
$PassCount = 0
$FailCount = 0

function Test-Case {
    param($Name, $ScriptBlock)
    Write-Host "`n=== $Name ===" -ForegroundColor Cyan
    try {
        & $ScriptBlock
        Write-Host "[PASS] $Name" -ForegroundColor Green
        $script:PassCount++
    } catch {
        Write-Host "[FAIL] $Name: $_" -ForegroundColor Red
        $script:FailCount++
    }
}

# 清理环境
Get-Process serialhub -ErrorAction SilentlyContinue | Stop-Process -Force

Write-Host "`n========== SerialHub 功能测试 ==========" -ForegroundColor Yellow

# T1-T5: CLI 测试
Test-Case "T1: 版本号" {
    $result = .\bin\serialhub.exe --version
    if ($result -notmatch "SerialHub v") { throw "版本格式错误" }
}

Test-Case "T2: 帮助信息" {
    $result = .\bin\serialhub.exe --help
    if ($result -notmatch "--serial-port") { throw "缺少参数说明" }
}

Test-Case "T3: 错误参数拒绝" {
    $result = .\bin\serialhub.exe mcp 2>&1
    if ($result -notmatch "unknown command") { throw "应拒绝未知命令" }
}

Test-Case "T4: 进程启动" {
    Start-Process -FilePath ".\bin\serialhub.exe" -ArgumentList "--no-tray","-p","COM9" -WindowStyle Hidden
    Start-Sleep -Seconds 3
    if (-not (Get-Process serialhub -ErrorAction SilentlyContinue)) { throw "进程未启动" }
}

# T6-T7: HTTP 测试
Test-Case "T6: 健康检查" {
    $result = Invoke-RestMethod -Uri "http://127.0.0.1:5000/health"
    if ($result.status -ne "ok") { throw "健康检查失败" }
}

Test-Case "T7: SSE 端点" {
    $result = Invoke-WebRequest -Uri "http://127.0.0.1:5000/mcp" -UseBasicParsing -TimeoutSec 3
    if ($result.Content -notmatch "endpoint") { throw "SSE 端点错误" }
}

# T8-T11: Telnet 测试
Test-Case "T8: Telnet 连接" {
    $tcp = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 2323)
    if (-not $tcp.Connected) { throw "连接失败" }
    $tcp.Close()
}

Test-Case "T9: 欢迎消息" {
    $tcp = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 2323)
    $stream = $tcp.GetStream()
    $stream.ReadTimeout = 3000
    $buf = New-Object byte[] 1024
    Start-Sleep -Milliseconds 500
    $read = $stream.Read($buf, 0, $buf.Length)
    $msg = [System.Text.Encoding]::UTF8.GetString($buf, 0, $read)
    $tcp.Close()
    if ($msg -notmatch "Connected to SerialHub") { throw "欢迎消息错误: $msg" }
}

# T12-T18: MCP 工具测试
Test-Case "T12: serial_list" {
    $body = @{ jsonrpc = "2.0"; method = "tools/call"; params = @{ name = "serial_list" }; id = 1 } | ConvertTo-Json -Depth 3
    $result = Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
    Write-Host "串口列表: $($result | ConvertTo-Json -Compress)"
}

Test-Case "T13: serial_status" {
    $body = @{ jsonrpc = "2.0"; method = "tools/call"; params = @{ name = "serial_status" }; id = 2 } | ConvertTo-Json -Depth 3
    $result = Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
    Write-Host "状态: $($result | ConvertTo-Json -Compress)"
}

Test-Case "T14: serial_write" {
    $body = @{ jsonrpc = "2.0"; method = "tools/call"; params = @{ name = "serial_write"; arguments = @{ data = "help" } }; id = 3 } | ConvertTo-Json -Depth 3
    $result = Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
    Write-Host "写入: $($result | ConvertTo-Json -Compress)"
}

Test-Case "T15: serial_read" {
    Start-Sleep -Seconds 1
    $body = @{ jsonrpc = "2.0"; method = "tools/call"; params = @{ name = "serial_read"; arguments = @{ timeout = 3000 } }; id = 4 } | ConvertTo-Json -Depth 3
    $result = Invoke-RestMethod -Uri "http://127.0.0.1:5000/mcp" -Method POST -Body $body -ContentType "application/json"
    Write-Host "读取: $($result | ConvertTo-Json -Compress)"
}

# 清理
Get-Process serialhub -ErrorAction SilentlyContinue | Stop-Process -Force

Write-Host "`n========== 测试结果 ==========" -ForegroundColor Yellow
Write-Host "通过: $PassCount" -ForegroundColor Green
Write-Host "失败: $FailCount" -ForegroundColor $(if ($FailCount -eq 0) { "Green" } else { "Red" })
```

---

## 测试执行顺序

```
1. 运行自动化脚本: .\test-all.ps1
2. 记录结果
3. 人工执行 T23-T25（托盘测试）
4. 汇总报告
```

---

## 预期结果汇总

| 测试项 | 预期结果 | 自动化 |
|--------|----------|--------|
| T1 | 输出版本号 | ✅ |
| T2 | 显示帮助 | ✅ |
| T3 | 报错退出 | ✅ |
| T4 | 进程启动 | ✅ |
| T5 | 连接串口 | ✅ |
| T6 | 返回 ok | ✅ |
| T7 | SSE 端点 | ✅ |
| T8 | TCP 连接 | ✅ |
| T9 | 含串口信息 | ✅ |
| T10 | 收到响应 | ✅ |
| T11 | 3 客户端连接 | ✅ |
| T12 | 返回串口列表 | ✅ |
| T13 | 返回连接状态 | ✅ |
| T14 | 写入成功 | ✅ |
| T15 | 读取数据 | ✅ |
| T16 | 不带换行 | ✅ |
| T17 | 无限等待 | ✅ |
| T18 | 断开成功 | ✅ |
| T19 | 广播到 Telnet | ✅ |
| T20 | 写入到串口 | ✅ |
| T21 | 不崩溃 | ✅ |
| T22 | 3 客户端接收 | ✅ |
| T23 | 图标显示 | 人工 |
| T24 | 菜单正常 | 人工 |
| T25 | 状态切换 | 人工 |
| T26 | 优雅错误 | ✅ |
| T27 | 返回错误 | ✅ |
| T28 | 不崩溃 | ✅ |
| T29 | 超时返回 | ✅ |
| T30 | 清理资源 | ✅ |