# SerialHub launcher script
# Usage: .\sr.ps1 [args]
# Example: .\sr.ps1 -p COM9 -D
# Named sr (not serialhub) so it never shadows serialhub.exe on PATH.

param(
    [string]$p = "",      # serial port name
    [int]$b = 115200,     # baud rate
    [string]$c = "",      # config file path
    [switch]$D,           # debug mode
    [int]$m = 5050,       # MCP port
    [string]$listen = "127.0.0.1"  # listen address
)

# exe layout compatibility: release package ships alongside this script,
# repo dev build lives in bin\
$exePath = Join-Path $PSScriptRoot "serialhub.exe"
if (-not (Test-Path $exePath)) {
    $exePath = Join-Path $PSScriptRoot "bin\serialhub.exe"
}

# Kill any running serialhub process first
$existingProcs = Get-Process -Name "serialhub" -ErrorAction SilentlyContinue
if ($existingProcs) {
    Write-Host "Terminating running serialhub process..."
    $existingProcs | ForEach-Object {
        Stop-Process -Id $_.Id -Force
        Write-Host "  Terminated PID: $($_.Id)"
    }
}

if (-not (Test-Path $exePath)) {
    Write-Error "SerialHub executable not found: $exePath"
    Write-Host "Place serialhub.exe next to this script (or in the bin\ subdirectory), or run: go build -o bin\serialhub.exe ./cmd/serialhub"
    exit 1
}

# Build the argument list
$args = @()
if ($p) { $args += "-p"; $args += $p }
if ($b -ne 115200) { $args += "-b"; $args += $b }
if ($c) { $args += "-c"; $args += $c }
if ($D) { $args += "-D" }
if ($m -ne 5050) { $args += "-m"; $args += $m }
if ($listen -ne "127.0.0.1") { $args += "--host"; $args += $listen }

# Start with a minimized window: logs stay available without blocking PowerShell
$args += "--minimized"
$proc = Start-Process -FilePath $exePath -ArgumentList $args -WindowStyle Minimized -PassThru

Write-Host "SerialHub started (PID: $($proc.Id))"
Write-Host "Log file: $(Join-Path (Split-Path $exePath -Parent) "logs\serialhub.log")"
Write-Host "Stop it with: Stop-Process -Id $($proc.Id)"
