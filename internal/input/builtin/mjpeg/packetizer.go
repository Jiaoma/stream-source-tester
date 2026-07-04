package mjpeg

import (
	"encoding/binary"
	"errors"
)

// MJPEG RTP packetization per RFC 2435.

// DefaultMTU for MJPEG RTP packets.
const DefaultMTU = 1400

var (
	ErrInvalidJPEG       = errors.New("invalid JPEG data")
	ErrQuantizationTable = errors.New("quantization table not found")
)

// RFC 2435 JPEG payload header (8 bytes total):
//   0           : MBZ (8 bits) - must be 0 for types 0-63
//   1           : Type (8 bits) - JPEG type (0=baseline DCT)
//   2-3         : Quantization table (16 bits) - 0 means tables in-band
//   4-7         : Fragment offset (32 bits) - byte offset of this fragment in frame
//
// For type 0 (baseline), quantization tables are typically in-band in the JPEG SOF0.

const (
	// JPEGTypeBaseline is baseline DCT JPEG (most common).
	JPEGTypeBaseline = 0
	// JPEGTypeSequentialDCT is sequential DCT.
	JPEGTypeSequentialDCT = 1
	// JPEGTypeProgressiveDCT is progressive DCT.
	JPEGTypeProgressiveDCT = 2
	// JPEGTypeLossless is lossless JPEG.
	JPEGTypeLossless = 3
)

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

// JPEGPacket represents a single MJPEG RTP packet.
type JPEGPacket struct {
	Header       RTPHeader
	TypeSpecific uint8 // Type-specific byte from RFC 2435
	JPEGType     uint8 // 0=baseline, etc.
	FragmentOff  uint32
	Payload      []byte // JPEG data for this fragment
}

// MakeJPEGPacket creates a single MJPEG RTP packet for a complete frame.
// The JPEG data should include quantization tables in-band.
func MakeJPEGPacket(jpegData []byte, seq uint16, ts uint32, ssrc uint32, pt uint8) []byte {
	// Check JPEG has required markers
	if len(jpegData) < 2 {
		return nil
	}

	jpegType := detectJPEGType(jpegData)
	if jpegType == 255 {
		jpegType = JPEGTypeBaseline
	}

	payloadLen := 12 + 8 + len(jpegData) // RTP header + JPEG payload header + JPEG data
	buf := make([]byte, payloadLen)
	b := 0

	// 12-byte RTP header
	buf[b] = 2 << 6
	b++
	buf[b] = (1 << 7) | (pt & 0x7F) // marker=1 for last packet of frame
	b++
	binary.BigEndian.PutUint16(buf[b:], seq)
	b += 2
	binary.BigEndian.PutUint32(buf[b:], ts)
	b += 4
	binary.BigEndian.PutUint32(buf[b:], ssrc)
	b += 4

	// 8-byte JPEG payload header
	buf[b] = 0 // MBZ
	b++
	buf[b] = jpegType & 0x3F
	b++
	// Quantization table present: 0 means in-band
	buf[b] = 0
	buf[b+1] = 0
	b += 2
	// Fragment offset (0 for single packet)
	buf[b] = 0
	buf[b+1] = 0
	buf[b+2] = 0
	buf[b+3] = 0
	b += 4

	// JPEG payload
	copy(buf[b:], jpegData)
	return buf
}

// MakeJPEGPackets fragments a JPEG frame into multiple RTP packets if needed.
// Returns one or more RTP packets for the JPEG frame.
func MakeJPEGPackets(jpegData []byte, mtu int, seq uint16, ts uint32, ssrc uint32, pt uint8) [][]byte {
	maxPayload := mtu - 12 - 8 // RTP header + JPEG payload header
	if maxPayload < 1 {
		maxPayload = DefaultMTU - 20
	}

	if len(jpegData) <= maxPayload {
		// Single packet
		return [][]byte{MakeJPEGPacket(jpegData, seq, ts, ssrc, pt)}
	}

	// Fragment into multiple packets
	var packets [][]byte
	fragOff := 0
	packetSeq := seq
	for fragOff < len(jpegData) {
		end := fragOff + maxPayload
		if end > len(jpegData) {
			end = len(jpegData)
		}
		isLast := end == len(jpegData)
		frag := jpegData[fragOff:end]

		payloadLen := 12 + 8 + len(frag) // RTP header + JPEG payload header + JPEG data
		buf := make([]byte, payloadLen)
		b := 0

		// RTP header
		buf[b] = 2 << 6
		b++
		markerBit := 0
		if isLast {
			markerBit = 1
		}
		buf[b] = (uint8(markerBit) << 7) | (pt & 0x7F)
		b++
		binary.BigEndian.PutUint16(buf[b:], packetSeq)
		b += 2
		binary.BigEndian.PutUint32(buf[b:], ts)
		b += 4
		binary.BigEndian.PutUint32(buf[b:], ssrc)
		b += 4

		// JPEG payload header (8 bytes)
		buf[b] = 0 // MBZ
		b++
		jpegType := detectJPEGType(jpegData)
		if jpegType == 255 {
			jpegType = JPEGTypeBaseline
		}
		buf[b] = jpegType & 0x3F
		b++
		// Quantization table present
		buf[b] = 0
		buf[b+1] = 0
		b += 2
		// Fragment offset
		binary.BigEndian.PutUint32(buf[b:], uint32(fragOff))
		b += 4

		// JPEG fragment data
		copy(buf[b:], frag)

		packets = append(packets, buf)
		fragOff = end
		packetSeq++
	}
	return packets
}

// detectJPEGType attempts to detect the JPEG type from the SOF0 marker.
// Returns 255 if unknown/uncertain.
func detectJPEGType(jpegData []byte) uint8 {
	// Look for SOF0 (0xFF 0xC0) or SOF2 (0xFF 0xC2) etc.
	for i := 0; i < len(jpegData)-1; i++ {
		if jpegData[i] == 0xFF {
			marker := jpegData[i+1]
			switch marker {
			case 0xC0:
				return JPEGTypeBaseline
			case 0xC1:
				return JPEGTypeSequentialDCT
			case 0xC2:
				return JPEGTypeProgressiveDCT
			case 0xC3:
				return JPEGTypeLossless
			case 0xC4:
				// DHT - skip this segment
				if i+3 < len(jpegData) {
					length := binary.BigEndian.Uint16(jpegData[i+2:])
					i += 2 + int(length)
				}
			case 0xD8:
				// SOI - skip
				i++
			case 0xD9:
				// EOI - skip
				i++
			case 0xDA:
				// SOS - start of scan, stop here
				return JPEGTypeBaseline
			case 0xDB:
				// DQT - skip
				if i+3 < len(jpegData) {
					length := binary.BigEndian.Uint16(jpegData[i+2:])
					i += 2 + int(length)
				}
			default:
				// Other markers have length field
				if i+3 < len(jpegData) && marker >= 0xE0 && marker <= 0xEF {
					length := binary.BigEndian.Uint16(jpegData[i+2:])
					i += 2 + int(length)
				} else if marker != 0x00 && marker != 0xFF {
					// End of markers list, probably SOS follows
					break
				}
			}
		}
	}
	return JPEGTypeBaseline
}

// ExtractQuantizationTables parses quantization tables from JPEG data.
// Returns table index 0 and 1 as separate byte slices, or nil if not found.
func ExtractQuantizationTables(jpegData []byte) (qt0, qt1 []byte, err error) {
	var found0, found1 bool
	for i := 0; i < len(jpegData)-1; {
		if jpegData[i] != 0xFF {
			i++
			continue
		}
		marker := jpegData[i+1]
		if marker == 0xD9 {
			i++
			continue
		}
		if marker == 0xDA {
			// Reached SOS - done
			break
		}
		if marker == 0xDB && i+4 < len(jpegData) {
			length := binary.BigEndian.Uint16(jpegData[i+2:])
			if int(length) < 3 || i+2+int(length) > len(jpegData) {
				break
			}
			pq := (jpegData[i+4] >> 4) & 0x0F // precision
			tq := jpegData[i+4] & 0x0F        // table id
			tableLen := int(pq)*64 + 2        // 64 8-bit values or 64 16-bit values + 2 bytes header
			if pq == 0 {
				tableLen = 65
			} else {
				tableLen = 130
			}
			if tq == 0 {
				qt0 = jpegData[i+5 : i+4+tableLen]
				found0 = true
			} else if tq == 1 {
				qt1 = jpegData[i+5 : i+4+tableLen]
				found1 = true
			}
			i += 2 + int(length)
			continue
		}
		if marker == 0xC4 && i+4 < len(jpegData) {
			// DHT - skip
			length := binary.BigEndian.Uint16(jpegData[i+2:])
			i += 2 + int(length)
			continue
		}
		if marker >= 0xE0 && marker <= 0xEF {
			// APP markers - skip
			if i+3 < len(jpegData) {
				length := binary.BigEndian.Uint16(jpegData[i+2:])
				i += 2 + int(length)
			} else {
				break
			}
			continue
		}
		i++
	}
	if !found0 && !found1 {
		return nil, nil, ErrQuantizationTable
	}
	return qt0, qt1, nil
}

// Packetizer handles MJPEG RTP packetization.
type Packetizer struct {
	MTU         int
	PayloadType uint8
	SSRC        uint32
	Sequence    uint16
	Timestamp   uint32
	ClockRate   uint32
	Quality     int // 1-100 quantization scale
}

// NewPacketizer creates a new MJPEG packetizer with defaults.
func NewPacketizer(pt uint8, ssrc uint32) *Packetizer {
	return &Packetizer{
		MTU:         DefaultMTU,
		PayloadType: pt,
		SSRC:        ssrc,
		Sequence:    1,
		Timestamp:   0,
		ClockRate:   90000,
		Quality:     80,
	}
}

// Packetize converts a JPEG frame into RTP packets.
// If jpegData fits in a single packet (MTU permitting), returns one packet.
// Otherwise fragments the frame across multiple RTP packets with marker=1 on last.
func (p *Packetizer) Packetize(jpegData []byte, frameMarker bool) [][]byte {
	// Use frameMarker to set RTP marker bit (indicates last packet of frame)
	pkts := MakeJPEGPackets(jpegData, p.MTU, p.Sequence, p.Timestamp, p.SSRC, p.PayloadType)
	// Set frame marker on the last packet
	if len(pkts) > 0 && frameMarker {
		lastPkt := pkts[len(pkts)-1]
		lastPkt[1] |= 0x80 // Set marker bit
		pkts[len(pkts)-1] = lastPkt
	}
	p.Sequence += uint16(len(pkts))
	p.Timestamp += p.ClockRate / 30 // Assuming 30fps
	return pkts
}
