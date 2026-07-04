package pcap

import (
	"testing"
	"time"
)

func TestBuildSessionBundleFromPackets_SortsByRTPTimestamp(t *testing.T) {
	// Create packets with RTP timestamps out of order (but capture times in order)
	now := time.Now()
	packets := []RTPPacket{
		{Sequence: 3, Timestamp: 3000, Marker: false, PayloadType: 96, SSRC: 1, Payload: []byte{0x65, 0x01}, CapturedAt: now.Add(3 * time.Millisecond)},
		{Sequence: 1, Timestamp: 1000, Marker: true, PayloadType: 96, SSRC: 1, Payload: []byte{0x65, 0x01}, CapturedAt: now.Add(1 * time.Millisecond)},
		{Sequence: 2, Timestamp: 2000, Marker: false, PayloadType: 96, SSRC: 1, Payload: []byte{0x65, 0x01}, CapturedAt: now.Add(2 * time.Millisecond)},
	}

	bundle, err := BuildSessionBundleFromPackets(packets, "test")
	if err != nil {
		t.Fatalf("BuildSessionBundleFromPackets error: %v", err)
	}

	// Verify timeline is ordered by RTP timestamp, not capture time
	if len(bundle.Timeline) != 3 {
		t.Fatalf("Expected 3 timeline events, got %d", len(bundle.Timeline))
	}

	// First packet should have timestamp 1000 (was originally 2nd in capture order)
	if bundle.Timeline[0].Timestamp != 1000 {
		t.Errorf("First packet timestamp = %d, want 1000", bundle.Timeline[0].Timestamp)
	}
	if bundle.Timeline[0].Sequence != 1 {
		t.Errorf("First packet sequence = %d, want 1", bundle.Timeline[0].Sequence)
	}

	// Second packet should have timestamp 2000 (was originally 3rd in capture order)
	if bundle.Timeline[1].Timestamp != 2000 {
		t.Errorf("Second packet timestamp = %d, want 2000", bundle.Timeline[1].Timestamp)
	}
	if bundle.Timeline[1].Sequence != 2 {
		t.Errorf("Second packet sequence = %d, want 2", bundle.Timeline[1].Sequence)
	}

	// Third packet should have timestamp 3000 (was originally 1st in capture order)
	if bundle.Timeline[2].Timestamp != 3000 {
		t.Errorf("Third packet timestamp = %d, want 3000", bundle.Timeline[2].Timestamp)
	}
	if bundle.Timeline[2].Sequence != 3 {
		t.Errorf("Third packet sequence = %d, want 3", bundle.Timeline[2].Sequence)
	}
}

func TestBuildSessionBundleFromPackets_EmittedAtFromRTPTimestamp(t *testing.T) {
	// Create packets with known RTP timestamp deltas
	now := time.Now()
	packets := []RTPPacket{
		{Sequence: 1, Timestamp: 90000, Marker: true, PayloadType: 96, SSRC: 1, Payload: []byte{0x65, 0x01}, CapturedAt: now},
		{Sequence: 2, Timestamp: 90090, Marker: false, PayloadType: 96, SSRC: 1, Payload: []byte{0x65, 0x01}, CapturedAt: now.Add(1 * time.Millisecond)},
	}

	bundle, err := BuildSessionBundleFromPackets(packets, "test")
	if err != nil {
		t.Fatalf("BuildSessionBundleFromPackets error: %v", err)
	}

	// First packet EmittedAt should be 0 (baseline)
	if bundle.Timeline[0].EmittedAt != 0 {
		t.Errorf("First packet EmittedAt = %v, want 0", bundle.Timeline[0].EmittedAt)
	}

	// Second packet EmittedAt: 90 RTP units * (1e9/90000) ns = 90 * 11111 ns = 999990 ns
	// Due to integer division truncation, this is approximately 1ms
	expectedEmittedAt := 999990 * time.Nanosecond // approximately 1ms
	if bundle.Timeline[1].EmittedAt != expectedEmittedAt {
		t.Errorf("Second packet EmittedAt = %v, want ~%v (approximately 1ms with truncation)", bundle.Timeline[1].EmittedAt, expectedEmittedAt)
	}
}

func TestBuildSessionBundleFromPackets_ReceivedAtPreserved(t *testing.T) {
	now := time.Now()
	packets := []RTPPacket{
		{Sequence: 1, Timestamp: 1000, Marker: true, PayloadType: 96, SSRC: 1, Payload: []byte{0x65, 0x01}, CapturedAt: now},
		{Sequence: 2, Timestamp: 2000, Marker: false, PayloadType: 96, SSRC: 1, Payload: []byte{0x65, 0x01}, CapturedAt: now.Add(5 * time.Millisecond)},
	}

	bundle, err := BuildSessionBundleFromPackets(packets, "test")
	if err != nil {
		t.Fatalf("BuildSessionBundleFromPackets error: %v", err)
	}

	// ReceivedAt should reflect capture time delta (wall clock)
	if bundle.Timeline[0].ReceivedAt != 0 {
		t.Errorf("First packet ReceivedAt = %v, want 0", bundle.Timeline[0].ReceivedAt)
	}
	if bundle.Timeline[1].ReceivedAt != 5*time.Millisecond {
		t.Errorf("Second packet ReceivedAt = %v, want 5ms", bundle.Timeline[1].ReceivedAt)
	}
}

func TestBuildSessionBundleFromPackets_HighFidelityMetadata(t *testing.T) {
	packets := []RTPPacket{
		{Sequence: 1, Timestamp: 12345, Marker: true, PayloadType: 96, SSRC: 0x12345678, Payload: []byte{0x65}, CapturedAt: time.Now()},
	}

	bundle, err := BuildSessionBundleFromPackets(packets, "test")
	if err != nil {
		t.Fatalf("BuildSessionBundleFromPackets error: %v", err)
	}

	// Check high-fidelity replay metadata
	if bundle.Metadata["replay.mode"] != "rtp-timestamp" {
		t.Errorf("replay.mode = %q, want rtp-timestamp", bundle.Metadata["replay.mode"])
	}
	if bundle.Metadata["rtp.first_timestamp_rtp"] != "12345" {
		t.Errorf("rtp.first_timestamp_rtp = %q, want 12345", bundle.Metadata["rtp.first_timestamp_rtp"])
	}
	if bundle.Metadata["capture_start_ntp"] == "" {
		t.Error("capture_start_ntp should not be empty")
	}
}
