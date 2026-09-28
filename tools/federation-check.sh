#!/usr/bin/env bash
# federation-check.sh — SerialHub 联邦状态巡检（Linux/WSL 侧）
# 用法: ./federation-check.sh [--port N]... [--scan] [-h]
#   --port N   追加探测端口（可重复，1-65535）
#   --scan     额外扫描 5051-5059（实例掉主顺延端口的场景）
#   -h         显示本帮助
# 只读巡检：仅调用 /health 与 MCP serial_list，不连接、不写入串口。
# 可见性说明：本脚本仅探测当前环境的 127.0.0.1 候选端口，
# 不保证覆盖双侧实例（WSL2 网络模式会影响可见性）；
# 对侧实例请在 Windows 侧运行 tools/federation-check.ps1 验证。
set -u

PROG="federation-check.sh"
CURL_T=3
MCP_ACCEPT='Accept: application/json, text/event-stream'

for dep in curl python3 awk; do
  command -v "$dep" >/dev/null 2>&1 || { echo "[federation-check] 缺少依赖: $dep"; exit 3; }
done

EXTRA_PORTS=""
SCAN=0
while [ $# -gt 0 ]; do
  case "$1" in
    --port)
      [ $# -ge 2 ] || { echo "[federation-check] --port 缺少参数值"; exit 3; }
      case "$2" in ''|*[!0-9]*) echo "[federation-check] 无效端口: $2"; exit 3 ;; esac
      # 先限长度再比较，避免超长数字触发算术表达式错误
      if [ "${#2}" -gt 5 ] || [ "$2" -lt 1 ] || [ "$2" -gt 65535 ]; then
        echo "[federation-check] 端口超出范围 1-65535: $2"; exit 3
      fi
      EXTRA_PORTS="$EXTRA_PORTS $2"; shift 2 ;;
    --scan) SCAN=1; shift ;;
    -h|--help) sed -n '2,9p' "$0"; exit 0 ;;
    *) echo "[federation-check] 未知参数: $1（-h 查看用法）"; exit 3 ;;
  esac
done

# ---- 端口发现: 默认 5050 + 本侧配置文件 [MCP] 段 HTTPPort ----
# XDG_CONFIG_HOME 仅绝对路径生效（与应用内 xdgConfigDir 语义一致）
xdg_base="$HOME/.config"
if [ -n "${XDG_CONFIG_HOME:-}" ] && [ "${XDG_CONFIG_HOME#/}" != "${XDG_CONFIG_HOME:-}" ]; then
  xdg_base="$XDG_CONFIG_HOME"
fi
cfg="$xdg_base/serialhub/config.toml"
cfg_port=""
if [ -f "$cfg" ]; then
  cfg_port=$(python3 -c "
import sys, re
txt = open(sys.argv[1], encoding='utf-8', errors='replace').read()
# [MCP] 段内的 httpPort 键（大小写不敏感、允许缩进；兼容示例小写与 Save 输出大写）
m = re.search(r'^\\[(?i:mcp)\\][^\\[]*?^[ \\t]*(?i:httpport)[ \\t]*=[ \\t]*(\\d+)', txt, re.M | re.S)
print(m.group(1) if m else '')
" "$cfg" 2>/dev/null)
  if [ -n "$cfg_port" ]; then
    if [ "${#cfg_port}" -gt 5 ] || [ "$cfg_port" -lt 1 ] || [ "$cfg_port" -gt 65535 ]; then
      echo "[federation-check] 配置文件 HTTPPort 超出范围，忽略: $cfg_port"
      cfg_port=""
    fi
  fi
fi

PORTS="$cfg_port$EXTRA_PORTS 5050"
[ "$SCAN" -eq 1 ] && PORTS="$PORTS 5051 5052 5053 5054 5055 5056 5057 5058 5059"
# 去重保序
PORTS=$(printf '%s\n' $PORTS | awk '!seen[$0]++' | tr '\n' ' ')

# ---- MCP 调用: initialize → initialized → tools/call serial_list → DELETE ----
# 结果经全局变量返回:
#   MCP_STATUS = ok / err
#   MCP_PORTS  = TSV 行（name<TAB>origin<TAB>side），成功时可为空（零串口）
#   MCP_ERR    = 失败原因
# 成功与失败都会清理: 临时文件 + 有会话 ID 时 DELETE（RETURN trap 统一兜底，
# 覆盖所有 return 路径；服务器为无状态实现时残留风险低，尽力而为）。
mcp_serial_list() {
  local base="$1" hdr body sid code parsed status
  MCP_STATUS="err"; MCP_PORTS=""; MCP_ERR=""
  hdr=$(mktemp) || { MCP_ERR="mktemp 失败"; return 1; }
  body=$(mktemp) || { rm -f "$hdr"; MCP_ERR="mktemp 失败"; return 1; }
  sid=""
  trap 'if [ -n "${sid:-}" ]; then curl -s --max-time 3 -o /dev/null -X DELETE "$base/mcp" -H "Mcp-Session-Id: $sid" >/dev/null 2>&1 || true; fi; rm -f "$hdr" "$body"' RETURN

  code=$(curl -s --max-time $CURL_T -D "$hdr" -o "$body" -w '%{http_code}' -X POST "$base/mcp" \
    -H 'Content-Type: application/json' -H "$MCP_ACCEPT" \
    -d '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"federation-check","version":"1.0"}}}' 2>/dev/null) \
    || { MCP_ERR="initialize 请求失败"; return 1; }
  [ "$code" = "200" ] || { MCP_ERR="initialize HTTP $code"; return 1; }
  sid=$(awk 'tolower($1)=="mcp-session-id:"{print $2}' "$hdr" | tr -d '\r')

  # 无 session ID 的无状态实现: 后续请求不带头继续
  local sess=()
  [ -n "$sid" ] && sess=(-H "Mcp-Session-Id: $sid")

  curl -s --max-time $CURL_T -o /dev/null -X POST "$base/mcp" \
    -H 'Content-Type: application/json' -H "$MCP_ACCEPT" ${sess[@]+"${sess[@]}"} \
    -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' >/dev/null 2>&1 || true

  code=$(curl -s --max-time $CURL_T -o "$body" -w '%{http_code}' -X POST "$base/mcp" \
    -H 'Content-Type: application/json' -H "$MCP_ACCEPT" ${sess[@]+"${sess[@]}"} \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"serial_list","arguments":{}}}' 2>/dev/null) \
    || { MCP_ERR="tools/call 请求失败"; return 1; }
  [ "$code" = "200" ] || { MCP_ERR="tools/call HTTP $code"; return 1; }

  # 解析并校验: JSON-RPC error / 工具 isError / ports 数组类型；
  # 成功且列表为空输出零行（零串口是正常结果，不是故障）。
  parsed=$(python3 -c "
import json, sys
try:
    d = json.load(open(sys.argv[1], encoding='utf-8'))
except Exception:
    print('ERR\t响应不是有效 JSON'); sys.exit(0)
if isinstance(d, dict) and d.get('error'):
    msg = d['error'].get('message', d['error']) if isinstance(d['error'], dict) else d['error']
    print('ERR\tJSON-RPC 错误: %s' % msg); sys.exit(0)
r = d.get('result') if isinstance(d, dict) else None
r = r if isinstance(r, dict) else {}
if r.get('isError'):
    print('ERR\t工具返回错误'); sys.exit(0)
ports = (r.get('structuredContent') or {}).get('ports') if isinstance(r.get('structuredContent'), dict) else None
if not isinstance(ports, list):
    print('ERR\t响应缺少 ports 数组'); sys.exit(0)
lines = []
for p in ports:
    if not isinstance(p, dict):
        print('ERR\\tports 条目不是对象: %r' % (p,)); sys.exit(0)
    name = p.get('name'); origin = p.get('origin')
    if not isinstance(name, str) or not name:
        print('ERR\\tports 条目缺少有效 name'); sys.exit(0)
    if origin not in ('local', 'federated'):
        print('ERR\\tports 条目 origin 无效: %r' % (origin,)); sys.exit(0)
    side = p.get('side', '')
    lines.append('%s\\t%s\\t%s' % (name, origin, side if isinstance(side, str) else ''))
print('OK')
for line in lines:
    print(line)
" "$body" 2>/dev/null) || { MCP_ERR="解析器异常"; return 1; }

  status="${parsed%%$'\n'*}"
  if [ "$status" != "OK" ]; then
    MCP_ERR="${parsed#ERR$'\t'}"
    return 1
  fi
  MCP_STATUS="ok"
  MCP_PORTS=$(printf '%s\n' "$parsed" | tail -n +2)
  return 0
}

echo "[SerialHub] 联邦状态巡检 ($(date '+%F %T'))"
echo "探测端口: $PORTS"
echo

masters=()
workers=()
declare -A SIDE_OF=()   # master 端口 → 本地串口侧别（无本地串口时缺省）
declare -A MCP_OK=()    # master 端口 → 1=串口聚合已验证 / 0=未验证
declare -A NF_OF=()     # master 端口 → 联邦串口数

for port in $PORTS; do
  url="http://127.0.0.1:$port"
  # 单次请求同时取 body 与状态码，避免两次请求间状态漂移
  resp=$(curl -s --max-time $CURL_T -w '\n%{http_code}' "$url/health" 2>/dev/null) || resp="000"
  code="${resp##*$'\n'}"
  health="${resp%$'\n'*}"
  if [ "$code" = "000" ]; then
    [ "$SCAN" -eq 1 ] || echo "  $url  →  无监听"
    continue
  fi
  if [ "$code" != "200" ]; then
    echo "  $url  →  /health HTTP $code（非 SerialHub 或服务异常）"
    continue
  fi
  info=$(printf '%s' "$health" | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    print('%s|%s' % (d.get('service',''), d.get('role','')))
except Exception:
    print('||')
" 2>/dev/null)
  service="${info%%|*}"; role="${info##*|}"

  if [ "$service" != "serialhub" ]; then
    echo "  $url  →  非 SerialHub 服务（/health 200 但无 service 字段）"
    continue
  fi

  if [ "$role" = "master" ]; then
    echo "  $url  →  SerialHub MASTER"
    masters+=("$port")
    if mcp_serial_list "$url"; then
      MCP_OK[$port]=1
      locals=""; feds=""; nl=0; nf=0
      while IFS=$'\t' read -r name origin side; do
        [ -z "$name" ] && continue
        if [ "$origin" = "local" ]; then
          locals+="$name "; nl=$((nl+1)); SIDE_OF[$port]="$side"
        else
          feds+="$name(side=$side) "; nf=$((nf+1))
        fi
      done <<< "$MCP_PORTS"
      NF_OF[$port]=$nf
      echo "      聚合串口 $((nl + nf)) 个（本地 $nl / 联邦 $nf）"
      [ "$nl" -gt 0 ] && echo "        本地:$locals"
      [ "$nf" -gt 0 ] && echo "        联邦:$feds"
      [ $((nl + nf)) -eq 0 ] && echo "        （无串口接入属正常，接入 USB 后重查）"
    else
      MCP_OK[$port]=0
      NF_OF[$port]=0
      echo "      串口聚合未能验证（MCP 调用失败: $MCP_ERR）"
    fi
  elif [ "$role" = "worker" ]; then
    echo "  $url  →  SerialHub WORKER（/health 为静态角色应答，/mcp 反代到主实例）"
    workers+=("$port")
  else
    echo "  $url  →  SerialHub（未知 role=$role）"
  fi
done

echo
echo "诊断:"
nm=${#masters[@]}; nw=${#workers[@]}
if [ "$nm" -eq 0 ] && [ "$nw" -eq 0 ]; then
  echo "  ✗ 未发现任何 SerialHub 实例（检查服务是否启动: serialhub --minimized -D）"
  exit 1
fi
if [ "$nm" -eq 0 ]; then
  echo "  ⚠ 当前探测范围只发现 WORKER，未发现 MASTER"
  echo "     主实例可能位于对侧或其他端口；请检查 worker 上游与主实例日志"
  exit 1
fi

rc=0
if [ "$nm" -gt 1 ]; then
  echo "  ⚠ 发现 $nm 个 MASTER 端点: ${masters[*]}"
  echo "     （无实例 ID，不能排除端口转发到同一进程；联邦正常时应只有一个主实例，"
  echo "      多主会各自聚合本侧串口，建议只保留一个）"
  rc=2
else
  echo "  ✓ MASTER 唯一: ${masters[0]}"
fi

# 串口聚合验证状态（首个 master；多主时逐个看上方明细）
if [ "${MCP_OK[${masters[0]}]:-0}" -eq 1 ]; then
  echo "  ✓ MASTER ${masters[0]} 的串口聚合已验证（serial_list 可用；其他端点见上方明细）"
else
  echo "  ⚠ 拓扑已明确，但串口聚合未能验证（MCP 调用失败）"
fi

# 联邦贡献: 任一 master 报告 federated 串口即说明有从实例注册过
any_fed=0
for m in "${masters[@]}"; do
  [ "${MCP_OK[$m]:-0}" -eq 1 ] && [ "${NF_OF[$m]:-0}" -gt 0 ] && any_fed=1
done
if [ "$any_fed" -eq 1 ]; then
  echo "  ℹ 主实例报告联邦串口贡献（有从实例注册并贡献了串口）"
fi

if [ "$nw" -gt 0 ]; then
  echo "  ℹ 发现 WORKER 监听: ${workers[*]}"
  echo "     （/health 是静态角色应答，注册状态未验证；确认注册请查主实例日志「从实例已注册」）"
elif [ "$any_fed" -eq 0 ]; then
  echo "  ℹ 本侧未发现 WORKER 监听（同侧从实例不占独立端口时不可见；对侧请运行对侧脚本验证）"
fi

side="${SIDE_OF[${masters[0]}]:-}"
[ -n "$side" ] && echo "  ℹ MASTER 侧别: $side"
echo "  ℹ 可见性: 本脚本仅探测当前环境的 127.0.0.1 候选端口，不保证覆盖双侧实例"
exit $rc
