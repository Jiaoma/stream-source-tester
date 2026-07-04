package mjpeg

import (
	"context"
	"fmt"
	"strconv"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/input"
	"stream-source-tester/internal/input/probe"
	"stream-source-tester/internal/model"
)

func init() {
	if err := input.Register("mjpeg", func() input.Source { return &Source{} }); err != nil {
		panic(err)
	}
}

const (
	OptQuality   = "quality"   // 1-100 quantization scale
	OptPacketize = "packetize" // single | multiple
	OptWidth     = "width"
	OptHeight    = "height"
	OptDepth     = "depth"
	OptType      = "type"
)

// Source wraps MJPEG input.
type Source struct{}

func (s *Source) Open(ctx context.Context, cfg config.InputConfig) (*model.SessionBundle, error) {
	_ = ctx

	info, err := probe.ReadFileInfo(cfg.Location, 32)
	if err != nil {
		return nil, err
	}

	header := info.Header

	// Detect JPEG from SOI marker (0xFF 0xD8)
	isJPEG := false
	if len(header) >= 2 && header[0] == 0xFF && header[1] == 0xD8 {
		isJPEG = true
	}

	bundle := model.NewMinimalSessionBundle(
		cfg.Name,
		model.CodecMJPEG,
		cfg.Kind,
		cfg.Location,
		cfg.Options,
	)
	bundle.Transport = []model.Protocol{model.ProtocolRTSP, model.ProtocolRTPUDP}
	bundle.Metadata["source.format"] = "image/jpeg"
	bundle.Metadata["source.jpeg.detected"] = fmt.Sprintf("%v", isJPEG)
	bundle.Metadata["probe.file_size"] = strconv.FormatInt(info.Size, 10)

	// Parse JPEG parameters from headers
	params := make(map[string]string)

	// Quality
	quality := cfg.Options[OptQuality]
	if quality == "" {
		quality = "80"
	}
	params["quality"] = quality

	// Packetization mode
	packetize := cfg.Options[OptPacketize]
	if packetize == "" {
		packetize = "single"
	}
	params["packetize"] = packetize

	// Try to detect width/height from JPEG SOF0
	if w := cfg.Options[OptWidth]; w != "" {
		params["width"] = w
	}
	if h := cfg.Options[OptHeight]; h != "" {
		params["height"] = h
	}
	if d := cfg.Options[OptDepth]; d != "" {
		params["depth"] = d
	}
	if t := cfg.Options[OptType]; t != "" {
		params["type"] = t
	}

	// Standard MJPEG payload type
	pt := uint8(26)
	if bundle.Streams[0].PayloadType != 0 {
		pt = bundle.Streams[0].PayloadType
	}

	if len(bundle.Streams) > 0 {
		bundle.Streams[0].PayloadType = pt
		bundle.Streams[0].ClockRate = 90000
		bundle.Streams[0].Parameters = params
	}

	return bundle, nil
}
