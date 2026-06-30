#!/bin/bash
# capture-rtsp.sh - 捕获 RTSP/RTP 流量为 PCAP 文件
# 用法: ./scripts/capture-rtsp.sh [输出文件] [持续秒数] [目标IP]
#   输出文件: 默认为 ./fixtures/captured.pcap
#   持续秒数: 默认为 30 秒
#   目标IP: RTSP 服务器 IP，默认为 192.168.3.109

set -e

OUTPUT="${1:-./fixtures/captured.pcap}"
DURATION="${2:-30}"
TARGET_IP="${3:-192.168.3.109}"
INTERFACE="eno1"

# 检查权限
if ! whoami | grep -q root; then
    echo "[WARNING] tcpdump 需要 root 权限，正在使用 sudo..."
fi

echo "=============================================="
echo "RTSP/RTP 流量捕获脚本"
echo "=============================================="
echo "接口:     $INTERFACE"
echo "目标IP:   $TARGET_IP"
echo "输出文件: $OUTPUT"
echo "持续时间: ${DURATION}秒"
echo "=============================================="

# 确保输出目录存在
mkdir -p "$(dirname "$OUTPUT")"

# 用 tcpdump 捕获 RTSP(554) 和 RTP(通常>3000) 端口的流量
# -i: 接口
# -w: 输出文件  
# -v: 详细输出
# port 554 or port range 3000-30000 covers typical RTP
echo "开始捕获... (Ctrl+C 提前停止)"
echo ""

sudo tcpdump -i "$INTERFACE" \
    -w "$OUTPUT" \
    -v \
    "((ip.src == $TARGET_IP or ip.dst == $TARGET_IP) and (tcp port 554 or udp portrange 3000-30000))" \
    -G "$DURATION" \
    -W 1 \
    2>&1

echo ""
echo "=============================================="
echo "捕获完成!"
echo "文件: $OUTPUT"
ls -lh "$OUTPUT"
echo "=============================================="
echo ""
echo "使用此 PCAP 文件启动虚拟相机:"
echo "  1. 编辑 examples/virtual-camera-pcap.yaml"
echo "  2. 将 location 改为: $OUTPUT"
echo "  3. 启动: ./stream-source-tester -config ./examples/virtual-camera-pcap.yaml"
