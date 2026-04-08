#!/usr/bin/env pwsh
# SerialHub 发布构建脚本 (Windows)
# 用法: .\build-release.ps1 [-Version <版本号>] [-OutputDir <输出目录>]
# 示例:
#   .\build-release.ps1                    # 使用代码中的版本号
#   .\build-release.ps1 -Version "0.2.0"   # 指定版本号
#   .\build-release.ps1 -OutputDir "dist"  # 指定输出目录

param(
    [string]$Version = "",
    [string]$OutputDir = "dist",
    [switch]$SkipVet = $false
)

$ErrorActionPreference = "Stop"

# 颜色定义
$Colors = @{
    Cyan = "Cyan"
    Yellow = "Yellow"
    Green = "Green"
    Red = "Red"
    Gray = "Gray"
    White = "White"
}

# 从代码中获取基础版本号
function Get-BaseVersion {
    $mainFile = Join-Path $PSScriptRoot "..\cmd\serialhub\main.go"
    if (Test-Path $mainFile) {
        $content = Get-Content $mainFile -Raw
        if ($content -match 'const\s+baseVersion\s*=\s*"([^"]+)"') {
            return $matches[1]
        }
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

Write-Host "========================================" -ForegroundColor $Colors.Cyan
Write-Host "SerialHub 发布构建脚本" -ForegroundColor $Colors.Cyan
Write-Host "版本: $Version" -ForegroundColor $Colors.Cyan
Write-Host "========================================" -ForegroundColor $Colors.Cyan

# 获取项目根目录
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$OriginalLocation = Get-Location

# 确保脚本退出时返回原目录
trap {
    Set-Location $OriginalLocation
    Write-Host "`n脚本异常退出，已返回原目录" -ForegroundColor Yellow
    break
}

Set-Location $ProjectRoot

# 创建输出目录
$DistDir = Join-Path $ProjectRoot $OutputDir
$BuildDir = Join-Path $DistDir "serialhub-$Version-windows-amd64"

$step = 1
$totalSteps = 5

# 步骤 1: 代码检查
Write-Host "`n[$step/$totalSteps] 代码检查..." -ForegroundColor $Colors.Yellow
if (-not $SkipVet) {
    Write-Host "  运行 go vet..." -ForegroundColor $Colors.Gray
    go vet ./...
    if ($LASTEXITCODE -ne 0) {
        Write-Error "go vet 检查失败，请修复错误后再构建"
        exit 1
    }
    Write-Host "  go vet 通过" -ForegroundColor $Colors.Green
} else {
    Write-Host "  跳过 go vet (使用 -SkipVet 参数)" -ForegroundColor $Colors.Gray
}

# 步骤 2: 清理旧构建文件
$step++
Write-Host "`n[$step/$totalSteps] 清理旧构建文件..." -ForegroundColor $Colors.Yellow
Write-Host "  输出目录: $DistDir" -ForegroundColor $Colors.Gray

if (Test-Path $DistDir) {
    Remove-Item -Path "$DistDir\*" -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "  清理输出目录完成" -ForegroundColor $Colors.Gray
}

# 创建新的构建目录
New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null

# 步骤 3: 构建可执行文件
$step++
Write-Host "`n[$step/$totalSteps] 构建可执行文件..." -ForegroundColor $Colors.Yellow
$env:CGO_ENABLED = "0"
$BinDir = Join-Path $BuildDir "bin"

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

# 构建设置
$ldflags = "-s -w -X main.version=$Version"
$buildTime = Get-Date -Format "yyyy-MM-dd_HH:mm:ss"
$gitCommit = (git rev-parse --short HEAD 2>$null) || "unknown"
$ldflags += " -X main.buildTime=$buildTime -X main.gitCommit=$gitCommit"

Write-Host "  构建参数: -ldflags \"$ldflags\"" -ForegroundColor $Colors.Gray
Write-Host "  编译: go build -o bin\serialhub.exe .\cmd\serialhub" -ForegroundColor $Colors.Gray

go build -ldflags "$ldflags" -o "$BinDir\serialhub.exe" .\cmd\serialhub

if ($LASTEXITCODE -ne 0) {
    Write-Error "构建失败"
    exit 1
}

$exeSize = (Get-Item "$BinDir\serialhub.exe").Length / 1KB
Write-Host "  构建成功: bin\serialhub.exe ($([math]::Round($exeSize, 2)) KB)" -ForegroundColor $Colors.Green

# 步骤 4: 复制配置文件和资源
$step++
Write-Host "`n[$step/$totalSteps] 复制配置文件和资源..." -ForegroundColor $Colors.Yellow

$filesToCopy = @(
    @{Source = ".\config.example.toml"; Destination = "$BuildDir\config.toml"; Name = "配置文件"},
    @{Source = ".\start.ps1"; Destination = "$BuildDir\start.ps1"; Name = "启动脚本"},
    @{Source = ".\README.md"; Destination = "$BuildDir\README.md"; Name = "README"},
    @{Source = ".\MCP.md"; Destination = "$BuildDir\MCP.md"; Name = "MCP文档"},
    @{Source = ".\QUICKSTART.md"; Destination = "$BuildDir\QUICKSTART.md"; Name = "快速开始"},
    @{Source = ".\scripts\opencode.json"; Destination = "$BuildDir\opencode.json"; Name = "OpenCode配置"}
)

foreach ($file in $filesToCopy) {
    if (Test-Path $file.Source) {
        Copy-Item -Path $file.Source -Destination $file.Destination -Force
        $fileSize = (Get-Item $file.Destination).Length / 1KB
        Write-Host "  复制: $($file.Name) -> $(Split-Path $file.Destination -Leaf) ($([math]::Round($fileSize, 2)) KB)" -ForegroundColor $Colors.Gray
    } else {
        Write-Warning "  跳过: $($file.Name) (文件不存在)"
    }
}

# 创建版本信息文件
$versionInfo = @"
SerialHub v$Version
Build Time: $buildTime
Git Commit: $gitCommit
Platform: windows-amd64
"@
$versionInfo | Out-File -FilePath "$BuildDir\VERSION" -Encoding UTF8
Write-Host "  创建: VERSION 文件" -ForegroundColor $Colors.Gray

# 步骤 5: 打包发布文件
$step++
Write-Host "`n[$step/$totalSteps] 打包发布文件..." -ForegroundColor $Colors.Yellow
$ZipFile = "$DistDir\serialhub-$Version-windows-amd64.zip"
Write-Host "  输出文件: $ZipFile" -ForegroundColor $Colors.Gray

# 显示打包内容
Write-Host "  打包内容:" -ForegroundColor $Colors.Gray
$items = Get-ChildItem $BuildDir -Recurse
foreach ($item in $items) {
    $relativePath = $item.FullName.Substring($BuildDir.Length + 1)
    if ($item.PSIsContainer) {
        Write-Host "    [DIR]  $relativePath" -ForegroundColor $Colors.Gray
    } else {
        $size = $item.Length / 1KB
        Write-Host "    [FILE] $relativePath ($([math]::Round($size, 2)) KB)" -ForegroundColor $Colors.Gray
    }
}

Compress-Archive -Path "$BuildDir\*" -DestinationPath $ZipFile -Force
$zipSize = (Get-Item $ZipFile).Length / 1MB
Write-Host "  打包完成: serialhub-$Version-windows-amd64.zip ($([math]::Round($zipSize, 2)) MB)" -ForegroundColor $Colors.Green

# 完成信息
Write-Host "`n========================================" -ForegroundColor $Colors.Green
Write-Host "发布构建完成!" -ForegroundColor $Colors.Green
Write-Host "========================================" -ForegroundColor $Colors.Green
Write-Host "版本号:   $Version" -ForegroundColor $Colors.White
Write-Host "构建时间: $buildTime" -ForegroundColor $Colors.White
Write-Host "Git Commit: $gitCommit" -ForegroundColor $Colors.White
Write-Host "输出文件: $ZipFile" -ForegroundColor $Colors.White
Write-Host "文件大小: $([math]::Round($zipSize, 2)) MB" -ForegroundColor $Colors.White

# 计算校验和
$hash = Get-FileHash -Path $ZipFile -Algorithm SHA256
Write-Host "`nSHA256 校验和:" -ForegroundColor $Colors.Gray
Write-Host "  $($hash.Hash)" -ForegroundColor $Colors.Gray
Write-Host "`n校验和命令:" -ForegroundColor $Colors.Gray
Write-Host "  certutil -hashfile \"$ZipFile\" SHA256" -ForegroundColor $Colors.Gray

# 返回原目录
Set-Location $OriginalLocation
