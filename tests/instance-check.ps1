# instance-check.ps1 - SerialHub 实例状态自检（Windows 侧）
# 用法: .\instance-check.ps1 [-Port 5050,5098]
#   -Port  追加探测端口（逗号分隔，可选）
# 检查内容（只读，不连接、不写入串口）：
#   1. lock 文件元数据（文件常驻；仅 OS 文件锁表示实例存活）
#   2. /health 实例识别（service=serialhub）
#   3. MCP serial_list 调用（完整握手，展示本实例串口列表）
# 退出码: 0=全部发现的实例检查通过; 1=未发现实例; 2=MCP 检查失败; 3=参数错误
param(
    [int[]]$Port = @()
)

$ErrorActionPreference = 'Continue'
$CurlT = 3
foreach ($p in $Port) {
    if ($p -lt 1 -or $p -gt 65535) {
        Write-Host "[instance-check] 端口超出范围 1-65535: $p" -ForegroundColor Red
        exit 3
    }
}

# ---- 端口发现: 默认 5050 + exe 同目录 config.toml [MCP] HTTPPort ----
$ports = New-Object System.Collections.Generic.List[int]
$exeDir = $null
$cmd = Get-Command serialhub -ErrorAction SilentlyContinue
if ($cmd) { $exeDir = Split-Path -Parent $cmd.Source }
if (-not $exeDir) { $exeDir = Split-Path -Parent $PSCommandPath }
$cfg = Join-Path $exeDir 'config.toml'
$cfgPort = 0
if (Test-Path $cfg) {
    $inMcp = $false
    foreach ($line in (Get-Content -Path $cfg -Encoding UTF8)) {
        if ($line -match '^\s*\[(.+)\]') {
            $inMcp = ($Matches[1] -match '^(?i:mcp)$')
            continue
        }
        if ($inMcp -and $line -match '^\s*(?i:httpport)\s*=\s*(\d+)') {
            $cfgPort = 0
            if ([int]::TryParse($Matches[1], [ref]$cfgPort) -and $cfgPort -ge 1 -and $cfgPort -le 65535) {
                $ports.Add($cfgPort)
            } else {
                Write-Host "  （config.toml 端口值无效: $($Matches[1])，已忽略）" -ForegroundColor Yellow
            }
            break
        }
    }
}
foreach ($p in $Port) { $ports.Add([int]$p) }
$ports.Add(5050)

Write-Host "[SerialHub] 实例状态自检 ($(Get-Date -Format 'yyyy-MM-dd HH:mm:ss'))"
Write-Host ""

# ---- lock 文件检查 ----
# lock 位置：Windows=exe 同目录；Linux/macOS=用户配置目录（与实例实际写法一致）
$onWindows = ($PSVersionTable.PSVersion.Major -lt 6) -or $IsWindows
if ($onWindows) {
    $lock = Join-Path $exeDir 'instance.lock'
} else {
    $xdg = $env:XDG_CONFIG_HOME
    if (-not $xdg -or -not [System.IO.Path]::IsPathRooted($xdg)) {
        $xdg = Join-Path $env:HOME '.config'
    }
    $lock = Join-Path (Join-Path $xdg 'serialhub') 'instance.lock'
}
if (Test-Path $lock) {
    Write-Host "lock 文件: 存在（$lock）"
    try {
        $info = Get-Content -Path $lock -Encoding UTF8 | ConvertFrom-Json
        Write-Host ("  记录: pid={0} port={1} host={2} 启动于 {3}" -f $info.pid, $info.port, $info.host, $info.started_at)
        $lockPort = 0
        if ([int]::TryParse([string]$info.port, [ref]$lockPort) -and $lockPort -ge 1 -and $lockPort -le 65535) {
            $lockHost = [string]$info.host
            if (-not $lockHost -or $lockHost -in @('0.0.0.0', '::')) { $lockHost = '127.0.0.1' }
            $lockAddress = @{ Host = $lockHost; Port = $lockPort }
            $ports.Insert(0, $lockPort)
        }
    } catch {
        Write-Host "  （内容异常: $($_.Exception.Message)）" -ForegroundColor Yellow
    }
    Write-Host "  说明: 文件可能常驻；仅 OS 文件锁能证明实例仍在运行"
} else {
    Write-Host "lock 文件: 不存在（$lock）"
}
Write-Host ""
$ports = $ports | Sort-Object -Unique
Write-Host "探测端口: $($ports -join ' ')"
Write-Host ""

# ---- MCP 调用: initialize -> initialized -> tools/call serial_list -> DELETE ----
# 返回 @{ Success=[bool]; Ports=[array]; Error=[string] }
function Invoke-SerialList {
    param([string]$Base)
    $result = @{ Success = $false; Ports = @(); Error = '' }
    $sid = $null
    try {
        $initBody = @{
            jsonrpc = '2.0'; id = 0; method = 'initialize'
            params  = @{ protocolVersion = '2024-11-05'; capabilities = @{}
                         clientInfo = @{ name = 'instance-check'; version = '1.0' } }
        } | ConvertTo-Json -Depth 5
        $resp = Invoke-WebRequest -UseBasicParsing -Uri "$Base/mcp" -Method Post `
            -ContentType 'application/json' -Headers @{ Accept = 'application/json, text/event-stream' } `
            -Body $initBody -TimeoutSec $CurlT
        $sid = $resp.Headers['Mcp-Session-Id']
        if (-not $sid) { $result.Error = '响应缺少 Mcp-Session-Id'; return $result }

        $acc = @{ 'Mcp-Session-Id' = $sid; Accept = 'application/json, text/event-stream' }
        Invoke-RestMethod -Uri "$Base/mcp" -Method Post -ContentType 'application/json' `
            -Headers $acc -TimeoutSec $CurlT -Body `
            '{"jsonrpc":"2.0","method":"notifications/initialized"}' | Out-Null

        $list = Invoke-RestMethod -Uri "$Base/mcp" -Method Post -ContentType 'application/json' `
            -Headers $acc -TimeoutSec $CurlT -Body `
            '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"serial_list","arguments":{}}}'
        $r = $list.result
        if ($list.error) {
            $result.Error = "JSON-RPC 错误: $($list.error.message)"; return $result
        }
        if ($r -and $r.isError) {
            $result.Error = '工具执行失败'; return $result
        }
        $sc = $r.structuredContent
        if (-not $sc) {
            $result.Error = '响应缺少 structuredContent.ports'; return $result
        }
        # 用属性存在性判定，避免空数组在部分 PS 版本解析为 $null 时被当成缺失
        $portsProp = $sc.PSObject.Properties['ports']
        if (-not $portsProp) {
            $result.Error = '响应缺少 structuredContent.ports'; return $result
        }
        if ($null -ne $portsProp.Value -and $portsProp.Value -isnot [array]) {
            $result.Error = 'ports 不是数组'; return $result
        }
        $portsList = @($portsProp.Value)
        foreach ($p in $portsList) {
            if (-not $p -or $p -isnot [PSCustomObject]) {
                $result.Error = 'ports 条目不是对象'; return $result
            }
            if (($p.name -isnot [string]) -or ([string]::IsNullOrWhiteSpace($p.name))) {
                $result.Error = 'ports 条目缺少有效 name'; return $result
            }
            if ($p.origin -notin @('local', 'federated')) {
                $result.Error = "ports 条目 origin 非法: $($p.origin)"; return $result
            }
        }
        $result.Success = $true
        $result.Ports = $portsList
        return $result
    } catch {
        $result.Error = $_.Exception.Message
        return $result
    } finally {
        if ($sid) {
            try { Invoke-WebRequest -UseBasicParsing -Uri "$Base/mcp" -Method Delete `
                -Headers @{ 'Mcp-Session-Id' = $sid } -TimeoutSec $CurlT | Out-Null } catch {}
        }
    }
}

$found = $false
$failed = $false
foreach ($p in $ports) {
    $probeHost = '127.0.0.1'
    if ($lockAddress -and $lockAddress.Port -eq $p) { $probeHost = $lockAddress.Host }
    if ($probeHost.Contains(':')) { $probeHost = "[$probeHost]" }
    $url = "http://${probeHost}:$p"
    try {
        $health = Invoke-RestMethod -Uri "$url/health" -TimeoutSec $CurlT
    } catch {
        Write-Host "  $url  ->  无监听"
        continue
    }

    if ($health.service -ne 'serialhub') {
        Write-Host "  $url  ->  非 SerialHub 服务（/health 200 但无 service 字段）"
        continue
    }

    Write-Host "  $url  ->  SerialHub 运行中（role=$($health.role)）"
    $found = $true
    $r = Invoke-SerialList $url
    if ($r.Success) {
        Write-Host ("      串口列表: {0} 个" -f @($r.Ports).Count)
        foreach ($pinfo in $r.Ports) {
            Write-Host ("        {0}（origin={1} side={2}）" -f $pinfo.name, $pinfo.origin, $pinfo.side)
        }
    } else {
        Write-Host "      串口列表: MCP 调用失败（$($r.Error)）" -ForegroundColor Yellow
        $failed = $true
    }
}

Write-Host ""
if ($failed) {
    Write-Host "诊断: ✗ 已发现 SerialHub，但 MCP 串口列表检查失败" -ForegroundColor Red
    exit 2
}
if ($found) {
    Write-Host "诊断: ✓ 发现健康的 SerialHub 实例"
    exit 0
}
Write-Host "诊断: ✗ 未发现 SerialHub 实例（检查服务是否启动: serialhub --minimized -D）"
exit 1
