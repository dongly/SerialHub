@echo off
rem SerialHub 启动脚本（转调 serialhub.ps1：自动杀旧进程、最小化窗口）
rem 用法: serialhub.bat [serialhub 参数，如 -p COM9 -D]
powershell -ExecutionPolicy Bypass -File "%~dp0serialhub.ps1" %*
