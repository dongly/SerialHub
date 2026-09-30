#!/usr/bin/env bash
# tests/test-deployed.sh — 编译产物 upgrade/uninstall 端到端测试
#
# 在 WSL 下交叉编译 Windows 产物，经 interop 在真实 Windows 侧以沙箱用户目录
# （USERPROFILE/LOCALAPPDATA/APPDATA 重定向，与安装目录分离）执行，验证：
#   A. uninstall dry-run：版本行、随包清单、对侧实例探活提示不拦截、答 n 取消
#   B. uninstall -y：延迟自删、随包清理、空目录回收、用户文件/MCP 条目保留与清理、锁目录清理
#   C. upgrade（mock GitHub API）：从→到版本显示、版本切换、.old 延迟删除、配置保留
#   D. 弯引号路径（U+2019）下的延迟自删与目录回收
#
# 布局约定（与产品语义一致，见 uninstall.go / mcpsetup.go）：
#   安装目录：exe + 启动脚本 + 随包文档 + .old 残留 + 平铺 config.toml + logs/
#   数据沙箱：home/.config/opencode/opencode.json（V2 mcp.servers）+ local/serialhub 锁目录
#
# 所有外部命令均包 timeout（含 -k 强杀），另有全局看门狗兜底：任何环节挂死，
# 脚本都会超时退出并清理，不会无限等待。
#
# 用法: ./tests/test-deployed.sh [windows|linux|all]（默认 all）
# 依赖: 公共——timeout + python3(zipfile/tarfile) + unshare（linux 场景网络隔离与探活验证）；
#       windows 场景另需 WSL + cmd.exe interop + /mnt/d 可写（非 WSL 自动降级仅 linux）
set -u

# 用法：test-deployed.sh [windows|linux|all]——默认 all（WSL interop 跑 Windows 产物 + 本机跑 Linux 产物）
MODE="${1:-all}"
case "$MODE" in
  windows|linux|all) ;;
  *) echo "用法: $0 [windows|linux|all]"; exit 2 ;;
esac

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK=/tmp/opencode/deployed
SB_BASE=/mnt/d/DevTools/serialhub-deployed-test-$$
PORT=$((18800 + $$ % 100))
FAILS=0
MOCK_PID=""
WATCHDOG=""

# 超时参数（秒）：单条 Windows 命令 / go build / curl / 全局看门狗
T_CMD=180
T_BUILD=300
T_CURL=15
T_TOTAL=900

CLEANED=0
cleanup() { # 幂等：EXIT 与 INT/TERM 双触发只清一次；杀悬挂子进程防泄漏
  [ "$CLEANED" = 1 ] && return; CLEANED=1
  [ -n "$WATCHDOG" ] && kill "$WATCHDOG" 2>/dev/null
  [ -n "$MOCK_PID" ] && kill "$MOCK_PID" 2>/dev/null
  pkill -TERM -P $$ 2>/dev/null
  timeout -k 5 20 rm -rf "$SB_BASE"* "$WORK/stage" 2>/dev/null
  return 0
}
trap cleanup EXIT
trap 'trap - EXIT; cleanup; exit 143' INT TERM

# 全局看门狗：总时长超限则先 TERM 直接子进程（含挂死的 cmd.exe），
# 再 TERM 主 shell，EXIT trap 执行清理——任何环节挂死都能退出。
( sleep "$T_TOTAL" && pkill -TERM -P $$ 2>/dev/null; kill -TERM $$ ) &
WATCHDOG=$!

pass() { echo "  PASS  $1"; }
fail() { echo "  FAIL  $1"; FAILS=$((FAILS+1)); }
assert_contains() { # desc haystack needle（字面量子串）
  if printf '%s' "$2" | grep -qF "$3"; then pass "$1"; else fail "$1（输出缺少: $3）"; fi
}
assert_regex() { # desc haystack ere（兼容中英文输出）
  if printf '%s' "$2" | grep -qE "$3"; then pass "$1"
  else fail "$1（输出未匹配: $3）"; printf '%s\n' "$2" | head -8 | sed 's/^/    │ /' >&2; fi
}
assert_exists() { if [ -e "$2" ]; then pass "$1"; else fail "$1（不存在: $2）"; fi; }
assert_gone()   { if [ ! -e "$2" ]; then pass "$1"; else fail "$1（仍存在: $2）"; fi; }
probe_version() { # $@=版本命令；输出裸版本（剥 SerialHub/v 前缀，免疫 stderr 噪音行）
  "$@" 2>&1 | tr -d '\r' | grep -oE 'SerialHub v?[0-9][^ ]*' | head -1 | sed 's/^SerialHub v\?//'
}

# ---------- 0. 环境检查 ----------
echo "=== 编译产物 upgrade/uninstall 端到端测试（总超时 ${T_TOTAL}s）==="
command -v timeout >/dev/null 2>&1 || { echo "需要 timeout（coreutils）"; exit 1; }
# WSL 检测：Windows 产物测试依赖 WSL interop（cmd.exe + /mnt/d）
if ! grep -qi microsoft /proc/version 2>/dev/null; then
  if [ "$MODE" = windows ]; then
    echo "不在 WSL 环境中，无法测试 Windows 产物（需要 cmd.exe 与 /mnt/d）"; exit 2
  elif [ "$MODE" = all ]; then
    echo "不在 WSL 环境中，跳过 Windows 产物测试，仅测试 Linux 产物"
    MODE=linux
  fi
fi
if [ "$MODE" != linux ]; then
  command -v cmd.exe >/dev/null 2>&1 || { echo "需要 WSL interop（cmd.exe 不可用）"; exit 1; }
  [ -d /mnt/d ] || { echo "需要 /mnt/d 挂载"; exit 1; }
fi
mkdir -p "$WORK"
WSL_IP=$(hostname -I | awk '{print $1}')

# ---------- 1. 编译 ----------
echo "--- [1/6] 编译产物 ---"
if [ "$MODE" != linux ]; then
  (cd "$ROOT" && GOOS=windows timeout -k 10 "$T_BUILD" go build -ldflags "-s -w" -o "$WORK/serialhub.exe" ./cmd/serialhub) || { echo "编译失败"; exit 1; }
  (cd "$ROOT" && GOOS=windows timeout -k 10 "$T_BUILD" go build -ldflags "-s -w -X github.com/dongly/serialhub/pkg/version.Version=9.9.9" -o "$WORK/fake999.exe" ./cmd/serialhub) || { echo "伪造版本编译失败"; exit 1; }
  pass "编译 Windows 当前版本与伪造 9.9.9 版本"
fi
if [ "$MODE" != windows ]; then
  (cd "$ROOT" && timeout -k 10 "$T_BUILD" go build -ldflags "-s -w -X github.com/dongly/serialhub/pkg/version.Version=9.9.9" -o "$WORK/fake999-linux" ./cmd/serialhub) || { echo "Linux 伪造版本编译失败"; exit 1; }
  pass "编译 Linux 伪造 9.9.9 版本"
fi

# ---------- 2. mock GitHub API + 伪造 release ----------
echo "--- [2/6] mock GitHub API ---"
# 资产名与 upgrade.go 一致：去 v 前缀的裸版本（serialhub-9.9.9-windows-amd64.zip）
cat > "$WORK/mockapi.py" <<PYEOF
import os, sys
from http.server import BaseHTTPRequestHandler, HTTPServer
WORK, BASE = sys.argv[1], sys.argv[2]
assets = []
for name in ("serialhub-9.9.9-windows-amd64.zip", "serialhub-9.9.9-linux-amd64.tar.gz"):
    if os.path.exists(WORK + "/" + name):
        assets.append({"name": name, "browser_download_url": BASE + "/dl/" + name})
        assets.append({"name": name + ".sha256", "browser_download_url": BASE + "/dl/" + name + ".sha256"})
LATEST = {"tag_name": "v9.9.9", "assets": assets}
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/repos/dongly/SerialHub/releases/latest":
            import json; body = json.dumps(LATEST).encode()
        elif self.path.startswith("/dl/"):
            body = open(WORK + "/" + self.path[4:], "rb").read()
        else:
            self.send_error(404); return
        self.send_response(200)
        self.send_header("Content-Type", "application/octet-stream")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *a): pass
HTTPServer(("0.0.0.0", int(sys.argv[3])), H).serve_forever()
PYEOF

if [ "$MODE" != linux ]; then
python3 - "$WORK" <<'PYEOF'
import sys, zipfile, hashlib, os, shutil
work = sys.argv[1]
stage = work + "/stage/serialhub-9.9.9-windows-amd64"
os.makedirs(stage, exist_ok=True)
shutil.copy(work + "/fake999.exe", stage + "/serialhub.exe")
zpath = work + "/serialhub-9.9.9-windows-amd64.zip"
with zipfile.ZipFile(zpath, "w") as z:
    z.write(stage + "/serialhub.exe", "serialhub-9.9.9-windows-amd64/serialhub.exe")
h = hashlib.sha256(open(zpath, "rb").read()).hexdigest()
open(zpath + ".sha256", "w").write(h + "  serialhub-9.9.9-windows-amd64.zip\n")
PYEOF
fi
if [ "$MODE" != windows ]; then
python3 - "$WORK" <<'PYEOF'
import sys, tarfile, hashlib, os, shutil
work = sys.argv[1]
stage = work + "/stage-l/serialhub-9.9.9-linux-amd64"
os.makedirs(stage, exist_ok=True)
shutil.copy(work + "/fake999-linux", stage + "/serialhub")
os.chmod(stage + "/serialhub", 0o755)
tpath = work + "/serialhub-9.9.9-linux-amd64.tar.gz"
with tarfile.open(tpath, "w:gz") as t:
    t.add(stage + "/serialhub", arcname="serialhub-9.9.9-linux-amd64/serialhub")
h = hashlib.sha256(open(tpath, "rb").read()).hexdigest()
open(tpath + ".sha256", "w").write(h + "  serialhub-9.9.9-linux-amd64.tar.gz\n")
PYEOF
fi
python3 "$WORK/mockapi.py" "$WORK" "http://$WSL_IP:$PORT" "$PORT" &
MOCK_PID=$!
mock_ready=""
for _ in 1 2 3 4 5; do
  if timeout -k 5 "$T_CURL" curl -fsS -m 10 "http://127.0.0.1:$PORT/repos/dongly/SerialHub/releases/latest" 2>/dev/null | grep -q 9.9.9; then
    mock_ready=1; break
  fi
  sleep 1
done
if [ -n "$mock_ready" ]; then
  pass "mock API 就绪（$WSL_IP:$PORT）"
else
  fail "mock API 未就绪"
fi

# ---------- 辅助 ----------
to_win() { timeout -k 5 20 wslpath -w "$1"; }
mk_layout() { # mk_layout <install-dir> <data-dir>
  # 安装目录＝产品清理对象；数据沙箱＝USERPROFILE/LOCALAPPDATA 重定向目标，二者分离
  local d="$1" data="$2"
  mkdir -p "$d" "$data/home/.config/opencode" "$data/local"
  cp "$WORK/serialhub.exe" "$d/serialhub.exe"
  for f in sr.ps1 sr.bat README.md README.en.md QUICKSTART.md MCP.md LICENSE VERSION; do
    echo "stub" > "$d/$f"
  done
  echo "stub" > "$d/serialhub.exe.old-0.5.0"
  # Windows 布局：exe 同目录平铺 config.toml + logs/（uninstall.go Windows 分支）
  printf '[serial]\nport = 5050\n' > "$d/config.toml"
  mkdir -p "$d/logs"; echo "log" > "$d/logs/serialhub.log"
  # OpenCode V2 mcp.servers 嵌套结构（mcpsetup.go）；os.UserHomeDir 沙箱内解析
  printf '{"mcp":{"servers":{"serialhub":{"command":"serialhub"},"other":{"command":"keepme"}}}}' > "$data/home/.config/opencode/opencode.json"
  mkdir -p "$data/local/serialhub"; echo lock > "$data/local/serialhub/instance.lock"
}
run_sandbox() { # run_sandbox <data-dir> <stdin> <cmd...>：环境重定向到数据沙箱，全部经 timeout 防挂死
  local data="$1" stdin="$2"; shift 2
  local dw; dw=$(to_win "$data")
  printf '%s' "$stdin" | timeout -k 5 "$T_CMD" cmd.exe /c "cd /d $dw&&set USERPROFILE=$dw\\home&&set LOCALAPPDATA=$dw\\local&&set APPDATA=$dw\\appdata&& $*" 2>&1 | tr -d '\r'
}
wait_delayed() { sleep 12; }  # PowerShell 延迟删除：2s 起步 + 3s×重试，12s 足够收敛

run_windows() {
# ---------- 0. 仓库启动脚本元数据（防 EOL/BOM 回归，直接对仓库文件断言） ----------
echo "--- [0/6] Windows 启动脚本元数据 ---"
if [ "$(grep -c $'\r' "$ROOT/sr.bat" 2>/dev/null || true)" -eq 0 ]; then
  fail "B0 sr.bat 应为 CRLF（cmd 兼容），当前无 CR"
else
  pass "B0 sr.bat CRLF"
fi
B3=$(head -c 3 "$ROOT/sr.bat" 2>/dev/null | od -An -tx1 | tr -d ' \n')
if [ "$B3" = "efbbbf" ]; then
  fail "B1 sr.bat 不应带 BOM（cmd 不剥 BOM，会拼进首行）"
else
  pass "B1 sr.bat 无 BOM"
fi
if [ "$(grep -c $'\r' "$ROOT/sr.ps1" 2>/dev/null || true)" -eq 0 ]; then
  fail "B2 sr.ps1 应为 CRLF（.editorconfig ps1 规则）"
else
  pass "B2 sr.ps1 CRLF"
fi
B4=$(head -c 3 "$ROOT/sr.ps1" 2>/dev/null | od -An -tx1 | tr -d ' \n')
if [ "$B4" = "efbbbf" ]; then
  pass "B3 sr.ps1 带 UTF-8 BOM（PS 5.1 兼容）"
else
  fail "B3 sr.ps1 应带 UTF-8 BOM"
fi
B5=$(head -c 3 "$ROOT/install.ps1" 2>/dev/null | od -An -tx1 | tr -d ' \n')
if [ "$B5" = "efbbbf" ]; then
  fail "B4 install.ps1 不应带 BOM（irm|iex 解析错乱，fix 8434e7b）"
else
  pass "B4 install.ps1 无 BOM"
fi
# ---------- 3. 场景 A：uninstall dry-run ----------
echo "--- [3/6] A. uninstall dry-run ---"
SB="$SB_BASE-a"; SBX="$SB_BASE-xa"; mk_layout "$SB" "$SBX"
OUT=$(run_sandbox "$SBX" "n
" "$(to_win "$SB")\\serialhub.exe uninstall")
assert_contains "A1 版本行" "$OUT" "[SerialHub] SerialHub v"
assert_regex   "A2 随包清单条目" "$OUT" '随包文件|Bundled install-dir'
if curl -fsS -m 5 http://127.0.0.1:5050/health >/dev/null 2>&1; then
  # 可达：探活命中对侧（WSL）实例 → 应打 remote 提示（对侧实例关键词+端口）
  assert_regex "A3 探活对侧实例不拦截（remote 提示）" "$OUT" '对侧实例.*5050|5050.*对侧实例|对侧实例|[Rr]emote instance'
else
  # 不可达：探活无实例 → 不拦截，正常进入确认（英文词条实为 Proceed）
  assert_regex "A3 无实例在跑时探活放行（出现确认提示，未拦截）" "$OUT" '确认执行卸载|[Pp]roceed'
fi
assert_regex   "A4 答 n 取消" "$OUT" '取消|[Cc]ancelled'
assert_exists  "A5 dry-run 未动文件（exe 仍在）" "$SB/serialhub.exe"

# ---------- 4. 场景 B：uninstall -y ----------
echo "--- [4/6] B. uninstall -y ---"
SB="$SB_BASE-a"; SBX="$SB_BASE-xa"
OUT=$(run_sandbox "$SBX" "" "$(to_win "$SB")\\serialhub.exe uninstall -y")
wait_delayed
assert_gone "B1 exe 延迟自删" "$SB/serialhub.exe"
assert_gone "B2 随包脚本清理（sr.ps1）" "$SB/sr.ps1"
assert_gone "B3 随包文档清理（README）" "$SB/README.md"
assert_gone "B4 升级残留清理（.old-0.5.0）" "$SB/serialhub.exe.old-0.5.0"
assert_gone "B5 平铺 config.toml 清理" "$SB/config.toml"
assert_gone "B6 logs 目录清理" "$SB/logs"
assert_gone "B7 安装目录回收（空目录移除）" "$SB"
assert_gone "B12 锁目录清理（LOCALAPPDATA\\serialhub）" "$SBX/local/serialhub"
if [ -f "$SBX/home/.config/opencode/opencode.json" ]; then
  if python3 - "$SBX/home/.config/opencode/opencode.json" <<'PYEOF'
import json, sys
d = json.load(open(sys.argv[1], encoding="utf-8"))
servers = d.get("mcp", {}).get("servers", {})
print("    serialhub 条目已清" if "serialhub" not in servers else "    serialhub 条目仍在")
print("    other 条目保留" if "other" in servers else "    other 条目被误删")
sys.exit(0 if "serialhub" not in servers and "other" in servers else 1)
PYEOF
  then pass "B10/B11 MCP serialhub 条目已清、其他条目保留"; else fail "B10/B11 MCP 条目清理异常"; fi
else
  fail "B10/B11 opencode.json 不存在"
fi

SB="$SB_BASE-b"; SBX="$SB_BASE-xb"; mk_layout "$SB" "$SBX"; echo "keep me" > "$SB/mynote.txt"
OUT=$(run_sandbox "$SBX" "" "$(to_win "$SB")\\serialhub.exe uninstall -y")
wait_delayed
assert_exists "B8 用户文件保留（mynote.txt）" "$SB/mynote.txt"
assert_exists "B9 非空目录保留" "$SB"

# ---------- 5. 场景 C：upgrade（mock API）----------
echo "--- [5/6] C. upgrade（mock GitHub API）---"
SB="$SB_BASE-c"; SBX="$SB_BASE-xc"; mk_layout "$SB" "$SBX"
rm -f "$SB/serialhub.exe.old-0.5.0"  # 排除 stub 残留干扰：C5 只验证 upgrade 自身产生的 .old
SBW="$(to_win "$SB")"
XW="$(to_win "$SBX")"
CURV=$(probe_version timeout -k 5 "$T_CMD" cmd.exe /c "$SBW\\serialhub.exe --version")
[ -n "$CURV" ] || fail "C2 升级前版本探测失败"
CURV_RE=${CURV//./\\.}  # regex 中 '.' 转义，避免宽匹配误配
OUT=$(timeout -k 5 "$T_CMD" cmd.exe /c "set USERPROFILE=$XW\\home&&set LOCALAPPDATA=$XW\\local&&set APPDATA=$XW\\appdata&&set SERIALHUB_GITHUB_API=http://$WSL_IP:$PORT&& $SBW\\serialhub.exe upgrade" 2>&1 | tr -d '\r')
assert_contains "C1 upgrade 执行输出" "$OUT" "9.9.9"
assert_regex "C2 从→到版本显示（$CURV → 9.9.9）" "$OUT" "${CURV_RE}.*→.*9\.9\.9"
VOUT=$(timeout -k 5 "$T_CMD" cmd.exe /c "$SBW\\serialhub.exe --version" 2>&1 | tr -d '\r')
assert_contains "C3 版本升级为 9.9.9" "$VOUT" "9.9.9"
assert_exists "C4 配置保留（config.toml）" "$SB/config.toml"
wait_delayed
OLD_GONE=1
for f in "$SB"/serialhub.exe.old-* "$SB"/serialhub.old-*; do [ -e "$f" ] && OLD_GONE=0; done
[ "$OLD_GONE" = 1 ] && pass "C5 .old 残留延迟清理" || fail "C5 .old 残留仍存在"

# ---------- 6. 场景 D：弯引号路径 ----------
echo "--- [6/6] D. 弯引号路径（U+2019）延迟自删 ---"
SB="$SB_BASE-don’t"; SBX="$SB_BASE-xd"
mkdir -p "$SB" "$SBX"
cp "$WORK/serialhub.exe" "$SB/serialhub.exe"
OUT=$(run_sandbox "$SBX" "" "$(to_win "$SB")\\serialhub.exe uninstall -y")
wait_delayed
assert_gone "D1 弯引号路径 exe 延迟自删" "$SB/serialhub.exe"
assert_gone "D2 弯引号路径目录回收" "$SB"
}

# Linux 布局：安装目录（exe+sh+README+VERSION+old 残留+平铺 config+用户文件）
# + 沙箱 home（伪造 opencode.json serialhub/other 键）+ 沙箱 xdg（serialhub config 5999）
mk_layout_linux() { # (install_dir, sandbox_dir)
  local inst="$1" x="$2"
  mkdir -p "$inst" "$x/home/.config/opencode" "$x/xdg/serialhub"
  cp "$WORK/dist-linux/serialhub" "$inst/serialhub" && chmod +x "$inst/serialhub"
  printf '#!/bin/sh\n' > "$inst/serialhub.sh"
  printf 'stub\n' > "$inst/README.md"
  printf '0.5.0\n' > "$inst/VERSION"
  printf 'old\n' > "$inst/serialhub.old-0.5.0"
  printf 'note\n' > "$inst/mynote.txt"
  printf 'mcpPort = 5999\n' > "$inst/config.toml"
  printf 'mcpPort = 5999\n' > "$x/xdg/serialhub/config.toml"
  printf '{"mcp":{"servers":{"serialhub":{"type":"http","url":"http://127.0.0.1:5050/mcp"},"other":{"command":"x"}}}}\n' > "$x/home/.config/opencode/opencode.json"
}

run_linux() {
  echo "--- Linux 产物（真实文件系统，沙箱 HOME/XDG）---"
  mkdir -p "$WORK/dist-linux"
  (cd "$ROOT" && timeout -k 10 "$T_BUILD" go build -o "$WORK/dist-linux/serialhub" ./cmd/serialhub) || { fail "E0 Linux 当前版本编译"; return; }
  head -c 4 "$WORK/dist-linux/serialhub" | od -An -tx1 | grep -q '7f 45 4c 46' && pass "E0 ELF 产物" || { fail "E0 非 ELF 产物"; return; }

  # Linux 场景不依赖 Windows 文件系统：沙箱一律用本机临时目录（非 WSL 纯 Linux 也可跑）
  local SB_BASE=/tmp/opencode/deployed-linux-$$
  local SB SBX OUT VOUT RC
  # E1 探活拦截：沙箱无 config → 探默认 5050；本机（WSL）实例与探测进程同侧 → 拒绝卸载
  SBX="$SB_BASE-xe1"; rm -rf "$SBX"; mkdir -p "$SBX/home"
  # 本机 5050 无实例时自动起临时实例（当前代码产物，配置沙箱隔离），保证拦截语义总是被验证
  local E1_PID="" i
  if ! timeout -k 5 "$T_CURL" curl -fsS -m 3 http://127.0.0.1:5050/health >/dev/null 2>&1; then
    env HOME="$SBX/home" XDG_CONFIG_HOME="$SBX/xdg" setsid nohup "$WORK/dist-linux/serialhub" --minimized --no-browser -m 5050 >/dev/null 2>&1 &
    E1_PID=$!
    for i in $(seq 1 20); do
      timeout -k 5 "$T_CURL" curl -fsS -m 2 http://127.0.0.1:5050/health >/dev/null 2>&1 && break
      sleep 0.5
    done
  fi
  OUT=$(timeout -k 5 "$T_CMD" env HOME="$SBX/home" XDG_CONFIG_HOME="$SBX/xdg" "$WORK/dist-linux/serialhub" uninstall -y 2>&1); RC=$?
  assert_regex "E1 同侧实例在跑 → 拒绝卸载" "$OUT" '正在运行|running'
  [ "$RC" != 0 ] && pass "E1 拒绝时退出码非 0" || fail "E1 拒绝时退出码应为非 0"
  [ -n "$E1_PID" ] && kill "$E1_PID" 2>/dev/null

  # E2 dry-run：沙箱 config（mcpPort=5999）→ 不拦 + 版本行 + 随包清单
  # unshare -rn：独立网络命名空间，探活零命中（真实 5050 实例不可见），专测清单与确认流程
  SB="$SB_BASE-linux"; SBX="$SB_BASE-xe2"; mk_layout_linux "$SB" "$SBX"
  OUT=$(unshare -rn timeout -k 5 "$T_CMD" env HOME="$SBX/home" XDG_CONFIG_HOME="$SBX/xdg" "$SB/serialhub" uninstall 2>&1 </dev/null)
  assert_regex "E2 版本行显示" "$OUT" 'SerialHub v'
  assert_regex "E2 随包清单条目" "$OUT" '随包文件|[Bb]undled'

  # E3 uninstall -y：随包清理 + ELF 即时自删 + xdg 清理 + MCP 条目 + 保留项（同 unshare 隔离探活）
  OUT=$(unshare -rn timeout -k 5 "$T_CMD" env HOME="$SBX/home" XDG_CONFIG_HOME="$SBX/xdg" "$SB/serialhub" uninstall -y 2>&1)
  assert_gone "E3 随包脚本清理（.sh）" "$SB/serialhub.sh"
  assert_gone "E3 随包文档清理（README）" "$SB/README.md"
  assert_gone "E3 升级残留清理（.old）" "$SB/serialhub.old-0.5.0"
  assert_gone "E3 ELF 自删（Linux 即时 unlink）" "$SB/serialhub"
  assert_exists "E3 平铺 config.toml 保留（Linux 不删安装目录配置）" "$SB/config.toml"
  assert_exists "E3 用户文件保留（mynote.txt）" "$SB/mynote.txt"
  assert_exists "E3 非空安装目录保留" "$SB"
  assert_gone "E3 xdg 配置目录清理" "$SBX/xdg/serialhub"
  if python3 -c "
import json, sys
d = json.load(open(sys.argv[1], encoding='utf-8'))
servers = d.get('mcp', {}).get('servers', {})
sys.exit(0 if 'serialhub' not in servers and 'other' in servers else 1)
" "$SBX/home/.config/opencode/opencode.json"; then
    pass "E3 MCP serialhub 条目已清、其他条目保留"
  else
    fail "E3 MCP 条目清理异常"
  fi

  # E4 upgrade（mock 127.0.0.1 直达，无需探活隔离）：tar.gz + sha256 → 9.9.9
  SB="$SB_BASE-linux2"; SBX="$SB_BASE-xe4"; mk_layout_linux "$SB" "$SBX"
  rm -f "$SB/serialhub.old-0.5.0"  # 排除布局 stub 干扰：只验证 upgrade 自身是否产生 .old
  CURV=$(probe_version timeout -k 5 "$T_CMD" "$SB/serialhub" --version)
  [ -n "$CURV" ] || fail "E4 升级前版本探测失败"
  CURV_RE=${CURV//./\\.}  # regex 中 '.' 转义，避免宽匹配误配
  OUT=$(timeout -k 5 "$T_CMD" env HOME="$SBX/home" XDG_CONFIG_HOME="$SBX/xdg" SERIALHUB_GITHUB_API="http://127.0.0.1:$PORT" "$SB/serialhub" upgrade 2>&1)
  assert_contains "E4 upgrade 执行输出" "$OUT" "9.9.9"
  assert_regex "E4 从→到版本显示（$CURV → 9.9.9）" "$OUT" "${CURV_RE}.*→.*9\.9\.9"
  VOUT=$(timeout -k 5 "$T_CMD" "$SB/serialhub" --version 2>&1)
  assert_contains "E4 版本升级为 9.9.9" "$VOUT" "9.9.9"
  assert_exists "E4 xdg 配置保留" "$SBX/xdg/serialhub/config.toml"
  local old_gone=1 f
  for f in "$SB"/serialhub.old-* "$SB"/serialhub.exe.old-*; do [ -e "$f" ] && old_gone=0; done
  [ "$old_gone" = 1 ] && pass "E4 Linux 无 .old 残留（rename 直接覆盖）" || fail "E4 .old 残留仍存在"
  rm -rf "$SB_BASE"*  # 与全局 cleanup 同款 glob：覆盖 -linux*/-xe* 全部后缀变体
}

[ "$MODE" != linux ] && run_windows
[ "$MODE" != windows ] && run_linux

echo "=============================="
if [ "$FAILS" -eq 0 ]; then echo "全部通过"; exit 0; else echo "失败 $FAILS 项"; exit 1; fi
