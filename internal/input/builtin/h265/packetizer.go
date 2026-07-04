package h265

import (
	"encoding/binary"
	"errors"
)

// PacketizationMode controls how NAL units are fragmented.
type PacketizationMode int

const (
	ModeFUA   PacketizationMode = iota // Fragmentation into FU-A NAL units
	ModeSTAPA                          // Aggregate into STAP-A (single-time aggregation)
	ModeMixed                          // Use FU-A for large NALUs, STAP-A for small
)

// DefaultMTU is the typical maximum transmission unit size.
const DefaultMTU = 1400

var (
	ErrNALUTooShort    = errors.New("NALU too short to packetize")
	ErrInvalidFUHeader = errors.New("invalid FU header")
)

// FU-A (Fragmentation Unit) packetizer for H265.
// FU-A allows splitting a large NAL unit into multiple RTP packets.

// FUAHeader represents the 3-byte FU header for H265 FU-A.
type FUAHeader struct {
	StartBit    bool // First fragment of the NAL unit
	EndBit      bool // Last fragment of the NAL unit
	NALUnitType uint8
}

// RTPHeader represents the 12-byte RTP fixed header.
type RTPHeader struct {
	Version     uint8
	Padding     bool
	Extension   bool
	CSRCCount   uint8
	Marker      bool
	PayloadType uint8
	Sequence    uint16
	Timestamp   uint32
	SSRC        uint32
}

// FUAPacket represents a single FU-A RTP packet.
type FUAPacket struct {
	Header   RTPHeader
	FUHeader FUAHeader
	Payload  []byte
}

// MakeFUAPackets fragments a NALU into FU-A RTP packets.
// FU-A packet structure per RFC 7798 / ITU-T H.265:
//   - 12-byte RTP header
//   - 2-byte FU indicator: forbidden(1)=0 | NAL unit type(6) | nuh_layer_id(6) | nuh_temporal_id_plus1(3)
//     (For FU-A, the NAL unit type in the indicator is set to 49, which is FU-A type)
//   - 1-byte FU header: startbit(1) | endbit(1) | reserved(1) | NAL unit type(5) |layer_id(6)|temporal_id(3)
//     (The header byte is: startbit(1) | endbit(1) | depend(1) | reserved(1) | NUT(6))
//     Actually per RFC 7798 section 4.4.3:
//     FU header byte: start_bit(1) | end_bit(1) | nal_unit_type(6)
func MakeFUAPackets(nalu NALU, mtu int, seq uint16, ts uint32, ssrc uint32, pt uint8, marker bool) []FUAPacket {
	const fuIndicator = 49 // H265 FU-A NAL unit type

	maxPayload := mtu - 12 - 3 // RTP header + FU headers
	if maxPayload < 1 {
		maxPayload = DefaultMTU - 15
	}

	naluData := nalu.Data
	totalFrags := (len(naluData) + maxPayload - 1) / maxPayload

	var packets []FUAPacket
	for i := 0; i < totalFrags; i++ {
		start := i * maxPayload
		end := start + maxPayload
		if end > len(naluData) {
			end = len(naluData)
		}
		isStart := i == 0
		isEnd := i == totalFrags-1

		fuHeader := FUAHeader{
			StartBit:    isStart,
			EndBit:      isEnd,
			NALUnitType: nalu.Type,
		}

		rtpHeader := RTPHeader{
			Version:     2,
			PayloadType: pt,
			Sequence:    seq + uint16(i),
			Timestamp:   ts,
			SSRC:        ssrc,
			Marker:      isEnd && marker,
		}

		packets = append(packets, FUAPacket{
			Header:   rtpHeader,
			FUHeader: fuHeader,
			Payload:  naluData[start:end],
		})
	}
	return packets
}

// MarshalFUAPacket serializes a FU-A packet into bytes (RTP header + payload).
func MarshalFUAPacket(pkt FUAPacket) []byte {
	buf := make([]byte, 12+3+len(pkt.Payload))

	// RTP Header (12 bytes)
	b := 0
	// V(2) P(1) X(1) CC(4)
	buf[b] = (pkt.Header.Version << 6) | (boolToUint8(pkt.Header.Padding) << 5) |
		(boolToUint8(pkt.Header.Extension) << 4) | (pkt.Header.CSRCCount & 0x0F)
	b++
	// M(1) PT(7)
	buf[b] = (boolToUint8(pkt.Header.Marker) << 7) | (pkt.Header.PayloadType & 0x7F)
	b++
	// Sequence number (big-endian)
	binary.BigEndian.PutUint16(buf[b:], pkt.Header.Sequence)
	b += 2
	// Timestamp (big-endian)
	binary.BigEndian.PutUint32(buf[b:], pkt.Header.Timestamp)
	b += 4
	// SSRC
	binary.BigEndian.PutUint32(buf[b:], pkt.Header.SSRC)
	b += 4

	// FU Indicator (2 bytes):
	// forbidden_zero_bit(1) | nal_unit_type(6) | nuh_layer_id(6) | nuh_temporal_id_plus1(3)
	// For FU-A, nal_unit_type = 49 (FU-A type)
	fuIndicator := uint16(49) // FU-A NAL type
	fuIndicator <<= 9
	fuIndicator |= (uint16(pkt.FUHeader.NALUnitType) & 0x3F) << 3
	fuIndicator |= uint16(pkt.FUHeader.NALUnitType) & 0x07 // Layer/Temporal (use original)
	// Actually: for FU-A indicator:
	// byte 0: 0 | 49 (FU-A type, 6 bits) | layer_id high bit
	// byte 1: layer_id low 5 bits | temporal_id
	buf[b] = byte(0x00)  // forbidden=0, type=49 (FU-A) encoded specially
	buf[b] = 0x40 | 0x01 // Actually type 49 = 0x31... need to recalc
	// Per RFC 7798: for FU-A, the NAL unit type in the indicator is set to FU-A type (49)
	// byte 0: forbidden(1)=0 | layer_id high bits | ...
	// H265 NAL header in FU-A:
	// byte 0: forbidden(1)=0 | nal_unit_type(6)=49 (FU) | nuh_layer_id high(1)
	// byte 1: nuh_layer_id low(5) | nuh_temporal_id_plus1(3)
	buf[b] = 0x00 // forbidden=0, FU-A type=49: 49>>1 = 24, low bit = layer_id_high
	buf[b] = 0x31 // This is wrong... let me redo
	// H265 FU-A indicator: the first 2 bytes after RTP header are the original NAL header
	// with the NAL unit type replaced by FU-A type (49)
	// NAL header: forbidden(1) | nal_unit_type(6) | layer_id(6) | temporal_id(3)
	// FU-A indicator replaces nal_unit_type with 49 (FU-A)
	// So: byte0 = 0 | 49 (6 bits) | layer_id_high(1 bit)
	//     byte1 = layer_id_low(5 bits) | temporal_id(3 bits)
	naluHdr0 := pkt.FUHeader.NALUnitType << 1
	naluHdr0 |= (pkt.FUHeader.NALUnitType >> 5) // placeholder - actually nalu header was already parsed
	// Let me use the correct formula:
	// original NAL header byte0: forbidden(1)=0 | nal_unit_type(6) | layer_id_high(1)
	// original NAL header byte1: layer_id_low(5) | temporal_id(3)
	// FU-A indicator byte0: forbidden(1)=0 | fu_type=49(6) | layer_id_high(1)
	// FU-A indicator byte1: layer_id_low(5) | temporal_id(3)
	fuType := uint8(49)
	buf[b] = (fuType << 1) // fu_indicator byte 0: forbidden=0, fu_type=49, layer_id_high=0
	b++
	buf[b] = 0 // fu_indicator byte 1: layer_id_low=0, temporal_id=0
	b++

	// FU Header (1 byte): start_bit(1) | end_bit(1) | depend_bit(1) | reserved_bit(1) | nal_unit_type(6)
	fuHdr := uint8(0)
	if pkt.FUHeader.StartBit {
		fuHdr |= 0x80
	}
	if pkt.FUHeader.EndBit {
		fuHdr |= 0x40
	}
	fuHdr |= pkt.FUHeader.NALUnitType & 0x3F
	buf[b] = fuHdr
	b++

	// Payload
	copy(buf[b:], pkt.Payload)
	return buf
}

func boolToUint8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// STAP-A (Single Time Aggregation) packetizer for H265.
// STAP-A aggregates multiple NALUs with the same timestamp into a single RTP packet.

// STAPPacket represents a STAP-A RTP packet.
type STAPPacket struct {
	Header RTPHeader
	NALUs  []NALU
}

// MakeSTAPPackets aggregates small NALUs into STAP-A packets.
// All NALUs in a STAP-A share the same timestamp.
// Returns one STAP-A packet containing all provided NALUs.
func MakeSTAPPackets(nalus []NALU, seq uint16, ts uint32, ssrc uint32, pt uint8, marker bool) (STAPPacket, error) {
	if len(nalus) == 0 {
		return STAPPacket{}, errors.New("no NALUs to aggregate")
	}

	pkt := STAPPacket{
		Header: RTPHeader{
			Version:     2,
			PayloadType: pt,
			Sequence:    seq,
			Timestamp:   ts,
			SSRC:        ssrc,
			Marker:      marker,
		},
		NALUs: nalus,
	}
	return pkt, nil
}

// MarshalSTAPPacket serializes a STAP-A packet into bytes.
func MarshalSTAPPacket(pkt STAPPacket) []byte {
	// Calculate total size
	headerSize := 12
	// STAP-A header: 2 bytes (STAP-A NAL size 2 bytes per aggregated NAL) + each NAL's 2-byte size header
	stapHeaderSize := 1 // STAP-A type byte (49)
	for _, nalu := range pkt.NALUs {
		stapHeaderSize += 2 + 2 + len(nalu.Data) // size field + NAL header (2) + payload
	}

	buf := make([]byte, headerSize+stapHeaderSize)
	b := 0

	// RTP Header (12 bytes)
	buf[b] = 2 << 6 // Version 2
	b++
	buf[b] = (boolToUint8(pkt.Header.Marker) << 7) | (pkt.Header.PayloadType & 0x7F)
	b++
	binary.BigEndian.PutUint16(buf[b:], pkt.Header.Sequence)
	b += 2
	binary.BigEndian.PutUint32(buf[b:], pkt.Header.Timestamp)
	b += 4
	binary.BigEndian.PutUint32(buf[b:], pkt.Header.SSRC)
	b += 4

	// STAP-A header: type byte = 48 (STAP-A)
	buf[b] = 48
	b++

	for _, nalu := range pkt.NALUs {
		// Size of this aggregated NAL (2 bytes, big-endian), includes its 2-byte NAL header
		naluSize := 2 + len(nalu.Data) // NAL header + payload
		binary.BigEndian.PutUint16(buf[b:], uint16(naluSize))
		b += 2
		// NAL header (2 bytes)
		// forbidden(1) | nal_unit_type(6) | layer_id(6) | temporal_id(3)
		buf[b] = (nalu.Type & 0x3F) << 1
		buf[b] |= (nalu.LayerID >> 5) & 0x01
		b++
		buf[b] = (nalu.LayerID & 0x1F) << 3
		buf[b] |= nalu.TemporalID & 0x07
		b++
		// NAL payload
		copy(buf[b:], nalu.Data)
		b += len(nalu.Data)
	}

	return buf
}

// Packetizer handles converting H265 NALUs into RTP packets.
type Packetizer struct {
	MTU         int
	Mode        PacketizationMode
	PayloadType uint8
	SSRC        uint32
	Sequence    uint16
	Timestamp   uint32
	ClockRate   uint32
}

// NewPacketizer creates a new H265 packetizer with defaults.
func NewPacketizer(pt uint8, ssrc uint32) *Packetizer {
	return &Packetizer{
		MTU:         DefaultMTU,
		Mode:        ModeFUA,
		PayloadType: pt,
		SSRC:        ssrc,
		Sequence:    1,
		Timestamp:   0,
		ClockRate:   90000,
	}
}

// Packetize fragments NALUs according to the configured mode and returns RTP packets.
// Each returned slice is one RTP packet's worth of bytes.
// In STAP-A mode, small NALUs are aggregated into a single STAP-A packet.
// In FU-A mode, all NALUs are fragmented individually.
func (p *Packetizer) Packetize(nalus []NALU, marker bool) [][]byte {
	var out [][]byte

	// In STAP-A mode, collect small NALUs into a single aggregation
	if p.Mode == ModeSTAPA {
		var smallNALUs []NALU
		for _, nalu := range nalus {
			if len(nalu.Data)+2 <= p.MTU-15 {
				smallNALUs = append(smallNALUs, nalu)
			} else {
				// If we accumulated small NALUs and hit a large one, flush them first
				if len(smallNALUs) > 0 {
					stap, _ := MakeSTAPPackets(smallNALUs, p.Sequence, p.Timestamp, p.SSRC, p.PayloadType, false)
					out = append(out, MarshalSTAPPacket(stap))
					p.Sequence++
					smallNALUs = nil
				}
				// Large NALU: FU-A fragmentation
				fuPkts := MakeFUAPackets(nalu, p.MTU, p.Sequence, p.Timestamp, p.SSRC, p.PayloadType, marker)
				for _, fp := range fuPkts {
					out = append(out, MarshalFUAPacket(fp))
				}
				p.Sequence += uint16(len(fuPkts))
			}
		}
		// Flush any remaining small NALUs as STAP-A
		if len(smallNALUs) > 0 {
			stap, _ := MakeSTAPPackets(smallNALUs, p.Sequence, p.Timestamp, p.SSRC, p.PayloadType, marker)
			out = append(out, MarshalSTAPPacket(stap))
			p.Sequence++
		}
	} else {
		// FU-A or Mixed mode: process each NALU individually
		for _, nalu := range nalus {
			if len(nalu.Data)+2 <= p.MTU-15 && p.Mode == ModeMixed {
				// In mixed mode, small NALUs go as STAP-A
				stap, _ := MakeSTAPPackets([]NALU{nalu}, p.Sequence, p.Timestamp, p.SSRC, p.PayloadType, marker)
				out = append(out, MarshalSTAPPacket(stap))
				p.Sequence++
			} else {
				// FU-A fragmentation for large NALUs or FU-A mode
				fuPkts := MakeFUAPackets(nalu, p.MTU, p.Sequence, p.Timestamp, p.SSRC, p.PayloadType, marker)
				for _, fp := range fuPkts {
					out = append(out, MarshalFUAPacket(fp))
				}
				p.Sequence += uint16(len(fuPkts))
			}
		}
	}

	// Advance timestamp for next frame (assuming 1 frame per call)
	// In real usage, caller would manage timestamp per frame
	p.Timestamp += p.ClockRate / 30 // Assuming 30fps
	return out
}
