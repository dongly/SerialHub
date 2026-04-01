# SerialHub GUI Test Runner
$ErrorActionPreference = "Stop"

Set-Location -Path "D:\Develop\SerialHub"

Write-Host "=========================================="
Write-Host "  SerialHub GUI Test"
Write-Host "  $(Get-Date)"
Write-Host "=========================================="
Write-Host ""

# Clean up old screenshots
if (Test-Path "test-screenshots") {
    Write-Host "Cleaning old screenshots..."
    Remove-Item -Path "test-screenshots" -Recurse -Force
}
New-Item -ItemType Directory -Path "test-screenshots" | Out-Null

Write-Host "Running test..."
npx tsx test-gui.ts

Write-Host ""
Write-Host "=========================================="
Write-Host "  Test completed"
Write-Host "=========================================="
Write-Host ""
Write-Host "Screenshots generated:"
Get-ChildItem -Path "test-screenshots" -Name

Read-Host -Prompt "Press Enter to exit"
