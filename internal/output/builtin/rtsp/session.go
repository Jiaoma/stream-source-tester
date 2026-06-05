package rtsp

type sessionState struct {
	described bool
	setupDone bool
	playing   bool
	tornDown  bool
	rtpTarget string
	transport *transportInfo
}
