package pcap

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"stream-source-tester/internal/model"
)

var (
	ErrInvalidPCAP     = errors.New("invalid pcap file")
	ErrInvalidPacket   = errors.New("invalid packet in pcap")
	ErrNoRTPPackets    = errors.New("no RTP packets found in pcap")
	ErrUnsupportedLink = errors.New("unsupported link type")
)

// RTPPacket represents a parsed RTP packet from a pcap file.
type RTPPacket struct {
	Timestamp   uint32    // RTP timestamp (from RTP header)
	Sequence    uint16    // RTP sequence number (from RTP header)
	Marker      bool      // RTP marker bit
	PayloadType uint8     // RTP payload type
	SSRC        uint32    // RTP SSRC
	Payload     []byte    // RTP payload (e.g. H.264 NAL units)
	CapturedAt  time.Time // Original capture timestamp from pcap
}

// ReadRTPPackets reads a pcap file and extracts all RTP packets.
// It handles Ethernet + IPv4 + UDP encapsulation.
// Returns packets in chronological order.
func ReadRTPPackets(pcapPath string) ([]RTPPacket, error) {
	f, err := os.Open(pcapPath)
	if err != nil {
		return nil, fmt.Errorf("open pcap file: %w", err)
	}
	defer f.Close()

	// Read global header (24 bytes)
	globalHeader := make([]byte, 24)
	if _, err := io.ReadFull(f, globalHeader); err != nil {
		return nil, fmt.Errorf("read pcap global header: %w", err)
	}

	parsed, err := parseHeader(globalHeader)
	if err != nil {
		return nil, err
	}

	var byteOrder binary.ByteOrder = binary.LittleEndian
	if parsed.Endian == "big" {
		byteOrder = binary.BigEndian
	}

	var packets []RTPPacket

	for {
		// Read packet record header (16 bytes)
		recHeader := make([]byte, 16)
		if _, err := io.ReadFull(f, recHeader); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			break
		}

		tsSec := byteOrder.Uint32(recHeader[0:4])
		tsUsec := byteOrder.Uint32(recHeader[4:8])
		capLen := byteOrder.Uint32(recHeader[8:12])
		_ = byteOrder.Uint32(recHeader[12:16]) // origLen, not needed

		capturedAt := time.Unix(int64(tsSec), int64(tsUsec)*1000)

		// Read packet data
		packetData := make([]byte, capLen)
		if _, err := io.ReadFull(f, packetData); err != nil {
			break
		}

		// Parse link layer (Ethernet, SLL, etc.)
		offset, err := skipLinkLayer(packetData, parsed.LinkType)
		if err != nil {
			continue
		}

		// Parse IP header
		offset, ipProto, err := skipIPHeader(packetData[offset:])
		if err != nil {
			continue
		}

		// Only handle UDP (IP protocol 17)
		if ipProto != 17 {
			continue
		}

		// Skip UDP header (8 bytes)
		offset += 8
		if offset >= len(packetData) {
			continue
		}

		// Parse RTP header
		rtpData := packetData[offset:]
		rtp, err := parseRTPHeader(rtpData)
		if err != nil {
			continue
		}
		rtp.CapturedAt = capturedAt
		packets = append(packets, rtp)
	}

	if len(packets) == 0 {
		return nil, ErrNoRTPPackets
	}

	return packets, nil
}

// skipLinkLayer skips the link layer header based on link type.
func skipLinkLayer(data []byte, linkType uint32) (int, error) {
	switch linkType {
	case 1: // Ethernet
		if len(data) < 14 {
			return 0, ErrInvalidPacket
		}
		etherTypeOff := 12
		if binary.BigEndian.Uint16(data[12:14]) == 0x8100 {
			etherTypeOff = 16 // VLAN tag
		}
		etherType := binary.BigEndian.Uint16(data[etherTypeOff : etherTypeOff+2])
		if etherType != 0x0800 {
			return 0, fmt.Errorf("not IPv4: 0x%04x", etherType)
		}
		return etherTypeOff + 2, nil

	case 101: // Linux cooked-mode (SLL)
		if len(data) < 16 {
			return 0, ErrInvalidPacket
		}
		if binary.BigEndian.Uint16(data[14:16]) != 0x0800 {
			return 0, fmt.Errorf("not IPv4")
		}
		return 16, nil

	case 104: // Linux cooked-mode v2 (SLL2)
		if len(data) < 20 {
			return 0, ErrInvalidPacket
		}
		if binary.BigEndian.Uint16(data[8:10]) != 0x0800 {
			return 0, fmt.Errorf("not IPv4")
		}
		return 20, nil

	case 228: // Raw IP (no link layer)
		return 0, nil

	default:
		return 0, fmt.Errorf("%w: link type %d", ErrUnsupportedLink, linkType)
	}
}

// skipIPHeader skips the IPv4 header and returns the offset into the remaining
// data and the IP protocol number.
func skipIPHeader(data []byte) (int, uint8, error) {
	if len(data) < 20 {
		return 0, 0, ErrInvalidPacket
	}
	version := data[0] >> 4
	if version != 4 {
		return 0, 0, fmt.Errorf("not IPv4: version=%d", version)
	}
	ihl := int(data[0]&0x0F) * 4
	if ihl < 20 || len(data) < ihl {
		return 0, 0, ErrInvalidPacket
	}
	return ihl, data[9], nil
}

// parseRTPHeader parses an RTP packet header.
// RTP header structure (no extension):
//   0: V(2) P(1) X(1) CC(4)
//   1: M(1) PT(7)
//   2-3: Sequence Number (16 bits, big-endian)
//   4-7: Timestamp (32 bits, big-endian)
//   8-11: SSRC (32 bits, big-endian)
//   12+: CSRC + Extension + Payload
func parseRTPHeader(data []byte) (RTPPacket, error) {
	if len(data) < 12 {
		return RTPPacket{}, fmt.Errorf("RTP header too short: %d bytes", len(data))
	}

	// Version must be 2
	if (data[0]>>6)&0x03 != 2 {
		return RTPPacket{}, fmt.Errorf("not RTP v2")
	}

	padding := (data[0] >> 5) & 0x01
	hasExt := (data[0] >> 4) & 0x01
	cc := int(data[0] & 0x0F)

	marker := (data[1] & 0x80) != 0
	payloadType := data[1] & 0x7F

	sequence := binary.BigEndian.Uint16(data[2:4])
	timestamp := binary.BigEndian.Uint32(data[4:8])
	ssrc := binary.BigEndian.Uint32(data[8:12])

	// Check we have enough data for the base header + CSRC list
	if len(data) < 12+cc*4 {
		return RTPPacket{}, fmt.Errorf("RTP header too short for CC=%d: have %d bytes need %d",
			cc, len(data), 12+cc*4)
	}

	headerLen := 12 + cc*4
	if hasExt != 0 {
		if len(data) < headerLen+4 {
			return RTPPacket{}, fmt.Errorf("RTP extension too short")
		}
		extLen := int(binary.BigEndian.Uint16(data[headerLen+2 : headerLen+4]))
		headerLen += 4 + extLen*4
	}

	payload := data[headerLen:]
	// Remove padding if present
	if padding != 0 && len(payload) > 0 {
		padLen := int(payload[len(payload)-1])
		if padLen <= len(payload) {
			payload = payload[:len(payload)-padLen]
		}
	}

	return RTPPacket{
		Timestamp:   timestamp,
		Sequence:    sequence,
		Marker:      marker,
		PayloadType: payloadType,
		SSRC:        ssrc,
		Payload:     payload,
	}, nil
}

// BuildSessionBundleFromPackets builds a complete SessionBundle from
// extracted RTP packets, with a properly constructed timeline.
func BuildSessionBundleFromPackets(packets []RTPPacket, name string) (*model.SessionBundle, error) {
	if len(packets) == 0 {
		return nil, ErrNoRTPPackets
	}

	pt := packets[0].PayloadType
	codec := codecFromPayloadType(pt)
	ssrc := packets[0].SSRC

	stream := model.Stream{
		ID:          "stream-0",
		Kind:        model.MediaVideo,
		Codec:       codec,
		ClockRate:   90000,
		PayloadType: pt,
		SSRC:        ssrc,
		Parameters:  map[string]string{},
	}

	// Compute relative EmittedAt from the first packet's capture time.
	firstCapture := packets[0].CapturedAt
	firstTimestamp := packets[0].Timestamp

	events := make([]model.PacketEvent, 0, len(packets))
	for _, pkt := range packets {
		elapsed := pkt.CapturedAt.Sub(firstCapture)
		events = append(events, model.PacketEvent{
			StreamID:   stream.ID,
			Sequence:   pkt.Sequence,
			Timestamp:  pkt.Timestamp,
			Marker:     pkt.Marker,
			Payload:    pkt.Payload,
			EmittedAt:  elapsed,
		})
	}

	bundle := model.NewMinimalSessionBundle(name, codec, "pcap", "", nil)
	bundle.Streams = []model.Stream{stream}
	bundle.Timeline = events
	bundle.Metadata["source.format"] = "capture/pcap"
	bundle.Metadata["bundle.mode"] = "reconstructed-from-rtp"
	bundle.Metadata["rtp.packet_count"] = fmt.Sprintf("%d", len(packets))
	bundle.Metadata["rtp.ssrc"] = fmt.Sprintf("0x%08x", ssrc)
	bundle.Metadata["rtp.payload_type"] = fmt.Sprintf("%d", pt)
	bundle.Metadata["rtp.first_timestamp"] = fmt.Sprintf("%d", firstTimestamp)

	return bundle, nil
}

func codecFromPayloadType(pt uint8) model.Codec {
	switch pt {
	case 0:
		return model.CodecMJPEG
	case 8:
		return model.CodecMJPEG // ALAW audio, not video but harmless
	default:
		return model.CodecH264
	}
}
