package config

import (
	"fmt"
	"strings"
)

const (
	VectorEncodingSQ8     = "sq8"
	VectorEncodingFloat32 = "float32"
)

// NormalizeVectorEncoding accepts sq8 (default) or float32.
func NormalizeVectorEncoding(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", VectorEncodingSQ8, "int8":
		return VectorEncodingSQ8, nil
	case VectorEncodingFloat32, "fp32":
		return VectorEncodingFloat32, nil
	default:
		return "", fmt.Errorf("vector encoding %q is not supported; use sq8 or float32", value)
	}
}
