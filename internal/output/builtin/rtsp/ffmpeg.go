package rtsp

import (
	"fmt"
	"os/exec"
)

func startFFMPEGRTP(sourcePath string, target string) (*exec.Cmd, error) {
	cmd := exec.Command(
		"ffmpeg",
		"-nostdin",
		"-loglevel", "error",
		"-re",
		"-stream_loop", "-1",
		"-i", sourcePath,
		"-map", "0:v:0",
		"-an",
		"-c:v", "copy",
		"-muxdelay", "0",
		"-pkt_size", "1200",
		"-f", "rtp",
		"udp://"+target,
	)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg RTP sender: %w", err)
	}
	return cmd, nil
}
