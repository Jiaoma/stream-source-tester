package rewritertptimestamp

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
)

func init() {
	if err := mutation.Register("rewrite-rtp-timestamp", func() mutation.Mutator { return &Mutator{} }); err != nil {
		panic(err)
	}
}

type Mutator struct{}

func (m *Mutator) Apply(ctx context.Context, bundle *model.SessionBundle, cfg config.MutationConfig) (*model.SessionBundle, error) {
	_ = ctx
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}

	// Default: no offset
	offset := int64(0)
	if raw := cfg.Options["rtp.offset"]; raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid rtp.offset %q: %w", raw, err)
		}
		offset = parsed
	}

	// Clock rate override (default 0 means use stream's clock rate)
	clockRate := 0
	if raw := cfg.Options["rtp.clockrate"]; raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid rtp.clockrate %q: %w", raw, err)
		}
		clockRate = parsed
	}

	// Capture offset: offset between RTP timestamp and NTP/capture time
	captureOffset := int64(0)
	if raw := cfg.Options["rtp.capture_offset"]; raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid rtp.capture_offset %q: %w", raw, err)
		}
		captureOffset = parsed
	}

	// Find the first timestamp and clock rate to use
	firstTimestamp := uint32(0)
	streamClockRate := 90000
	if len(bundle.Streams) > 0 {
		streamClockRate = bundle.Streams[0].ClockRate
		if streamClockRate == 0 {
			streamClockRate = 90000
		}
	}

	// Apply clock rate override if specified
	effectiveClockRate := streamClockRate
	if clockRate > 0 {
		effectiveClockRate = clockRate
	}

	// Compute timestamp scale factor for clock rate override
	// If switching from 90000 to 30fps (90000/3000), we need to scale timestamps
	scaleFactor := float64(effectiveClockRate) / float64(streamClockRate)

	// First pass: find first timestamp
	for _, event := range bundle.Timeline {
		if firstTimestamp == 0 {
			firstTimestamp = event.Timestamp
			break
		}
	}

	// Second pass: rewrite timestamps
	for i := range bundle.Timeline {
		event := &bundle.Timeline[i]

		// Apply offset
		newTimestamp := int64(event.Timestamp) + offset

		// Apply clock rate override (scale relative to first timestamp)
		if clockRate > 0 && clockRate != streamClockRate {
			// Scale timestamp delta from first packet
			delta := int64(event.Timestamp) - int64(firstTimestamp)
			scaledDelta := int64(float64(delta) * scaleFactor)
			newTimestamp = int64(firstTimestamp) + scaledDelta
		}

		// Handle wraparound (RTP timestamps are uint32)
		if newTimestamp < 0 {
			newTimestamp += 1 << 32
		}
		if newTimestamp > (1<<32)-1 {
			newTimestamp -= 1 << 32
		}

		event.Timestamp = uint32(newTimestamp)
	}

	// Apply capture offset if specified
	if captureOffset != 0 {
		bundle.CaptureOffset = bundle.CaptureOffset + time.Duration(captureOffset*1000000) // in microseconds
	}

	// Record metadata
	if offset != 0 {
		bundle.Metadata["mutation.rewrite-rtp-timestamp.offset"] = strconv.FormatInt(offset, 10)
	}
	if clockRate > 0 {
		bundle.Metadata["mutation.rewrite-rtp-timestamp.clockrate"] = strconv.Itoa(clockRate)
	}
	if captureOffset != 0 {
		bundle.Metadata["mutation.rewrite-rtp-timestamp.capture_offset"] = strconv.FormatInt(captureOffset, 10)
	}

	return bundle, nil
}
