package pcap

import (
	"fmt"
	"os"
	"testing"
)

func TestReadRTPPackets(t *testing.T) {
	path := "../../../../fixtures/test_rtp_10s.pcap"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not available: %v", err)
	}

	packets, err := ReadRTPPackets(path)
	if err != nil {
		t.Fatalf("ReadRTPPackets error: %v", err)
	}
	if len(packets) == 0 {
		t.Fatal("expected RTP packets")
	}
	fmt.Printf("Found %d RTP packets\n", len(packets))
	fmt.Printf("First: Seq=%d TS=%d PT=%d SSRC=0x%08x PayloadLen=%d\n",
		packets[0].Sequence, packets[0].Timestamp,
		packets[0].PayloadType, packets[0].SSRC, len(packets[0].Payload))
	last := packets[len(packets)-1]
	fmt.Printf("Last:  Seq=%d TS=%d PT=%d SSRC=0x%08x PayloadLen=%d\n",
		last.Sequence, last.Timestamp,
		last.PayloadType, last.SSRC, len(last.Payload))
}
