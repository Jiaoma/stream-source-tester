package switchssrc

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
)

func init() {
	if err := mutation.Register("switch-ssrc", func() mutation.Mutator { return &Mutator{} }); err != nil {
		panic(err)
	}
}

// Mutator implements mid-stream SSRC switching.
type Mutator struct{}

func (m *Mutator) Apply(ctx context.Context, bundle *model.SessionBundle, cfg config.MutationConfig) (*model.SessionBundle, error) {
	_ = ctx
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}

	if len(bundle.Streams) == 0 {
		bundle.Metadata["mutation.switch-ssrc"] = "skipped (no streams)"
		return bundle, nil
	}

	// Parse switch.at - sequence number or timestamp trigger
	switchAt := 100 // default: switch at packet 100
	if raw := cfg.Options["ssrc.switch.at"]; raw != "" {
		if val, err := strconv.Atoi(raw); err == nil {
			switchAt = val
		}
	}

	// Parse switch.to - new SSRC value
	newSSRC := uint32(0x12345678) // default
	if raw := cfg.Options["ssrc.switch.to"]; raw != "" {
		// Try hex format first (0x...)
		if len(raw) > 2 && raw[0] == '0' && (raw[1] == 'x' || raw[1] == 'X') {
			if parsed, err := hex.DecodeString(raw[2:]); err == nil && len(parsed) == 4 {
				newSSRC = binary.BigEndian.Uint32(parsed)
			}
		} else if val, err := strconv.ParseUint(raw, 10, 32); err == nil {
			newSSRC = uint32(val)
		}
	}

	// Parse switch.count - number of switches to perform
	switchCount := 1
	if raw := cfg.Options["ssrc.switch.count"]; raw != "" {
		if val, err := strconv.Atoi(raw); err == nil && val > 0 {
			switchCount = val
		}
	}

	// Parse switch.mode
	switchMode := "immediate"
	if raw := cfg.Options["ssrc.switch.mode"]; raw != "" {
		switchMode = raw
	}

	// Track initial SSRC for history
	initialSSRC := bundle.Streams[0].SSRC

	// Create a map for per-packet SSRC overrides via Payload metadata
	// The SSRC override will be stored in PacketEvent.Metadata["override.ssrc"]
	// and the rtpudp/sink.go will need to check for this

	switchCountLeft := switchCount
	nextSwitchAt := switchAt

	switch switchMode {
	case "immediate":
		// Find the packet at switchAt and change SSRC from there
		for i := range bundle.Timeline {
			if i+1 >= nextSwitchAt { // 1-indexed sequence numbers
				if switchCountLeft > 0 {
					bundle.Timeline[i].Metadata = addSSRCOverride(bundle.Timeline[i].Metadata, newSSRC)
					switchCountLeft--
					nextSwitchAt = switchAt + (switchCount-switchCountLeft)*(switchAt+1)
				}
			}
		}

	case "gradual":
		// Transition over N packets
		transitionPackets := 10
		if raw := cfg.Options["ssrc.switch.transition-packets"]; raw != "" {
			if val, err := strconv.Atoi(raw); err == nil && val > 0 {
				transitionPackets = val
			}
		}

		transitionStarted := false
		transitionIndex := 0
		originalSSRC := initialSSRC
		ssrcDelta := int64(newSSRC) - int64(originalSSRC)

		for i := range bundle.Timeline {
			if i+1 >= nextSwitchAt && switchCountLeft > 0 {
				if !transitionStarted {
					transitionStarted = true
					transitionIndex = 0
				}
			}

			if transitionStarted {
				progress := float64(transitionIndex+1) / float64(transitionPackets)
				if progress > 1.0 {
					progress = 1.0
				}
				interpolatedSSRC := uint32(int64(originalSSRC) + int64(float64(ssrcDelta)*progress))
				bundle.Timeline[i].Metadata = addSSRCOverride(bundle.Timeline[i].Metadata, interpolatedSSRC)
				transitionIndex++

				if transitionIndex >= transitionPackets {
					transitionStarted = false
					switchCountLeft--
					nextSwitchAt = switchAt + (switchCount-switchCountLeft)*(switchAt+1)
					originalSSRC = newSSRC // Next transition starts from this SSRC
					ssrcDelta = int64(newSSRC) - int64(originalSSRC)
				}
			}
		}

	case "burst":
		// Send duplicate packets with new SSRC briefly
		burstCount := 5
		if raw := cfg.Options["ssrc.switch.burst-count"]; raw != "" {
			if val, err := strconv.Atoi(raw); err == nil && val > 0 {
				burstCount = val
			}
		}

		var newTimeline []model.PacketEvent
		for i := range bundle.Timeline {
			newTimeline = append(newTimeline, bundle.Timeline[i])

			if i+1 == nextSwitchAt && switchCountLeft > 0 {
				// Insert burst packets after the switch trigger packet
				for b := 0; b < burstCount; b++ {
					burstEvent := bundle.Timeline[i]
					burstEvent.Metadata = addSSRCOverride(burstEvent.Metadata, newSSRC)
					newTimeline = append(newTimeline, burstEvent)
				}
				switchCountLeft--
				nextSwitchAt = switchAt + (switchCount-switchCountLeft)*(switchAt+1)
			}
		}
		bundle.Timeline = newTimeline

	default:
		// Default to immediate
		for i := range bundle.Timeline {
			if i+1 >= nextSwitchAt && switchCountLeft > 0 {
				bundle.Timeline[i].Metadata = addSSRCOverride(bundle.Timeline[i].Metadata, newSSRC)
				switchCountLeft--
				nextSwitchAt = switchAt + (switchCount-switchCountLeft)*(switchAt+1)
			}
		}
	}

	bundle.Metadata["mutation.switch-ssrc"] = fmt.Sprintf("mode=%s,at=%d,to=0x%x,count=%d", switchMode, switchAt, newSSRC, switchCount)
	bundle.Metadata["ssrc.history"] = fmt.Sprintf("initial=0x%x,current=0x%x", initialSSRC, newSSRC)

	return bundle, nil
}

func addSSRCOverride(metadata map[string]string, ssrc uint32) map[string]string {
	if metadata == nil {
		metadata = make(map[string]string)
	}
	metadata["override.ssrc"] = strconv.FormatUint(uint64(ssrc), 10)
	return metadata
}
