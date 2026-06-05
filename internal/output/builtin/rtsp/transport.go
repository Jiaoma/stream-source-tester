package rtsp

import (
	"fmt"
	"strconv"
	"strings"
)

type transportInfo struct {
	ClientRTPPort  int
	ClientRTCPPort int
	Interleaved    string
	LowerTransport string
	Mode           string
}

func parseTransportHeader(value string) (*transportInfo, error) {
	info := &transportInfo{}
	parts := strings.Split(value, ";")
	found := false
	for i, part := range parts {
		part = strings.TrimSpace(part)
		lower := strings.ToLower(part)
		if i == 0 {
			if strings.Contains(lower, "/tcp") {
				info.LowerTransport = "tcp"
			} else {
				info.LowerTransport = "udp"
			}
		}
		if strings.HasPrefix(lower, "client_port=") {
			found = true
			rangeValue := strings.TrimSpace(strings.TrimPrefix(part, "client_port="))
			first, second, hasSecond := strings.Cut(rangeValue, "-")
			port, err := strconv.Atoi(first)
			if err != nil {
				return nil, fmt.Errorf("parse client_port %q: %w", first, err)
			}
			info.ClientRTPPort = port
			if hasSecond {
				if rtcp, err := strconv.Atoi(second); err == nil {
					info.ClientRTCPPort = rtcp
				}
			}
			continue
		}
		if strings.HasPrefix(lower, "interleaved=") {
			info.Interleaved = strings.TrimSpace(strings.TrimPrefix(part, "interleaved="))
			continue
		}
		if lower == "unicast" || lower == "multicast" {
			info.Mode = lower
		}
	}
	if info.LowerTransport == "tcp" && info.Interleaved != "" {
		return info, nil
	}
	if !found {
		return nil, fmt.Errorf("missing client_port in transport header %q", value)
	}
	return info, nil
}
