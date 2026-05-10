package services

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
)

// ExtractInfoHash computes the SHA-1 info-hash of a .torrent file.
//
// A .torrent file is a bencoded dictionary whose top-level "info" key
// maps to another bencoded dictionary.  The info-hash is:
//
//	hex( SHA1( bencode(info_dict) ) )
//
// We implement just enough bencode parsing to locate and bound the raw
// bytes of the "info" value; the SHA-1 is computed over those raw bytes
// exactly as qBittorrent / libtorrent would.
//
// No third-party library is required.
func ExtractInfoHash(torrentData []byte) (string, error) {
	// The top-level value must be a dict (starts with 'd').
	if len(torrentData) == 0 || torrentData[0] != 'd' {
		return "", fmt.Errorf("not a bencoded dict")
	}

	pos := 1 // skip the opening 'd'
	for pos < len(torrentData) && torrentData[pos] != 'e' {
		// Read the key (always a bencoded string).
		keyStart := pos
		keyEnd, err := bencodeEnd(torrentData, pos)
		if err != nil {
			return "", fmt.Errorf("bad key at %d: %w", keyStart, err)
		}
		key, err := bencodeString(torrentData, pos)
		if err != nil {
			return "", fmt.Errorf("key not a string at %d: %w", keyStart, err)
		}
		pos = keyEnd

		// Read the value.
		valStart := pos
		valEnd, err := bencodeEnd(torrentData, pos)
		if err != nil {
			return "", fmt.Errorf("bad value at %d: %w", valStart, err)
		}

		if key == "info" {
			raw := torrentData[valStart:valEnd]
			h := sha1.Sum(raw)
			return hex.EncodeToString(h[:]), nil
		}

		pos = valEnd
	}

	return "", fmt.Errorf("no 'info' key found in torrent dict")
}

// bencodeEnd returns the index of the byte immediately after the bencoded
// value that starts at pos.
func bencodeEnd(data []byte, pos int) (int, error) {
	if pos >= len(data) {
		return 0, fmt.Errorf("unexpected end of data at %d", pos)
	}
	b := data[pos]
	switch {
	case b == 'i': // integer  iNNNe
		end := bytes.IndexByte(data[pos:], 'e')
		if end < 0 {
			return 0, fmt.Errorf("unterminated integer at %d", pos)
		}
		return pos + end + 1, nil

	case b == 'l': // list  l<item>...e
		pos++
		for pos < len(data) && data[pos] != 'e' {
			var err error
			pos, err = bencodeEnd(data, pos)
			if err != nil {
				return 0, err
			}
		}
		if pos >= len(data) {
			return 0, fmt.Errorf("unterminated list")
		}
		return pos + 1, nil // skip 'e'

	case b == 'd': // dict  d<key><value>...e
		pos++
		for pos < len(data) && data[pos] != 'e' {
			var err error
			// key
			pos, err = bencodeEnd(data, pos)
			if err != nil {
				return 0, err
			}
			// value
			pos, err = bencodeEnd(data, pos)
			if err != nil {
				return 0, err
			}
		}
		if pos >= len(data) {
			return 0, fmt.Errorf("unterminated dict")
		}
		return pos + 1, nil // skip 'e'

	case b >= '0' && b <= '9': // string  N:...
		return bencodeStringEnd(data, pos)

	default:
		return 0, fmt.Errorf("unknown bencode type 0x%02x at %d", b, pos)
	}
}

// bencodeStringEnd returns the index after the bencoded string starting at pos.
func bencodeStringEnd(data []byte, pos int) (int, error) {
	colon := bytes.IndexByte(data[pos:], ':')
	if colon < 0 {
		return 0, fmt.Errorf("no colon in string length at %d", pos)
	}
	n := 0
	for _, c := range data[pos : pos+colon] {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid length digit '%c' at %d", c, pos)
		}
		n = n*10 + int(c-'0')
	}
	end := pos + colon + 1 + n
	if end > len(data) {
		return 0, fmt.Errorf("string length %d exceeds data at %d", n, pos)
	}
	return end, nil
}

// bencodeString returns the string value of the bencoded string starting at pos.
func bencodeString(data []byte, pos int) (string, error) {
	if pos >= len(data) || data[pos] < '0' || data[pos] > '9' {
		return "", fmt.Errorf("not a bencoded string at %d", pos)
	}
	colon := bytes.IndexByte(data[pos:], ':')
	if colon < 0 {
		return "", fmt.Errorf("no colon at %d", pos)
	}
	n := 0
	for _, c := range data[pos : pos+colon] {
		n = n*10 + int(c-'0')
	}
	start := pos + colon + 1
	if start+n > len(data) {
		return "", fmt.Errorf("string overflow at %d", pos)
	}
	return string(data[start : start+n]), nil
}
