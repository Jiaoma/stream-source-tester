package h265

import (
	"testing"
)

func TestMakeFUAPackets(t *testing.T) {
	t.Parallel()

	// Single small NALU that doesn't need fragmentation
	smallNalu := NALU{
		Type:       33, // SPS
		LayerID:    0,
		TemporalID: 0,
		Data:       []byte{0x01, 0x02, 0x03, 0x04, 0x05},
		Raw:        []byte{0, 0, 0, 1, 0x42, 0x01, 0x01, 0x02, 0x03, 0x04, 0x05},
	}

	pkts := MakeFUAPackets(smallNalu, 1400, 100, 0x12345678, 0xABCD, 98, true)
	if len(pkts) != 1 {
		t.Errorf("len(FU-A packets for small NALU) = %d, want 1", len(pkts))
	}
	if pkts[0].Header.Sequence != 100 {
		t.Errorf("pkts[0].Header.Sequence = %d, want 100", pkts[0].Header.Sequence)
	}
	if !pkts[0].FUHeader.StartBit || !pkts[0].FUHeader.EndBit {
		t.Errorf("small NALU should have start=end=true, got start=%v end=%v",
			pkts[0].FUHeader.StartBit, pkts[0].FUHeader.EndBit)
	}
	if pkts[0].Header.Marker != true {
		t.Errorf("pkts[0].Header.Marker = %v, want true", pkts[0].Header.Marker)
	}
}

func TestMakeFUAPacketsLarge(t *testing.T) {
	t.Parallel()

	// Large NALU that requires fragmentation (MTU 1400, so FU-A payload ~1385 bytes after headers)
	largeData := make([]byte, 3000)
	for i := range largeData {
		largeData[i] = byte(i & 0xFF)
	}

	largeNalu := NALU{
		Type:       1, // TRAIL_N
		LayerID:    0,
		TemporalID: 0,
		Data:       largeData,
	}

	pkts := MakeFUAPackets(largeNalu, 1400, 200, 0x50000000, 0x1234, 99, true)
	if len(pkts) < 2 {
		t.Errorf("large NALU should produce multiple FU-A packets, got %d", len(pkts))
	}

	// Check first packet
	if !pkts[0].FUHeader.StartBit {
		t.Errorf("first FU-A packet should have start bit set")
	}
	if pkts[0].FUHeader.EndBit {
		t.Errorf("first FU-A packet should have end bit clear")
	}

	// Check last packet
	last := pkts[len(pkts)-1]
	if !last.FUHeader.EndBit {
		t.Errorf("last FU-A packet should have end bit set")
	}
	if !last.Header.Marker {
		t.Errorf("last FU-A packet should have marker set")
	}

	// Check sequence numbers are contiguous
	for i := range pkts {
		if pkts[i].Header.Sequence != uint16(200+i) {
			t.Errorf("packet[%d].Sequence = %d, want %d", i, pkts[i].Header.Sequence, 200+i)
		}
	}
}

func TestMarshalFUAPacket(t *testing.T) {
	t.Parallel()

	pkt := FUAPacket{
		Header: RTPHeader{
			Version:     2,
			PayloadType: 98,
			Sequence:    5,
			Timestamp:   0x12345678,
			SSRC:        0xABCD1234,
		},
		FUHeader: FUAHeader{
			StartBit:    true,
			EndBit:      false,
			NALUnitType: 33, // SPS
		},
		Payload: []byte{0x01, 0x02, 0x03},
	}

	data := MarshalFUAPacket(pkt)

	// Should be 12 (RTP) + 3 (FU headers) + 3 (payload) = 18 bytes
	if len(data) != 18 {
		t.Errorf("len(MarshalFUAPacket) = %d, want 18", len(data))
	}

	// RTP version
	if data[0]>>6 != 2 {
		t.Errorf("RTP version = %d, want 2", data[0]>>6)
	}

	// Check sequence number (bytes 2-3, big-endian)
	seq := uint16(data[2])<<8 | uint16(data[3])
	if seq != 5 {
		t.Errorf("RTP sequence = %d, want 5", seq)
	}

	// Check timestamp (bytes 4-7, big-endian)
	ts := uint32(data[4])<<24 | uint32(data[5])<<16 | uint32(data[6])<<8 | uint32(data[7])
	if ts != 0x12345678 {
		t.Errorf("RTP timestamp = 0x%08x, want 0x12345678", ts)
	}
}

func TestPacketizerFUAMode(t *testing.T) {
	t.Parallel()

	p := NewPacketizer(98, 0x12345678)
	p.Mode = ModeFUA
	p.Sequence = 1
	p.Timestamp = 0

	nalus := []NALU{
		{Type: 33, LayerID: 0, TemporalID: 0, Data: []byte{0x01, 0x02, 0x03}},
		{Type: 34, LayerID: 0, TemporalID: 0, Data: []byte{0x04, 0x05}},
	}

	packets := p.Packetize(nalus, true)
	if len(packets) == 0 {
		t.Error("Packetize returned no packets")
	}

	// Check that packets are valid RTP (version field check)
	for _, pkt := range packets {
		if pkt[0]>>6 != 2 {
			t.Errorf("packet version = %d, want 2", pkt[0]>>6)
		}
	}
}

func TestPacketizerSTAPAMode(t *testing.T) {
	t.Parallel()

	p := NewPacketizer(98, 0x12345678)
	p.Mode = ModeSTAPA
	p.Sequence = 1
	p.Timestamp = 0

	nalus := []NALU{
		{Type: 32, LayerID: 0, TemporalID: 0, Data: []byte{0x01}},       // VPS
		{Type: 33, LayerID: 0, TemporalID: 0, Data: []byte{0x02, 0x03}}, // SPS
		{Type: 34, LayerID: 0, TemporalID: 0, Data: []byte{0x04}},       // PPS
	}

	packets := p.Packetize(nalus, true)
	// In STAP-A mode, small NALUs are aggregated into one packet
	if len(packets) != 1 {
		t.Errorf("STAP-A mode: got %d packets, want 1", len(packets))
	}
}
