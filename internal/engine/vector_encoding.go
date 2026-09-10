package engine

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

const (
	// EncodingSQ8 is the default mmap layout: int8 coords plus scale and
	// reconstructed-norm floats. This is the density path (USP 3).
	EncodingSQ8 = "sq8"
	// EncodingFloat32 stores little-endian float32 coords plus an L2 norm.
	// Opt-in per tenant; do not treat this as the density path.
	EncodingFloat32 = "float32"
)

// NormalizeVectorEncoding accepts sq8 (default) or float32.
func NormalizeVectorEncoding(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", EncodingSQ8, "int8":
		return EncodingSQ8, nil
	case EncodingFloat32, "fp32":
		return EncodingFloat32, nil
	default:
		return "", fmt.Errorf("vector encoding %q is not supported; use sq8 or float32", value)
	}
}

// VectorRowSize is the on-disk stride of one mmap row.
func VectorRowSize(dim int, encoding string) int {
	if encoding == EncodingFloat32 {
		return dim*4 + 4
	}
	return dim + 8
}

func (idx *MMapVectorIndex) rowBytes() int {
	return VectorRowSize(idx.dim, idx.encoding)
}

func (s *VectorStore) rowBytes(dim int) int {
	encoding := EncodingSQ8
	if s != nil && s.encoding != "" {
		encoding = s.encoding
	}
	return VectorRowSize(dim, encoding)
}

func (idx *MMapVectorIndex) writeRow(row int, vec []float32) {
	off := row * idx.rowBytes()
	dst := idx.mmap[off : off+idx.rowBytes()]
	if idx.encoding == EncodingFloat32 {
		writeFloat32Row(dst, vec)
		return
	}
	writeQuantized(dst, vec)
}

func writeFloat32Row(dst []byte, vec []float32) {
	var sum float32
	for i, v := range vec {
		binary.LittleEndian.PutUint32(dst[i*4:], math.Float32bits(v))
		sum += v * v
	}
	norm := float32(math.Sqrt(float64(sum)))
	binary.LittleEndian.PutUint32(dst[len(vec)*4:], math.Float32bits(norm))
}

func mmapRowInvEncoded(mmapSlice []byte, dim, row int, encoding string) float32 {
	if encoding == EncodingFloat32 {
		rowSize := VectorRowSize(dim, EncodingFloat32)
		off := row * rowSize
		if off < 0 || off+rowSize > len(mmapSlice) {
			return 0
		}
		norm := math.Float32frombits(binary.LittleEndian.Uint32(mmapSlice[off+dim*4 : off+dim*4+4]))
		if norm == 0 {
			return 0
		}
		return 1 / norm
	}
	return mmapRowInv(mmapSlice, dim, row)
}

func cosineF32Native(query []float32, queryNorm float32, mmapSlice []byte, dim, row int, rowInv []float32) float32 {
	inv := float32(0)
	if row >= 0 && row < len(rowInv) {
		inv = rowInv[row]
	} else {
		inv = mmapRowInvEncoded(mmapSlice, dim, row, EncodingFloat32)
	}
	if queryNorm == 0 || inv == 0 {
		return 0
	}
	rowSize := VectorRowSize(dim, EncodingFloat32)
	off := row * rowSize
	if off < 0 || off+dim*4 > len(mmapSlice) {
		return 0
	}
	var dot float32
	for i := 0; i < dim; i++ {
		dot += query[i] * math.Float32frombits(binary.LittleEndian.Uint32(mmapSlice[off+i*4:off+i*4+4]))
	}
	return (dot * inv) / queryNorm
}

func cosineF32Rows(mmapSlice []byte, dim, rowA, rowB int, rowInv []float32) float32 {
	if rowA < 0 || rowB < 0 || rowA >= len(rowInv) || rowB >= len(rowInv) {
		return 0
	}
	invA, invB := rowInv[rowA], rowInv[rowB]
	if invA == 0 || invB == 0 {
		return 0
	}
	rowSize := VectorRowSize(dim, EncodingFloat32)
	offA := rowA * rowSize
	offB := rowB * rowSize
	if offA < 0 || offB < 0 || offA+dim*4 > len(mmapSlice) || offB+dim*4 > len(mmapSlice) {
		return 0
	}
	var dot float32
	for i := 0; i < dim; i++ {
		a := math.Float32frombits(binary.LittleEndian.Uint32(mmapSlice[offA+i*4 : offA+i*4+4]))
		b := math.Float32frombits(binary.LittleEndian.Uint32(mmapSlice[offB+i*4 : offB+i*4+4]))
		dot += a * b
	}
	return dot * invA * invB
}
