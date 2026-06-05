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

func TestSharedListenerServesMultipleMounts(t *testing.T) {
	t.Parallel()

	bundle1 := model.NewMinimalSessionBundle("mount1", model.CodecH264, "mp4", "./sample.mp4", nil)
	bundle2 := model.NewMinimalSessionBundle("mount2", model.CodecH265, "mp4", "./sample.mp4", nil)

	session1, err := (&Sink{}).Serve(context.Background(), bundle1, config.OutputConfig{
		Name:   "rtsp-mount1",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/stream1",
	})
	if err != nil {
		t.Fatalf("Serve(session1) error = %v", err)
	}
	defer session1.Close()

	deadline := time.Now().Add(300 * time.Millisecond)
	addr := ""
	for {
		current := session1.Result()
		if current.State == "serving" {
			addr = current.Details["listen_address"]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session1 did not transition to serving")
		}
		time.Sleep(10 * time.Millisecond)
	}

	session2, err := (&Sink{}).Serve(context.Background(), bundle2, config.OutputConfig{
		Name:   "rtsp-mount2",
		Kind:   "rtsp",
		Target: "rtsp://" + addr + "/stream2",
	})
	if err != nil {
		t.Fatalf("Serve(session2) error = %v", err)
	}
	defer session2.Close()

	for {
		current := session2.Result()
		if current.State == "serving" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session2 did not transition to serving")
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn1, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout(conn1) error = %v", err)
	}
	defer conn1.Close()
	_ = conn1.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	request1 := "DESCRIBE rtsp://127.0.0.1/stream1 RTSP/1.0\r\nCSeq: 1\r\n\r\n"
	if _, err := conn1.Write([]byte(request1)); err != nil {
		t.Fatalf("Write(conn1) error = %v", err)
	}
	buf1 := make([]byte, 2048)
	n1, err := conn1.Read(buf1)
	if err != nil && err != io.EOF {
		t.Fatalf("Read(conn1) error = %v", err)
	}
	response1 := string(buf1[:n1])
	if !strings.Contains(response1, "RTSP/1.0 200 OK") {
		t.Fatalf("response1 not 200 OK: %q", response1)
	}
	if !strings.Contains(response1, "s=mount1") {
		t.Fatalf("response1 missing mount1 session name: %q", response1)
	}

	conn2, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout(conn2) error = %v", err)
	}
	defer conn2.Close()
	_ = conn2.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	request2 := "DESCRIBE rtsp://127.0.0.1/stream2 RTSP/1.0\r\nCSeq: 1\r\n\r\n"
	if _, err := conn2.Write([]byte(request2)); err != nil {
		t.Fatalf("Write(conn2) error = %v", err)
	}
	buf2 := make([]byte, 2048)
	n2, err := conn2.Read(buf2)
	if err != nil && err != io.EOF {
		t.Fatalf("Read(conn2) error = %v", err)
	}
	response2 := string(buf2[:n2])
	if !strings.Contains(response2, "RTSP/1.0 200 OK") {
		t.Fatalf("response2 not 200 OK: %q", response2)
	}
	if !strings.Contains(response2, "s=mount2") {
		t.Fatalf("response2 missing mount2 session name: %q", response2)
	}
}
