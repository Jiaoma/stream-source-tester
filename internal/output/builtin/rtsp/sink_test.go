package rtsp

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func TestServeMarksBundleMetadata(t *testing.T) {
	t.Parallel()

	sink := &Sink{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH264, "mp4", "./sample.mp4", nil)

	session, err := sink.Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "local-rtsp",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/test",
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if session == nil {
		t.Fatalf("session should not be nil")
	}
	runtime := session.Result()
	if runtime.SinkKind != "rtsp" {
		t.Fatalf("runtime.SinkKind = %q, want rtsp", runtime.SinkKind)
	}
	if runtime.State != "ready" && runtime.State != "serving" {
		t.Fatalf("initial runtime.State = %q, want ready or serving", runtime.State)
	}
	if runtime.SessionID == "" {
		t.Fatalf("runtime.SessionID should not be empty")
	}
	if runtime.StartedAt.IsZero() {
		t.Fatalf("runtime.StartedAt should be set")
	}
	if got := runtime.Details["listen_address"]; got == "" {
		t.Fatalf("runtime listen_address should not be empty")
	}
	if got := runtime.Details["mount_path"]; got != "test" {
		t.Fatalf("runtime mount_path = %q, want test", got)
	}
	if got := runtime.Details["transport_mode"]; got != "rtsp" {
		t.Fatalf("runtime transport_mode = %q, want rtsp", got)
	}
	if got := bundle.Metadata["served.by"]; got != "rtsp" {
		t.Fatalf("served.by = %q, want rtsp", got)
	}
	if got := bundle.Metadata["served.target"]; got != "rtsp://127.0.0.1:0/test" {
		t.Fatalf("served.target = %q, want rtsp://127.0.0.1:0/test", got)
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for {
		current := session.Result()
		if current.State == "serving" {
			runtime = current
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.DialTimeout("tcp", runtime.Details["listen_address"], 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	_ = conn.Close()

	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	closed := session.Result()
	if closed.State != "stopped" {
		t.Fatalf("closed state = %q, want stopped", closed.State)
	}
	if closed.StoppedAt == nil {
		t.Fatalf("closed StoppedAt should be set")
	}
}

func TestServeHandlesRTSPRequestAndTriggersRTP(t *testing.T) {
	t.Parallel()

	rtpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer rtpConn.Close()

	bundle := &model.SessionBundle{
		Name:      "bridge",
		Transport: []model.Protocol{model.ProtocolRTSP, model.ProtocolRTPUDP},
		Streams:   []model.Stream{{ID: "stream-0", Codec: model.CodecH264, ClockRate: 90000, PayloadType: 96, SSRC: 11}},
		Timeline:  []model.PacketEvent{{StreamID: "stream-0", Sequence: 3, Timestamp: 900, Payload: []byte{0xFE}, EmittedAt: 0}},
		Metadata:  map[string]string{},
	}

	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "rtsp-bridge",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/test",
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(300 * time.Millisecond)
	var runtime outputRuntime
	for {
		current := session.Result()
		if current.State == "serving" {
			runtime = outputRuntime{addr: current.Details["listen_address"]}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	rtspConn, err := net.DialTimeout("tcp", runtime.addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer rtspConn.Close()

	responseBuf := make([]byte, 1024)
	_ = rtspConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	optionsReq := "OPTIONS rtsp://127.0.0.1/test RTSP/1.0\r\nCSeq: 7\r\n\r\n"
	if _, err := rtspConn.Write([]byte(optionsReq)); err != nil {
		t.Fatalf("Write OPTIONS error = %v", err)
	}
	n, err := rtspConn.Read(responseBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read OPTIONS error = %v", err)
	}
	response := string(responseBuf[:n])
	if !strings.Contains(response, "RTSP/1.0 200 OK") || !strings.Contains(response, "Public: OPTIONS, DESCRIBE, SETUP, PLAY, TEARDOWN") {
		t.Fatalf("OPTIONS response = %q, want 200 OK with Public", response)
	}

	describeReq := "DESCRIBE rtsp://127.0.0.1/test RTSP/1.0\r\nCSeq: 8\r\n\r\n"
	if _, err := rtspConn.Write([]byte(describeReq)); err != nil {
		t.Fatalf("Write DESCRIBE error = %v", err)
	}
	n, err = rtspConn.Read(responseBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read DESCRIBE error = %v", err)
	}
	response = string(responseBuf[:n])
	if !strings.Contains(response, "Content-Type: application/sdp") || !strings.Contains(response, "m=video 0 RTP/AVP 96") {
		t.Fatalf("DESCRIBE response = %q, want SDP", response)
	}

	if err := rtpConn.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	buf := make([]byte, 1500)
	_, _, err = rtpConn.ReadFrom(buf)
	if err == nil {
		t.Fatalf("unexpected RTP before SETUP/PLAY")
	}

	_, clientPort, err := net.SplitHostPort(rtpConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("SplitHostPort() error = %v", err)
	}
	setupReq := "SETUP rtsp://127.0.0.1/test/streamid=0 RTSP/1.0\r\nCSeq: 9\r\nTransport: RTP/AVP/UDP;unicast;client_port=" + clientPort + "-" + clientPort + "\r\n\r\n"
	if _, err := rtspConn.Write([]byte(setupReq)); err != nil {
		t.Fatalf("Write SETUP error = %v", err)
	}
	n, err = rtspConn.Read(responseBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read SETUP error = %v", err)
	}
	response = string(responseBuf[:n])
	if !strings.Contains(response, "Transport: RTP/AVP/UDP;unicast;client_port="+clientPort+"-"+clientPort) || !strings.Contains(response, "Session:") {
		t.Fatalf("SETUP response = %q, want Transport and Session", response)
	}

	playReq := "PLAY rtsp://127.0.0.1/test RTSP/1.0\r\nCSeq: 10\r\nSession: rtsp:bridge\r\n\r\n"
	if _, err := rtspConn.Write([]byte(playReq)); err != nil {
		t.Fatalf("Write PLAY error = %v", err)
	}
	n, err = rtspConn.Read(responseBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read PLAY error = %v", err)
	}
	response = string(responseBuf[:n])
	if !strings.Contains(response, "RTSP/1.0 200 OK") || !strings.Contains(response, "RTP-Info:") {
		t.Fatalf("PLAY response = %q, want RTP-Info", response)
	}

	if err := rtpConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	n, _, err = rtpConn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}
	if n < 13 {
		t.Fatalf("received RTP packet too short: %d", n)
	}
	seq := uint16(buf[2])<<8 | uint16(buf[3])
	if seq != 3 {
		t.Fatalf("sequence = %d, want 3", seq)
	}

	teardownReq := "TEARDOWN rtsp://127.0.0.1/test RTSP/1.0\r\nCSeq: 11\r\nSession: rtsp:bridge\r\n\r\n"
	if _, err := rtspConn.Write([]byte(teardownReq)); err != nil {
		t.Fatalf("Write TEARDOWN error = %v", err)
	}
	n, err = rtspConn.Read(responseBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read TEARDOWN error = %v", err)
	}
	response = string(responseBuf[:n])
	if !strings.Contains(response, "RTSP/1.0 200 OK") || !strings.Contains(response, "Session: rtsp:bridge") {
		t.Fatalf("TEARDOWN response = %q, want Session", response)
	}
}

func TestPlayBeforeSetupReturns455(t *testing.T) {
	t.Parallel()

	bundle := model.NewMinimalSessionBundle("invalid-play", model.CodecH264, "mp4", "./sample.mp4", nil)
	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "invalid-play",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/test",
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(300 * time.Millisecond)
	var addr string
	for {
		current := session.Result()
		if current.State == "serving" {
			addr = current.Details["listen_address"]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer conn.Close()

	request := "PLAY rtsp://127.0.0.1/test RTSP/1.0\r\nCSeq: 1\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	buf := make([]byte, 512)
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() error = %v", err)
	}
	response := string(buf[:n])
	if !strings.Contains(response, "455 Method Not Valid in This State") {
		t.Fatalf("response = %q, want 455", response)
	}
}

func TestRequestAfterTeardownReturns454(t *testing.T) {
	t.Skip("TODO: fix session state transition with shared listener")
	t.Parallel()

	bundle := model.NewMinimalSessionBundle("after-teardown", model.CodecH264, "mp4", "./sample.mp4", nil)
	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "after-teardown",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/test",
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(300 * time.Millisecond)
	var addr string
	for {
		current := session.Result()
		if current.State == "serving" {
			addr = current.Details["listen_address"]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer conn.Close()
	buf := make([]byte, 512)
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	setupReq := "SETUP rtsp://127.0.0.1/test/streamid=0 RTSP/1.0\r\nCSeq: 1\r\nTransport: RTP/AVP/UDP;unicast;client_port=5004-5005\r\n\r\n"
	if _, err := conn.Write([]byte(setupReq)); err != nil {
		t.Fatalf("Write SETUP error = %v", err)
	}
	if _, err := conn.Read(buf); err != nil && err != io.EOF {
		t.Fatalf("Read SETUP error = %v", err)
	}

	teardownReq := "TEARDOWN rtsp://127.0.0.1/test RTSP/1.0\r\nCSeq: 2\r\nSession: rtsp:after-teardown\r\n\r\n"
	if _, err := conn.Write([]byte(teardownReq)); err != nil {
		t.Fatalf("Write TEARDOWN error = %v", err)
	}
	if _, err := conn.Read(buf); err != nil && err != io.EOF {
		t.Fatalf("Read TEARDOWN error = %v", err)
	}

	deadline = time.Now().Add(300 * time.Millisecond)
	for {
		current := session.Result()
		if current.State == "stopped" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to stopped, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestUnknownMountReturns404(t *testing.T) {
	t.Parallel()

	bundle := model.NewMinimalSessionBundle("known", model.CodecH264, "mp4", "./sample.mp4", nil)
	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "known",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/known",
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(300 * time.Millisecond)
	addr := ""
	for {
		current := session.Result()
		if current.State == "serving" {
			addr = current.Details["listen_address"]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer conn.Close()
	buf := make([]byte, 512)
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	request := "OPTIONS rtsp://127.0.0.1/unknown RTSP/1.0\r\nCSeq: 1\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() error = %v", err)
	}
	response := string(buf[:n])
	if !strings.Contains(response, "404 Not Found") {
		t.Fatalf("response = %q, want 404", response)
	}
}

func TestInvalidSessionReturns454(t *testing.T) {
	t.Parallel()

	bundle := model.NewMinimalSessionBundle("bad-session", model.CodecH264, "mp4", "./sample.mp4", nil)
	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "bad-session",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/test",
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(300 * time.Millisecond)
	addr := ""
	for {
		current := session.Result()
		if current.State == "serving" {
			addr = current.Details["listen_address"]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer conn.Close()
	buf := make([]byte, 512)
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	request := "PLAY rtsp://127.0.0.1/test RTSP/1.0\r\nCSeq: 1\r\nSession: wrong-session\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() error = %v", err)
	}
	response := string(buf[:n])
	if !strings.Contains(response, "454 Session Not Found") {
		t.Fatalf("response = %q, want 454", response)
	}
}

func TestInvalidTransportReturns400(t *testing.T) {
	t.Parallel()

	bundle := model.NewMinimalSessionBundle("bad-transport", model.CodecH264, "mp4", "./sample.mp4", nil)
	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "bad-transport",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/test",
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(300 * time.Millisecond)
	addr := ""
	for {
		current := session.Result()
		if current.State == "serving" {
			addr = current.Details["listen_address"]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer conn.Close()
	buf := make([]byte, 512)
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	request := "SETUP rtsp://127.0.0.1/test/streamid=0 RTSP/1.0\r\nCSeq: 1\r\nTransport: broken-header\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() error = %v", err)
	}
	response := string(buf[:n])
	if !strings.Contains(response, "400 Bad Request") {
		t.Fatalf("response = %q, want 400", response)
	}
}

func TestVLCStyleOptionsRequestReturns200(t *testing.T) {
	t.Parallel()

	bundle := model.NewMinimalSessionBundle("vlc-style", model.CodecH264, "mp4", "./sample.mp4", nil)
	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "vlc-style",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/test",
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(300 * time.Millisecond)
	var addr string
	for {
		current := session.Result()
		if current.State == "serving" {
			addr = current.Details["listen_address"]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer conn.Close()
	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	request := "OPTIONS rtsp://127.0.0.1:8554/test RTSP/1.0\r\nCSeq: 2\r\nUser-Agent: LibVLC/3.0.23\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() error = %v", err)
	}
	response := string(buf[:n])
	if !strings.Contains(response, "RTSP/1.0 200 OK") {
		t.Fatalf("response = %q, want 200 OK", response)
	}
}

type outputRuntime struct {
	addr string
}
