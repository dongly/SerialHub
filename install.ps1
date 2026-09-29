# SerialHub 在线安装脚本（Windows x64）
# 用法: irm https://raw.githubusercontent.com/dongly/serialhub/main/install.ps1 | iex
# 可用环境变量:
#   SERIALHUB_GITHUB_API   GitHub API 基址（默认 https://api.github.com，私有加速用）
#   SERIALHUB_INSTALL_DIR  安装目录（默认 %LOCALAPPDATA%\Programs\serialhub）
$ErrorActionPreference = 'Stop'

$repo = 'dongly/serialhub'
$apiBase = if ($env:SERIALHUB_GITHUB_API) { $env:SERIALHUB_GITHUB_API } else { 'https://api.github.com' }
$installDir = if ($env:SERIALHUB_INSTALL_DIR) { $env:SERIALHUB_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\serialhub" }

# PowerShell 5.1 默认不开 TLS 1.2
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch {}

try {
    Write-Host '>> 查询最新版本...'
    $rel = Invoke-RestMethod -Uri "$apiBase/repos/$repo/releases/latest"
    $tag = $rel.tag_name
    Write-Host ">> 最新版本: $tag"
} catch {
    throw "无法获取最新版本: $_（网络受限可设 HTTPS_PROXY，或用 SERIALHUB_GITHUB_API 指定加速基址）"
}
$ver = $tag.TrimStart('v')
$pkg = "serialhub-$ver-windows-amd64"
$dlBase = "https://github.com/$repo/releases/download/$tag"

# 运行中的实例会锁住 exe，先提示退出
$running = Get-Process serialhub -ErrorAction SilentlyContinue
if ($running) {
    throw "检测到正在运行的 SerialHub（pid $($running.Id -join ', ')），请先退出（托盘右键退出或 Stop-Process）再安装"
}

$tmp = Join-Path $env:TEMP "serialhub-install-$ver"
if (Test-Path $tmp) { Remove-Item $tmp -Recurse -Force }
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    Write-Host ">> 下载 $pkg.zip ..."
    $zip = Join-Path $tmp "$pkg.zip"
    Invoke-WebRequest -UseBasicParsing -Uri "$dlBase/$pkg.zip" -OutFile $zip

    Write-Host '>> 校验 sha256 ...'
    $sumText = (Invoke-RestMethod -Uri "$dlBase/$pkg.zip.sha256").ToString()
    $expected = ($sumText.Trim() -split '\s+')[0].ToLower()
    $actual = (Get-FileHash -Path $zip -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) { throw "sha256 校验失败（期望 $expected，实际 $actual）" }

    Write-Host ">> 解压到 $installDir ..."
    Expand-Archive -Path $zip -DestinationPath $tmp
    $bin = Join-Path $tmp "$pkg\serialhub.exe"
    if (-not (Test-Path $bin)) { throw "压缩包内未找到 serialhub.exe" }
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    Copy-Item $bin (Join-Path $installDir 'serialhub.exe') -Force
    # 启动脚本（如包内有）一并安装
    Get-ChildItem (Join-Path $tmp $pkg) -Filter 'serialhub-*.ps1' -ErrorAction SilentlyContinue |
        ForEach-Object { Copy-Item $_.FullName $installDir -Force }
} finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

# 用户 PATH 追加（已含则跳过）
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$installDir", 'User')
    Write-Host '>> 已加入用户 PATH（新开终端生效）'
}

& (Join-Path $installDir 'serialhub.exe') --version
Write-Host ">> 安装完成: $installDir\serialhub.exe（新开终端运行 serialhub；配置文件与实例锁在固定位置，与 exe 位置无关）"
