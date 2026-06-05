package mp4

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/input"
	"stream-source-tester/internal/input/probe"
	"stream-source-tester/internal/model"
)

func init() {
	if err := input.Register("mp4", func() input.Source { return &Source{} }); err != nil {
		panic(err)
	}
}

type Source struct{}

func (s *Source) Open(ctx context.Context, cfg config.InputConfig) (*model.SessionBundle, error) {
	_ = ctx

	info, err := probe.ReadFileInfo(cfg.Location, 32)
	if err != nil {
		return nil, err
	}
	if len(info.Header) < 12 || !bytes.Contains(info.Header[4:], []byte("ftyp")) {
		return nil, fmt.Errorf("file %q does not look like MP4/ISO BMFF", cfg.Location)
	}

	ftyp, err := parseFTYP(info.Header)
	if err != nil {
		return nil, fmt.Errorf("parse ftyp from %q: %w", cfg.Location, err)
	}

	bundle := model.NewMinimalSessionBundle(
		cfg.Name,
		model.ParseCodec(cfg.Codec),
		cfg.Kind,
		cfg.Location,
		cfg.Options,
	)
	bundle.Transport = []model.Protocol{model.ProtocolRTSP, model.ProtocolRTPUDP}
	bundle.Metadata["source.format"] = "container/mp4"
	bundle.Metadata["bundle.mode"] = "probed-from-mp4"
	bundle.Metadata["probe.file_size"] = strconv.FormatInt(info.Size, 10)
	bundle.Metadata["probe.header_hex"] = fmt.Sprintf("%x", info.Header)
	bundle.Metadata["probe.major_brand"] = ftyp.MajorBrand
	bundle.Metadata["probe.minor_version"] = strconv.FormatUint(uint64(ftyp.MinorVersion), 10)
	bundle.Metadata["probe.compatible_brands"] = strings.Join(ftyp.CompatibleBrands, ",")
	return bundle, nil
}
