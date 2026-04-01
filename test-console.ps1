$ErrorActionPreference = "Stop"

Set-Location -Path "D:\Develop\SerialHub"

Write-Host "Running test with console output..."

if (Test-Path "test-screenshots") {
    Remove-Item -Path "test-screenshots" -Recurse -Force
}
New-Item -ItemType Directory -Path "test-screenshots" | Out-Null

npx tsx test-gui.ts 2>&1 | ForEach-Object { Write-Host $_ }

Write-Host ""
Write-Host "Test completed!"
Write-Host "Screenshots:"
Get-ChildItem -Path "test-screenshots" -Name
