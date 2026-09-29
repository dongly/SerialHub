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
$dlBase = "https://github.com/$repo/releases/download/$tag"

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
    Get-ChildItem (Join-Path $tmp $pkg) -Filter 'serialhub-*.ps1' -ErrorAction SilentlyContinue |
        ForEach-Object { Copy-Item $_.FullName $installDir -Force }
} finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

# Add the installation directory to user PATH if it is not already present.
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$installDir", 'User')
    Write-Host '>> Added to user PATH (open a new terminal to use it)'
}

& (Join-Path $installDir 'serialhub.exe') --version
Write-Host ">> Installation complete: $installDir\serialhub.exe (open a new terminal and run serialhub; configuration and the instance lock use fixed locations)"
