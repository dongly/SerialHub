# federation-check.ps1 - SerialHub 联邦状态巡检（Windows 侧）
# 用法: .\federation-check.ps1 [-Port 5050,5098] [-Scan]
#   -Port  追加探测端口（逗号分隔，可选，1-65535）
#   -Scan  额外扫描 5051-5059（实例掉主顺延端口的场景）
# 只读巡检：仅调用 /health 与 MCP serial_list，不连接、不写入串口。
# 可见性说明：本脚本仅探测当前环境的 127.0.0.1 候选端口，
# 不保证覆盖双侧实例（Windows 的 localhost 转发通常可看到 WSL 实例，
# 但受网络模式影响）；对侧实例请在 WSL 侧运行 tools/federation-check.sh 验证。
param(
    [int[]]$Port = @(),
    [switch]$Scan
)

$ErrorActionPreference = 'Continue'
$CurlT = 3
$McpAccept = 'application/json, text/event-stream'

# ---- 参数校验 ----
foreach ($p in $Port) {
    if ($p -lt 1 -or $p -gt 65535) {
        Write-Host "[federation-check] 端口超出范围 1-65535: $p"
        exit 3
    }
}

# ---- 端口发现: 默认 5050 + exe 同目录 config.toml [MCP] 段 HTTPPort ----
$ports = New-Object System.Collections.Generic.List[int]
$cmd = Get-Command serialhub -ErrorAction SilentlyContinue
if ($cmd) {
    $exeDir = Split-Path -Parent $cmd.Source
} else {
    # 未找到 serialhub 时退回脚本所在目录（脚本若不在安装目录则探测范围可能不含配置端口）
    $exeDir = Split-Path -Parent $PSCommandPath
}
$cfg = Join-Path $exeDir 'config.toml'
if (Test-Path $cfg) {
    # 仅匹配 [MCP] 段内的顶层 HTTPPort（显式 UTF-8 读取）
    $inMcp = $false
    foreach ($line in (Get-Content -Path $cfg -Encoding UTF8)) {
        if ($line -match '^\s*\[(.+)\]\s*$') {
            $inMcp = ($Matches[1] -match '^(?i:mcp)$')
            continue
        }
        if ($inMcp -and $line -match '^\s*HTTPPort\s*=\s*(\d+)') {
            $cfgPort = 0
            if (-not [int]::TryParse($Matches[1], [ref]$cfgPort) -or $cfgPort -lt 1 -or $cfgPort -gt 65535) {
                Write-Host "[federation-check] 配置文件 HTTPPort 超出范围，忽略: $($Matches[1])"
            } else {
                $ports.Add($cfgPort)
            }
            break
        }
    }
}
foreach ($p in $Port) { $ports.Add([int]$p) }
$ports.Add(5050)
if ($Scan) { 5051..5059 | ForEach-Object { $ports.Add($_) } }
$ports = $ports | Sort-Object -Unique

Write-Host "[SerialHub] 联邦状态巡检 ($(Get-Date -Format 'yyyy-MM-dd HH:mm:ss'))"
Write-Host "探测端口: $($ports -join ' ')"
Write-Host ""

# ---- MCP 调用: initialize -> initialized -> tools/call serial_list -> DELETE ----
# 返回 @{ Success=bool; Ports=数组; Error=string }；
# 成功且零串口时 Success=$true、Ports 为空数组（零串口是正常结果，不是故障）。
function Invoke-SerialList {
    param([string]$Base)
    $sid = $null
    $result = @{ Success = $false; Ports = @(); Error = '' }
    try {
        $initBody = @{
            jsonrpc = '2.0'; id = 0; method = 'initialize'
            params  = @{ protocolVersion = '2024-11-05'; capabilities = @{}
                         clientInfo = @{ name = 'federation-check'; version = '1.0' } }
        } | ConvertTo-Json -Depth 5
        $resp = Invoke-WebRequest -UseBasicParsing -Uri "$Base/mcp" -Method Post `
            -ContentType 'application/json' -Headers @{ Accept = $McpAccept } `
            -Body $initBody -TimeoutSec $CurlT
        $sid = $resp.Headers['Mcp-Session-Id']

        # 无 session ID 的无状态实现: 后续请求不带头继续
        $headers = @{ Accept = $McpAccept }
        if ($sid) { $headers['Mcp-Session-Id'] = $sid }

        Invoke-RestMethod -Uri "$Base/mcp" -Method Post -ContentType 'application/json' `
            -Headers $headers -TimeoutSec $CurlT -Body `
            '{"jsonrpc":"2.0","method":"notifications/initialized"}' | Out-Null

        $list = Invoke-RestMethod -Uri "$Base/mcp" -Method Post -ContentType 'application/json' `
            -Headers $headers -TimeoutSec $CurlT -Body `
            '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"serial_list","arguments":{}}}'

        if ($list.error) {
            $result.Error = "JSON-RPC 错误: $($list.error.message)"
            return $result
        }
        if ($list.result -and $list.result.isError) {
            $result.Error = '工具返回错误'
            return $result
        }
        $sc = $null
        if ($list.result) { $sc = $list.result.structuredContent }
        # 严格类型校验: ports 必须本身是数组（@() 是包装不是验证），条目必须有 name 与合法 origin
        if ($sc -and $null -ne $sc.ports -and $sc.ports -is [array]) {
            $valid = $true
            foreach ($p in $sc.ports) {
                if ($null -eq $p -or $p -isnot [System.Management.Automation.PSCustomObject] -or
                    ($p.name -isnot [string]) -or ([string]::IsNullOrWhiteSpace($p.name)) -or
                    ($p.origin -notin @('local', 'federated'))) {
                    $valid = $false; break
                }
            }
            if ($valid) {
                $result.Success = $true
                $result.Ports = @($sc.ports)
            } else {
                $result.Error = 'ports 条目格式无效（缺少 name 或 origin 非法）'
            }
        } else {
            $result.Error = '响应缺少 ports 数组'
        }
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

$masters = New-Object System.Collections.Generic.List[string]
$workers = New-Object System.Collections.Generic.List[string]
$sideOf = @{}
$mcpOk = @{}
$fedOf = @{}

foreach ($p in $ports) {
    $url = "http://127.0.0.1:$p"
    try {
        $health = Invoke-RestMethod -Uri "$url/health" -TimeoutSec $CurlT
    } catch {
        if (-not $Scan) { Write-Host "  $url  ->  无监听" }
        continue
    }

    if ($health.service -ne 'serialhub') {
        Write-Host "  $url  ->  非 SerialHub 服务（/health 200 但无 service 字段）"
        continue
    }

    if ($health.role -eq 'master') {
        Write-Host "  $url  ->  SerialHub MASTER"
        $masters.Add("$p")
        $r = Invoke-SerialList $url
        if ($r.Success) {
            $mcpOk["$p"] = $true
            $locals = @($r.Ports | Where-Object { $_.origin -eq 'local' })
            $feds   = @($r.Ports | Where-Object { $_.origin -ne 'local' })
            $fedOf["$p"] = $feds.Count
            Write-Host ("      聚合串口 {0} 个（本地 {1} / 联邦 {2}）" -f ($locals.Count + $feds.Count), $locals.Count, $feds.Count)
            if ($locals.Count -gt 0) {
                Write-Host ("        本地: " + (($locals | ForEach-Object { $_.name }) -join ' '))
                $sideOf["$p"] = $locals[0].side
            }
            if ($feds.Count -gt 0) {
                Write-Host ("        联邦: " + (($feds | ForEach-Object { "$($_.name)(side=$($_.side))" }) -join ' '))
            }
            if (($locals.Count + $feds.Count) -eq 0) {
                Write-Host "        （无串口接入属正常，接入 USB 后重查）"
            }
        } else {
            $mcpOk["$p"] = $false
            $fedOf["$p"] = 0
            Write-Host "      串口聚合未能验证（MCP 调用失败: $($r.Error)）"
        }
    } elseif ($health.role -eq 'worker') {
        Write-Host "  $url  ->  SerialHub WORKER（/health 为静态角色应答，/mcp 反代到主实例）"
        $workers.Add("$p")
    } else {
        Write-Host "  $url  ->  SerialHub（未知 role=$($health.role)）"
    }
}

Write-Host ""
Write-Host "诊断:"
$rc = 0
if ($masters.Count -eq 0 -and $workers.Count -eq 0) {
    Write-Host "  ✗ 未发现任何 SerialHub 实例（检查服务是否启动: serialhub --minimized -D）"
    exit 1
}
if ($masters.Count -eq 0) {
    Write-Host "  ⚠ 当前探测范围只发现 WORKER，未发现 MASTER"
    Write-Host "     主实例可能位于对侧或其他端口；请检查 worker 上游与主实例日志"
    exit 1
}

if ($masters.Count -gt 1) {
    Write-Host ("  ⚠ 发现 {0} 个 MASTER 端点: {1}" -f $masters.Count, ($masters -join ' '))
    Write-Host "     （无实例 ID，不能排除端口转发到同一进程；联邦正常时应只有一个主实例，"
    Write-Host "      多主会各自聚合本侧串口，建议只保留一个）"
    $rc = 2
} else {
    Write-Host ("  ✓ MASTER 唯一: " + ($masters[0]))
}

if ($mcpOk[$masters[0]]) {
    Write-Host ("  ✓ MASTER {0} 的串口聚合已验证（serial_list 可用；其他端点见上方明细）" -f $masters[0])
} else {
    Write-Host "  ⚠ 拓扑已明确，但串口聚合未能验证（MCP 调用失败）"
}

# 联邦贡献: 任一 master 报告 federated 串口即说明有从实例注册过
$anyFed = $false
foreach ($m in $masters) {
    if ($mcpOk["$m"] -and $fedOf["$m"] -gt 0) { $anyFed = $true }
}
if ($anyFed) {
    Write-Host "  ℹ 主实例报告联邦串口贡献（有从实例注册并贡献了串口）"
}

if ($workers.Count -gt 0) {
    Write-Host ("  ℹ 发现 WORKER 监听: " + ($workers -join ' '))
    Write-Host "     （/health 是静态角色应答，注册状态未验证；确认注册请查主实例日志「从实例已注册」）"
} elseif (-not $anyFed) {
    Write-Host "  ℹ 本侧未发现 WORKER 监听（同侧从实例不占独立端口时不可见；对侧请运行对侧脚本验证）"
}

if ($sideOf[$masters[0]]) { Write-Host ("  ℹ MASTER 侧别: " + $sideOf[$masters[0]]) }
Write-Host "  ℹ 可见性: 本脚本仅探测当前环境的 127.0.0.1 候选端口，不保证覆盖双侧实例"
exit $rc
