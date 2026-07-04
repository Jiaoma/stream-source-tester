package packetize

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
)

func init() {
	if err := mutation.Register("packetize", func() mutation.Mutator { return &Mutator{} }); err != nil {
		panic(err)
	}
}

// Mutator implements FU-A / STAP-A packetization for H.264 / H.265 NAL units.
type Mutator struct{}

func (m *Mutator) Apply(ctx context.Context, bundle *model.SessionBundle, cfg config.MutationConfig) (*model.SessionBundle, error) {
	_ = ctx
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}

	mode := "mixed"
	if raw := cfg.Options["packetize.mode"]; raw != "" {
		mode = raw
	}

	mtu := 1400
	if raw := cfg.Options["packetize.mtu"]; raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			mtu = parsed
		}
	}

	fuThreshold := 1400
	if raw := cfg.Options["packetize.fu.threshold"]; raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			fuThreshold = parsed
		}
	}

	stapMinAggregated := 2
	if raw := cfg.Options["packetize.stap.min-aggregated"]; raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			stapMinAggregated = parsed
		}
	}

	stapMaxAggregated := 8
	if raw := cfg.Options["packetize.stap.max-aggregated"]; raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			stapMaxAggregated = parsed
		}
	}

	if len(bundle.Streams) == 0 || len(bundle.Timeline) == 0 {
		bundle.Metadata["mutation.packetize"] = "skipped (empty timeline)"
		return bundle, nil
	}

	stream := bundle.Streams[0]
	isH264 := stream.Codec == model.CodecH264
	isH265 := stream.Codec == model.CodecH265

	if !isH264 && !isH265 {
		bundle.Metadata["mutation.packetize"] = "skipped (not h264/h265)"
		return bundle, nil
	}

	var newTimeline []model.PacketEvent
	pendingSTAP := make([][]byte, 0, stapMaxAggregated)

	flushSTAP := func() {
		if len(pendingSTAP) >= stapMinAggregated {
			stapPayload := buildSTAPAPacket(pendingSTAP, isH265)
			newTimeline = append(newTimeline, model.PacketEvent{
				StreamID:  stream.ID,
				Sequence:  0, // will be set later
				Timestamp: 0, // will be set later
				Marker:    true,
				Payload:   stapPayload,
				Metadata:  map[string]string{"packetize": "stap-a", "nalus": fmt.Sprintf("%d", len(pendingSTAP))},
			})
		} else {
			// Send individual NALUs
			for _, nal := range pendingSTAP {
				newTimeline = append(newTimeline, model.PacketEvent{
					StreamID:  stream.ID,
					Sequence:  0,
					Timestamp: 0,
					Marker:    true,
					Payload:   nal,
					Metadata:  map[string]string{"packetize": "single-nalu"},
				})
			}
		}
		pendingSTAP = pendingSTAP[:0]
	}

	for i, event := range bundle.Timeline {
		nalus := extractNALUs(event.Payload, isH265)
		if len(nalus) == 0 {
			// Non-video payload, pass through
			newTimeline = append(newTimeline, event)
			continue
		}

		switch mode {
		case "fu-a":
			for _, nal := range nalus {
				fragments := fragmentFU_A(nal, mtu, isH265)
				for j, frag := range fragments {
					e := model.PacketEvent{
						StreamID:  event.StreamID,
						Sequence:  event.Sequence + uint16(j),
						Timestamp: event.Timestamp,
						Marker:    j == len(fragments)-1,
						Payload:   frag,
						Metadata:  map[string]string{"packetize": "fu-a", "fragment": fmt.Sprintf("%d/%d", j+1, len(fragments))},
					}
					newTimeline = append(newTimeline, e)
				}
			}

		case "stap-a":
			// Aggregate small NALUs into STAP-A
			for _, nal := range nalus {
				if len(nal) <= fuThreshold && len(pendingSTAP) < stapMaxAggregated {
					pendingSTAP = append(pendingSTAP, nal)
				} else {
					if len(pendingSTAP) > 0 {
						flushSTAP()
					}
					if len(nal) > mtu {
						fragments := fragmentFU_A(nal, mtu, isH265)
						for j, frag := range fragments {
							e := model.PacketEvent{
								StreamID:  event.StreamID,
								Sequence:  event.Sequence + uint16(j),
								Timestamp: event.Timestamp,
								Marker:    j == len(fragments)-1,
								Payload:   frag,
								Metadata:  map[string]string{"packetize": "fu-a", "fragment": fmt.Sprintf("%d/%d", j+1, len(fragments))},
							}
							newTimeline = append(newTimeline, e)
						}
					} else {
						pendingSTAP = append(pendingSTAP, nal)
					}
				}
			}
			// Don't flush here, wait for marker

		case "single-nalu":
			for _, nal := range nalus {
				if len(nal) <= mtu {
					newTimeline = append(newTimeline, model.PacketEvent{
						StreamID:  event.StreamID,
						Sequence:  event.Sequence,
						Timestamp: event.Timestamp,
						Marker:    event.Marker,
						Payload:   nal,
						Metadata:  map[string]string{"packetize": "single-nalu"},
					})
				} else {
					// Too large, fragment with FU-A
					fragments := fragmentFU_A(nal, mtu, isH265)
					for j, frag := range fragments {
						e := model.PacketEvent{
							StreamID:  event.StreamID,
							Sequence:  event.Sequence + uint16(j),
							Timestamp: event.Timestamp,
							Marker:    j == len(fragments)-1,
							Payload:   frag,
							Metadata:  map[string]string{"packetize": "fu-a-fallback", "fragment": fmt.Sprintf("%d/%d", j+1, len(fragments))},
						}
						newTimeline = append(newTimeline, e)
					}
				}
			}

		case "mixed":
			fallthrough
		default:
			// FU-A for large NALUs, STAP-A for small ones
			for _, nal := range nalus {
				if len(nal) > fuThreshold {
					if len(pendingSTAP) > 0 {
						flushSTAP()
					}
					fragments := fragmentFU_A(nal, mtu, isH265)
					for j, frag := range fragments {
						e := model.PacketEvent{
							StreamID:  event.StreamID,
							Sequence:  event.Sequence + uint16(j),
							Timestamp: event.Timestamp,
							Marker:    j == len(fragments)-1,
							Payload:   frag,
							Metadata:  map[string]string{"packetize": "fu-a", "fragment": fmt.Sprintf("%d/%d", j+1, len(fragments))},
						}
						newTimeline = append(newTimeline, e)
					}
				} else {
					pendingSTAP = append(pendingSTAP, nal)
				}
			}

			// On marker, flush pending STAP
			if event.Marker && len(pendingSTAP) > 0 {
				flushSTAP()
			}
		}

		_ = i // suppress unused warning
	}

	// Final flush for any pending STAP-A packets
	if mode == "stap-a" && len(pendingSTAP) > 0 {
		flushSTAP()
	}

	// Renormalize sequence numbers
	for i := range newTimeline {
		newTimeline[i].Sequence = uint16(i + 1)
	}

	bundle.Timeline = newTimeline
	bundle.Metadata["mutation.packetize"] = mode
	bundle.Metadata["mutation.packetize.frags"] = strconv.Itoa(len(newTimeline))

	return bundle, nil
}

// extractNALUs extracts NAL units from a payload that may contain start codes.
// Handles both Annex B (0x000001 / 0x00000001) and MP4 format.
func extractNALUs(payload []byte, isH265 bool) [][]byte {
	if len(payload) == 0 {
		return nil
	}

	var nalus [][]byte
	offset := 0

	// Find start codes
	for offset < len(payload) {
		// Look for 0x000001 or 0x00000001
		var scLen int
		if offset+4 <= len(payload) && payload[offset] == 0x00 && payload[offset+1] == 0x00 && payload[offset+2] == 0x00 && payload[offset+3] == 0x01 {
			scLen = 4
		} else if offset+3 <= len(payload) && payload[offset] == 0x00 && payload[offset+1] == 0x00 && payload[offset+2] == 0x01 {
			scLen = 3
		} else {
			offset++
			continue
		}

		offset += scLen
		if offset >= len(payload) {
			break
		}

		// Find next start code or end
		nextOffset := offset
		for nextOffset < len(payload) {
			if nextOffset+4 <= len(payload) && payload[nextOffset] == 0x00 && payload[nextOffset+1] == 0x00 && payload[nextOffset+2] == 0x00 && payload[nextOffset+3] == 0x01 {
				break
			}
			if nextOffset+3 <= len(payload) && payload[nextOffset] == 0x00 && payload[nextOffset+1] == 0x00 && payload[nextOffset+2] == 0x01 {
				break
			}
			nextOffset++
		}

		nal := make([]byte, nextOffset-offset)
		copy(nal, payload[offset:nextOffset])
		if len(nal) > 0 {
			nalus = append(nalus, nal)
		}
		offset = nextOffset
	}

	// If no start codes found, treat the whole payload as a single NALU
	// but strip 2-byte NAL header if present (MP4 format)
	if len(nalus) == 0 && len(payload) > 2 {
		// Check for 2-byte NAL header (MP4 format)
		firstByte := payload[0]
		if (firstByte&0x1f) != 0x1f && firstByte != 0x00 {
			// Has NAL header, extract the unit
			if len(payload) > 2 {
				nalus = append(nalus, payload[2:])
			}
		} else {
			nalus = append(nalus, payload)
		}
	}

	if len(nalus) == 0 && len(payload) > 0 {
		nalus = append(nalus, payload)
	}

	return nalus
}

// fragmentFU_A fragments a NALU into FU-A packets.
func fragmentFU_A(nalu []byte, mtu int, isH265 bool) [][]byte {
	if len(nalu) == 0 {
		return nil
	}

	// Calculate maximum fragment size
	// FU-A header is 2 bytes (H.264) or 2 bytes (H.265), plus FU indicator
	// Actually for H.264 FU-A: 1 byte FU indicator + 1 byte FU header = 2 bytes overhead
	// For H.265: 2 bytes header ( FU indicator + FU header) = 2 bytes overhead

	if len(nalu) <= mtu-2 {
		// NALU fits in one packet, no fragmentation needed
		return [][]byte{nalu}
	}

	var fragments [][]byte

	// Parse NAL header
	var nalHeader []byte
	if isH265 {
		// H.265 NAL header is 2 bytes
		if len(nalu) < 2 {
			return [][]byte{nalu}
		}
		nalHeader = nalu[:2]
	} else {
		// H.264 NAL header is 1 byte
		if len(nalu) < 1 {
			return [][]byte{nalu}
		}
		nalHeader = nalu[:1]
	}

	// FU indicator = original NAL header with forbidden_bit = 0
	fuIndicator := make([]byte, len(nalHeader))
	copy(fuIndicator, nalHeader)
	// Clear the forbidden bit (bit 0 of first byte for H.264, bit 0 of first byte for H.265)
	fuIndicator[0] &^= 0x80

	// NAL unit type from original header
	nalType := getNalUnitType(nalHeader, isH265)

	// Fragment data starts after NAL header
	data := nalu[len(nalHeader):]
	fragIndex := 0
	totalFrags := (len(data) + mtu - 3) / (mtu - 2) // -2 for FU headers, -1 for data per packet minimum

	for len(data) > 0 {
		fragSize := mtu - 2 // FU indicator + FU header + payload
		if fragSize > len(data) {
			fragSize = len(data)
		}

		// FU header
		var fuHeader byte
		startBit := byte(0x80) // start bit
		endBit := byte(0x00)   // end bit

		if len(data) <= fragSize && len(data) == len(nalu)-len(nalHeader) {
			// This is the first and last fragment
			startBit = 0x80
			endBit = 0x40
		} else if fragIndex == 0 {
			startBit = 0x80
			endBit = 0x00
		} else if len(data)-fragSize <= 0 {
			startBit = 0x00
			endBit = 0x40
		} else {
			startBit = 0x00
			endBit = 0x00
		}

		if isH265 {
			// H.265 FU header structure differs - using type field
			fuHeader = startBit | endBit | nalType
		} else {
			// H.264 FU header: S|E|R|type
			fuHeader = startBit | endBit | nalType
		}

		frag := make([]byte, 2+fragSize)
		frag[0] = fuIndicator[0]
		if isH265 && len(fuIndicator) > 1 {
			frag[1] = fuHeader | (fuIndicator[1] & 0xC0) // preserve layer_id and tid
		} else {
			frag[1] = fuHeader
		}
		copy(frag[2:], data[:fragSize])

		fragments = append(fragments, frag)
		data = data[fragSize:]
		fragIndex++
		_ = totalFrags
	}

	return fragments
}

// getNalUnitType extracts the NAL unit type from a NAL header.
func getNalUnitType(header []byte, isH265 bool) byte {
	if len(header) == 0 {
		return 0
	}
	if isH265 {
		if len(header) < 2 {
			return 0
		}
		// H.265: NAL type is bits 0-5 of first byte (with layer_id and tid)
		return header[0] & 0x3E
	}
	// H.264: NAL type is bits 0-4 of first byte
	return header[0] & 0x1F
}

// buildSTAPAPacket builds an STAP-A aggregated packet from multiple NALUs.
func buildSTAPAPacket(nalus [][]byte, isH265 bool) []byte {
	if len(nalus) == 0 {
		return nil
	}

	// STAP-A header: 1 byte (should be the NAL header of first NALU with type changed to STAP-A)
	// Actually STAP-A uses the same NAL header format but the type is replaced with 24 for H.264
	// For H.265, STAP-B is used, so we use STAP-A (type 24)
	stapType := byte(24) // STAP-A NAL type

	var sizeHeaderLen int
	if isH265 {
		// H.265 sizes are 2 bytes (16-bit)
		sizeHeaderLen = 2
	} else {
		// H.264 sizes are 2 bytes (16-bit)
		sizeHeaderLen = 2
	}

	// Calculate total size
	totalSize := 1 // STAP header byte
	for _, nal := range nalus {
		totalSize += sizeHeaderLen + len(nal)
	}

	packet := make([]byte, totalSize)
	offset := 0

	// STAP header
	packet[offset] = stapType
	offset++

	// Each NALU preceded by 2-byte size
	for _, nal := range nalus {
		if isH265 && len(nal) >= 2 {
			// For H.265, preserve the layer_id and tid from original NAL header
			// but use the NAL type from the header
			binary.BigEndian.PutUint16(packet[offset:], uint16(len(nal)))
		} else if len(nal) < 2 {
			binary.BigEndian.PutUint16(packet[offset:], uint16(len(nal)))
		} else {
			// For H.264, preserve NAL header but clear type to 0 for size field
			binary.BigEndian.PutUint16(packet[offset:], uint16(len(nal)))
		}
		offset += sizeHeaderLen
		copy(packet[offset:], nal)
		offset += len(nal)
	}

	return packet
}
