# SerialHub 启动脚本
# 用法: .\start.ps1 [参数]
# 示例: .\start.ps1 -p COM9 -D

param(
    [string]$p = "",      # 串口名
    [int]$b = 115200,     # 波特率
    [string]$c = "",      # 配置文件路径
    [switch]$D,           # 调试模式
    [int]$t = 2323,       # Telnet 端口
    [int]$m = 5000,       # MCP 端口
    [string]$listen = "127.0.0.1",  # 监听地址
    [switch]$NoTray       # 禁用系统托盘
)

$exePath = Join-Path $PSScriptRoot "bin\serialhub.exe"

# 先杀掉已运行的 serialhub 进程
$existingProcs = Get-Process -Name "serialhub" -ErrorAction SilentlyContinue
if ($existingProcs) {
    Write-Host "正在终止已运行的 serialhub 进程..."
    $existingProcs | ForEach-Object { 
        Stop-Process -Id $_.Id -Force
        Write-Host "  已终止 PID: $($_.Id)"
    }
}

if (-not (Test-Path $exePath)) {
    Write-Error "找不到 SerialHub 可执行文件: $exePath"
    Write-Host "请先运行: go build -o bin\serialhub.exe ./cmd/serialhub"
    exit 1
}

# 构建参数数组
$args = @()
if ($p) { $args += "-p"; $args += $p }
if ($b -ne 115200) { $args += "-b"; $args += $b }
if ($c) { $args += "-c"; $args += $c }
if ($D) { $args += "-D" }
if ($t -ne 2323) { $args += "-t"; $args += $t }
if ($m -ne 5000) { $args += "-m"; $args += $m }
if ($listen -ne "127.0.0.1") { $args += "--host"; $args += $listen }
if ($NoTray) { $args += "--no-tray" }

# 使用 Start-Process 启动，-WindowStyle Minimized 最小化窗口
# 这样可以看到日志输出，但不会阻塞 PowerShell
$args += "--minimized"
$proc = Start-Process -FilePath $exePath -ArgumentList $args -WindowStyle Minimized -PassThru

Write-Host "SerialHub 已启动 (PID: $($proc.Id))"
Write-Host "日志文件: $(Join-Path $PSScriptRoot "bin\logs\serialhub.log")"
Write-Host "使用 Stop-Process -Id $($proc.Id) 停止服务"
