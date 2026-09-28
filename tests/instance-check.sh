#!/usr/bin/env bash
# instance-check.sh — SerialHub 实例状态自检（Linux/WSL 侧）
# 用法: ./instance-check.sh [--port N]... [-h]
#   --port N   追加探测端口（可重复）
#   -h         显示本帮助
# 检查内容（只读，不连接、不写入串口）：
#   1. lock 文件元数据（文件常驻；仅 OS 文件锁表示实例存活）
#   2. /health 实例识别（service=serialhub）
#   3. MCP serial_list 调用（完整握手，展示本实例串口列表）
# 退出码: 0=全部发现的实例检查通过; 1=未发现实例; 2=MCP 检查失败; 3=参数/依赖错误
set -u

PROG="instance-check.sh"
CURL_T=3

for dep in curl python3; do
  command -v "$dep" >/dev/null 2>&1 || { echo "[instance-check] 缺少依赖: $dep"; exit 3; }
done

EXTRA_PORTS=""
while [ $# -gt 0 ]; do
  case "$1" in
    --port)
      [ $# -lt 2 ] && { echo "[instance-check] --port 缺少端口号"; exit 3; }
      p="$2"
      [[ "$p" =~ ^[0-9]+$ ]] || { echo "[instance-check] 端口须为数字: $p"; exit 3; }
      { [ "${#p}" -gt 5 ] || [ "$p" -lt 1 ] || [ "$p" -gt 65535 ]; } && {
        echo "[instance-check] 端口超出范围 1-65535: $p"; exit 3; }
      EXTRA_PORTS="$EXTRA_PORTS $p"; shift 2 ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    *) echo "[instance-check] 未知参数: $1"; exit 3 ;;
  esac
done

# ---- 端口发现: 默认 5050 + 本侧配置文件 [MCP] HTTPPort ----
xdg="${XDG_CONFIG_HOME:-}"
[[ "$xdg" = /* ]] || xdg="$HOME/.config"
cfg="$xdg/serialhub/config.toml"
cfg_port=""
if [ -f "$cfg" ]; then
  cfg_port=$(python3 -c "
import sys, re
txt = open(sys.argv[1], encoding='utf-8', errors='replace').read()
m = re.search(r'^\[(?i:mcp)\][^\[]*?^[ \t]*(?i:httpport)[ \t]*=[ \t]*(\d+)', txt, re.M)
v = m.group(1) if m else ''
if v and (len(v) > 5 or not (1 <= int(v) <= 65535)):
    print('', end='')
else:
    print(v, end='')
" "$cfg" 2>/dev/null)
fi

PORTS="$cfg_port$EXTRA_PORTS 5050"

# ---- MCP 调用: initialize → initialized → tools/call serial_list → DELETE ----
# 全局变量返回: MCP_STATUS(ok/err) / MCP_PORTS(TSV 行) / MCP_ERR(错误描述)
MCP_STATUS="err"; MCP_PORTS=""; MCP_ERR=""
mcp_serial_list() {
  local base="$1" hdr body sid http_code
  MCP_STATUS="err"; MCP_PORTS=""; MCP_ERR=""
  hdr=$(mktemp) || return 1
  body=$(mktemp) || { rm -f "$hdr"; return 1; }
  local sid=""
  trap 'if [ -n "$sid" ]; then curl -s --max-time $CURL_T -o /dev/null -X DELETE "$base/mcp" -H "Mcp-Session-Id: $sid" || true; fi; rm -f "$hdr" "$body"' RETURN

  http_code=$(curl -s --max-time $CURL_T -D "$hdr" -o "$body" -w '%{http_code}' -X POST "$base/mcp" \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"instance-check","version":"1.0"}}}') || { MCP_ERR="网络请求失败"; return 1; }
  [ "$http_code" = "200" ] || { MCP_ERR="initialize HTTP $http_code"; return 1; }
  sid=$(awk 'tolower($1)=="mcp-session-id:"{print $2}' "$hdr" | tr -d '\r')
  [ -z "$sid" ] && { MCP_ERR="响应缺少 Mcp-Session-Id"; return 1; }

  curl -s --max-time $CURL_T -o /dev/null -X POST "$base/mcp" \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -H "Mcp-Session-Id: $sid" \
    -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' 2>/dev/null

  http_code=$(curl -s --max-time $CURL_T -o "$body" -w '%{http_code}' -X POST "$base/mcp" \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -H "Mcp-Session-Id: $sid" \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"serial_list","arguments":{}}}') || { MCP_ERR="网络请求失败"; return 1; }
  [ "$http_code" = "200" ] || { MCP_ERR="tools/call HTTP $http_code"; return 1; }

  # 解析与校验：JSON-RPC error / isError / ports 非数组 / 条目结构
  # 全部条目校验通过才输出 OK（先输出后校验会让 shell 误判成功）
  local out
  out=$(python3 -c "
import json, sys
try:
    d = json.load(open(sys.argv[1], encoding='utf-8'))
except Exception as e:
    print('ERR\t响应不是有效 JSON: %s' % e); sys.exit(0)
if 'error' in d and d['error']:
    print('ERR\tJSON-RPC 错误: %s' % d['error'].get('message', d['error'])); sys.exit(0)
r = d.get('result') or {}
if r.get('isError'):
    print('ERR\t工具执行失败'); sys.exit(0)
sc = r.get('structuredContent')
if not isinstance(sc, dict) or 'ports' not in sc:
    print('ERR\t响应缺少 structuredContent.ports'); sys.exit(0)
ports = sc['ports']
if not isinstance(ports, list):
    print('ERR\tports 不是数组'); sys.exit(0)
lines = []
for p in ports:
    if not isinstance(p, dict):
        print('ERR\tports 条目不是对象: %r' % (p,)); sys.exit(0)
    name = p.get('name')
    origin = p.get('origin')
    if not isinstance(name, str) or not name.strip():
        print('ERR\tports 条目缺少有效 name'); sys.exit(0)
    if origin not in ('local', 'federated'):
        print('ERR\tports 条目 origin 非法: %r' % (origin,)); sys.exit(0)
    side = p.get('side')
    if not isinstance(side, str):
        side = ''
    lines.append('%s\t%s\t%s' % (name, origin, side))
print('OK')
for l in lines:
    print(l)
" "$body" 2>/dev/null) || { MCP_ERR="解析脚本失败"; return 1; }

  if [ "$(head -n1 <<< "$out")" = "OK" ]; then
    MCP_STATUS="ok"
    MCP_PORTS=$(tail -n +2 <<< "$out")
  else
    MCP_ERR=$(head -n1 <<< "$out" | cut -f2-)
  fi
}

echo "[SerialHub] 实例状态自检 ($(date '+%F %T'))"
echo

# ---- lock 文件检查 ----
lock="$xdg/serialhub/instance.lock"
lock_target=""
if [ -f "$lock" ]; then
  echo "lock 文件: 存在（$lock）"
  lock_target=$(python3 -c "
import json, sys
try:
    d = json.load(open(sys.argv[1], encoding='utf-8'))
    host = d.get('host') or '127.0.0.1'
    if host in ('0.0.0.0', '::'): host = '127.0.0.1'
    p = d.get('port')
    if isinstance(host, str) and isinstance(p, int) and 1 <= p <= 65535:
        print('%s|%d' % (host, p))
except Exception as e:
    pass
" "$lock" 2>/dev/null)
  [ -n "$lock_target" ] && echo "  记录地址: ${lock_target/|/:}（仅元数据，不代表实例仍存活）"
  echo "  说明: 文件可能常驻；仅 OS 文件锁能证明实例仍在运行"
else
  echo "lock 文件: 不存在（$lock）"
fi
echo

found=0
failed=0
if [ -n "$lock_target" ]; then
  lock_host="${lock_target%%|*}"
  lock_port="${lock_target##*|}"
  PORTS="$lock_port $PORTS"
fi
PORTS=$(printf '%s\n' $PORTS | awk '!seen[$0]++' | tr '\n' ' ')
echo "探测端口: $PORTS"
echo
for port in $PORTS; do
  probe_host="127.0.0.1"
  if [ -n "$lock_target" ] && [ "$port" = "$lock_port" ]; then probe_host="$lock_host"; fi
  if [[ "$probe_host" == *:* ]]; then probe_host="[$probe_host]"; fi
  url="http://$probe_host:$port"
  resp=$(curl -s --max-time $CURL_T -w '\n%{http_code}' "$url/health" 2>/dev/null)
  http_code="${resp##*$'\n'}"
  body="${resp%$'\n'*}"
  if [ "$http_code" = "000" ]; then
    echo "  $url  →  无监听"
    continue
  fi
  if [ "$http_code" != "200" ]; then
    echo "  $url  →  /health HTTP $http_code（非 SerialHub 或服务异常）"
    continue
  fi
  info=$(printf '%s' "$body" | python3 -c "
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

  echo "  $url  →  SerialHub 运行中（role=$role）"
  found=1
  mcp_serial_list "$url"
  if [ "$MCP_STATUS" = "ok" ]; then
    n=$(grep -c . <<< "$MCP_PORTS" || true)
    echo "      串口列表: $n 个"
    if [ "$n" -gt 0 ]; then
      while IFS=$'\t' read -r name origin side; do
        [ -z "$name" ] && continue
        echo "        $name（origin=$origin side=$side）"
      done <<< "$MCP_PORTS"
    fi
  else
    echo "      串口列表: MCP 调用失败（$MCP_ERR）"
    failed=1
  fi
done

echo
if [ "$failed" -eq 1 ]; then
  echo "诊断: ✗ 已发现 SerialHub，但 MCP 串口列表检查失败"
  exit 2
fi
if [ "$found" -eq 1 ]; then
  echo "诊断: ✓ 发现健康的 SerialHub 实例"
  exit 0
fi
echo "诊断: ✗ 未发现 SerialHub 实例（检查服务是否启动: serialhub --minimized -D）"
exit 1
