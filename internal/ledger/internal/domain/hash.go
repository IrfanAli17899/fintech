package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// HashOf returns a hex sha256 of parts joined with a separator, so that
// ("ab","c") and ("a","bc") hash differently.
func HashOf(parts ...string) string {
	h := sha256.New()
	h.Write([]byte(strings.Join(parts, "\x1f"))) // ASCII unit separator
	return hex.EncodeToString(h.Sum(nil))
}
