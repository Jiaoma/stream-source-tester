package injectcodecconfig

import (
	"context"
	"fmt"
	"strconv"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
)

// NAL unit types
const (
	// H.264 NAL types
	NALH264SPS = 7
	NALH264PPS = 8
	NALH264IDR = 5

	// H.265 NAL types
	NALH265VPS = 32
	NALH265SPS = 33
	NALH265PPS = 34
	NALH265IDR = 19
)

func init() {
	if err := mutation.Register("inject-codec-config", func() mutation.Mutator { return &Mutator{} }); err != nil {
		panic(err)
	}
}

type Mutator struct{}

func (m *Mutator) Apply(ctx context.Context, bundle *model.SessionBundle, cfg config.MutationConfig) (*model.SessionBundle, error) {
	_ = ctx
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}

	strategy := "first-frame"
	if raw := cfg.Options["injection.strategy"]; raw != "" {
		strategy = raw
	}

	strip := false
	if raw := cfg.Options["injection.strip"]; raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			strip = parsed
		}
	}

	interval := 30
	if raw := cfg.Options["injection.interval"]; raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			interval = parsed
		}
	}

	extraCount := 0
	if raw := cfg.Options["injection.extra.count"]; raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			extraCount = parsed
		}
	}

	bundle.Metadata["mutation.inject-codec-config"] = strategy
	bundle.Metadata["mutation.inject-codec-config.strip"] = strconv.FormatBool(strip)

	codec := model.CodecUnknown
	if len(bundle.Streams) > 0 {
		codec = bundle.Streams[0].Codec
	}

	switch strategy {
	case "none":
		return bundle, nil

	case "setup-only":
		// Out-of-band: put SPS/PPS/VPS in stream parameters (SDP fmtp)
		return bundle, injectSetupOnly(bundle, codec)

	case "first-frame":
		return bundle, injectBeforeFirstFrame(bundle, codec, strip, extraCount)

	case "every-gop":
		return bundle, injectBeforeEveryGOP(bundle, codec, strip, extraCount)

	case "periodic":
		return bundle, injectPeriodically(bundle, codec, strip, extraCount, interval)

	default:
		return nil, fmt.Errorf("unknown injection strategy: %q", strategy)
	}
}

// injectSetupOnly adds codec config to stream parameters (out-of-band SDP)
func injectSetupOnly(bundle *model.SessionBundle, codec model.Codec) error {
	// Collect SPS/PPS/VPS from timeline and store in stream parameters
	var sps, pps, vps []byte

	for _, event := range bundle.Timeline {
		if len(event.Payload) < 1 {
			continue
		}
		nalType := getNALType(event.Payload, codec)
		switch nalType {
		case NALH264SPS:
			sps = event.Payload
		case NALH264PPS:
			pps = event.Payload
		case NALH265VPS:
			vps = event.Payload
		case NALH265SPS:
			sps = event.Payload
		case NALH265PPS:
			pps = event.Payload
		}
	}

	// Store in stream parameters for SDP fmtp sprop-parameter-sets
	if len(bundle.Streams) > 0 {
		params := bundle.Streams[0].Parameters
		if params == nil {
			params = make(map[string]string)
		}
		if sps != nil {
			params["sprop-parameter-sets"] = encodeBase64Multi(sps, pps)
		}
		if vps != nil && sps != nil && pps != nil {
			params["sprop-vps"] = encodeBase64(vps)
			params["sprop-sps"] = encodeBase64(sps)
			params["sprop-pps"] = encodeBase64(pps)
		}
		bundle.Streams[0].Parameters = params
	}

	bundle.Metadata["mutation.inject-codec-config.mode"] = "setup-only"
	return nil
}

// injectBeforeFirstFrame injects SPS/PPS/VPS before the first video frame
func injectBeforeFirstFrame(bundle *model.SessionBundle, codec model.Codec, strip bool, extraCount int) error {
	// Find codec config NALUs in timeline
	codecConfigs := extractCodecConfigs(bundle.Timeline, codec)
	if len(codecConfigs) == 0 {
		bundle.Metadata["mutation.inject-codec-config.warning"] = "no codec config found in stream"
		return nil
	}

	// Build new timeline with codec configs prepended
	newTimeline := make([]model.PacketEvent, 0, len(bundle.Timeline)+len(codecConfigs))

	// Add codec config packets with Marker=true (RTP marker before video starts)
	for i, cfg := range codecConfigs {
		evt := cfg
		evt.Marker = (i == len(codecConfigs)-1) // Mark last config packet
		evt.EmittedAt = 0
		evt.Metadata = map[string]string{"codec.config": "true"}
		newTimeline = append(newTimeline, evt)
	}

	// Find first video frame and track IDR position
	firstIDR := -1
	for i, event := range bundle.Timeline {
		if isIDRFrame(event.Payload, codec) {
			firstIDR = i
			break
		}
	}

	// Add timeline events before first IDR
	if firstIDR > 0 {
		newTimeline = append(newTimeline, bundle.Timeline[:firstIDR]...)
	}

	// Add video frames (optionally stripping existing SPS/PPS)
	for i, event := range bundle.Timeline {
		if i < firstIDR || firstIDR == -1 {
			continue
		}
		if strip && isCodecConfig(event.Payload, codec) {
			continue // Skip existing codec config
		}
		newTimeline = append(newTimeline, event)
	}

	bundle.Timeline = newTimeline
	bundle.Metadata["mutation.inject-codec-config.mode"] = "first-frame"
	bundle.Metadata["mutation.inject-codec-config.injected"] = strconv.Itoa(len(codecConfigs))
	return nil
}

// injectBeforeEveryGOP injects codec config before every IDR/GOP start
func injectBeforeEveryGOP(bundle *model.SessionBundle, codec model.Codec, strip bool, extraCount int) error {
	codecConfigs := extractCodecConfigs(bundle.Timeline, codec)
	if len(codecConfigs) == 0 {
		bundle.Metadata["mutation.inject-codec-config.warning"] = "no codec config found in stream"
		return nil
	}

	newTimeline := make([]model.PacketEvent, 0, len(bundle.Timeline)*2)
	lastInject := -1

	for i, event := range bundle.Timeline {
		if isIDRFrame(event.Payload, codec) {
			if i > lastInject {
				// Insert codec configs before this IDR
				for j, cfg := range codecConfigs {
					evt := cfg
					evt.Marker = (j == len(codecConfigs)-1)
					evt.EmittedAt = event.EmittedAt
					evt.Metadata = map[string]string{"codec.config": "true", "injected.gop": strconv.Itoa(i)}
					newTimeline = append(newTimeline, evt)
				}
				lastInject = i
			}
		}
		if !strip || !isCodecConfig(event.Payload, codec) {
			newTimeline = append(newTimeline, event)
		}
	}

	bundle.Timeline = newTimeline
	bundle.Metadata["mutation.inject-codec-config.mode"] = "every-gop"
	bundle.Metadata["mutation.inject-codec-config.gops"] = strconv.Itoa(lastInject + 1)
	return nil
}

// injectPeriodically injects codec config every N frames
func injectPeriodically(bundle *model.SessionBundle, codec model.Codec, strip bool, extraCount int, interval int) error {
	codecConfigs := extractCodecConfigs(bundle.Timeline, codec)
	if len(codecConfigs) == 0 {
		bundle.Metadata["mutation.inject-codec-config.warning"] = "no codec config found in stream"
		return nil
	}

	newTimeline := make([]model.PacketEvent, 0, len(bundle.Timeline)*2)
	frameCount := 0

	for _, event := range bundle.Timeline {
		// Check if this is a video frame (not just codec config)
		if !isCodecConfig(event.Payload, codec) {
			frameCount++
			if frameCount%interval == 0 {
				// Insert codec configs
				for j, cfg := range codecConfigs {
					evt := cfg
					evt.Marker = (j == len(codecConfigs)-1)
					evt.EmittedAt = event.EmittedAt
					evt.Metadata = map[string]string{"codec.config": "true", "injected.frame": strconv.Itoa(frameCount)}
					newTimeline = append(newTimeline, evt)
				}
			}
		}
		if !strip || !isCodecConfig(event.Payload, codec) {
			newTimeline = append(newTimeline, event)
		}
	}

	bundle.Timeline = newTimeline
	bundle.Metadata["mutation.inject-codec-config.mode"] = "periodic"
	bundle.Metadata["mutation.inject-codec-config.interval"] = strconv.Itoa(interval)
	return nil
}

// extractCodecConfigs extracts all SPS/PPS/VPS NALUs from the timeline
func extractCodecConfigs(timeline []model.PacketEvent, codec model.Codec) []model.PacketEvent {
	seen := make(map[string]bool)
	var configs []model.PacketEvent

	for _, event := range timeline {
		if len(event.Payload) < 1 {
			continue
		}
		if !isCodecConfig(event.Payload, codec) {
			continue
		}
		// Deduplicate by payload content
		key := string(event.Payload)
		if seen[key] {
			continue
		}
		seen[key] = true
		configs = append(configs, event)
	}

	return configs
}

// isCodecConfig returns true if the NALU is SPS, PPS, or VPS
func isCodecConfig(payload []byte, codec model.Codec) bool {
	if len(payload) < 1 {
		return false
	}
	nalType := getNALType(payload, codec)
	switch codec {
	case model.CodecH264:
		return nalType == NALH264SPS || nalType == NALH264PPS
	case model.CodecH265:
		return nalType == NALH265VPS || nalType == NALH265SPS || nalType == NALH265PPS
	default:
		return false
	}
}

// isIDRFrame returns true if this NALU starts an IDR frame
func isIDRFrame(payload []byte, codec model.Codec) bool {
	if len(payload) < 1 {
		return false
	}
	nalType := getNALType(payload, codec)
	switch codec {
	case model.CodecH264:
		return nalType == NALH264IDR
	case model.CodecH265:
		return nalType == NALH265IDR
	default:
		return false
	}
}

// getNALType extracts NAL type from H.264/H.265 Annex B format
// For Annex B, the NAL header is the first byte (or second if there's a start code)
func getNALType(payload []byte, codec model.Codec) int {
	if len(payload) < 1 {
		return -1
	}
	// H.264/H.265 Annex B: first byte is 0x00 0x00 0x00 0x01 or 0x00 0x00 0x01, then NAL header
	start := 0
	if len(payload) >= 4 && payload[0] == 0 && payload[1] == 0 && payload[2] == 0 && payload[3] == 1 {
		start = 4
	} else if len(payload) >= 3 && payload[0] == 0 && payload[1] == 0 && payload[2] == 1 {
		start = 3
	}
	if start >= len(payload) {
		return -1
	}
	switch codec {
	case model.CodecH264:
		// H.264: NAL type is lower 5 bits
		return int(payload[start] & 0x1F)
	case model.CodecH265:
		// H.265: NAL type is in bits 1-6 of byte (after forbidden_zero_bit at bit 7)
		return int(payload[start] >> 1 & 0x3F)
	default:
		return -1
	}
}

func encodeBase64(data []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	result := make([]byte, (len(data)+2)/3*4)
	for i, j := 0, 0; i < len(data); i, j = i+3, j+4 {
		var val uint32
		switch len(data) - i {
		case 1:
			val = uint32(data[i]) << 16
		case 2:
			val = uint32(data[i])<<16 | uint32(data[i+1])<<8
		default:
			val = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
		}
		result[j] = alphabet[(val>>18)&0x3F]
		result[j+1] = alphabet[(val>>12)&0x3F]
		if len(data)-i >= 2 {
			result[j+2] = alphabet[(val>>6)&0x3F]
		}
		if len(data)-i >= 3 {
			result[j+3] = alphabet[val&0x3F]
		}
	}
	// Add padding
	switch len(data) % 3 {
	case 1:
		result[len(result)-1] = '='
		result[len(result)-2] = '='
	case 2:
		result[len(result)-1] = '='
	}
	return string(result)
}

func encodeBase64Multi(sps, pps []byte) string {
	return encodeBase64(sps) + "," + encodeBase64(pps)
}
