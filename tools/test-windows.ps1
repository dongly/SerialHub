# Windows-side runner: executes a Go test binary cross-compiled from WSL.
# Called by tools/test-windows.sh; manual usage:
#   powershell -NoProfile -ExecutionPolicy Bypass -File tools\test-windows.ps1 `
#     -Exe '\\wsl.localhost\Ubuntu\tmp\x.test.exe' [-Run 'TestFoo|TestBar'] [-Langs 'zh,en']
#     [-EnvPairs 'K=V'] [-Hw 'auto' | -Hw 'COM22:COM23']
param(
    [Parameter(Mandatory = $true)][string]$Exe,
    [string]$Run = '',
    [string]$Langs = 'zh,en',
    [string[]]$EnvPairs = @(),
    [string]$Hw = ''
)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

if (-not (Test-Path -LiteralPath $Exe)) { throw "test binary not found: $Exe" }
foreach ($pair in $EnvPairs) {
    $name, $value = $pair -split '=', 2
    Set-Item -Path ("Env:" + $name) -Value $value
}

# Auto-detect a com0com port pair from the SERIALCOMM registry map.
# Ports are sorted by their device number and the first two are returned;
# with multiple com0com pairs installed, pass -hw TEST:PEER explicitly.
function Get-Com0ComPair {
    $item = Get-ItemProperty 'HKLM:\HARDWARE\DEVICEMAP\SERIALCOMM'
    $ports = $item.PSObject.Properties |
        Where-Object { $_.Name -like '\Device\com0com*' } |
        ForEach-Object { [pscustomobject]@{ N = [int]($_.Name -replace '\D', ''); Port = [string]$_.Value } } |
        Sort-Object N
    if (@($ports).Count -lt 2) {
        throw "com0com pair not found (found $(@($ports).Count) port(s)); pass -hw TEST:PEER explicitly"
    }
    return ,@($ports[0].Port, $ports[1].Port)
}

$testArgs = '-test.v'
# -test.run 的值加引号：cmd 会把未加引号的 | & < > 当管道/重定向（PS 5.1 必须经 cmd 执行）
if ($Run -ne '') { $testArgs = "$testArgs -test.run=`"$Run`"" }

$echoProc = $null
try {
    if ($Hw -ne '') {
        if ($Hw -match ':') {
            $testPort, $peerPort = $Hw -split ':'
        }
        else {
            $pair = Get-Com0ComPair
            $testPort, $peerPort = $pair[0], $pair[1]
        }
        Write-Host ">> com0com loopback: test=$testPort peer=$peerPort"
        Set-Item -Path Env:SERIALHUB_HARDWARE_TEST -Value '1'
        Set-Item -Path Env:SERIALHUB_TEST_PORT -Value $testPort
        $echoScript = Join-Path $PSScriptRoot 'com0com-echo.ps1'
        if (-not (Test-Path -LiteralPath $echoScript)) { throw "echo script not found: $echoScript" }
        $echoProc = Start-Process powershell.exe -ArgumentList @(
            '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"{0}"' -f $echoScript), '-Port', $peerPort
        ) -PassThru -WindowStyle Hidden
        Start-Sleep -Seconds 2  # 等对端回显进程完成串口打开
    }

    foreach ($lang in $Langs.Split(',')) {
        $lang = $lang.Trim()
        if ($lang -eq '') { continue }
        $env:SERIALHUB_LANG = $lang
        Write-Host ">> run $Exe (SERIALHUB_LANG=$lang)"
        $prevCwd = (Get-Location).Path
        Set-Location $env:TEMP  # cmd 不支持 UNC cwd，切到本地目录消除告警
        cmd /c ('"' + $Exe + '" ' + $testArgs + ' 2>&1')
        Set-Location $prevCwd
        if ($LASTEXITCODE -ne 0) {
            throw "FAILED: $Exe (SERIALHUB_LANG=$lang, exit=$LASTEXITCODE)"
        }
        Write-Host "PASS $Exe (SERIALHUB_LANG=$lang)"
    }
}
finally {
    if ($echoProc -and -not $echoProc.HasExited) {
        Stop-Process -Id $echoProc.Id -Force -ErrorAction SilentlyContinue
    }
    Remove-Item Env:SERIALHUB_LANG -ErrorAction SilentlyContinue
}
Write-Host 'PASS windows-tests'
