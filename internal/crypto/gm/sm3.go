//go:build gm

package gm

import (
	"hash"

	"github.com/emmansun/gmsm/sm3"
)

// SM3Hash computes the SM3 hash of data and returns the 32-byte digest.
func SM3Hash(data []byte) []byte {
	h := sm3.New()
	h.Write(data)
	return h.Sum(nil)
}

// SM3New returns a new hash.Hash computing the SM3 checksum.
func SM3New() hash.Hash {
	return sm3.New()
}
