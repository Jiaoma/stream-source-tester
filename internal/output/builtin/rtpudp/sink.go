package rtpudp

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/output"
)

func init() {
	if err := output.Register("rtp-udp", func() output.Sink { return &Sink{} }); err != nil {
		panic(err)
	}
}

type Sink struct{}

func (s *Sink) Serve(ctx context.Context, bundle *model.SessionBundle, cfg config.OutputConfig) (output.SessionHandle, error) {
	_ = ctx
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}
	if len(bundle.Streams) == 0 {
		return nil, fmt.Errorf("bundle %q has no streams", bundle.Name)
	}

	host := ""
	port := ""
	if parsedHost, parsedPort, err := net.SplitHostPort(cfg.Target); err == nil {
		host = parsedHost
		port = parsedPort
	} else {
		return nil, fmt.Errorf("invalid rtp-udp target %q: %w", cfg.Target, err)
	}
	payloadType := ""
	ssrc := ""
	if len(bundle.Streams) > 0 {
		payloadType = strconv.Itoa(int(bundle.Streams[0].PayloadType))
		ssrc = strconv.FormatUint(uint64(bundle.Streams[0].SSRC), 10)
	}
	streamIndex := make(map[string]model.Stream, len(bundle.Streams))
	for _, stream := range bundle.Streams {
		streamIndex[stream.ID] = stream
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
	bundle.Metadata["served.host"] = host
	bundle.Metadata["served.port"] = port

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
			"output.name":  cfg.Name,
			"host":         host,
			"port":         port,
			"payload_type": payloadType,
			"ssrc":         ssrc,
			"state":        string(output.StateReady),
		},
	})

	timeline := append([]model.PacketEvent(nil), bundle.Timeline...)
	go func() {
		addr, err := net.ResolveUDPAddr("udp", cfg.Target)
		if err != nil {
			session.SetState(output.StateFailed)
			return
		}
		conn, err := net.DialUDP("udp", nil, addr)
		if err != nil {
			session.SetState(output.StateFailed)
			return
		}
		defer conn.Close()

		session.SetState(output.StateServing)
		if len(timeline) > 0 {
			start := time.Now()
			for _, event := range timeline {
				wait := event.EmittedAt - time.Since(start)
				if wait > 0 {
					timer := time.NewTimer(wait)
					select {
					case <-ctx.Done():
						timer.Stop()
						_ = session.Close()
						return
					case <-timer.C:
					}
				}
				if session.Result().State == output.StateStopped {
					return
				}
				stream, ok := streamIndex[event.StreamID]
				if !ok {
					continue
				}
				packet := encodePacket(stream, event)
				_, _ = conn.Write(packet)
			}
		}

		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				_ = session.Close()
				return
			case <-ticker.C:
				if session.Result().State == output.StateStopped {
					return
				}
			}
		}
	}()

	return session, nil
}
