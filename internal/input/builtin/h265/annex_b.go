package h265

import (
	"bytes"
	"errors"
	"io"
	"strings"
)

var ErrInvalidH265Data = errors.New("invalid H265 data")

// NALU represents a parsed H265 NAL unit extracted from Annex B bitstream.
type NALU struct {
	Type       uint8  // NAL unit type (6 bits from header)
	LayerID    uint8  // nuh_layer_id (6 bits)
	TemporalID uint8  // nuh_temporal_id_plus1 (3 bits)
	Data       []byte // NAL unit payload (after 2-byte header)
	Raw        []byte // Full NAL unit including 2-byte header
}

// parseNALHeader parses the 2-byte H265 NAL header.
// Byte 0: forbidden(1) | nal_unit_type(6) | nuh_layer_id high(1)
// Byte 1: nuh_layer_id low(5) | nuh_temporal_id_plus1(3)
func parseNALHeader(data []byte) (typ, layerID, tempID uint8, err error) {
	if len(data) < 2 {
		return 0, 0, 0, errors.New("need at least 2 bytes for H265 NAL header")
	}
	b0, b1 := data[0], data[1]
	typ = (b0 >> 1) & 0x3F
	layerID = ((b0 & 0x01) << 5) | ((b1 >> 3) & 0x1F)
	tempID = b1 & 0x07
	return typ, layerID, tempID, nil
}

// FindAnnexBStartCodes scans data for Annex B start codes (0x000001 or 0x00000001).
func FindAnnexBStartCodes(data []byte) []int {
	var offsets []int
	for i := 0; i+3 < len(data); i++ {
		if data[i] == 0 && data[i+1] == 0 {
			// 3-byte start code: 0x000001
			if data[i+2] == 1 {
				offsets = append(offsets, i)
			}
		}
	}
	for i := 0; i+4 < len(data); i++ {
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			offsets = append(offsets, i)
		}
	}
	return offsets
}

// ExtractNALUs extracts all H265 NAL units from an Annex B bitstream.
func ExtractNALUs(annexBData []byte) ([]NALU, error) {
	if len(annexBData) < 3 {
		return nil, ErrInvalidH265Data
	}

	startCodes := FindAnnexBStartCodes(annexBData)
	if len(startCodes) < 2 {
		// Single NALU without leading start code
		if len(annexBData) >= 2 {
			typ, layerID, tempID, _ := parseNALHeader(annexBData)
			return []NALU{{Type: typ, LayerID: layerID, TemporalID: tempID, Data: annexBData[2:], Raw: annexBData}}, nil
		}
		return nil, ErrInvalidH265Data
	}

	var nalus []NALU
	for i := 0; i < len(startCodes)-1; i++ {
		startOff := startCodes[i]
		startCodeLen := 4
		if startOff+3 >= len(annexBData) || annexBData[startOff+2] != 0 || annexBData[startOff+3] != 1 {
			startCodeLen = 3
		}
		dataStart := startOff + startCodeLen
		dataEnd := startCodes[i+1]
		nalData := annexBData[dataStart:dataEnd]
		if len(nalData) < 2 {
			continue
		}
		typ, layerID, tempID, err := parseNALHeader(nalData)
		if err != nil {
			continue
		}
		raw := annexBData[startOff:dataEnd]
		nalus = append(nalus, NALU{
			Type:       typ,
			LayerID:    layerID,
			TemporalID: tempID,
			Data:       nalData[2:],
			Raw:        raw,
		})
	}

	// Last NALU
	lastOff := startCodes[len(startCodes)-1]
	startCodeLen := 4
	if lastOff+3 >= len(annexBData) || annexBData[lastOff+2] != 0 || annexBData[lastOff+3] != 1 {
		startCodeLen = 3
	}
	dataStart := lastOff + startCodeLen
	if dataStart < len(annexBData) {
		nalData := annexBData[dataStart:]
		if len(nalData) >= 2 {
			typ, layerID, tempID, _ := parseNALHeader(nalData)
			raw := annexBData[lastOff:]
			nalus = append(nalus, NALU{
				Type:       typ,
				LayerID:    layerID,
				TemporalID: tempID,
				Data:       nalData[2:],
				Raw:        raw,
			})
		}
	}
	return nalus, nil
}

// ExtractParamSets extracts VPS, SPS, and PPS NALUs from Annex B data.
func ExtractParamSets(annexBData []byte) (vps, sps, pps []byte, err error) {
	nalus, err := ExtractNALUs(annexBData)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, nalu := range nalus {
		switch nalu.Type {
		case NUT_VPS:
			vps = nalu.Raw
		case NUT_SPS:
			sps = nalu.Raw
		case NUT_PPS:
			pps = nalu.Raw
		}
	}
	return vps, sps, pps, nil
}

// EncodeNALU encodes a NALU into Annex B format with a 4-byte start code.
func EncodeNALU(naluType uint8, layerID, temporalID uint8, payload []byte) []byte {
	header := make([]byte, 2)
	header[0] = (naluType & 0x3F) << 1
	header[0] |= (layerID >> 5) & 0x01
	header[1] = (layerID & 0x1F) << 3
	header[1] |= temporalID & 0x07
	var buf bytes.Buffer
	buf.Write([]byte{0, 0, 0, 1})
	buf.Write(header)
	buf.Write(payload)
	return buf.Bytes()
}

// H265 NAL unit type constants (Table 7-1 of ITU-T H.265)
const (
	NUT_VPS     = 32
	NUT_SPS     = 33
	NUT_PPS     = 34
	NUT_APS     = 35
	NUT_AUD     = 36
	NUT_FD      = 37
	NUT_SEI     = 39
	NUT_IDR     = 19
	NUT_IDR_N   = 20
	NUT_TRAIL_N = 1
)

// IsParamSet returns true if the NAL unit type is a parameter set.
func IsParamSet(naluType uint8) bool {
	return naluType == NUT_VPS || naluType == NUT_SPS || naluType == NUT_PPS
}

// IsKeyframe returns true if this NAL unit starts a random access point.
func IsKeyframe(naluType uint8) bool {
	return naluType >= 16 && naluType <= 23
}

// BitReader reads bits from a byte stream (MSB first).
type BitReader struct {
	buf []byte
	pos int
	bit int
}

// NewBitReader creates a bit reader from a byte slice.
func NewBitReader(data []byte) *BitReader {
	return &BitReader{buf: data}
}

// ReadBits reads n bits (up to 32) from the stream, MSB first.
func (br *BitReader) ReadBits(n int) (uint32, error) {
	if n > 32 {
		n = 32
	}
	var val uint32
	bitsRead := 0
	for bitsRead < n {
		if br.pos >= len(br.buf) {
			return val, io.EOF
		}
		bitsLeftInByte := 8 - br.bit
		bitsToRead := bitsLeftInByte
		if bitsToRead > n-bitsRead {
			bitsToRead = n - bitsRead
		}
		mask := byte(0xFF >> (8 - bitsToRead))
		shift := bitsLeftInByte - bitsToRead
		val = (val << uint(bitsToRead)) | uint32((br.buf[br.pos]>>shift)&mask)
		br.bit += bitsToRead
		if br.bit >= 8 {
			br.pos++
			br.bit = 0
		}
		bitsRead += bitsToRead
	}
	return val, nil
}

// ReadBit reads a single bit.
func (br *BitReader) ReadBit() (uint32, error) {
	return br.ReadBits(1)
}

// ReadExpGolomb reads an unsigned exponential-Golomb coded value.
func (br *BitReader) ReadExpGolomb() (uint32, error) {
	leadingZeros := 0
	for {
		bit, err := br.ReadBit()
		if err != nil {
			return 0, err
		}
		if bit == 1 {
			break
		}
		leadingZeros++
		if leadingZeros > 32 {
			return 0, errors.New("exp-golomb overflow")
		}
	}
	val, err := br.ReadBits(leadingZeros)
	if err != nil {
		return 0, err
	}
	return val + (1 << leadingZeros) - 1, nil
}

// SPSInfo holds parsed SPS information useful for SDP generation.
type SPSInfo struct {
	ProfileSpace uint8
	TierFlag     uint8
	ProfileID    uint32
	LevelID      uint32
	ChromaFormat uint32
	PicWidth     uint32
	PicHeight    uint32
}

// ParseSPS parses an H265 SPS NAL unit (raw data including NAL header).
func ParseSPS(naluData []byte) (*SPSInfo, error) {
	if len(naluData) < 6 {
		return nil, errors.New("SPS NALU too short")
	}
	br := NewBitReader(naluData)
	// Skip 2-byte NAL header
	_, err := br.ReadBits(16)
	if err != nil {
		return nil, err
	}
	br.ReadExpGolomb() // sps_video_parameter_set_id
	maxSubLayersMinus1, err := br.ReadExpGolomb()
	if err != nil {
		return nil, err
	}
	_ = maxSubLayersMinus1
	br.ReadBit() // sps_temporal_id_nesting_flag
	profileSpace := uint8(0)
	if maxSubLayersMinus1 > 0 {
		bits, _ := br.ReadBits(2)
		profileSpace = uint8(bits)
	}
	profileID, _ := br.ReadBits(8)
	tierFlag, _ := br.ReadBits(1)
	levelID, _ := br.ReadBits(5)
	// Skip profile compatibility
	for i := 0; i < 31; i++ {
		br.ReadBit()
	}
	numConstr, err := br.ReadExpGolomb()
	if err != nil {
		return nil, err
	}
	for i := uint32(0); i < numConstr; i++ {
		br.ReadBits(32)
		br.ReadBits(32)
	}
	br.ReadExpGolomb() // sps_seq_parameter_set_id
	chromaFormat, _ := br.ReadExpGolomb()
	picWidth, _ := br.ReadExpGolomb()
	picHeight, _ := br.ReadExpGolomb()
	return &SPSInfo{
		ProfileSpace: profileSpace,
		TierFlag:     uint8(tierFlag),
		ProfileID:    profileID,
		LevelID:      levelID,
		ChromaFormat: chromaFormat,
		PicWidth:     picWidth,
		PicHeight:    picHeight,
	}, nil
}

// Base64SPS extracts SPS from Annex B data and returns it base64-encoded.
func Base64SPS(annexBData []byte) (string, error) {
	_, sps, _, err := ExtractParamSets(annexBData)
	if err != nil || sps == nil {
		return "", err
	}
	return Base64Encode(stripStartCode(sps)), nil
}

// Base64VPS extracts VPS from Annex B data and returns it base64-encoded.
func Base64VPS(annexBData []byte) (string, error) {
	vps, _, _, err := ExtractParamSets(annexBData)
	if err != nil || vps == nil {
		return "", err
	}
	return Base64Encode(stripStartCode(vps)), nil
}

// Base64PPS extracts PPS from Annex B data and returns it base64-encoded.
func Base64PPS(annexBData []byte) (string, error) {
	_, _, pps, err := ExtractParamSets(annexBData)
	if err != nil || pps == nil {
		return "", err
	}
	return Base64Encode(stripStartCode(pps)), nil
}

func stripStartCode(data []byte) []byte {
	if len(data) >= 4 && data[0] == 0 && data[1] == 0 && data[2] == 0 && data[3] == 1 {
		return data[4:]
	}
	if len(data) >= 3 && data[0] == 0 && data[1] == 0 && data[2] == 1 {
		return data[3:]
	}
	return data
}

// Base64Encode encodes bytes to standard base64.
func Base64Encode(data []byte) string {
	const b64chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var sb strings.Builder
	sb.Grow((len(data)+2)/3*4 + 4)
	for i := 0; i < len(data); i += 3 {
		var val uint32
		remaining := len(data) - i
		if remaining >= 1 {
			val |= uint32(data[i]) << 16
		}
		if remaining >= 2 {
			val |= uint32(data[i+1]) << 8
		}
		if remaining >= 3 {
			val |= uint32(data[i+2])
		}
		pads := 0
		if remaining == 1 {
			pads = 2
		} else if remaining == 2 {
			pads = 1
		}
		sb.WriteByte(b64chars[(val>>18)&0x3F])
		sb.WriteByte(b64chars[(val>>12)&0x3F])
		if remaining >= 2 {
			sb.WriteByte(b64chars[(val>>6)&0x3F])
		}
		if remaining >= 3 {
			sb.WriteByte(b64chars[val&0x3F])
		}
		sb.WriteString(strings.Repeat("=", pads))
	}
	return sb.String()
}
