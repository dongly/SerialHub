$ErrorActionPreference = "Stop"
Set-Location -Path "D:\Develop\SerialHub"

Write-Host "=========================================="
Write-Host "  SerialHub Full E2E Test"
Write-Host "  $(Get-Date)"
Write-Host "=========================================="

npx tsx test-full.ts