package rtsp

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func dialRTSP(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	return conn
}

func sendRTSP(t *testing.T, conn net.Conn, reader *bufio.Reader, raw string) string {
	t.Helper()
	if _, err := conn.Write([]byte(raw)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	// read status line + headers until blank line, then optional body
	var builder strings.Builder
	contentLength := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			t.Fatalf("read response error = %v", err)
		}
		builder.WriteString(line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "content-length:") {
			_, v, _ := strings.Cut(trimmed, ":")
			contentLength = atoiSafe(strings.TrimSpace(v))
		}
		if trimmed == "" {
			break
		}
		if err == io.EOF {
			return builder.String()
		}
	}
	if contentLength > 0 {
		body := make([]byte, contentLength)
		if _, err := io.ReadFull(reader, body); err == nil {
			builder.Write(body)
		}
	}
	return builder.String()
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func startAuthCamera(t *testing.T, options map[string]string) (string, func()) {
	t.Helper()
	ResetListenerRegistryForTest()
	bundle := model.NewMinimalSessionBundle("cam", model.CodecH264, "mp4", "./sample.mp4", nil)
	cfg := config.OutputConfig{
		Name:    "cam",
		Kind:    "rtsp",
		Target:  "rtsp://127.0.0.1:0/cam",
		Options: options,
	}
	session, err := (&Sink{}).Serve(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	addr := waitServingAddr(t, session)
	return addr, func() { _ = session.Close() }
}

func TestUnauthenticatedDescribeReturns401(t *testing.T) {
	addr, stop := startAuthCamera(t, map[string]string{
		"auth.mode":     "basic",
		"auth.username": "admin",
		"auth.password": "secret",
	})
	defer stop()

	conn := dialRTSP(t, addr)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	resp := sendRTSP(t, conn, reader, "DESCRIBE rtsp://127.0.0.1/cam RTSP/1.0\r\nCSeq: 1\r\n\r\n")
	if !strings.Contains(resp, "401 Unauthorized") {
		t.Fatalf("response = %q, want 401", resp)
	}
	if !strings.Contains(resp, "WWW-Authenticate: Basic realm=") {
		t.Fatalf("response missing WWW-Authenticate challenge: %q", resp)
	}
}

func TestAuthenticatedDescribeSucceeds(t *testing.T) {
	addr, stop := startAuthCamera(t, map[string]string{
		"auth.mode":     "basic",
		"auth.username": "admin",
		"auth.password": "secret",
	})
	defer stop()

	conn := dialRTSP(t, addr)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// "admin:secret" base64 = YWRtaW46c2VjcmV0
	resp := sendRTSP(t, conn, reader, "DESCRIBE rtsp://127.0.0.1/cam RTSP/1.0\r\nCSeq: 1\r\nAuthorization: Basic YWRtaW46c2VjcmV0\r\n\r\n")
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("response = %q, want 200 OK", resp)
	}
	if !strings.Contains(resp, "application/sdp") {
		t.Fatalf("response missing SDP: %q", resp)
	}
}

func TestOptionsStaysAnonymous(t *testing.T) {
	addr, stop := startAuthCamera(t, map[string]string{
		"auth.mode":     "basic",
		"auth.username": "admin",
		"auth.password": "secret",
	})
	defer stop()

	conn := dialRTSP(t, addr)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	resp := sendRTSP(t, conn, reader, "OPTIONS rtsp://127.0.0.1/cam RTSP/1.0\r\nCSeq: 1\r\n\r\n")
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("OPTIONS response = %q, want 200 OK", resp)
	}
	if !strings.Contains(resp, "SET_PARAMETER") || !strings.Contains(resp, "GET_PARAMETER") {
		t.Fatalf("OPTIONS Public missing parameter methods: %q", resp)
	}
}

func TestSetParameterBehaviors(t *testing.T) {
	addr, stop := startAuthCamera(t, nil)
	defer stop()

	conn := dialRTSP(t, addr)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// keepalive: empty body
	resp := sendRTSP(t, conn, reader, "SET_PARAMETER rtsp://127.0.0.1/cam RTSP/1.0\r\nCSeq: 1\r\n\r\n")
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("keepalive SET_PARAMETER = %q, want 200 OK", resp)
	}

	// valid assignment
	body := "framerate: 30\r\n"
	req := "SET_PARAMETER rtsp://127.0.0.1/cam RTSP/1.0\r\nCSeq: 2\r\nContent-Length: " + itoa(len(body)) + "\r\n\r\n" + body
	resp = sendRTSP(t, conn, reader, req)
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("valid SET_PARAMETER = %q, want 200 OK", resp)
	}

	// unsupported parameter
	body = "unknown_param: 1\r\n"
	req = "SET_PARAMETER rtsp://127.0.0.1/cam RTSP/1.0\r\nCSeq: 3\r\nContent-Length: " + itoa(len(body)) + "\r\n\r\n" + body
	resp = sendRTSP(t, conn, reader, req)
	if !strings.Contains(resp, "451 Parameter Not Understood") {
		t.Fatalf("unsupported SET_PARAMETER = %q, want 451", resp)
	}
}

func TestGetParameterEchoesStoredValue(t *testing.T) {
	addr, stop := startAuthCamera(t, nil)
	defer stop()

	conn := dialRTSP(t, addr)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	body := "framerate: 25\r\n"
	req := "SET_PARAMETER rtsp://127.0.0.1/cam RTSP/1.0\r\nCSeq: 1\r\nContent-Length: " + itoa(len(body)) + "\r\n\r\n" + body
	if resp := sendRTSP(t, conn, reader, req); !strings.Contains(resp, "200 OK") {
		t.Fatalf("SET_PARAMETER = %q, want 200 OK", resp)
	}

	q := "framerate\r\n"
	getReq := "GET_PARAMETER rtsp://127.0.0.1/cam RTSP/1.0\r\nCSeq: 2\r\nContent-Length: " + itoa(len(q)) + "\r\n\r\n" + q
	resp := sendRTSP(t, conn, reader, getReq)
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("GET_PARAMETER = %q, want 200 OK", resp)
	}
	if !strings.Contains(resp, "framerate: 25") {
		t.Fatalf("GET_PARAMETER did not echo stored value: %q", resp)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
