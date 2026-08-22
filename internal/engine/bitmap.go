package engine

import (
	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
)

// BitmapStore provides bitmap operations.
type BitmapStore struct{ kv *KVStore }

func NewBitmapStore(kv *KVStore) *BitmapStore { return &BitmapStore{kv: kv} }

func (b *BitmapStore) getBitmap(key string) ([]byte, func(), error) {
	e, unlock := b.kv.GetForWrite(key)
	if e == nil {
		return nil, unlock, nil
	}
	if e.Type != protocol.TypeBitmap && e.Type != protocol.TypeString {
		unlock()
		return nil, func() {}, util.ErrWrongType
	}
	v, ok := e.Value.([]byte)
	if !ok {
		unlock()
		return nil, func() {}, util.ErrWrongType
	}
	return v, unlock, nil
}

func (b *BitmapStore) ensureSize(buf []byte, bit int) []byte {
	byteIdx := bit / 8
	for byteIdx >= len(buf) {
		buf = append(buf, 0)
	}
	return buf
}

// SetBit sets bit at offset to value. Returns old bit value.
func (b *BitmapStore) SetBit(key string, offset int, value int) (int, error) {
	buf, unlock, err := b.getBitmap(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	buf = b.ensureSize(buf, offset)
	byteIdx := offset / 8
	bitIdx := 7 - (offset % 8)
	oldBit := int((buf[byteIdx] >> bitIdx) & 1)
	if value == 1 {
		buf[byteIdx] |= 1 << bitIdx
	} else {
		buf[byteIdx] &^= 1 << bitIdx
	}
	sh := b.kv.shard(key)
	if sh.data[key] == nil {
		sh.data[key] = &Entry{Value: buf, Type: protocol.TypeBitmap}
	} else {
		sh.data[key].Value = buf
	}
	return oldBit, nil
}

// GetBit returns bit at offset.
func (b *BitmapStore) GetBit(key string, offset int) (int, error) {
	e := b.kv.Get(key)
	if e == nil {
		return 0, nil
	}
	if e.Type != protocol.TypeBitmap && e.Type != protocol.TypeString {
		return 0, util.ErrWrongType
	}
	buf := e.Value.([]byte)
	byteIdx := offset / 8
	if byteIdx >= len(buf) {
		return 0, nil
	}
	bitIdx := 7 - (offset % 8)
	return int((buf[byteIdx] >> bitIdx) & 1), nil
}

// BitCount counts set bits in the byte range [start, end].
func (b *BitmapStore) BitCount(key string, start, end int, rangeSet bool) (int, error) {
	e := b.kv.Get(key)
	if e == nil {
		return 0, nil
	}
	if e.Type != protocol.TypeBitmap && e.Type != protocol.TypeString {
		return 0, util.ErrWrongType
	}
	buf := e.Value.([]byte)
	if !rangeSet {
		start = 0
		end = len(buf) - 1
	}
	if start < 0 {
		start = len(buf) + start
	}
	if end < 0 {
		end = len(buf) + end
	}
	if start < 0 {
		start = 0
	}
	if end >= len(buf) {
		end = len(buf) - 1
	}
	count := 0
	for i := start; i <= end; i++ {
		count += popCount(buf[i])
	}
	return count, nil
}

func popCount(b byte) int {
	count := 0
	for b != 0 {
		count += int(b & 1)
		b >>= 1
	}
	return count
}

// BitPos returns the position of the first bit set to the given value (0 or 1).
func (b *BitmapStore) BitPos(key string, bitVal int) (int, error) {
	e := b.kv.Get(key)
	if e == nil {
		if bitVal == 0 {
			return 0, nil
		}
		return -1, nil
	}
	if e.Type != protocol.TypeBitmap && e.Type != protocol.TypeString {
		return -1, util.ErrWrongType
	}
	buf := e.Value.([]byte)
	for byteIdx, byt := range buf {
		for bitIdx := 7; bitIdx >= 0; bitIdx-- {
			bit := int((byt >> bitIdx) & 1)
			if bit == bitVal {
				return byteIdx*8 + (7 - bitIdx), nil
			}
		}
	}
	if bitVal == 0 {
		return len(buf) * 8, nil
	}
	return -1, nil
}
