package h265

import (
	"context"
	"fmt"
	"strings"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/input"
	"stream-source-tester/internal/input/probe"
	"stream-source-tester/internal/model"
)

func init() {
	if err := input.Register("h265", func() input.Source { return &Source{} }); err != nil {
		panic(err)
	}
}

// Packetization options for H265.
const (
	OptPacketization   = "packetization"     // fu-a | stap-a | mixed (default: fu-a)
	OptSPSPPSInjection = "sps-pps-injection" // first-frame | every-gop | setup-only | none
	OptProfileSpace    = "profile-space"     // default 0
	OptTierFlag        = "tier-flag"         // default 0
	OptProfileID       = "profile-id"        // default 1 (Main Profile)
	OptLevelID         = "level-id"          // default 93 (Level 3.1)
)

// Source wraps H265 Annex B input and packetization.
type Source struct{}

func (s *Source) Open(ctx context.Context, cfg config.InputConfig) (*model.SessionBundle, error) {
	_ = ctx

	info, err := probe.ReadFileInfo(cfg.Location, 32)
	if err != nil {
		return nil, err
	}

	// Check for Annex B start code prefix
	header := info.Header
	isAnnexB := false
	if len(header) >= 4 && header[0] == 0 && header[1] == 0 && (header[2] == 1 || (header[2] == 0 && header[3] == 1)) {
		isAnnexB = true
	}

	bundle := model.NewMinimalSessionBundle(
		cfg.Name,
		model.CodecH265,
		cfg.Kind,
		cfg.Location,
		cfg.Options,
	)
	bundle.Transport = []model.Protocol{model.ProtocolRTSP, model.ProtocolRTPUDP}
	bundle.Metadata["source.format"] = "AnnexB/h265"
	bundle.Metadata["source.annex_b"] = fmt.Sprintf("%v", isAnnexB)

	// Extract and encode VPS/SPS/PPS for SDP
	vpsStr, _ := Base64VPS(header)
	spsStr, _ := Base64SPS(header)
	ppsStr, _ := Base64PPS(header)

	// Build stream parameters
	params := make(map[string]string)

	// Packetization mode
	packetization := strings.ToLower(cfg.Options[OptPacketization])
	if packetization == "" {
		packetization = "fu-a"
	}
	params["packetization"] = packetization

	// SDP fmtp parameters
	profileSpace := cfg.Options[OptProfileSpace]
	if profileSpace == "" {
		profileSpace = "0"
	}
	tierFlag := cfg.Options[OptTierFlag]
	if tierFlag == "" {
		tierFlag = "0"
	}
	profileID := cfg.Options[OptProfileID]
	if profileID == "" {
		profileID = "1"
	}
	levelID := cfg.Options[OptLevelID]
	if levelID == "" {
		levelID = "93"
	}

	params["profile-space"] = profileSpace
	params["tier-flag"] = tierFlag
	params["profile-id"] = profileID
	params["level-id"] = levelID
	params["profile-compatibility"] = "1"

	if vpsStr != "" {
		params["sprop-vps"] = vpsStr
	}
	if spsStr != "" {
		params["sprop-sps"] = spsStr
	}
	if ppsStr != "" {
		params["sprop-pps"] = ppsStr
	}

	// Update stream with parameters
	if len(bundle.Streams) > 0 {
		bundle.Streams[0].Parameters = params
		bundle.Streams[0].PayloadType = 98 // Standard HEVC payload type
		bundle.Streams[0].ClockRate = 90000
	}

	bundle.Metadata["probe.file_size"] = fmt.Sprintf("%d", info.Size)
	bundle.Metadata["h265.packetization"] = packetization

	return bundle, nil
}
