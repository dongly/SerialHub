#!/usr/bin/env pwsh
# SerialHub 发布构建脚本 (Windows)
# 用法: .\build-release.ps1 [-Version <版本号>]

param(
    [string]$Version = "",
    [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"

# 从代码中获取基础版本号
function Get-BaseVersion {
    $mainFile = Join-Path $PSScriptRoot "..\cmd\serialhub\main.go"
    $content = Get-Content $mainFile -Raw
    if ($content -match 'const\s+baseVersion\s*=\s*"([^"]+)"') {
        return $matches[1]
    }
    return "0.1.0"
}

# 获取版本号: 手动指定 > 代码中的 baseVersion
$baseVersion = Get-BaseVersion
if (-not $Version) {
    $Version = $baseVersion
}

# 移除版本号前缀 v（如果有）
$Version = $Version -replace '^v', ''

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "SerialHub 发布构建脚本" -ForegroundColor Cyan
Write-Host "版本: $Version" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

# 获取项目根目录
$ProjectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $ProjectRoot

# 创建输出目录
$DistDir = Join-Path $ProjectRoot $OutputDir
$BuildDir = Join-Path $DistDir "serialhub-$Version-windows-amd64"
New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null

Write-Host "`n[1/5] 清理旧构建文件..." -ForegroundColor Yellow
Remove-Item -Path "$DistDir\*.zip" -ErrorAction SilentlyContinue

Write-Host "`n[2/5] 运行测试..." -ForegroundColor Yellow
$testResult = go test ./... 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Error "测试失败，停止构建"
    exit 1
}
Write-Host "测试通过!" -ForegroundColor Green

Write-Host "`n[3/5] 构建可执行文件..." -ForegroundColor Yellow
$env:CGO_ENABLED = "0"
go build -ldflags "-s -w" -o "$BuildDir\serialhub.exe" .\cmd\serialhub

if ($LASTEXITCODE -ne 0) {
    Write-Error "构建失败"
    exit 1
}
Write-Host "构建成功: $BuildDir\serialhub.exe" -ForegroundColor Green

Write-Host "`n[4/5] 复制配置文件和资源..." -ForegroundColor Yellow
# 复制配置文件模板
Copy-Item -Path ".\config.example.toml" -Destination "$BuildDir\config.toml" -ErrorAction SilentlyContinue
# 复制启动脚本
Copy-Item -Path ".\start.ps1" -Destination "$BuildDir\" -ErrorAction SilentlyContinue
# 复制文档
Copy-Item -Path ".\README.md" -Destination "$BuildDir\" -ErrorAction SilentlyContinue
Copy-Item -Path ".\MCP.md" -Destination "$BuildDir\" -ErrorAction SilentlyContinue

Write-Host "`n[5/5] 打包发布文件..." -ForegroundColor Yellow
$ZipFile = "$DistDir\serialhub-$Version-windows-amd64.zip"
Compress-Archive -Path "$BuildDir\*" -DestinationPath $ZipFile -Force

Write-Host "`n========================================" -ForegroundColor Green
Write-Host "发布构建完成!" -ForegroundColor Green
Write-Host "输出文件: $ZipFile" -ForegroundColor Green
Write-Host "文件大小: $([math]::Round((Get-Item $ZipFile).Length / 1MB, 2)) MB" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green

# 计算校验和
$hash = Get-FileHash -Path $ZipFile -Algorithm SHA256
Write-Host "`nSHA256: $($hash.Hash)" -ForegroundColor Gray

Set-Location $PSScriptRoot
