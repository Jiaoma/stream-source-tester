package rtsp

import "os/exec"

type sessionState struct {
	described     bool
	setupDone     bool
	playing       bool
	tornDown      bool
	authenticated bool
	rtpTarget     string
	transport     *transportInfo
	ffmpeg        *exec.Cmd
	parameters    map[string]string
}
