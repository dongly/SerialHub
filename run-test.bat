@echo off
cd /d D:\Develop\SerialHub
echo Starting test...
npx tsx test-full.ts --skip-tray > test-result.txt 2>&1
echo Test completed.
type test-result.txt