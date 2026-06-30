#!/bin/bash
# generate-rtp-pcap.sh - 用 ffmpeg 从 MP4 生成带 RTP 的 pcap 文件
# 原理: 启动 RTSP 服务器 -> tcpdump 抓 UDP 流量 -> ffmpeg 拉流触发 RTP 发送
# sudo 密码: yahboom

set -e

MP4_FILE="${1:-./fixtures/test_video.mp4}"
OUTPUT_PCAP="${2:-./fixtures/rtp_stream.pcap}"
DURATION="${3:-10}"
RTSP_PORT=8554

echo "=============================================="
echo "RTSP/RTP 流量抓包工具"
echo "=============================================="
echo "MP4 文件:   $MP4_FILE"
echo "输出 PCAP:  $OUTPUT_PCAP"
echo "抓包时长:   ${DURATION}秒"
echo "RTSP 端口:  $RTSP_PORT"
echo "=============================================="

if [ ! -f "$MP4_FILE" ]; then
    echo "[ERROR] MP4 文件不存在: $MP4_FILE"
    exit 1
fi

# 清理残留进程
pkill -9 stream-source 2>/dev/null || true
fuser -k $RTSP_PORT/tcp 2>/dev/null || true
sleep 1

# 生成临时 RTSP 配置
WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cat > /tmp/gen-rtp-rtsp.yaml << EOF
name: gen-rtp-rtsp
server:
  rtspPort: $RTSP_PORT
inputs:
  - name: mp4-input
    kind: mp4
    codec: h264
    location: $MP4_FILE
outputs:
  - name: rtsp-out
    kind: rtsp
    target: rtsp://127.0.0.1:$RTSP_PORT/test
mutations:
  - name: passthrough
    kind: identity
    enabled: true
profiles:
  - name: stream
    input: mp4-input
    output: rtsp-out
    mutations: [passthrough]
EOF

# 步骤1: 启动 RTSP 服务器
echo "[1/3] 启动 RTSP 服务器 (端口 $RTSP_PORT)..."
cd "$WORK_DIR"
./stream-source-tester -config /tmp/gen-rtp-rtsp.yaml > /tmp/rtsp_srv.log 2>&1 &
SERVER_PID=$!
sleep 2

if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo "[ERROR] RTSP 服务器启动失败:"
    cat /tmp/rtsp_srv.log
    exit 1
fi
echo "      OK (PID: $SERVER_PID)"

# 步骤2: tcpdump 抓 UDP 流量
echo "[2/3] 开始抓包..."
echo 'yahboom' | sudo -S tcpdump -i lo -w "$OUTPUT_PCAP" "udp" 2>/dev/null &
TCPDUMP_PID=$!
sleep 1

# 步骤3: ffprobe 触发拉流（触发 RTSP 服务器发送 RTP）
echo "[3/3] 触发 RTSP 拉流 (${DURATION}秒)..."
timeout ${DURATION} ffprobe -v quiet -show_streams "rtsp://127.0.0.1:$RTSP_PORT/test" >/dev/null 2>&1 || true
sleep 1

# 停止所有进程
echo "      停止抓包..."
sudo kill $TCPDUMP_PID 2>/dev/null || echo 'yahboom' | sudo -S kill $TCPDUMP_PID 2>/dev/null
kill $SERVER_PID 2>/dev/null || true
wait 2>/dev/null

# 步骤4: 验证
echo ""
echo "=============================================="
echo "验证 PCAP 文件"
echo "=============================================="
if [ ! -f "$OUTPUT_PCAP" ]; then
    echo "[ERROR] PCAP 文件未生成!"
    exit 1
fi

PCAP_SIZE=$(stat -c%s "$OUTPUT_PCAP" 2>/dev/null || stat -f%z "$OUTPUT_PCAP" 2>/dev/null)
echo "文件大小: $PCAP_SIZE bytes"

# 统计包数量
UDP_COUNT=$(echo 'yahboom' | sudo -S tcpdump -r "$OUTPUT_PCAP" -n 2>&1 | grep -c "UDP")
echo "UDP 包数: $UDP_COUNT"

# 检查 RTP 包（端口 5004/5005 是 RTP/RTCP 标准端口）
RTP_COUNT=$(echo 'yahboom' | sudo -S tcpdump -r "$OUTPUT_PCAP" -n 2>&1 | grep -c "5004\|5005")
echo "RTP/RTCP 包数 (端口 5004-5005): $RTP_COUNT"

# 显示前几行
echo ""
echo "包预览 (前10行):"
echo 'yahboom' | sudo -S tcpdump -r "$OUTPUT_PCAP" -n 2>&1 | head -10

echo ""
if [ "$UDP_COUNT" -gt 10 ]; then
    echo "✅ PCAP 生成成功!"
    echo ""
    echo "文件: $OUTPUT_PCAP"
    echo ""
    echo "使用方式:"
    echo "  1. 复制到 fixtures/: cp $OUTPUT_PCAP ./fixtures/"
    echo "  2. 编辑配置指向此文件:"
    echo "     编辑 examples/virtual-camera-pcap.yaml"
    echo "     将 location 改为: $OUTPUT_PCAP"
    echo "  3. 启动: ./stream-source-tester -config ./examples/virtual-camera-pcap.yaml"
    echo "  4. VLC 播放: rtsp://127.0.0.1:8555/camera"
    echo ""
    echo "注意: PCAP 模式目前仅用于元数据/协议测试，"
    echo "      真实视频重放请使用 MP4 模式。"
else
    echo "⚠️ 包数量较少，可能抓包失败。"
fi
