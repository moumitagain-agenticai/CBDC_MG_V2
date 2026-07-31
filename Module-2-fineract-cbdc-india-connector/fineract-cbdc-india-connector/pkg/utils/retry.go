package utils

import (
	"math/big"
	"strings"
)

// IsPositiveAmount reports whether s is a well-formed positive decimal amount.
// Money is handled as a string / big.Rat to avoid float rounding errors.
func IsPositiveAmount(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return false
	}
	return r.Sign() > 0
}
