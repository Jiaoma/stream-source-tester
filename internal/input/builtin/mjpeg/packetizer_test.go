package mjpeg

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestMakeJPEGPacketSingle(t *testing.T) {
	t.Parallel()

	// Minimal JPEG: SOI + minimal structure
	jpeg := []byte{
		0xFF, 0xD8, // SOI
		0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, // APP0
		0xFF, 0xDB, 0x00, 0x43, 0x00, // DQT
		0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01, 0x00, 0x01, 0x01, 0x01, 0x11, 0x00, // SOF0
		0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00, // SOS
		0xD2, 0xCF, 0x20, // minimal scan data
		0xFF, 0xD9, // EOI
	}

	pkt := MakeJPEGPacket(jpeg, 1, 0x12345678, 0xABCD1234, 26)
	if pkt == nil {
		t.Fatal("MakeJPEGPacket returned nil")
	}

	// Should be 12 (RTP) + 8 (JPEG header) + len(jpeg) bytes
	expectedLen := 12 + 8 + len(jpeg)
	if len(pkt) != expectedLen {
		t.Errorf("len(packet) = %d, want %d", len(pkt), expectedLen)
	}

	// Check RTP version
	if pkt[0]>>6 != 2 {
		t.Errorf("RTP version = %d, want 2", pkt[0]>>6)
	}

	// Check marker bit is set (byte 1, bit 7)
	if pkt[1]&0x80 == 0 {
		t.Errorf("RTP marker bit should be set for single packet")
	}

	// Check sequence number (bytes 2-3)
	seq := binary.BigEndian.Uint16(pkt[2:4])
	if seq != 1 {
		t.Errorf("RTP sequence = %d, want 1", seq)
	}

	// Check timestamp (bytes 4-7)
	ts := binary.BigEndian.Uint32(pkt[4:8])
	if ts != 0x12345678 {
		t.Errorf("RTP timestamp = 0x%08x, want 0x12345678", ts)
	}

	// Check SSRC (bytes 8-11)
	ssrc := binary.BigEndian.Uint32(pkt[8:12])
	if ssrc != 0xABCD1234 {
		t.Errorf("RTP SSRC = 0x%08x, want 0xABCD1234", ssrc)
	}

	// Check JPEG payload header (bytes 12-19)
	// MBZ = 0
	if pkt[12] != 0 {
		t.Errorf("JPEG MBZ = %d, want 0", pkt[12])
	}
	// Type = 0 (baseline)
	if pkt[13] != 0 {
		t.Errorf("JPEG type = %d, want 0", pkt[13])
	}
}

func TestMakeJPEGPacketsFragments(t *testing.T) {
	t.Parallel()

	// Create a large JPEG that exceeds MTU
	largeJPEG := make([]byte, 3000)
	largeJPEG[0] = 0xFF
	largeJPEG[1] = 0xD8 // SOI
	for i := 2; i < len(largeJPEG)-2; i++ {
		largeJPEG[i] = byte(i & 0xFF)
	}
	largeJPEG[len(largeJPEG)-2] = 0xFF
	largeJPEG[len(largeJPEG)-1] = 0xD9 // EOI

	packets := MakeJPEGPackets(largeJPEG, 500, 10, 0x50000000, 0x1111, 26)
	if len(packets) < 2 {
		t.Errorf("large JPEG should fragment into multiple packets, got %d", len(packets))
	}

	// Check fragment offsets are correct
	for i, pkt := range packets {
		offset := binary.BigEndian.Uint32(pkt[16:20])
		expectedOffset := uint32(i) * uint32(500-20) // MTU - RTP - JPEG headers
		if offset != expectedOffset {
			t.Errorf("packet[%d] offset = %d, want %d", i, offset, expectedOffset)
		}
	}

	// Check last packet has marker bit set
	lastPkt := packets[len(packets)-1]
	if lastPkt[1]&0x80 == 0 {
		t.Errorf("last packet should have RTP marker bit set")
	}
}

func TestMakeJPEGPacketsSingleFits(t *testing.T) {
	t.Parallel()

	smallJPEG := []byte{
		0xFF, 0xD8, 0xFF, 0xD9, // SOI + EOI
	}

	packets := MakeJPEGPackets(smallJPEG, 1400, 1, 0x1000, 0x2222, 26)
	if len(packets) != 1 {
		t.Errorf("small JPEG should be single packet, got %d", len(packets))
	}
}

func TestDetectJPEGType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		jpeg   []byte
		expect uint8
	}{
		{
			name:   "SOF0 baseline",
			jpeg:   []byte{0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01, 0x00, 0x01, 0x01, 0x01, 0x11, 0xFF, 0xDA},
			expect: JPEGTypeBaseline,
		},
		{
			name:   "SOF2 progressive",
			jpeg:   []byte{0xFF, 0xD8, 0xFF, 0xC2, 0x00, 0x0B, 0x08, 0x00, 0x01, 0x00, 0x01, 0x01, 0x01, 0x11, 0xFF, 0xDA},
			expect: JPEGTypeProgressiveDCT,
		},
		{
			name:   "SOI only",
			jpeg:   []byte{0xFF, 0xD8},
			expect: JPEGTypeBaseline,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectJPEGType(tc.jpeg)
			if got != tc.expect {
				t.Errorf("detectJPEGType = %d, want %d", got, tc.expect)
			}
		})
	}
}

func TestPacketizer(t *testing.T) {
	t.Parallel()

	p := NewPacketizer(26, 0x12345678)
	p.Sequence = 1
	p.Timestamp = 0

	jpeg := []byte{
		0xFF, 0xD8, 0xFF, 0xD9, // SOI + EOI
	}

	packets := p.Packetize(jpeg, true)
	if len(packets) != 1 {
		t.Errorf("Packetize returned %d packets, want 1", len(packets))
	}

	// Sequence and timestamp should advance
	if p.Sequence != 2 {
		t.Errorf("p.Sequence = %d, want 2", p.Sequence)
	}
	if p.Timestamp == 0 {
		t.Errorf("p.Timestamp should have advanced from 0")
	}
}

func TestPacketizerLargeFrame(t *testing.T) {
	t.Parallel()

	p := NewPacketizer(26, 0x12345678)
	p.MTU = 200
	p.Sequence = 1
	p.Timestamp = 0

	largeJPEG := make([]byte, 500)
	largeJPEG[0] = 0xFF
	largeJPEG[1] = 0xD8
	largeJPEG[len(largeJPEG)-2] = 0xFF
	largeJPEG[len(largeJPEG)-1] = 0xD9

	packets := p.Packetize(largeJPEG, true)
	if len(packets) < 2 {
		t.Errorf("large JPEG should produce multiple packets, got %d", len(packets))
	}

	// Last packet should have marker bit
	lastPkt := packets[len(packets)-1]
	if lastPkt[1]&0x80 == 0 {
		t.Errorf("last packet should have marker bit")
	}
}

func TestMarshalJPEGPacketPayloadHeader(t *testing.T) {
	t.Parallel()

	// Test that JPEG payload header bytes are correctly structured
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	pkt := MakeJPEGPacket(jpeg, 5, 0x1000, 0x2222, 26)

	// MBZ at offset 12
	if pkt[12] != 0 {
		t.Errorf("MBZ = 0x%02x, want 0x00", pkt[12])
	}

	// Type at offset 13
	// Should be baseline (0) since we detect SOF0
	if pkt[13] != 0 {
		t.Errorf("JPEG type = %d, want 0 (baseline)", pkt[13])
	}

	// Quantization table field at offset 14-15 (little or big endian?)
	// Per RFC 2435, this is a 16-bit field: bit 15 = qt present, bits 14-0 = table length
	// A value of 0 means qt are in-band
	qtField := binary.BigEndian.Uint16(pkt[14:16])
	if qtField != 0 {
		t.Errorf("qt field = 0x%04x, want 0 (in-band)", qtField)
	}

	// Fragment offset at offset 16-19
	fragOff := binary.BigEndian.Uint32(pkt[16:20])
	if fragOff != 0 {
		t.Errorf("fragment offset = %d, want 0", fragOff)
	}

	// JPEG data should start at offset 20
	if !bytes.Equal(pkt[20:], jpeg) {
		t.Errorf("JPEG payload mismatch")
	}
}
