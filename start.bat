@echo off
chcp 65001 >nul
cd /d "%~dp0"

:: 先杀掉已运行的 serialhub 进程
tasklist /fi "imagename eq serialhub.exe" 2>nul | find /i "serialhub.exe" >nul
if %errorlevel%==0 (
    echo 正在终止已运行的 serialhub 进程...
    taskkill /f /im serialhub.exe >nul 2>&1
)

if not exist "bin\serialhub.exe" (
    echo 找不到 SerialHub 可执行文件: %~dp0bin\serialhub.exe
    echo 请先运行: go build -o bin\serialhub.exe ./cmd/serialhub
    pause
    exit /b 1
)

:: 构建参数
set "ARGS=--minimized"

:: 默认参数（按需取消注释）
:: set "ARGS=%ARGS% -p COM7"
:: set "ARGS=%ARGS% -b 115200"
:: set "ARGS=%ARGS% -D"
:: set "ARGS=%ARGS% -m 5000"
:: set "ARGS=%ARGS% --host 127.0.0.1"
:: set "ARGS=%ARGS% --no-tray"

:: 追加命令行参数
set "ARGS=%ARGS% %*"

start "SerialHub" /min bin\serialhub.exe %ARGS%

echo SerialHub 已启动
echo Use: taskkill /f /im serialhub.exe to stop
