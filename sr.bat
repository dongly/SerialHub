@echo off
rem SerialHub launcher: forwards to sr.ps1 (kills old process, minimized window)
rem Usage: sr.bat [serialhub args, e.g. -p COM9 -D]
rem Named sr (not serialhub) so it never shadows serialhub.exe on PATH.
powershell -ExecutionPolicy Bypass -File "%~dp0sr.ps1" %*
