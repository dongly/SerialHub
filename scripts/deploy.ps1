#!/usr/bin/env pwsh
# SerialHub 部署脚本 (Windows)
# 用法: .\deploy.ps1 [-InstallDir <路径>] [-Port <端口>] [-SerialPort <串口>]

param(
    [string]$InstallDir = "$env:ProgramFiles\SerialHub",
    [int]$Port = 5000,
    [string]$SerialPort = "",
    [switch]$CreateService,
    [switch]$StartAfterInstall
)

$ErrorActionPreference = "Stop"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "SerialHub 部署脚本" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

# 检查管理员权限
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole] "Administrator")
if (-not $isAdmin) {
    Write-Warning "建议以管理员权限运行此脚本，以便安装到系统目录或创建服务"
}

# 获取项目根目录
$ProjectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $ProjectRoot

Write-Host "`n[1/5] 检查构建文件..." -ForegroundColor Yellow
if (-not (Test-Path ".\bin\serialhub.exe")) {
    Write-Host "未找到构建文件，先执行构建..." -ForegroundColor Yellow
    go build -o .\bin\serialhub.exe .\cmd\serialhub
    if ($LASTEXITCODE -ne 0) {
        Write-Error "构建失败"
        exit 1
    }
}

Write-Host "`n[2/5] 创建安装目录..." -ForegroundColor Yellow
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path "$InstallDir\logs" | Out-Null

Write-Host "`n[3/5] 复制文件..." -ForegroundColor Yellow
Copy-Item -Path ".\bin\serialhub.exe" -Destination "$InstallDir\" -Force
Copy-Item -Path ".\config.example.toml" -Destination "$InstallDir\config.toml" -Force
Copy-Item -Path ".\scripts\start.ps1" -Destination "$InstallDir\" -Force
Copy-Item -Path ".\README.md" -Destination "$InstallDir\" -Force
Copy-Item -Path ".\DEPLOY.md" -Destination "$InstallDir\" -Force
Copy-Item -Path ".\MCP.md" -Destination "$InstallDir\" -Force

Write-Host "`n[4/5] 生成配置文件..." -ForegroundColor Yellow
$configContent = @"
# SerialHub 配置文件
# 生成时间: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')

[serial]
port = "$SerialPort"
baudRate = 115200
dataBits = 8
parity = "none"
stopBits = 1

[mcp]
httpPort = $Port

[log]
level = "info"
dir = "$InstallDir\logs"
"@

$configContent | Out-File -FilePath "$InstallDir\config.toml" -Encoding UTF8

Write-Host "`n[5/5] 配置环境..." -ForegroundColor Yellow

# 添加到 PATH（用户级）
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($userPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$userPath;$InstallDir", "User")
    Write-Host "已添加到用户 PATH: $InstallDir" -ForegroundColor Green
}

# 创建服务
if ($CreateService -and $isAdmin) {
    Write-Host "创建 Windows 服务..." -ForegroundColor Yellow
    $serviceName = "SerialHub"
    $serviceExists = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if ($serviceExists) {
        Write-Host "服务已存在，先删除..." -ForegroundColor Yellow
        sc delete $serviceName | Out-Null
        Start-Sleep -Seconds 2
    }
    
    $binPath = "$InstallDir\serialhub.exe -c $InstallDir\config.toml"
    sc create $serviceName binPath= $binPath start= auto | Out-Null
    Write-Host "服务已创建: $serviceName" -ForegroundColor Green
    
    if ($StartAfterInstall) {
        Start-Service -Name $serviceName
        Write-Host "服务已启动" -ForegroundColor Green
    }
} elseif ($StartAfterInstall) {
    Write-Host "启动 SerialHub..." -ForegroundColor Yellow
    Start-Process -FilePath "$InstallDir\serialhub.exe" -ArgumentList "-c", "$InstallDir\config.toml" -WindowStyle Hidden
    Write-Host "SerialHub 已启动" -ForegroundColor Green
}

Write-Host "`n========================================" -ForegroundColor Green
Write-Host "部署完成!" -ForegroundColor Green
Write-Host "安装目录: $InstallDir" -ForegroundColor Green
Write-Host "配置文件: $InstallDir\config.toml" -ForegroundColor Green
Write-Host "日志目录: $InstallDir\logs" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green

Write-Host "`n使用说明:" -ForegroundColor Cyan
Write-Host "  1. 编辑配置文件: $InstallDir\config.toml" -ForegroundColor White
Write-Host "  2. 手动启动: $InstallDir\serialhub.exe -c $InstallDir\config.toml" -ForegroundColor White
Write-Host "  3. 或使用启动脚本: $InstallDir\start.ps1" -ForegroundColor White
Write-Host "  4. Web 终端: http://localhost:$Port/terminal" -ForegroundColor White
Write-Host "  5. MCP 服务: http://localhost:$Port/mcp" -ForegroundColor White

if ($CreateService) {
    Write-Host "`n服务命令:" -ForegroundColor Cyan
    Write-Host "  启动: Start-Service SerialHub" -ForegroundColor White
    Write-Host "  停止: Stop-Service SerialHub" -ForegroundColor White
    Write-Host "  重启: Restart-Service SerialHub" -ForegroundColor White
}

Set-Location $PSScriptRoot
