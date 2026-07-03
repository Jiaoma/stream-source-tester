#!/bin/bash
# pcap-tool.sh - RTSP 流媒体测试工具箱
# 整合两个功能:
#   1. 从 MP4 生成 RTP pcap (模拟真实相机抓包)
#   2. 录制真实 RTSP 流并通过 VLC 重放
#
# 用法:
#   ./scripts/pcap-tool.sh capture [mp4文件] [输出pcap] [持续秒数]
#   ./scripts/pcap-tool.sh record  <rtsp_url>  [输出mp4] [录制秒数] [replay]
#   ./scripts/pcap-tool.sh serve  [mp4文件]  [端口]   [用户名] [密码]

ACTION="${1:-help}"
WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
. "$WORK_DIR/scripts/lib.sh"

# 颜色输出
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

help() {
    echo "RTSP 流媒体测试工具箱"
    echo ""
    echo "用法:"
    echo "  $0 capture [mp4文件] [输出pcap] [持续秒数]"
    echo "         从 MP4 发送 H.264/RTP 到本机 UDP，并抓取为 pcap"
    echo "         例: $0 capture ./fixtures/test_video.mp4 ./fixtures/capture.pcap 10"
    echo ""
    echo "  $0 record  <rtsp_url>  [输出mp4] [录制秒数] [replay]"
    echo "         录制 RTSP 流到 MP4，可选直接重放"
    echo "         例: $0 record rtsp://192.168.1.100:554/stream1 ./rec.mp4 30"
    echo "         例: $0 record rtsp://192.168.1.100:554/stream1 ./rec.mp4 60 replay"
    echo ""
    echo "  $0 serve  [mp4文件]  [端口]   [用户名] [密码]"
    echo "         启动带鉴权的 RTSP 虚拟相机"
    echo "         例: $0 serve ./fixtures/test_video.mp4 8554 admin secret123"
    echo ""
    echo "  $0 vlc     <rtsp_url>"
    echo "         用 VLC 播放 RTSP 流"
    echo "         例: $0 vlc rtsp://127.0.0.1:8554/test"
    echo "         例: $0 vlc rtsp://admin:secret@127.0.0.1:8554/camera"
}

# ---------- 功能1: 从 MP4 生成 RTP pcap ----------
do_capture() {
    require_root "抓包" "$@"

    MP4_FILE="${2:-$WORK_DIR/fixtures/test_video.mp4}"
    OUTPUT_PCAP="${3:-$WORK_DIR/fixtures/rtp_capture.pcap}"
    DURATION="${4:-10}"
    LOOPBACK_IF="$(loopback_if)"
    RTP_PORT=5004

    echo -e "${GREEN}[目标1] 从 MP4 生成 H.264/RTP pcap${NC}"
    echo "MP4: $MP4_FILE"
    echo "输出: $OUTPUT_PCAP"
    echo "时长: ${DURATION}秒"
    echo "接口: $LOOPBACK_IF"

    [ ! -f "$MP4_FILE" ] && { echo -e "${RED}[错误] MP4 不存在: $MP4_FILE${NC}"; exit 1; }
    mkdir -p "$(dirname "$OUTPUT_PCAP")"
    rm -f "$OUTPUT_PCAP"

    echo -e "${YELLOW}[1/2] 开始抓包 (${DURATION}秒)...${NC}"
    tcpdump -i "$LOOPBACK_IF" -w "$OUTPUT_PCAP" "udp port $RTP_PORT" >/tmp/capture_tcpdump.log 2>&1 &
    TCPDUMP_PID=$!
    sleep 1

    echo -e "${YELLOW}[2/2] ffmpeg 发送 RTP...${NC}"
    ffmpeg -hide_banner -loglevel error \
        -re \
        -stream_loop -1 \
        -i "$MP4_FILE" \
        -an \
        -c:v copy \
        -f rtp \
        "rtp://127.0.0.1:$RTP_PORT?pkt_size=1400" >/tmp/capture_ffmpeg.log 2>&1 &
    FFMPEG_PID=$!
    sleep "$DURATION"

    kill "$FFMPEG_PID" 2>/dev/null || true
    wait "$FFMPEG_PID" 2>/dev/null || true
    sleep 1
    kill "$TCPDUMP_PID" 2>/dev/null || true
    wait "$TCPDUMP_PID" 2>/dev/null || true

    if [ ! -f "$OUTPUT_PCAP" ]; then
        echo -e "${RED}[错误] PCAP 未生成${NC}"
        echo "tcpdump 日志:"
        cat /tmp/capture_tcpdump.log 2>/dev/null || true
        echo "ffmpeg 日志:"
        cat /tmp/capture_ffmpeg.log 2>/dev/null || true
        exit 1
    fi
    SIZE=$(file_size "$OUTPUT_PCAP")
    COUNT=$(tcpdump -r "$OUTPUT_PCAP" -n 2>&1 | grep -c "UDP")
    echo ""
    echo -e "${GREEN}✅ 成功!${NC}"
    echo "文件: $OUTPUT_PCAP ($SIZE bytes, $COUNT 个 UDP 包)"
    echo ""
    echo "分析 pcap:"
    echo "  tcpdump -r $OUTPUT_PCAP -n | head -20"
    echo ""
    echo "用此 pcap 作为虚拟相机输入:"
    echo "  1. cp $OUTPUT_PCAP $WORK_DIR/fixtures/"
    echo "  2. 编辑 $WORK_DIR/examples/virtual-camera-pcap.yaml"
    echo "  3. location: $WORK_DIR/fixtures/$(basename $OUTPUT_PCAP)"
    echo "  4. ./stream-source-tester -config $WORK_DIR/examples/virtual-camera-pcap.yaml"
}

# ---------- 功能2: 录制 RTSP 流 ----------
do_record() {
    RTSP_URL="${2}"
    OUTPUT_MP4="${3:-$WORK_DIR/fixtures/recorded.mp4}"
    DURATION="${4:-30}"
    DO_REPLAY="${5:-}"

    [ -z "$RTSP_URL" ] && { echo -e "${RED}[错误] 请提供 RTSP URL${NC}"; help; exit 1; }

    echo -e "${GREEN}[目标2] 录制 RTSP 流并重放${NC}"
    echo "URL: $RTSP_URL"
    echo "输出: $OUTPUT_MP4"
    echo "时长: ${DURATION}秒"

    mkdir -p "$(dirname "$OUTPUT_MP4")"

    echo -e "${YELLOW}[1/2] 探测流信息...${NC}"
    PROBE=$(run_with_timeout 10 ffprobe -v quiet -show_streams "$RTSP_URL" 2>&1) || {
        echo -e "${RED}[错误] 无法连接 RTSP 流${NC}"; 
        echo "提示: 鉴权格式 rtsp://user:pass@host:port/path"; 
        exit 1; }
    echo "      $(echo "$PROBE" | grep codec_name= | head -1 | cut -d= -f2) "
    echo "      $(echo "$PROBE" | grep width= | head -1 | cut -d= -f2)x$(echo "$PROBE" | grep height= | head -1 | cut -d= -f2)"

    echo -e "${YELLOW}[2/2] 录制中...${NC}"
    ffmpeg -rtsp_transport udp -i "$RTSP_URL" -c copy -t "$DURATION" -y "$OUTPUT_MP4" 2>&1 | tail -3

    [ -f "$OUTPUT_MP4" ] && SIZE=$(file_size "$OUTPUT_MP4")
    echo -e "${GREEN}✅ 录制完成: $SIZE bytes${NC}"

    if [ "$DO_REPLAY" = "replay" ]; then
        echo -e "${YELLOW}[重放] 启动 RTSP 服务器...${NC}"
        RTSP_PORT=8554
        stop_port_listener "$RTSP_PORT"
        cat > /tmp/replay_rtsp.yaml << EOF
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
        ./stream-source-tester -config /tmp/replay_rtsp.yaml >/tmp/replay_srv.log 2>&1 &
        SERVER_PID=$!
        sleep 2
        echo -e "${GREEN}✅ 重放服务器已启动 (PID: $SERVER_PID)${NC}"
        echo "VLC: rtsp://127.0.0.1:$RTSP_PORT/replay"
    fi
}

# ---------- 功能3: 启动 RTSP 虚拟相机 ----------
do_serve() {
    MP4_FILE="${2:-$WORK_DIR/fixtures/test_video.mp4}"
    PORT="${3:-8554}"
    USER="${4:-admin}"
    PASS="${5:-secret}"

    echo -e "${GREEN}[RTSP 虚拟相机]${NC}"
    echo "MP4: $MP4_FILE"
    echo "端口: $PORT"
    echo "用户: $USER / $PASS"

    [ ! -f "$MP4_FILE" ] && { echo -e "${RED}[错误] MP4 不存在: $MP4_FILE${NC}"; exit 1; }

    stop_port_listener "$PORT"
    sleep 1

    cat > /tmp/serve_rtsp.yaml << EOF
name: virtual-camera
server:
  rtspPort: $PORT
inputs:
  - name: video-input
    kind: mp4
    codec: h264
    location: $MP4_FILE
outputs:
  - name: camera-out
    kind: rtsp
    target: rtsp://127.0.0.1:$PORT/camera
    options:
      auth.mode: basic
      auth.username: $USER
      auth.password: $PASS
      auth.realm: VirtualCamera
mutations:
  - name: passthrough
    kind: identity
    enabled: true
profiles:
  - name: camera
    input: video-input
    output: camera-out
    mutations: [passthrough]
EOF

    cd "$WORK_DIR"
    ./stream-source-tester -config /tmp/serve_rtsp.yaml
}

# ---------- 功能4: VLC 播放 ----------
do_vlc() {
    URL="${2}"
    [ -z "$URL" ] && { echo -e "${RED}[错误] 请提供 RTSP URL${NC}"; exit 1; }
    echo "VLC 打开: $URL"
    if command -v cvlc >/dev/null 2>&1; then
        cvlc --no-audio "$URL" --sout "#duplicate{dst=void}" >/dev/null 2>&1 &
        echo "VLC 已启动 (后台)"
        return
    fi
    if command -v vlc >/dev/null 2>&1; then
        vlc "$URL" >/dev/null 2>&1 &
        echo "VLC 已启动 (后台)"
        return
    fi
    if [ "$(os_name)" = "Darwin" ] && command -v open >/dev/null 2>&1; then
        open -a VLC "$URL"
        echo "VLC.app 已启动"
        return
    fi
    echo -e "${RED}[错误] 未找到 VLC/cvlc，请手动打开:${NC} $URL"
    exit 1
}

# ---------- 入口 ----------
case "$ACTION" in
    capture|record|serve|vlc) do_$ACTION "$@" ;;
    *) help ;;
esac
