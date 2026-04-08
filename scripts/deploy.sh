#!/bin/bash
# SerialHub 部署脚本 (Linux/macOS)
# 用法: ./deploy.sh [选项]

set -e

INSTALL_DIR="/opt/serialhub"
PORT=5000
SERIAL_PORT=""
CREATE_SERVICE=false
START_AFTER_INSTALL=false

show_help() {
    echo "SerialHub 部署脚本"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -d, --dir <路径>        安装目录 (默认: /opt/serialhub)"
    echo "  -p, --port <端口>       MCP 服务端口 (默认: 5000)"
    echo "  -s, --serial <串口>     默认串口 (如: /dev/ttyUSB0)"
    echo "  --service               创建系统服务"
    echo "  --start                 安装后启动服务"
    echo "  -h, --help              显示帮助"
    echo ""
    echo "示例:"
    echo "  $0 --dir /usr/local/serialhub --port 8080 --service --start"
}

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        -p|--port)
            PORT="$2"
            shift 2
            ;;
        -s|--serial)
            SERIAL_PORT="$2"
            shift 2
            ;;
        --service)
            CREATE_SERVICE=true
            shift
            ;;
        --start)
            START_AFTER_INSTALL=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo "未知选项: $1"
            show_help
            exit 1
            ;;
    esac
done

echo -e "\033[36m========================================\033[0m"
echo -e "\033[36mSerialHub 部署脚本\033[0m"
echo -e "\033[36m========================================\033[0m"

# 检查 root 权限
if [ "$EUID" -ne 0 ] && [ "$CREATE_SERVICE" = true ]; then
    echo -e "\033[31m创建系统服务需要 root 权限\033[0m"
    exit 1
fi

# 获取项目根目录
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

echo -e "\n\033[33m[1/5] 检查构建文件...\033[0m"
if [ ! -f "./bin/serialhub" ]; then
    echo -e "\033[33m未找到构建文件，先执行构建...\033[0m"
    go build -o ./bin/serialhub ./cmd/serialhub
fi

echo -e "\n\033[33m[2/5] 创建安装目录...\033[0m"
mkdir -p "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR/logs"

echo -e "\n\033[33m[3/5] 复制文件...\033[0m"
cp -f ./bin/serialhub "$INSTALL_DIR/"
cp -f ./config.example.toml "$INSTALL_DIR/config.toml"
cp -f ./scripts/start.sh "$INSTALL_DIR/"
cp -f ./README.md "$INSTALL_DIR/"
cp -f ./DEPLOY.md "$INSTALL_DIR/"
cp -f ./MCP.md "$INSTALL_DIR/"

echo -e "\n\033[33m[4/5] 生成配置文件...\033[0m"
cat > "$INSTALL_DIR/config.toml" << EOF
# SerialHub 配置文件
# 生成时间: $(date '+%Y-%m-%d %H:%M:%S')

[serial]
port = "$SERIAL_PORT"
baudRate = 115200
dataBits = 8
parity = "none"
stopBits = 1

[mcp]
httpPort = $PORT

[log]
level = "info"
dir = "$INSTALL_DIR/logs"
EOF

echo -e "\n\033[33m[5/5] 配置环境...\033[0m"

# 创建符号链接
if [ -d "/usr/local/bin" ]; then
    ln -sf "$INSTALL_DIR/serialhub" /usr/local/bin/serialhub
    echo -e "\033[32m已创建符号链接: /usr/local/bin/serialhub\033[0m"
fi

# 创建 systemd 服务
if [ "$CREATE_SERVICE" = true ]; then
    echo -e "\033[33m创建 systemd 服务...\033[0m"
    
    cat > /etc/systemd/system/serialhub.service << EOF
[Unit]
Description=SerialHub - Serial to Web/MCP Bridge
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/serialhub -c $INSTALL_DIR/config.toml
Restart=always
RestartSec=5
User=root
WorkingDirectory=$INSTALL_DIR

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable serialhub.service
    echo -e "\033[32m服务已创建并启用: serialhub.service\033[0m"
    
    if [ "$START_AFTER_INSTALL" = true ]; then
        systemctl start serialhub.service
        echo -e "\033[32m服务已启动\033[0m"
    fi
elif [ "$START_AFTER_INSTALL" = true ]; then
    echo -e "\033[33m启动 SerialHub...\033[0m"
    nohup "$INSTALL_DIR/serialhub" -c "$INSTALL_DIR/config.toml" > "$INSTALL_DIR/logs/serialhub.log" 2>&1 &
    echo $! > "$INSTALL_DIR/serialhub.pid"
    echo -e "\033[32mSerialHub 已启动 (PID: $(cat "$INSTALL_DIR/serialhub.pid"))\033[0m"
fi

echo -e "\n\033[32m========================================\033[0m"
echo -e "\033[32m部署完成!\033[0m"
echo -e "\033[32m安装目录: $INSTALL_DIR\033[0m"
echo -e "\033[32m配置文件: $INSTALL_DIR/config.toml\033[0m"
echo -e "\033[32m日志目录: $INSTALL_DIR/logs\033[0m"
echo -e "\033[32m========================================\033[0m"

echo -e "\n\033[36m使用说明:\033[0m"
echo -e "  \033[37m1. 编辑配置文件: $INSTALL_DIR/config.toml\033[0m"
echo -e "  \033[37m2. 手动启动: $INSTALL_DIR/serialhub -c $INSTALL_DIR/config.toml\033[0m"
echo -e "  \033[37m3. 或使用启动脚本: $INSTALL_DIR/start.sh\033[0m"
echo -e "  \033[37m4. Web 终端: http://localhost:$PORT/terminal\033[0m"
echo -e "  \033[37m5. MCP 服务: http://localhost:$PORT/mcp\033[0m"

if [ "$CREATE_SERVICE" = true ]; then
    echo -e "\n\033[36m服务命令:\033[0m"
    echo -e "  \033[37m启动: sudo systemctl start serialhub\033[0m"
    echo -e "  \033[37m停止: sudo systemctl stop serialhub\033[0m"
    echo -e "  \033[37m重启: sudo systemctl restart serialhub\033[0m"
    echo -e "  \033[37m状态: sudo systemctl status serialhub\033[0m"
    echo -e "  \033[37m日志: sudo journalctl -u serialhub -f\033[0m"
fi
