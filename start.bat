@echo off
rem SerialHub 启动脚本（cmd 极简版；完整功能见 start.ps1）
rem 用法: start.bat [serialhub 参数，如 -p COM9 -D]
"%~dp0serialhub.exe" %*
