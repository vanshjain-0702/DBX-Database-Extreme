package engine

import "fmt"

const (
	maxSpaceNameBytes = 64
	vectorSpaceMark   = "\x00spc:"
	vectorSpaceSep    = "\x1f"
)

// ValidateSpaceName accepts a caller-chosen modality/space label.
func ValidateSpaceName(name string) error {
	if name == "" {
		return fmt.Errorf("space name must not be empty")
	}
	if len(name) > maxSpaceNameBytes {
		return fmt.Errorf("space name must be at most %d bytes", maxSpaceNameBytes)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.'
		if !ok {
			return fmt.Errorf("space name may contain letters, digits, '_', '-', and '.' only")
		}
	}
	return nil
}

// VectorSpaceKey maps an index plus optional SPACE onto a distinct mmap/HNSW
// key. The default space (empty name) is the index itself, so existing
// VADD/VSEARCH keys are unchanged.
func VectorSpaceKey(index, space string) string {
	if space == "" {
		return index
	}
	return vectorSpaceMark + index + vectorSpaceSep + space
}
