package h265

import (
	"testing"
)

func TestFindAnnexBStartCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		data   []byte
		expect []int
	}{
		{
			name:   "empty",
			data:   []byte{},
			expect: nil,
		},
		{
			name:   "single-four-byte-start-code",
			data:   []byte{0, 0, 0, 1, 0x40},
			expect: []int{1, 0}, // 3-byte at pos1, 4-byte at pos0
		},
		{
			name:   "three-byte-start-code",
			data:   []byte{0, 0, 1, 0x40, 0x01, 0x00},
			expect: []int{0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FindAnnexBStartCodes(tc.data)
			if !slicesEqual(got, tc.expect) {
				t.Errorf("FindAnnexBStartCodes(%x) = %v, want %v", tc.data, got, tc.expect)
			}
		})
	}
}

func slicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEncodeNALU(t *testing.T) {
	t.Parallel()

	payload := []byte{0x01, 0x02, 0x03, 0x04}
	encoded := EncodeNALU(32, 0, 0, payload)

	// Should start with 4-byte start code
	if encoded[0] != 0 || encoded[1] != 0 || encoded[2] != 0 || encoded[3] != 1 {
		t.Errorf("start code = %x, want 00000001", encoded[:4])
	}

	// NAL header byte 0: forbidden=0 | type=32(6 bits) | layer_high(1 bit)
	// 32 = 0b100000, shifted left 1 = 0x40
	if encoded[4] != 0x40 {
		t.Errorf("NAL header byte 0 = 0x%02x, want 0x40", encoded[4])
	}

	// NAL header byte 1: layer_low(5 bits) | temporal(3 bits)
	if encoded[5] != 0x00 {
		t.Errorf("NAL header byte 1 = 0x%02x, want 0x00", encoded[5])
	}
}

func TestIsParamSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		naluType uint8
		want     bool
	}{
		{32, true},  // VPS
		{33, true},  // SPS
		{34, true},  // PPS
		{35, false}, // APS
		{36, false}, // AUD
		{39, false}, // SEI
		{19, false}, // IDR
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			if got := IsParamSet(tc.naluType); got != tc.want {
				t.Errorf("IsParamSet(%d) = %v, want %v", tc.naluType, got, tc.want)
			}
		})
	}
}

func TestIsKeyframe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		naluType uint8
		want     bool
	}{
		{16, true},  // BLA_W_LP
		{17, true},  // BLA_W_RADL
		{18, true},  // BLA_N_LP
		{19, true},  // IDR_W_RADL
		{20, true},  // IDR_N_LP
		{21, true},  // CRA_NUT
		{22, true},  // RSV_RAP
		{23, true},  // RSV_RAP
		{32, false}, // VPS
		{33, false}, // SPS
		{34, false}, // PPS
		{36, false}, // AUD
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			if got := IsKeyframe(tc.naluType); got != tc.want {
				t.Errorf("IsKeyframe(%d) = %v, want %v", tc.naluType, got, tc.want)
			}
		})
	}
}

func TestBase64Encode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  []byte
		expect string
	}{
		{"empty", []byte{}, ""},
		{"three-bytes", []byte{0x01, 0x02, 0x03}, "AQID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Base64Encode(tc.input)
			if got != tc.expect {
				t.Errorf("Base64Encode(%x) = %q, want %q", tc.input, got, tc.expect)
			}
		})
	}
}

func TestStripStartCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{
			name:  "four-byte",
			input: []byte{0, 0, 0, 1, 0x40, 0x01},
			want:  []byte{0x40, 0x01},
		},
		{
			name:  "three-byte",
			input: []byte{0, 0, 1, 0x40, 0x01},
			want:  []byte{0x40, 0x01},
		},
		{
			name:  "no-start-code",
			input: []byte{0x40, 0x01},
			want:  []byte{0x40, 0x01},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripStartCode(tc.input)
			if string(got) != string(tc.want) {
				t.Errorf("stripStartCode(%x) = %x, want %x", tc.input, got, tc.want)
			}
		})
	}
}
