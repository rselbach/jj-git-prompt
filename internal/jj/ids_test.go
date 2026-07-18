package jj

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReverseHex(t *testing.T) {
	tests := map[string]struct {
		data []byte
		want string
	}{
		"empty":      {data: nil, want: ""},
		"all digits": {data: []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}, want: "zyxwvutsrqponmlk"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(tc.want, EncodeReverseHex(tc.data))
			decoded, ok := DecodeReverseHex(tc.want)
			r.True(ok)
			if len(tc.data) == 0 {
				r.Empty(decoded)
				return
			}
			r.Equal(tc.data, decoded)
		})
	}
}

func TestDecodeReverseHexInvalid(t *testing.T) {
	r := require.New(t)
	for _, input := range []string{"z", "jj", "0a"} {
		_, ok := DecodeReverseHex(input)
		r.False(ok, "input %q", input)
	}
}

func TestCommonHexLen(t *testing.T) {
	tests := map[string]struct {
		a, b []byte
		want int
	}{
		"identical":          {a: []byte{0xab, 0xcd}, b: []byte{0xab, 0xcd}, want: 4},
		"differ high nibble": {a: []byte{0xab}, b: []byte{0x1b}, want: 0},
		"differ low nibble":  {a: []byte{0xab}, b: []byte{0xac}, want: 1},
		"second byte":        {a: []byte{0xab, 0xcd}, b: []byte{0xab, 0xce}, want: 3},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.New(t).Equal(tc.want, commonHexLen(tc.a, tc.b))
		})
	}
}

func TestSyntheticChangeID(t *testing.T) {
	r := require.New(t)
	id := CommitID(make([]byte, 20))
	r.Equal(ChangeID(make([]byte, 16)), syntheticChangeID(id))

	// Mirrors the Rust algorithm: last 16 bytes reversed, bits reversed.
	commit := make([]byte, 20)
	commit[19] = 0x01
	want := make([]byte, 16)
	want[0] = 0x80
	r.Equal(ChangeID(want), syntheticChangeID(CommitID(commit)))
}
