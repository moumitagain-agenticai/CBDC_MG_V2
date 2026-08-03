package utils

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"math/big"
	"strings"
)

// IsPositiveAmount reports whether s parses as a decimal strictly greater than 0.
func IsPositiveAmount(s string) bool {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	return ok && r.Sign() > 0
}

// IsNonNegativeAmount reports whether s parses as a decimal >= 0.
func IsNonNegativeAmount(s string) bool {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	return ok && r.Sign() >= 0
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_amount.log at runtime.
var _ = func() bool { flog.For("10_amount").Info("source file initialized"); return true }()
