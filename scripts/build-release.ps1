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

Write-Host "`n[1/4] 清理旧构建文件..." -ForegroundColor Yellow
Write-Host "  输出目录: $DistDir"

# 清理整个 $DistDir 目录
if (Test-Path $DistDir) {
    Remove-Item -Path "$DistDir\*" -Recurse -Force
    Write-Host "  清理输出目录完成" -ForegroundColor Gray
}

# 创建新的构建目录
New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null

Write-Host "`n[2/4] 构建可执行文件..." -ForegroundColor Yellow
$env:CGO_ENABLED = "0"
$BinDir = Join-Path $BuildDir "bin"

# 清理并创建 bin 目录
if (Test-Path $BinDir) {
    Remove-Item -Path "$BinDir\*" -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "  清理旧构建文件" -ForegroundColor Gray
}
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

Write-Host "  构建目录: $BinDir"
Write-Host "  编译: go build -ldflags \"-s -w\" -o bin\serialhub.exe .\cmd\serialhub"
go build -ldflags "-s -w" -o "$BinDir\serialhub.exe" .\cmd\serialhub

if ($LASTEXITCODE -ne 0) {
    Write-Error "构建失败"
    exit 1
}
$exeSize = (Get-Item "$BinDir\serialhub.exe").Length / 1KB
Write-Host "  构建成功: bin\serialhub.exe ($([math]::Round($exeSize, 2)) KB)" -ForegroundColor Green

Write-Host "`n[3/4] 复制配置文件和资源..." -ForegroundColor Yellow
$filesToCopy = @(
    @{Source = ".\config.example.toml"; Destination = "$BuildDir\config.toml"; Name = "配置文件"},
    @{Source = ".\start.ps1"; Destination = "$BuildDir\start.ps1"; Name = "启动脚本"},
    @{Source = ".\README.md"; Destination = "$BuildDir\README.md"; Name = "README"},
    @{Source = ".\MCP.md"; Destination = "$BuildDir\MCP.md"; Name = "MCP文档"}
)

foreach ($file in $filesToCopy) {
    if (Test-Path $file.Source) {
        Copy-Item -Path $file.Source -Destination $file.Destination -Force
        $fileSize = (Get-Item $file.Destination).Length / 1KB
        Write-Host "  复制: $($file.Name) -> $(Split-Path $file.Destination -Leaf) ($([math]::Round($fileSize, 2)) KB)"
    } else {
        Write-Warning "  跳过: $($file.Name) (文件不存在)"
    }
}

Write-Host "`n[4/4] 打包发布文件..." -ForegroundColor Yellow
$ZipFile = "$DistDir\serialhub-$Version-windows-amd64.zip"
Write-Host "  输出文件: $ZipFile"

# 显示打包内容
Write-Host "  打包内容:"
$items = Get-ChildItem $BuildDir -Recurse
foreach ($item in $items) {
    $relativePath = $item.FullName.Substring($BuildDir.Length + 1)
    if ($item.PSIsContainer) {
        Write-Host "    [DIR]  $relativePath"
    } else {
        $size = $item.Length / 1KB
        Write-Host "    [FILE] $relativePath ($([math]::Round($size, 2)) KB)"
    }
}

Compress-Archive -Path "$BuildDir\*" -DestinationPath $ZipFile -Force
$zipSize = (Get-Item $ZipFile).Length / 1MB
Write-Host "  打包完成: serialhub-$Version-windows-amd64.zip ($([math]::Round($zipSize, 2)) MB)" -ForegroundColor Green

Write-Host "`n========================================" -ForegroundColor Green
Write-Host "发布构建完成!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "输出文件: $ZipFile" -ForegroundColor White
Write-Host "文件大小: $([math]::Round($zipSize, 2)) MB" -ForegroundColor White
Write-Host "版本号:   $Version" -ForegroundColor White
Write-Host "构建目录: $BuildDir" -ForegroundColor White

# 计算校验和
$hash = Get-FileHash -Path $ZipFile -Algorithm SHA256
Write-Host "`nSHA256 校验和:" -ForegroundColor Gray
Write-Host "  $($hash.Hash)" -ForegroundColor Gray

Set-Location $PSScriptRoot
