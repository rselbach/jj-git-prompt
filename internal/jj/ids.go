// Package jj implements a read-only subset of jj-lib: enough to load a
// repo's view at the current operation, read commits from the Git backend,
// and query the default index for change-id lookups.
//
// On-disk formats match jj 0.42. See FORMATS.md at the repo root.
package jj

import "encoding/hex"

// CommitID is a raw (binary, not hex) commit id. Stored in a string so it
// can be used as a map key.
type CommitID string

// ChangeID is a raw (binary) change id, 16 bytes for the Git backend.
type ChangeID string

const changeIDLength = 16

// Hex returns the id in forward hex form as used for commit ids.
func (id CommitID) Hex() string { return hex.EncodeToString([]byte(id)) }

// rootCommitID returns the virtual root commit id (all zeros) for the given
// commit id length.
func rootCommitID(idLen int) CommitID {
	return CommitID(make([]byte, idLen))
}

// IsRoot reports whether the id is the virtual root commit id.
func (id CommitID) IsRoot() bool {
	for i := 0; i < len(id); i++ {
		if id[i] != 0 {
			return false
		}
	}
	return true
}

const reverseHexChars = "zyxwvutsrqponmlk"

// EncodeReverseHex encodes data using jj's z-k "digits", the presentation
// form of change ids.
func EncodeReverseHex(data []byte) string {
	out := make([]byte, 0, len(data)*2)
	for _, b := range data {
		out = append(out, reverseHexChars[b>>4], reverseHexChars[b&0xf])
	}
	return string(out)
}

// DecodeReverseHex decodes a full z-k reverse hex string. Returns false if
// the input has odd length or invalid digits.
func DecodeReverseHex(s string) ([]byte, bool) {
	if len(s)%2 != 0 {
		return nil, false
	}
	out := make([]byte, 0, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi, ok1 := reverseHexValue(s[i])
		lo, ok2 := reverseHexValue(s[i+1])
		if !ok1 || !ok2 {
			return nil, false
		}
		out = append(out, hi<<4|lo)
	}
	return out, true
}

func reverseHexValue(b byte) (byte, bool) {
	switch {
	case b >= 'k' && b <= 'z':
		return 'z' - b, true
	case b >= 'K' && b <= 'Z':
		return 'Z' - b, true
	default:
		return 0, false
	}
}

// commonHexLen returns the length of the common prefix of a and b counted in
// hexadecimal digits.
func commonHexLen(a, b []byte) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		d := a[i] ^ b[i]
		switch {
		case d == 0:
			continue
		case d&0xf0 == 0:
			return i*2 + 1
		default:
			return i * 2
		}
	}
	return n * 2
}
