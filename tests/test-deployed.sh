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
# 用法: ./tests/test-deployed.sh
# 依赖: WSL + cmd.exe interop + /mnt/d 可写 + python3(zipfile)
set -u

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
  if printf '%s' "$2" | grep -qE "$3"; then pass "$1"; else fail "$1（输出未匹配: $3）"; fi
}
assert_exists() { if [ -e "$2" ]; then pass "$1"; else fail "$1（不存在: $2）"; fi; }
assert_gone()   { if [ ! -e "$2" ]; then pass "$1"; else fail "$1（仍存在: $2）"; fi; }

# ---------- 0. 环境检查 ----------
echo "=== 编译产物 upgrade/uninstall 端到端测试（总超时 ${T_TOTAL}s）==="
command -v cmd.exe >/dev/null 2>&1 || { echo "需要 WSL interop（cmd.exe 不可用）"; exit 1; }
command -v timeout >/dev/null 2>&1 || { echo "需要 timeout（coreutils）"; exit 1; }
[ -d /mnt/d ] || { echo "需要 /mnt/d 挂载"; exit 1; }
mkdir -p "$WORK"
WSL_IP=$(hostname -I | awk '{print $1}')

# ---------- 1. 编译 ----------
echo "--- [1/6] 编译产物 ---"
(cd "$ROOT" && GOOS=windows timeout -k 10 "$T_BUILD" go build -ldflags "-s -w" -o "$WORK/serialhub.exe" ./cmd/serialhub) || { echo "编译失败"; exit 1; }
(cd "$ROOT" && GOOS=windows timeout -k 10 "$T_BUILD" go build -ldflags "-s -w -X github.com/dongly/serialhub/pkg/version.Version=9.9.9" -o "$WORK/fake999.exe" ./cmd/serialhub) || { echo "伪造版本编译失败"; exit 1; }
pass "编译当前版本与伪造 9.9.9 版本"

# ---------- 2. mock GitHub API + 伪造 release ----------
# 防御：产物必须是 Windows PE（MZ 头），否则 Windows 侧会弹
# “不支持的 16 位应用程序”错误窗并卡死 cmd.exe
for f in "$WORK/serialhub.exe" "$WORK/fake999.exe"; do
    if [ "$(head -c 2 "$f")" != "MZ" ]; then
        echo "FATAL: $f 不是 Windows PE（交叉编译 GOOS 错误）"
        exit 1
    fi
done

echo "--- [2/6] mock GitHub API ---"
# 资产名与 upgrade.go 一致：去 v 前缀的裸版本（serialhub-9.9.9-windows-amd64.zip）
cat > "$WORK/mockapi.py" <<PYEOF
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
WORK, BASE = sys.argv[1], sys.argv[2]
ZIP = "serialhub-9.9.9-windows-amd64.zip"
LATEST = {"tag_name": "v9.9.9", "assets": [
    {"name": ZIP, "browser_download_url": BASE + "/dl/" + ZIP},
    {"name": ZIP + ".sha256", "browser_download_url": BASE + "/dl/" + ZIP + ".sha256"},
]}
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
  for f in serialhub.ps1 serialhub.bat README.md README.en.md QUICKSTART.md MCP.md LICENSE VERSION; do
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
assert_gone "B2 随包脚本清理（ps1）" "$SB/serialhub.ps1"
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
OUT=$(timeout -k 5 "$T_CMD" cmd.exe /c "set USERPROFILE=$XW\\home&&set LOCALAPPDATA=$XW\\local&&set APPDATA=$XW\\appdata&&set SERIALHUB_GITHUB_API=http://$WSL_IP:$PORT&& $SBW\\serialhub.exe upgrade" 2>&1 | tr -d '\r')
assert_contains "C1 upgrade 执行输出" "$OUT" "9.9.9"
assert_contains "C2 从→到版本显示（→ 9.9.9）" "$OUT" "→ 9.9.9"
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

echo "=============================="
if [ "$FAILS" -eq 0 ]; then echo "全部通过"; exit 0; else echo "失败 $FAILS 项"; exit 1; fi
