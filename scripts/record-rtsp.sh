#!/bin/bash
# record-rtsp.sh - 录制 RTSP 流到 MP4，并可选择直接重放
# 用法: 
#   ./scripts/record-rtsp.sh rtsp://IP:PORT/path [输出MP4] [录制秒数]
#   ./scripts/record-rtsp.sh rtsp://IP:PORT/path [输出MP4] [录制秒数] replay
# 
# sudo 密码: yahboom

set -e

RTSP_URL="${1}"
OUTPUT_MP4="${2:-./fixtures/recorded_stream.mp4}"
DURATION="${3:-30}"
DO_REPLAY="${4:-}"

if [ -z "$RTSP_URL" ]; then
    echo "用法: $0 <rtsp_url> [输出MP4] [录制秒数] [replay]"
    echo "  例: $0 rtsp://192.168.1.100:554/stream1 ./myRecording.mp4 60"
    echo "  例: $0 rtsp://192.168.1.100:554/stream1 ./myRecording.mp4 60 replay"
    exit 1
fi

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SUDOPASS="yahboom"

echo "=============================================="
echo "RTSP 流录制工具"
echo "=============================================="
echo "RTSP URL:   $RTSP_URL"
echo "输出文件:   $OUTPUT_MP4"
echo "录制时长:   ${DURATION}秒"
echo "录制后重放: ${DO_REPLAY:-否}"
echo "=============================================="

# 清理
pkill -9 stream-source 2>/dev/null || true
fuser -k 8554/tcp 8555/tcp 2>/dev/null || true
sleep 1

mkdir -p "$(dirname "$OUTPUT_MP4")"

# 步骤1: 探测流信息
echo "[1/3] 探测流信息..."
PROBE_OUTPUT=$(timeout 10 ffprobe -v quiet -show_streams -show_format "$RTSP_URL" 2>&1) || {
    echo "[ERROR] 无法连接到 RTSP 流，请检查 URL 是否正确"
    echo "提示: 如果需要鉴权，使用格式: rtsp://user:pass@host:port/path"
    exit 1
}

CODEC=$(echo "$PROBE_OUTPUT" | grep "codec_name" | head -1 | cut -d= -f2)
RES=$(echo "$PROBE_OUTPUT" | grep "width=" | head -1 | cut -d= -f2)
HEIGHT=$(echo "$PROBE_OUTPUT" | grep "height=" | head -1 | cut -d= -f2)
FPS=$(echo "$PROBE_OUTPUT" | grep "r_frame_rate" | head -1 | cut -d= -f2)
echo "      视频编码: $CODEC"
echo "      分辨率: ${RES}x${HEIGHT}"
echo "      帧率: $FPS"

# 步骤2: 录制
echo "[2/3] 开始录制 (${DURATION}秒)..."
echo "$SUDOPASS" | sudo -S fuser -k $((8554))/tcp $((8555))/tcp 2>/dev/null || true

ffmpeg -rtsp_transport udp \
       -i "$RTSP_URL" \
       -c copy \
       -t "$DURATION" \
       -y "$OUTPUT_MP4" \
       2>&1 | tail -5

if [ ! -f "$OUTPUT_MP4" ] || [ $(stat -c%s "$OUTPUT_MP4" 2>/dev/null || stat -f%z "$OUTPUT_MP4" 2>/dev/null) -lt 1000 ]; then
    echo "[ERROR] 录制失败，文件太小或不存在"
    exit 1
fi

RECORD_SIZE=$(stat -c%s "$OUTPUT_MP4" 2>/dev/null || stat -f%z "$OUTPUT_MP4" 2>/dev/null)
echo "      录制完成: $(stat -c%s "$OUTPUT_MP4" 2>/dev/null || stat -f%z "$OUTPUT_MP4" 2>/dev/null) bytes"

# 步骤3: 可选 - 以 RTSP 流重放
if [ "$DO_REPLAY" = "replay" ]; then
    echo "[3/3] 启动 RTSP 重放服务器..."
    
    # 生成配置文件
    RTSP_PORT=8554
    cat > /tmp/replay-rtsp.yaml << EOF
name: replay-rtsp
server:
  rtspPort: $RTSP_PORT
inputs:
  - name: replay-input
    kind: mp4
    codec: h264
    location: $OUTPUT_MP4
outputs:
  - name: rtsp-out
    kind: rtsp
    target: rtsp://127.0.0.1:$RTSP_PORT/replay
mutations:
  - name: passthrough
    kind: identity
    enabled: true
profiles:
  - name: stream
    input: replay-input
    output: rtsp-out
    mutations: [passthrough]
EOF

    cd "$WORK_DIR"
    ./stream-source-tester -config /tmp/replay-rtsp.yaml > /tmp/replay_srv.log 2>&1 &
    SERVER_PID=$!
    sleep 2

    if ! kill -0 $SERVER_PID 2>/dev/null; then
        echo "[ERROR] 重放服务器启动失败"
        cat /tmp/replay_srv.log
        exit 1
    fi

    echo "      重放服务器已启动 (PID: $SERVER_PID)"
    echo ""
    echo "=============================================="
    echo "录制并重放完成!"
    echo "=============================================="
    echo "录制的文件: $OUTPUT_MP4"
    echo ""
    echo "重放地址:"
    echo "  VLC: rtsp://127.0.0.1:$RTSP_PORT/replay"
    echo "  ffprobe: ffprobe rtsp://127.0.0.1:$RTSP_PORT/replay"
    echo ""
    echo "停止服务器: kill $SERVER_PID"
else
    echo "[3/3] (跳过重放)"
    echo ""
    echo "=============================================="
    echo "录制完成!"
    echo "=============================================="
    echo "文件: $OUTPUT_MP4"
    echo ""
    echo "用录制的文件启动 RTSP 重放:"
    echo "  ./scripts/record-rtsp.sh \"\" \"$OUTPUT_MP4\" 30 replay"
fi
