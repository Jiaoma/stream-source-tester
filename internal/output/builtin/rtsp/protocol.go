package rtsp

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"stream-source-tester/internal/model"
)

type request struct {
	Method    string
	URI       string
	Proto     string
	CSeq      string
	Transport string
	Session   string
}

func readRequest(r io.Reader) (*request, error) {
	return readRequestFromReader(bufio.NewReader(r))
}

func readRequestFromReader(reader *bufio.Reader) (*request, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid request line %q", strings.TrimSpace(line))
	}

	req := &request{Method: parts[0], URI: parts[1], Proto: parts[2]}
	for {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		header = strings.TrimSpace(header)
		if header == "" {
			break
		}
		key, value, ok := strings.Cut(header, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "cseq":
			req.CSeq = strings.TrimSpace(value)
		case "transport":
			req.Transport = strings.TrimSpace(value)
		case "session":
			req.Session = strings.TrimSpace(value)
		}
	}
	return req, nil
}

func responseFor(req *request, sessionID string, statusLine string, headers map[string]string, body string) string {
	cseq := req.CSeq
	if cseq == "" {
		cseq = "1"
	}

	var response strings.Builder
	response.WriteString(statusLine)
	response.WriteString("\r\n")
	response.WriteString(fmt.Sprintf("CSeq: %s\r\n", cseq))
	if sessionID != "" {
		response.WriteString(fmt.Sprintf("Session: %s\r\n", sessionID))
	}
	for key, value := range headers {
		response.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	response.WriteString("\r\n")
	response.WriteString(body)
	return response.String()
}

func okResponseFor(req *request, bundle *model.SessionBundle, sessionID string) string {
	switch req.Method {
	case "OPTIONS":
		return responseFor(req, sessionID, "RTSP/1.0 200 OK", map[string]string{
			"Public": "OPTIONS, DESCRIBE, SETUP, PLAY, TEARDOWN",
		}, "")
	case "DESCRIBE":
		sdp := maybeGenerateSDPFromSource(bundle)
		if sdp == "" {
			sdp = buildSDP(bundle)
		}
		return responseFor(req, sessionID, "RTSP/1.0 200 OK", map[string]string{
			"Content-Type":   "application/sdp",
			"Content-Length": fmt.Sprintf("%d", len(sdp)),
		}, sdp)
	case "SETUP":
		transport := req.Transport
		if transport == "" {
			transport = "RTP/AVP/UDP;unicast;client_port=5004-5005;server_port=5006-5007"
		}
		return responseFor(req, sessionID, "RTSP/1.0 200 OK", map[string]string{
			"Transport": transport,
		}, "")
	case "PLAY":
		return responseFor(req, sessionID, "RTSP/1.0 200 OK", map[string]string{
			"RTP-Info": fmt.Sprintf("url=%s", req.URI),
		}, "")
	case "TEARDOWN":
		return responseFor(req, sessionID, "RTSP/1.0 200 OK", nil, "")
	default:
		return responseFor(req, sessionID, "RTSP/1.0 200 OK", nil, "")
	}
}

func buildSDP(bundle *model.SessionBundle) string {
	var response strings.Builder
	response.WriteString(fmt.Sprintf("v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=%s\r\nt=0 0\r\n", bundle.Name))
	for i, stream := range bundle.Streams {
		codec := "H264"
		switch stream.Codec {
		case model.CodecH265:
			codec = "H265"
		case model.CodecMJPEG:
			codec = "JPEG"
		case model.CodecUnknown:
			if stream.Kind == model.MediaAudio {
				codec = "L16"
			} else {
				codec = "UNKNOWN"
			}
		default:
			codec = "H264"
		}
		mediaType := "video"
		if stream.Kind == model.MediaAudio {
			mediaType = "audio"
		}
		response.WriteString(fmt.Sprintf("m=%s 0 RTP/AVP %d\r\n", mediaType, stream.PayloadType))
		response.WriteString(fmt.Sprintf("a=control:streamid=%d\r\n", i))
		response.WriteString(fmt.Sprintf("a=rtpmap:%d %s/%d\r\n", stream.PayloadType, codec, stream.ClockRate))
		if len(stream.Parameters) > 0 {
			params := make([]string, 0, len(stream.Parameters))
			for key, value := range stream.Parameters {
				params = append(params, fmt.Sprintf("%s=%s", key, value))
			}
			sort.Strings(params)
			response.WriteString(fmt.Sprintf("a=fmtp:%d %s\r\n", stream.PayloadType, strings.Join(params, ";")))
		}
	}
	return response.String()
}
