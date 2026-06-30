package pcap

import (
	"fmt"
	"testing"
)

func TestReadRTPPackets(t *testing.T) {
	packets, err := ReadRTPPackets("/home/jetson/stream-source-tester/fixtures/ffmpeg_rtp_120s_v2.pcap")
	if err != nil {
		t.Fatalf("ReadRTPPackets error: %v", err)
	}
	fmt.Printf("Found %d RTP packets\n", len(packets))
	if len(packets) > 0 {
		fmt.Printf("First: Seq=%d TS=%d PT=%d SSRC=0x%08x PayloadLen=%d\n",
			packets[0].Sequence, packets[0].Timestamp,
			packets[0].PayloadType, packets[0].SSRC, len(packets[0].Payload))
		last := packets[len(packets)-1]
		fmt.Printf("Last:  Seq=%d TS=%d PT=%d SSRC=0x%08x PayloadLen=%d\n",
			last.Sequence, last.Timestamp,
			last.PayloadType, last.SSRC, len(last.Payload))
	}
}
