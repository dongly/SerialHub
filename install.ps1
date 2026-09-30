# SerialHub online installer (Windows x64).
# Usage: irm https://raw.githubusercontent.com/dongly/serialhub/main/install.ps1 | iex
# Environment variables:
#   SERIALHUB_GITHUB_API   GitHub API base URL (default: https://api.github.com)
#   SERIALHUB_INSTALL_DIR  Installation directory (default: %LOCALAPPDATA%\Programs\serialhub)
$ErrorActionPreference = 'Stop'

$repo = 'dongly/serialhub'
$apiBase = if ($env:SERIALHUB_GITHUB_API) { $env:SERIALHUB_GITHUB_API } else { 'https://api.github.com' }
$installDir = if ($env:SERIALHUB_INSTALL_DIR) { $env:SERIALHUB_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\serialhub" }

# Enable TLS 1.2 for Windows PowerShell 5.1.
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch {}

try {
    Write-Host '>> Checking the latest release...'
    $rel = Invoke-RestMethod -Uri "$apiBase/repos/$repo/releases/latest"
    $tag = $rel.tag_name
    Write-Host ">> Latest release: $tag"
} catch {
    throw "Cannot fetch the latest release: $_ (try HTTPS_PROXY or SERIALHUB_GITHUB_API)"
}
$ver = $tag.TrimStart('v')
$pkg = "serialhub-$ver-windows-amd64"
# SERIALHUB_DOWNLOAD_BASE lets mirrors/CI redirect asset downloads.
$dlBase = if ($env:SERIALHUB_DOWNLOAD_BASE) { $env:SERIALHUB_DOWNLOAD_BASE } else { "https://github.com/$repo/releases/download" }
$dlBase = "$dlBase/$tag"

# A running process keeps the executable open; stop it before installing.
$running = Get-Process serialhub -ErrorAction SilentlyContinue
if ($running) {
    throw "SerialHub is running (PID $($running.Id -join ', ')); quit via the tray menu or Stop-Process before installing"
}

$tmp = Join-Path $env:TEMP "serialhub-install-$ver"
if (Test-Path $tmp) { Remove-Item $tmp -Recurse -Force }
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    Write-Host ">> Downloading $pkg.zip ..."
    $zip = Join-Path $tmp "$pkg.zip"
    Invoke-WebRequest -UseBasicParsing -Uri "$dlBase/$pkg.zip" -OutFile $zip

    Write-Host '>> Verifying sha256 ...'
    $sumText = (Invoke-RestMethod -Uri "$dlBase/$pkg.zip.sha256").ToString()
    $expected = ($sumText.Trim() -split '\s+')[0].ToLower()
    $actual = (Get-FileHash -Path $zip -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) { throw "sha256 verification failed (expected $expected, got $actual)" }

    Write-Host ">> Extracting to $installDir ..."
    Expand-Archive -Path $zip -DestinationPath $tmp
    $bin = Join-Path $tmp "$pkg\serialhub.exe"
    if (-not (Test-Path $bin)) { throw "Archive does not contain serialhub.exe" }
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    Copy-Item $bin (Join-Path $installDir 'serialhub.exe') -Force
    # Install bundled launcher scripts when present.
    # -Include only matches when the path has a wildcard; the path must end with \*.
    Get-ChildItem (Join-Path $tmp "$pkg\*") -Include 'sr.ps1', 'sr.bat' -File -ErrorAction SilentlyContinue |
        ForEach-Object { Copy-Item $_.FullName $installDir -Force }
    # Remove obsolete legacy launchers that shadow the exe name (older releases shipped them).
    foreach ($legacy in 'serialhub.ps1', 'serialhub.bat') {
        $old = Join-Path $installDir $legacy
        if (Test-Path $old) { Remove-Item $old -Force; Write-Host ">> Removed obsolete launcher: $legacy" }
    }
} finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

# Add the installation directory to user PATH, but only for the default
# location; custom (e.g. sandboxed) installs just get a note, mirroring
# install.sh behaviour and avoiding surprise registry writes.
$defaultDir = "$env:LOCALAPPDATA\Programs\serialhub"
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($installDir -ieq $defaultDir) {
    if ($userPath -notlike "*$installDir*") {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$installDir", 'User')
        Write-Host '>> Added to user PATH (open a new terminal to use it)'
    }
} else {
    Write-Host ">> Note: $installDir is not on your PATH; add it manually"
}

& (Join-Path $installDir 'serialhub.exe') --version
Write-Host ">> Installation complete: $installDir\serialhub.exe (open a new terminal and run serialhub; configuration and the instance lock use fixed locations)"
