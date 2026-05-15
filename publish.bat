@echo off
setlocal

set "SOURCE=%~dp0"
set "TARGET=D:\DevTools\serialhub"

echo === SerialHub Publish ===
echo Source: %SOURCE%
echo Target: %TARGET%
echo.

if not exist "%SOURCE%bin\serialhub.exe" (
    echo [ERROR] bin\serialhub.exe not found. Run build first.
    pause
    exit /b 1
)

if not exist "%TARGET%" mkdir "%TARGET%"
if not exist "%TARGET%\bin" mkdir "%TARGET%\bin"
if not exist "%TARGET%\bin\logs" mkdir "%TARGET%\bin\logs"

tasklist /fi "imagename eq serialhub.exe" 2>nul | find /i "serialhub.exe" >nul
if %errorlevel%==0 (
    echo [1/4] Stopping serialhub...
    taskkill /f /im serialhub.exe >nul 2>&1
    timeout /t 1 /nobreak >nul
) else (
    echo [1/4] No running serialhub process
)

echo [2/4] Copying executable...
copy /y "%SOURCE%bin\serialhub.exe" "%TARGET%\bin\serialhub.exe" >nul
if %errorlevel% neq 0 (
    echo [ERROR] Failed to copy serialhub.exe
    pause
    exit /b 1
)

echo [3/4] Copying docs and config...
copy /y "%SOURCE%README.md" "%TARGET%\README.md" >nul
copy /y "%SOURCE%MCP.md" "%TARGET%\MCP.md" >nul
copy /y "%SOURCE%config.example.toml" "%TARGET%config.toml" >nul
copy /y "%SOURCE%start.bat" "%TARGET%\start.bat" >nul
copy /y "%SOURCE%start.ps1" "%TARGET%\start.ps1" >nul

if not exist "%TARGET%\bin\config.toml" (
    copy /y "%SOURCE%config.example.toml" "%TARGET%\bin\config.toml" >nul
)

echo [4/4] Verifying...
if exist "%TARGET%\bin\serialhub.exe" (
    for %%f in ("%TARGET%\bin\serialhub.exe") do echo   serialhub.exe  %%~zf bytes
) else (
    echo [ERROR] serialhub.exe not found after copy
    pause
    exit /b 1
)

echo.
echo === Publish Done ===
echo Target: %TARGET%
echo Run:    %TARGET%\start.bat
for /f "tokens=*" %%v in ('"%TARGET%\bin\serialhub.exe" --version 2^>nul') do echo Version: %%v
