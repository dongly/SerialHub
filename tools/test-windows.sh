#!/usr/bin/env bash
# 从 WSL 交叉编译 Go 测试二进制，并经真实 Windows PowerShell 实跑（默认 zh/en 双语）。
#
# 用法:
#   tools/test-windows.sh                          # 默认: ./pkg/tray 全部用例
#   tools/test-windows.sh ./pkg/serial ./pkg/web   # 指定包（可多个）
#   tools/test-windows.sh -run 'TestFoo|TestBar' ./pkg/tray
#   tools/test-windows.sh -hw ./pkg/serial              # 硬件回环: 自动探测 com0com 对并自动起对端回显
#   tools/test-windows.sh -hw COM22:COM23 ./pkg/serial  # 硬件回环: 显式指定串口对（TEST:PEER）
#
# 环境变量:
#   SERIALHUB_LANG=zh|en    只跑指定语言（默认 zh,en 都跑）
#   SERIALHUB_HARDWARE_TEST=1 + SERIALHUB_TEST_PORT=COM22   硬件回环用例（手动模式，亦可直接用 -hw）
#
# 注意: 不要在 WSL 内直接 `GOOS=windows go test`——Wine 会自动执行并因
# systray 无效菜单句柄 panic；本脚本用真实 Windows 进程运行测试。
set -euo pipefail

if [[ "$(uname -r)" != *microsoft* ]]; then
    echo "错误: 本脚本需在 WSL 内运行（依赖 powershell.exe interop）" >&2
    exit 1
fi
if ! command -v powershell.exe >/dev/null 2>&1; then
    echo "错误: 未找到 powershell.exe" >&2
    exit 1
fi

run=''
hw=''
pkgs=()
while [[ $# -gt 0 ]]; do
    case "$1" in
    -run)
        run="$2"
        shift 2
        ;;
    -hw)
        # -hw [TEST:PEER]：省略值（或写 auto）时在 Windows 侧自动探测 com0com 对
        if [[ $# -ge 2 && "$2" != -* ]]; then
            hw="$2"
            shift 2
        else
            hw="auto"
            shift
        fi
        ;;
    -h | --help)
        grep '^#' "$0" | sed 's/^# \{0,1\}//'
        exit 0
        ;;
    *)
        pkgs+=("$1")
        shift
        ;;
    esac
done
if [[ ${#pkgs[@]} -eq 0 ]]; then
    if [[ -n $hw ]]; then pkgs=(./pkg/serial); else pkgs=(./pkg/tray); fi
fi
langs="${SERIALHUB_LANG:-zh,en}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ps1_win="$(wslpath -w "$script_dir/test-windows.ps1")"

work="$(mktemp -d /tmp/serialhub-win-test.XXXXXX)"
trap 'rm -rf "$work"' EXIT

overall=0
for pkg in "${pkgs[@]}"; do
    name="$(sed 's#[/.]#_#g' <<<"$pkg")"
    exe="$work/$name.test.exe"
    echo ">> 交叉编译 $pkg"
    GOOS=windows GOARCH=amd64 go test -c -o "$exe" "$pkg"

    echo ">> Windows 实跑 $pkg (langs=$langs${run:+, run=$run})"
    args=(-Exe "$(wslpath -w "$exe")" -Langs "$langs")
    [[ -n $run ]] && args+=(-Run "$run")
    [[ -n $hw ]] && args+=(-Hw "$hw")
    env_pairs=()
    [[ ${SERIALHUB_HARDWARE_TEST:-} == 1 ]] && env_pairs+=("SERIALHUB_HARDWARE_TEST=1")
    [[ -n ${SERIALHUB_TEST_PORT:-} ]] && env_pairs+=("SERIALHUB_TEST_PORT=$SERIALHUB_TEST_PORT")
    [[ ${#env_pairs[@]} -gt 0 ]] && args+=(-EnvPairs "${env_pairs[@]}")

    if ! powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$ps1_win" "${args[@]}"; then
        overall=1
    fi
done
exit $overall
