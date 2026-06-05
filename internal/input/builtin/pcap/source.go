package pcap

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
	if err := input.Register("pcap", func() input.Source { return &Source{} }); err != nil {
		panic(err)
	}
}

type Source struct{}

func (s *Source) Open(ctx context.Context, cfg config.InputConfig) (*model.SessionBundle, error) {
	_ = ctx

	info, err := probe.ReadFileInfo(cfg.Location, 24)
	if err != nil {
		return nil, err
	}
	parsed, err := parseHeader(info.Header)
	if err != nil {
		return nil, fmt.Errorf("parse pcap header from %q: %w", cfg.Location, err)
	}

	bundle := model.NewMinimalSessionBundle(
		cfg.Name,
		model.ParseCodec(cfg.Codec),
		cfg.Kind,
		cfg.Location,
		cfg.Options,
	)
	bundle.Transport = []model.Protocol{model.ProtocolRTSP, model.ProtocolRTPUDP}
	bundle.Metadata["source.format"] = "capture/pcap"
	bundle.Metadata["bundle.mode"] = "probed-from-capture"
	bundle.Metadata["probe.file_size"] = strconv.FormatInt(info.Size, 10)
	bundle.Metadata["probe.magic"] = fmt.Sprintf("0x%08x", parsed.Magic)
	bundle.Metadata["probe.endian"] = parsed.Endian
	bundle.Metadata["probe.version_major"] = strconv.FormatUint(uint64(parsed.VersionMajor), 10)
	bundle.Metadata["probe.version_minor"] = strconv.FormatUint(uint64(parsed.VersionMinor), 10)
	bundle.Metadata["probe.snaplen"] = strconv.FormatUint(uint64(parsed.SnapLen), 10)
	bundle.Metadata["probe.linktype"] = strconv.FormatUint(uint64(parsed.LinkType), 10)
	return bundle, nil
}
