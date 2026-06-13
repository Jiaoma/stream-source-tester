package rtsp

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"stream-source-tester/internal/model"
)

func maybeGenerateSDPFromSource(bundle *model.SessionBundle) string {
	if bundle == nil || bundle.Metadata == nil {
		return ""
	}
	if bundle.Metadata["source.kind"] != "mp4" {
		return ""
	}
	sourcePath := bundle.Metadata["source.location"]
	if sourcePath == "" {
		return ""
	}
	tmp, err := os.CreateTemp("", "stream-source-tester-*.sdp")
	if err != nil {
		return ""
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command(
		"ffmpeg",
		"-v", "error",
		"-i", sourcePath,
		"-an",
		"-c:v", "copy",
		"-t", "0.1",
		"-f", "rtp",
		"-sdp_file", tmpPath,
		"rtp://127.0.0.1:5004",
	)
	if err := cmd.Run(); err != nil {
		return ""
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return ""
	}
	return normalizeGeneratedSDP(string(data), bundle)
}

func normalizeGeneratedSDP(raw string, bundle *model.SessionBundle) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines)+4)
	track := -1
	controlInserted := false
	flushControl := func() {
		if track >= 0 && !controlInserted {
			out = append(out, fmt.Sprintf("a=control:streamid=%d", track))
		}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "m=") {
			flushControl()
			track++
			controlInserted = false
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				parts[1] = "0"
				line = strings.Join(parts, " ")
			}
			out = append(out, line)
			continue
		}
		if strings.HasPrefix(line, "a=control:") {
			controlInserted = true
		}
		out = append(out, line)
	}
	flushControl()

	if len(out) == 0 {
		return buildFallbackSDP(bundle)
	}
	return strings.Join(out, "\r\n") + "\r\n"
}

func buildFallbackSDP(bundle *model.SessionBundle) string {
	return buildSDP(bundle)
}
