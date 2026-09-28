@echo off
rem SerialHub launcher: forwards to serialhub.ps1 (kills old process, minimized window)
rem Usage: serialhub.bat [serialhub args, e.g. -p COM9 -D]
powershell -ExecutionPolicy Bypass -File "%~dp0serialhub.ps1" %*
