package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseBytes parses a positive memory size such as 512mb or 2GiB.
func ParseBytes(value string) (int64, error) {
	text := strings.ToLower(strings.TrimSpace(value))
	multipliers := []struct {
		suffix string
		value  int64
	}{
		{"gib", 1 << 30}, {"gb", 1_000_000_000},
		{"mib", 1 << 20}, {"mb", 1_000_000},
		{"kib", 1 << 10}, {"kb", 1_000},
		{"b", 1},
	}
	for _, unit := range multipliers {
		if !strings.HasSuffix(text, unit.suffix) {
			continue
		}
		number := strings.TrimSpace(strings.TrimSuffix(text, unit.suffix))
		n, err := strconv.ParseInt(number, 10, 64)
		if err != nil || n <= 0 || n > (1<<63-1)/unit.value {
			return 0, fmt.Errorf("invalid memory size %q", value)
		}
		return n * unit.value, nil
	}
	return 0, fmt.Errorf("invalid memory size %q: unit is required", value)
}
