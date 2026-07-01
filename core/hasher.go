package core

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

const hashBufSize = 4 << 20 // 4 MB

// HashReader computes the SHA-256 hash of a stream's contents.
func HashReader(r io.Reader) (string, error) {
	h := sha256.New()
	buf := make([]byte, hashBufSize)
	if _, err := io.CopyBuffer(h, r, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
