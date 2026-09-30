#!/usr/bin/env bash
# tests/test-install.sh — install.sh / install.ps1 端到端安装测试
#
# 用法: bash tests/test-install.sh [windows|linux|all]   （默认 all）
#
# 原理：本地起 mock GitHub API（releases/latest + 伪造 v9.9.9 资产），
# 通过 SERIALHUB_GITHUB_API / SERIALHUB_DOWNLOAD_BASE / SERIALHUB_INSTALL_DIR
# 三个环境变量把安装脚本指向 mock，在沙箱目录实跑安装并断言结果。
#   linux   场景：bash install.sh（ELF tar.gz），纯本地 127.0.0.1；
#   windows 场景：经 interop 实跑 install.ps1（zip + sr 启动脚本复制 +
#           废弃旧启动脚本清理），资产经 WSL IP 送达；跑前停 serialhub.exe
#           （install.ps1 有运行实例检查），跑后重启 :5051 实例恢复部署态。
# 依赖：go、python3、curl、timeout、unshare 不需要；windows 场景另需
#       cmd.exe/powershell.exe（WSL interop）与 /mnt/d 可写。
# 不在 WSL 时提示无法测试 Windows 产物：windows 模式退出、all 自动降级仅 linux。
set -u

MODE="${1:-all}"
case "$MODE" in windows|linux|all) ;; *)
  echo "Usage: bash tests/test-install.sh [windows|linux|all]" >&2; exit 2 ;;
esac
T_CMD="${T_CMD:-120}"        # 单命令超时秒数
WATCHDOG_S="${WATCHDOG_S:-600}"

WORK=$(mktemp -d /tmp/opencode/install-test-XXXXXX)
WIN_SB="/mnt/d/DevTools/install-test-$$"
MOCK_PID=""        # mock 进程 PID（cleanup 只杀自己起的）
WAS_5051_PID=""    # 测试前 :5051 原实例 PID（空=原本无实例）
REPO_ROOT=""       # 仓库根（restore 用；:64 赋值）

# restore_windows_instance — 恢复 Windows 部署态：仅在测试前本有 :5051 实例、
# 且现在无监听时，用仓库 bin/serialhub.exe 重新拉起。cleanup trap 与正常
# 流程共用（幂等：有监听即跳过）。
restore_windows_instance() {
  [ -n "$WAS_5051_PID" ] || return 0
  command -v netstat.exe >/dev/null 2>&1 || return 0
  if timeout -k 5 20 netstat.exe -ano 2>/dev/null | tr -d '\r' | grep -q ':5051 .*LISTENING'; then return 0; fi
  ( cd "$REPO_ROOT/bin" 2>/dev/null && setsid nohup ./serialhub.exe --minimized -m 5051 >/dev/null 2>&1 & )
  sleep 3
}

cleanup() {
  restore_windows_instance
  rm -rf "$WORK" "$WIN_SB"
  [ -n "$MOCK_PID" ] && kill "$MOCK_PID" 2>/dev/null
}
trap cleanup EXIT
trap 'exit 130' INT    # 传导到 EXIT trap，保证中断也恢复实例/清理
trap 'exit 143' TERM
( sleep "$WATCHDOG_S" && kill $$ ) 2>/dev/null &   # 全局看门狗

PASS=0; FAIL=0
ok()   { PASS=$((PASS+1)); echo "  ok - $1"; }
fail() { FAIL=$((FAIL+1)); echo "  FAIL - $1"; }
assert_contains() { # $1=描述 $2=haystack文件 $3=needle(ERE)
  if timeout -k 5 10 grep -qE "$3" "$2" 2>/dev/null; then ok "$1"; else fail "$1"; echo "    └ 缺少: $3"; fi
}
assert_not_contains() { # $1=描述 $2=haystack文件 $3=needle(ERE)
  if timeout -k 5 10 grep -qE "$3" "$2" 2>/dev/null; then fail "$1"; echo "    └ 不应出现: $3"; else ok "$1"; fi
}
assert_exists() { [ -e "$1" ] && ok "$2" || fail "$2 ($1 不存在)"; }
assert_gone()   { [ ! -e "$1" ] && ok "$2" || fail "$2 ($1 仍在)"; }
probe_version() { # $@=版本命令；内部限时（防无参误启服务挂起）；输出裸版本（剥 SerialHub/v 前缀，免疫 stderr 噪音行）
  timeout -k 5 20 "$@" 2>&1 | tr -d '\r' | grep -oE 'SerialHub v?[0-9][^ ]*' | head -1 | sed 's/^SerialHub v\?//'
}

# ---------- 0. 环境检查 ----------
if ! grep -qi microsoft /proc/version 2>/dev/null; then
  case "$MODE" in
    windows) echo ">> 不在 WSL 环境中，无法测试 Windows 安装脚本"; exit 2 ;;
    all)     echo ">> 不在 WSL 环境中，跳过 Windows 场景，仅测试 Linux"; MODE=linux ;;
  esac
fi
if [ "$MODE" != linux ] && ! command -v cmd.exe >/dev/null 2>&1; then
  echo ">> 找不到 cmd.exe（需要 WSL interop），跳过 Windows 场景"; MODE=linux
fi
command -v go       >/dev/null || { echo ">> 需要 go";    exit 2; }
command -v python3  >/dev/null || { echo ">> 需要 python3"; exit 2; }
command -v curl     >/dev/null || { echo ">> 需要 curl";  exit 2; }
command -v timeout  >/dev/null || { echo ">> 需要 timeout"; exit 2; }

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$REPO_ROOT"

# ---------- 1. 编译伪造 v9.9.9 产物 ----------
echo ">> [1] 编译伪造 v9.9.9 产物"
LDF="-s -w -X github.com/dongly/serialhub/pkg/version.Version=9.9.9"
LINUX_FAKE="$WORK/serialhub-fake999"
WIN_FAKE="$WORK/serialhub-fake999.exe"
if [ "$MODE" != windows ]; then
  timeout -k 5 300 go build -trimpath -ldflags "$LDF" -o "$LINUX_FAKE" ./cmd/serialhub || exit 1
fi
if [ "$MODE" != linux ]; then
  timeout -k 5 300 env GOOS=windows go build -trimpath -ldflags "$LDF" -o "$WIN_FAKE" ./cmd/serialhub || exit 1
fi

# ---------- 2. mock GitHub API + 伪造资产 ----------
echo ">> [2] 构造 mock 资产与 API"
ASSETS="$WORK/assets"; mkdir -p "$ASSETS"
PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("0.0.0.0",0));print(s.getsockname()[1]);s.close()')

cat > "$WORK/mock_install.py" <<'PYEOF'
import http.server, os
WORK = os.environ["MOCK_WORK"]
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/repos/dongly/serialhub/releases/latest":
            body = b'{"tag_name": "v9.9.9"}'
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers(); self.wfile.write(body); return
        if self.path.startswith("/download/"):
            # GitHub release 下载 URL 带 tag 段（/download/<tag>/<asset>），取 basename 匹配资产
            p = os.path.join(WORK, "assets", os.path.basename(self.path))
            if os.path.isfile(p):
                self.send_response(200)
                self.send_header("Content-Length", str(os.path.getsize(p)))
                self.end_headers()
                with open(p, "rb") as f:
                    self.wfile.write(f.read())
                return
        self.send_error(404)
    def log_message(self, *a): pass
http.server.ThreadingHTTPServer(("0.0.0.0", int(os.environ["MOCK_PORT"])), H).serve_forever()
PYEOF

if [ "$MODE" != windows ]; then
  STAGE="$WORK/stage-linux/serialhub-9.9.9-linux-amd64"
  mkdir -p "$STAGE"
  cp "$LINUX_FAKE" "$STAGE/serialhub" && chmod 755 "$STAGE/serialhub"
  tar -czf "$ASSETS/serialhub-9.9.9-linux-amd64.tar.gz" -C "$WORK/stage-linux" serialhub-9.9.9-linux-amd64
  ( cd "$ASSETS" && sha256sum serialhub-9.9.9-linux-amd64.tar.gz > serialhub-9.9.9-linux-amd64.tar.gz.sha256 )
fi
if [ "$MODE" != linux ]; then
  STAGEW="$WORK/stage-win/serialhub-9.9.9-windows-amd64"
  mkdir -p "$STAGEW"
  cp "$WIN_FAKE" "$STAGEW/serialhub.exe"
  cp sr.ps1 sr.bat "$STAGEW/"
  python3 - "$WORK/stage-win" "$ASSETS/serialhub-9.9.9-windows-amd64.zip" <<'PYEOF'
import sys, zipfile, os
stage, out = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
    for root, _, files in os.walk(stage):
        for f in files:
            p = os.path.join(root, f)
            z.write(p, os.path.relpath(p, stage))
PYEOF
  ( cd "$ASSETS" && sha256sum serialhub-9.9.9-windows-amd64.zip > serialhub-9.9.9-windows-amd64.zip.sha256 )
fi

MOCK_PORT=$PORT MOCK_WORK="$WORK" python3 "$WORK/mock_install.py" &
MOCK_PID=$!
for _ in $(seq 1 20); do curl -fsS -m 2 "http://127.0.0.1:$PORT/repos/dongly/serialhub/releases/latest" >/dev/null 2>&1 && break; sleep 0.3; done
curl -fsS -m 2 "http://127.0.0.1:$PORT/repos/dongly/serialhub/releases/latest" >/dev/null || { echo ">> mock 未就绪"; exit 1; }

# ---------- 3. Linux 场景 ----------
run_linux() {
  echo ">> [3] Linux: bash install.sh"
  local SBX="$WORK/sbx-linux" LOG="$WORK/l1.log"
  mkdir -p "$SBX/bin"
  : > "$LOG"
  if timeout -k 5 "$T_CMD" env \
      SERIALHUB_GITHUB_API="http://127.0.0.1:$PORT" \
      SERIALHUB_DOWNLOAD_BASE="http://127.0.0.1:$PORT/download" \
      SERIALHUB_INSTALL_DIR="$SBX/bin" \
      bash install.sh > "$LOG" 2>&1; then ok "L1 install.sh 退出码 0"; else fail "L1 install.sh 退出码非 0"; fi
  assert_contains "L1 提示 Latest release: v9.9.9"   "$LOG" 'Latest release: v9\.9\.9'
  assert_contains "L1 提示 Installation complete"     "$LOG" 'Installation complete'
  assert_exists "$SBX/bin/serialhub" "L1 二进制已安装"
  local V; V=$(probe_version "$SBX/bin/serialhub" --version)
  case "$V" in 9.9.9*) ok "L1 安装产物 --version = $V";; *) fail "L1 版本不符: $V";; esac
  assert_contains "L1 非 PATH 目录给出提示" "$LOG" 'not in PATH'

  echo ">> [3b] Linux: 下载失败分支"
  local LOG2="$WORK/l2.log"; : > "$LOG2"
  timeout -k 5 "$T_CMD" env \
      SERIALHUB_GITHUB_API="http://127.0.0.1:$PORT" \
      SERIALHUB_DOWNLOAD_BASE="http://127.0.0.1:$PORT/download-missing" \
      SERIALHUB_INSTALL_DIR="$SBX/bin2" \
      bash install.sh > "$LOG2" 2>&1 && fail "L2 损坏下载应失败" || ok "L2 损坏下载退出码非 0"
  assert_contains "L2 报 Download failed" "$LOG2" 'Error: Download failed'
}

# ---------- 4. Windows 场景 ----------
run_windows() {
  echo ">> [4] Windows: install.ps1（interop 实跑）"
  local WSL_IP; WSL_IP=$(hostname -I | awk '{print $1}')
  [ -n "$WSL_IP" ] || { fail "取不到 WSL IP"; return; }
  mkdir -p "$WIN_SB"
  local WIN_SB_WIN; WIN_SB_WIN=$(timeout -k 5 20 wslpath -w "$WIN_SB")
  # install.ps1 复制到 /mnt/d 沙箱再用 powershell -Command 内设环境变量调用：
  # cmd /c 嵌套引号会毁掉 -File 的 UNC 路径（「路径中有非法字符」），绕开两者。
  cp install.ps1 "$WIN_SB/install.ps1"
  local PS_WIN; PS_WIN=$(timeout -k 5 20 wslpath -w "$WIN_SB/install.ps1")

  echo ">> [4a] 停 Windows 侧 serialhub（install.ps1 有运行实例检查）"
  # 安全阀：存在监听非 :5051 端口的 serialhub 实例时跳过本场景（无差别杀会
  # 破坏用户其他实例且无法知其原样恢复）；本测试只认 :5051 部署态。
  local OTHER="" P p LP port
  # tasklist CSV 首字段残留开头引号（-F'","' 只剥中间分隔），须先 gsub 剥净
  local OTHER="" P p LP port TL_RAW TLRC
  TL_RAW=$(timeout -k 5 30 tasklist.exe /FO CSV /NH 2>/dev/null); TLRC=$?
  # rc=1 既可能是「无匹配进程」的正常返回，也可能是 interop 瞬时故障；
  # 二次采样一致才采信，避免误判「无实例」后直通 taskkill。
  if [ "$TLRC" -eq 1 ]; then
    sleep 1
    local TL_RAW2 TLRC2
    TL_RAW2=$(timeout -k 5 30 tasklist.exe /FO CSV /NH 2>/dev/null); TLRC2=$?
    if [ "$TLRC2" -ne "$TLRC" ]; then
      fail "tasklist 两次采样不一致（rc=$TLRC/$TLRC2），跳过安装场景以免误杀"
      return 1
    fi
  elif [ "$TLRC" -ne 0 ]; then
    fail "tasklist 枚举异常（rc=$TLRC），跳过安装场景以免误杀"
    return 1
  fi
  P=$(printf '%s\n' "$TL_RAW" | tr -d '\r' | awk -F'","' '{gsub(/^"/,"",$1)} $1=="serialhub.exe"{gsub(/"/,"",$2);print $2}')
  if [ "$TLRC" -eq 0 ] && [ -z "$P" ]; then
    fail "tasklist 有输出但未解析到进程，解析不可信，跳过安装场景以免误杀"
    return 1
  fi
  for p in $P; do
    local NS_RAW NSRC
    NS_RAW=$(timeout -k 5 30 netstat.exe -ano 2>/dev/null); NSRC=$?
    if [ "$NSRC" -ne 0 ]; then
      fail "netstat 枚举异常（rc=$NSRC），跳过安装场景以免误杀"
      return 1
    fi
    LP=$(printf '%s\n' "$NS_RAW" | tr -d '\r' | awk -v p="$p" '$4=="LISTENING" && $5==p {print $2}' | sed 's/.*://' | sort -u)
    for port in $LP; do [ "$port" != "5051" ] && OTHER="$OTHER $port"; done
  done
  if [ -n "$OTHER" ]; then
    fail "Windows 侧存在非 :5051 监听的 serialhub 实例（端口:$OTHER），跳过安装场景以免误杀"
    return 1
  fi
  local NS5_RAW NS5RC
  NS5_RAW=$(timeout -k 5 30 netstat.exe -ano 2>/dev/null); NS5RC=$?
  if [ "$NS5RC" -ne 0 ]; then
    fail "netstat 枚举异常（rc=$NS5RC），无法记录原 :5051 实例，跳过安装场景以免误杀"
    return 1
  fi
  WAS_5051_PID=$(printf '%s\n' "$NS5_RAW" | tr -d '\r' | grep ':5051 .*LISTENING' | awk '{print $5}' | head -1)
  if [ -n "$WAS_5051_PID" ]; then ok "记录原 :5051 实例（PID $WAS_5051_PID），测试后恢复"; else ok "原无 :5051 实例，测试后不恢复"; fi
  timeout -k 5 30 cmd.exe /c "taskkill /IM serialhub.exe /F" >/dev/null 2>&1 || true
  sleep 2

  echo ">> [4b] 沙箱安装"
  local LOG="$WORK/w2.log"; : > "$LOG"
  if timeout -k 5 "$T_CMD" powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "\$env:SERIALHUB_GITHUB_API='http://$WSL_IP:$PORT'; \$env:SERIALHUB_DOWNLOAD_BASE='http://$WSL_IP:$PORT/download'; \$env:SERIALHUB_INSTALL_DIR='$WIN_SB_WIN'; & '$PS_WIN'" > "$LOG" 2>&1; then
    ok "W1 install.ps1 退出码 0"
  else
    fail "W1 install.ps1 退出码非 0"
  fi
  assert_contains "W1 提示 Latest release: v9.9.9"   "$LOG" 'Latest release: v9\.9\.9'
  assert_contains "W1 提示 Installation complete"     "$LOG" 'Installation complete'
  assert_contains "W1 自定义目录不写 PATH（给出提示）" "$LOG" 'not on your PATH'
  assert_exists "$WIN_SB/serialhub.exe" "W1 exe 已安装"
  assert_exists "$WIN_SB/sr.ps1"        "W1 启动脚本 sr.ps1 已复制"
  assert_exists "$WIN_SB/sr.bat"        "W1 启动脚本 sr.bat 已复制"
  local V; V=$(probe_version "$WIN_SB/serialhub.exe" --version)
  case "$V" in 9.9.9*) ok "W1 安装产物 --version = $V";; *) fail "W1 版本不符: $V";; esac

  echo ">> [4c] 用户 PATH 注册表未被污染"
  # reg.exe 轻量不易挂（powershell 从 interop 偶发挂起且 timeout 杀不掉）
  local REGOUT REGRC
  REGOUT=$(timeout -k 5 30 reg.exe query "HKCU\Environment" /v Path 2>/dev/null); REGRC=$?
  if [ "$REGRC" -ne 0 ]; then
    fail "W2 reg query 失败（rc=$REGRC），无法验证 PATH"
  else
    REGOUT=$(printf '%s' "$REGOUT" | tr -d '\r')
    if printf '%s' "$REGOUT" | grep -qi "install-test"; then fail "W2 用户 PATH 被污染"; else ok "W2 用户 PATH 干净"; fi
  fi

  echo ">> [4d] 恢复部署态（原有 :5051 实例才恢复）"
  restore_windows_instance
  sleep 5
  local NS3_RAW NS3RC NOW5051
  NS3_RAW=$(timeout -k 5 30 netstat.exe -ano 2>/dev/null); NS3RC=$?
  if [ "$NS3RC" -ne 0 ]; then
    fail "W3 netstat 验证异常（rc=$NS3RC），无法确认恢复状态"
  else
    NOW5051=$(printf '%s\n' "$NS3_RAW" | tr -d '\r' | grep ':5051 .*LISTENING' | awk '{print $5}' | head -1)
    if [ -n "$WAS_5051_PID" ]; then
      [ -n "$NOW5051" ] && ok "W3 Windows 实例已恢复（5051 LISTENING）" || fail "W3 Windows 实例恢复失败"
    else
      [ -z "$NOW5051" ] && ok "W3 原无实例且未被误启" || fail "W3 原无实例却出现监听"
    fi
  fi

  # [4e] 字符串执行路径等价验证：irm|iex 场景 BOM 会混入首行导致解析错乱
  # （文件执行路径 PS 自动剥 BOM，测不出此问题），故对 install.ps1 副本做
  # 字节级无 BOM 断言 + Parser::ParseInput 等价 iex 首步解析 0 错误。
  echo ">> [4e] install.ps1 字符串执行路径（irm|iex 等价）验证"
  local ERRS BOM3
  ERRS=$(timeout -k 5 30 powershell.exe -NoProfile -Command "\$t='$(to_win "$WORK/install.ps1")'; \$b=[System.IO.File]::ReadAllBytes(\$t); if (\$b[0] -eq 0xEF -and \$b[1] -eq 0xBB -and \$b[2] -eq 0xBF) { 'BOM-DETECTED' } else { 'NO-BOM' }; \$e=\$null; [void][System.Management.Automation.Language.Parser]::ParseInput([System.IO.File]::ReadAllText(\$t), [ref]\$null, [ref]\$e); 'PARSE-ERRORS=' + \$e.Count" 2>/dev/null | tr -d '\r')
  case "$ERRS" in
    *NO-BOM*) ok "E1 install.ps1 无 UTF-8 BOM（irm|iex 兼容）" ;;
    *BOM-DETECTED*) fail "E1 install.ps1 带 UTF-8 BOM，irm|iex 字符串执行会解析错乱" ;;
    *) fail "E1 install.ps1 BOM 检查异常（输出: $ERRS）" ;;
  esac
  case "$ERRS" in
    *PARSE-ERRORS=0*) ok "E2 install.ps1 字符串解析 0 错误（iex 等价）" ;;
    *) fail "E2 install.ps1 字符串解析异常（输出: $ERRS）" ;;
  esac
}

[ "$MODE" != linux ]   && run_windows
[ "$MODE" != windows ] && run_linux

echo
echo "==== install 测试汇总: PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -eq 0 ]
