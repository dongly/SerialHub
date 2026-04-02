# SerialHub 系统托盘交互式测试 (T23-T25)
# 在 Windows 桌面上运行

$ErrorActionPreference = "Continue"

function Show-Header($title) {
    Clear-Host
    Write-Host "==========================================" -ForegroundColor Cyan
    Write-Host $title -ForegroundColor Cyan
    Write-Host "==========================================" -ForegroundColor Cyan
    Write-Host ""
}

function Wait-Key($message = "按任意键继续...") {
    Write-Host $message -ForegroundColor Yellow
    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
}

function Get-YesNo($prompt) {
    while ($true) {
        Write-Host ""
        $response = Read-Host $prompt
        if ($response -eq "y" -or $response -eq "Y") { return $true }
        if ($response -eq "n" -or $response -eq "N") { return $false }
        Write-Host "请输入 y 或 n" -ForegroundColor Red
    }
}

# ============ 开始测试 ============
Show-Header "SerialHub 系统托盘交互测试 (T23-T25)"

Write-Host "请确保：" -ForegroundColor Yellow
Write-Host "  - 在 Windows 桌面上运行此脚本"
Write-Host "  - 能看到屏幕右下角的系统托盘区"
Write-Host "  - COM9 已连接 RT-Thread 设备"
Write-Host ""

Wait-Key "按任意键开始测试..."

# ============ T23 ============
Show-Header "[T23] 托盘图标显示测试"

Write-Host "即将启动 SerialHub..." -ForegroundColor Green
Write-Host "命令: .\bin\serialhub.exe -p COM9" -ForegroundColor Gray
Write-Host ""

$serialhubPath = Join-Path $PSScriptRoot "bin\serialhub.exe"
$process = Start-Process -FilePath $serialhubPath -ArgumentList "-p","COM9" -PassThru

Write-Host "等待 3 秒启动..." -ForegroundColor Gray
Start-Sleep -Seconds 3

Show-Header "[T23] 请检查托盘图标"

Write-Host "请查看屏幕右下角系统托盘区：" -ForegroundColor Yellow
Write-Host ""
Write-Host "  [1] 是否看到 SerialHub 图标？" -ForegroundColor White
Write-Host "      （绿色 = 已连接，灰色 = 未连接）" -ForegroundColor Gray
Write-Host ""
Write-Host "  [2] 鼠标悬停在图标上" -ForegroundColor White
Write-Host "      是否显示 'SerialHub v0.1.0'？" -ForegroundColor Gray
Write-Host ""
Write-Host "  [3] 图标是否清晰可见？" -ForegroundColor White
Write-Host ""
Write-Host "==========================================" -ForegroundColor Cyan

$T23Pass = Get-YesNo "图标显示正常吗？(y/n)"

if ($T23Pass) {
    Write-Host "✅ T23 通过" -ForegroundColor Green
} else {
    Write-Host "❌ T23 失败" -ForegroundColor Red
    Write-Host "请检查：" -ForegroundColor Yellow
    Write-Host "  - 程序是否已启动"
    Write-Host "  - 托盘区是否被折叠"
    Write-Host "  - 是否有错误提示"
}

# ============ T24 ============
if ($T23Pass) {
    Show-Header "[T24] 托盘菜单操作测试"
    
    Write-Host "请执行以下操作：" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  [1] 右键点击 SerialHub 托盘图标" -ForegroundColor White
    Write-Host ""
    Write-Host "  [2] 查看弹出的菜单，确认包含：" -ForegroundColor White
    Write-Host "      - 📡 串口连接" -ForegroundColor Cyan
    Write-Host "      - ⚙️ 串口参数" -ForegroundColor Cyan
    Write-Host "      - 🌐 服务端口" -ForegroundColor Cyan
    Write-Host "      - 📋 版本 v0.1.0" -ForegroundColor Cyan
    Write-Host "      - ❌ 退出" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  [3] 点击 '🌐 服务端口'" -ForegroundColor White
    Write-Host "      查看显示的端口信息" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  [4] 点击其他地方关闭菜单" -ForegroundColor White
    Write-Host ""
    Write-Host "==========================================" -ForegroundColor Cyan
    
    $T24Pass = Get-YesNo "菜单操作正常吗？(y/n)"
    
    if ($T24Pass) {
        Write-Host "✅ T24 通过" -ForegroundColor Green
    } else {
        Write-Host "❌ T24 失败" -ForegroundColor Red
    }
} else {
    $T24Pass = $false
}

# ============ T25 ============
if ($T24Pass) {
    Show-Header "[T25] 状态切换测试"
    
    Write-Host "当前状态：图标应该是绿色的（已连接）" -ForegroundColor Green
    Write-Host ""
    Write-Host "请执行以下操作：" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  [1] 右键点击 SerialHub 图标" -ForegroundColor White
    Write-Host "  [2] 点击 '📡 串口连接' 断开连接" -ForegroundColor White
    Write-Host "  [3] 观察图标是否变为灰色" -ForegroundColor White
    Write-Host ""
    
    Wait-Key "操作完成后按任意键继续..."
    
    Show-Header "[T25] 状态切换测试（续）"
    
    Write-Host "当前状态：图标应该是灰色的（未连接）" -ForegroundColor Gray
    Write-Host ""
    Write-Host "请执行以下操作：" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  [4] 右键点击 SerialHub 图标" -ForegroundColor White
    Write-Host "  [5] 点击 '📡 串口连接' 重新连接" -ForegroundColor White
    Write-Host "  [6] 观察图标是否变回绿色" -ForegroundColor White
    Write-Host ""
    Write-Host "==========================================" -ForegroundColor Cyan
    
    $T25Pass = Get-YesNo "状态切换正常吗？(y/n)"
    
    if ($T25Pass) {
        Write-Host "✅ T25 通过" -ForegroundColor Green
    } else {
        Write-Host "❌ T25 失败" -ForegroundColor Red
    }
} else {
    $T25Pass = $false
}

# ============ 汇总 ============
Show-Header "测试结果汇总"

Write-Host ""
if ($T23Pass) { Write-Host "✅ T23 托盘图标显示: 通过" -ForegroundColor Green }
else { Write-Host "❌ T23 托盘图标显示: 失败" -ForegroundColor Red }

if ($T24Pass) { Write-Host "✅ T24 托盘菜单操作: 通过" -ForegroundColor Green }
else { Write-Host "❌ T24 托盘菜单操作: 失败" -ForegroundColor Red }

if ($T25Pass) { Write-Host "✅ T25 状态切换: 通过" -ForegroundColor Green }
else { Write-Host "❌ T25 状态切换: 失败" -ForegroundColor Red }

Write-Host ""
if ($T23Pass -and $T24Pass -and $T25Pass) {
    Write-Host "🎉 所有托盘测试通过！" -ForegroundColor Green
} else {
    Write-Host "⚠️ 部分测试未通过" -ForegroundColor Yellow
}
Write-Host ""

# 关闭程序
Write-Host "关闭 SerialHub..." -ForegroundColor Gray
if ($process) {
    Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
}
Get-Process serialhub -ErrorAction SilentlyContinue | Stop-Process -Force

Write-Host ""
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "测试结束" -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan

Wait-Key "按任意键退出..."
