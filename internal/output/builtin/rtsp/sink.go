package rtsp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/output"
)

func init() {
	if err := output.Register("rtsp", func() output.Sink { return &Sink{} }); err != nil {
		panic(err)
	}
}

type Sink struct{}

func (s *Sink) Serve(ctx context.Context, bundle *model.SessionBundle, cfg config.OutputConfig) (output.SessionHandle, error) {
	_ = ctx
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}

	listenAddress := ""
	mountPath := ""
	transportMode := "rtsp"
	if parsed, err := url.Parse(cfg.Target); err == nil {
		listenAddress = parsed.Host
		mountPath = strings.TrimPrefix(parsed.Path, "/")
		if mountPath == "" {
			mountPath = "stream"
		}
	} else {
		return nil, fmt.Errorf("invalid rtsp target %q: %w", cfg.Target, err)
	}

	sessionID := fmt.Sprintf("%s:%s", cfg.Kind, bundle.Name)
	startedAt := time.Now().UTC()
	bundle.Metadata["served.by"] = cfg.Kind
	bundle.Metadata["served.target"] = cfg.Target
	bundle.Metadata["served.name"] = cfg.Name
	bundle.Metadata["served.streams"] = strconv.Itoa(len(bundle.Streams))
	bundle.Metadata["served.timeline"] = strconv.Itoa(len(bundle.Timeline))
	bundle.Metadata["served.state"] = string(output.StateReady)
	bundle.Metadata["served.session_id"] = sessionID
	bundle.Metadata["served.listen_address"] = listenAddress
	bundle.Metadata["served.mount_path"] = mountPath

	sl, err := getOrCreateListener(ctx, listenAddress)
	if err != nil {
		return nil, err
	}
	actualAddress := sl.addr
	bundle.Metadata["served.listen_address"] = actualAddress

	session := output.NewManagedSession(output.RuntimeResult{
		SessionID:   sessionID,
		ProfileName: bundle.Name,
		SinkKind:    cfg.Kind,
		Target:      cfg.Target,
		State:       output.StateReady,
		Streams:     len(bundle.Streams),
		Timeline:    len(bundle.Timeline),
		StartedAt:   startedAt,
		Details: map[string]string{
			"output.name":    cfg.Name,
			"listen_address": actualAddress,
			"mount_path":     mountPath,
			"transport_mode": transportMode,
			"state":          string(output.StateReady),
		},
	})

	auth := loadAuthConfig(cfg)
	handler := func(conn net.Conn) {
		defer conn.Close()
		reader := bufio.NewReader(conn)
		state := &sessionState{}
		for {
			req, err := readRequestFromReader(reader)
			if err != nil {
				return
			}

			if state.tornDown {
				_, _ = conn.Write([]byte(responseFor(req, sessionID, "RTSP/1.0 454 Session Not Found", nil, "")))
				return
			}
			if req.Session != "" && req.Session != sessionID {
				_, _ = conn.Write([]byte(responseFor(req, sessionID, "RTSP/1.0 454 Session Not Found", nil, "")))
				continue
			}
			if statusLine, ok := configuredStatusLine(cfg, req.Method); ok {
				_, _ = conn.Write([]byte(responseFor(req, sessionID, statusLine, nil, "")))
				continue
			}
			if auth.enabled() && requiresAuth(req.Method) && !auth.checkBasicAuth(req.Authorization) {
				_, _ = conn.Write([]byte(responseFor(req, sessionID, "RTSP/1.0 401 Unauthorized", auth.challengeHeader(), "")))
				continue
			}

			switch req.Method {
			case "DESCRIBE":
				state.described = true
				_, _ = conn.Write([]byte(okResponseFor(req, bundle, sessionID)))
			case "SETUP":
				parsed, err := parseTransportHeader(req.Transport)
				if err != nil {
					_, _ = conn.Write([]byte(responseFor(req, sessionID, "RTSP/1.0 400 Bad Request", nil, "")))
					continue
				}
				state.setupDone = true
				state.transport = parsed
				if parsed.ClientRTPPort > 0 {
					if remote, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
						state.rtpTarget = fmt.Sprintf("%s:%d", remote.IP.String(), parsed.ClientRTPPort)
					}
				}
				transportHeader := transportHeaderForResponse(req, parsed)
				_, _ = conn.Write([]byte(responseFor(req, sessionID, "RTSP/1.0 200 OK", map[string]string{"Transport": transportHeader}, "")))
			case "PLAY":
				if !state.setupDone {
					_, _ = conn.Write([]byte(responseFor(req, sessionID, "RTSP/1.0 455 Method Not Valid in This State", nil, "")))
					continue
				}
				state.playing = true
				_, _ = conn.Write([]byte(okResponseFor(req, bundle, sessionID)))
				if state.transport != nil && state.transport.LowerTransport == "tcp" {
					channel := state.transport.Interleaved
					if channel == "" {
						channel = "0-1"
					}
					sendInterleavedOnce(conn, bundle, channel)
					continue
				}
				target := state.rtpTarget
				if target == "" {
					target = cfg.Options["rtpTarget"]
				}
				if target != "" {
					if bundle.Metadata["source.kind"] == "mp4" && bundle.Metadata["mutation.names"] == "passthrough" {
						if sourcePath := bundle.Metadata["source.location"]; sourcePath != "" {
							cmd, err := startFFMPEGRTP(sourcePath, target)
							if err == nil {
								state.ffmpeg = cmd
								continue
							}
						}
					}
					sendTimelineOnce(bundle, target)
				}
			case "TEARDOWN":
				state.tornDown = true
				_, _ = conn.Write([]byte(okResponseFor(req, bundle, sessionID)))
				if state.ffmpeg != nil && state.ffmpeg.Process != nil {
					_ = state.ffmpeg.Process.Kill()
					_, _ = state.ffmpeg.Process.Wait()
					state.ffmpeg = nil
				}
				session.SetState(output.StateStopped)
				_ = session.Close()
				return
			case "SET_PARAMETER":
				stored, unsupported := applySetParameter(state, req.Body)
				if unsupported != "" {
					_, _ = conn.Write([]byte(responseFor(req, sessionID, "RTSP/1.0 451 Parameter Not Understood", nil, "")))
					continue
				}
				_ = stored
				_, _ = conn.Write([]byte(responseFor(req, sessionID, "RTSP/1.0 200 OK", nil, "")))
			case "GET_PARAMETER":
				body := buildGetParameterBody(state, req.Body)
				headers := map[string]string{}
				if body != "" {
					headers["Content-Type"] = "text/parameters"
					headers["Content-Length"] = strconv.Itoa(len(body))
				}
				_, _ = conn.Write([]byte(responseFor(req, sessionID, "RTSP/1.0 200 OK", headers, body)))
			default:
				_, _ = conn.Write([]byte(okResponseFor(req, bundle, sessionID)))
			}
		}
	}

	sl.registerMount(mountPath, handler)
	session.SetState(output.StateServing)

	go func() {
		for {
			if session.Result().State == output.StateStopped {
				sl.unregisterMount(mountPath)
				return
			}
			select {
			case <-ctx.Done():
				_ = session.Close()
				sl.unregisterMount(mountPath)
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()

	return session, nil
}
