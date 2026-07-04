package rtsp

import (
	"context"
	"os/exec"
)

type sessionState struct {
	described     bool
	setupDone     bool
	playing       bool
	tornDown      bool
	authenticated bool
	rtpTarget     string
	transport     *transportInfo
	ffmpeg        *exec.Cmd
	playbackStop  context.CancelFunc
	parameters    map[string]string
}

func (s *sessionState) stopPlayback() {
	if s.playbackStop != nil {
		s.playbackStop()
		s.playbackStop = nil
	}
	if s.ffmpeg != nil && s.ffmpeg.Process != nil {
		_ = s.ffmpeg.Process.Kill()
		_, _ = s.ffmpeg.Process.Wait()
		s.ffmpeg = nil
	}
}
