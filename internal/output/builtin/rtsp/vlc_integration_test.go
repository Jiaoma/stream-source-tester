package rtsp

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func TestVLCCommandDoesNotReceive404(t *testing.T) {
	vlcPath := "/Applications/VLC.app/Contents/MacOS/VLC"
	if _, err := os.Stat(vlcPath); err != nil {
		t.Skipf("VLC binary not available: %v", err)
	}

	ResetListenerRegistryForTest()

	bundle := model.NewMinimalSessionBundle("vlc-cli", model.CodecH264, "mp4", "./sample.mp4", nil)
	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "vlc-cli",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/test",
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	addr := waitServingAddr(t, session)
	url := "rtsp://" + addr + "/test"

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, vlcPath, "-vv", "--verbose=2", url)
	out, err := cmd.CombinedOutput()
	output := string(out)

	if strings.Contains(output, "404 Not Found") {
		t.Fatalf("VLC output contains 404 Not Found:\n%s", output)
	}
	if !strings.Contains(output, "Sending request: OPTIONS") {
		t.Fatalf("VLC output did not reach OPTIONS request stage:\n%s", output)
	}
	if err != nil && !strings.Contains(output, "Received a complete") {
		t.Fatalf("VLC command error: %v\nOutput:\n%s", err, output)
	}
}
