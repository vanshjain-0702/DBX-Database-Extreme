// Package persistence provides WAL, snapshots, recovery, compaction, and backup.
package persistence

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DecodeRecord decodes one length-framed replication payload.
func DecodeRecord(data []byte) (*WALRecord, error) {
	records, err := decodeRecords(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if len(records) != 1 {
		return nil, fmt.Errorf("expected one WAL record, got %d", len(records))
	}
	return records[0], nil
}

// WALRecord types.
const (
	RecordSet       = byte(1)
	RecordDelete    = byte(2)
	RecordExpire    = byte(3)
	RecordVAdd      = byte(4)
	RecordVAddBatch = byte(5)
)

// WALRecord is a single write-ahead log entry.
type WALRecord struct {
	Type      byte
	Key       string
	Value     []byte
	TTLNano   int64
	Timestamp int64
	Sequence  uint64
	done      chan struct{}
	err       error
}

// WAL is a write-ahead log for durability.
type WAL struct {
	mu       sync.Mutex
	file     *os.File
	writer   *bufio.Writer
	dir      string
	syncMode string // "always", "everysec", "no"
	seq      uint64
	lastSync time.Time
	size     int64
	maxSize  int64

	recordCh chan *WALRecord
	doneCh   chan struct{}
	subsMu   sync.RWMutex
	subs     []func(*WALRecord)
}

// Subscribe registers a callback invoked after a record is durably flushed.
func (w *WAL) Subscribe(callback func(*WALRecord)) {
	if callback == nil {
		return
	}
	w.subsMu.Lock()
	w.subs = append(w.subs, callback)
	w.subsMu.Unlock()
}

// OpenWAL opens or creates a WAL in dir.
func OpenWAL(dir, syncMode string, maxSizeMB int) (*WAL, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("wal: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "wal.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("wal: open %s: %w", path, err)
	}
	info, _ := f.Stat()
	w := &WAL{
		file:     f,
		writer:   bufio.NewWriterSize(f, 64*1024),
		dir:      dir,
		syncMode: syncMode,
		size:     info.Size(),
		maxSize:  int64(maxSizeMB) * 1024 * 1024,
		lastSync: time.Now(),
		recordCh: make(chan *WALRecord, 10000), // Bounded buffer for backpressure
		doneCh:   make(chan struct{}),
	}
	go w.flusher()
	return w, nil
}

// Write queues a record to the WAL asynchronously.
func (w *WAL) Write(rec *WALRecord) error {
	rec.done = make(chan struct{})
	w.mu.Lock()
	w.seq++
	rec.Sequence = w.seq
	rec.Timestamp = time.Now().UnixNano()
	w.mu.Unlock()

	// Push to background flusher; blocks if buffer is completely full (backpressure)
	w.recordCh <- rec

	// Do not acknowledge until the record is flushed and synced.
	<-rec.done
	return rec.err
}

func (w *WAL) flusher() {
	ticker := time.NewTicker(time.Millisecond * 50)
	defer ticker.Stop()

	var pending []*WALRecord

	syncAndNotify := func() {
		if len(pending) == 0 {
			return
		}
		w.mu.Lock()
		err := w.writer.Flush()
		if err == nil {
			err = w.file.Sync()
		}
		w.lastSync = time.Now()
		w.mu.Unlock()
		for _, rec := range pending {
			if rec != nil {
				rec.err = err
				close(rec.done)
				if err == nil {
					w.subsMu.RLock()
					subs := append([]func(*WALRecord){}, w.subs...)
					w.subsMu.RUnlock()
					for _, callback := range subs {
						callbackCopy := callback
						recordCopy := *rec
						recordCopy.Value = append([]byte(nil), rec.Value...)
						go callbackCopy(&recordCopy)
					}
				}
			}
		}
		pending = pending[:0]
	}

	for {
		select {
		case rec, ok := <-w.recordCh:
			if !ok {
				syncAndNotify()
				w.Sync()
				close(w.doneCh)
				return
			}
			data := w.encodeRecord(rec)
			w.mu.Lock()
			n, err := w.writer.Write(data)
			w.size += int64(n)
			w.mu.Unlock()
			if err != nil {
				rec.err = err
				close(rec.done)
				continue
			}

			pending = append(pending, rec)

			if w.syncMode == "always" || len(pending) > 1000 {
				syncAndNotify()
			}
		case <-ticker.C:
			if len(pending) > 0 {
				syncAndNotify()
			} else if w.syncMode == "everysec" {
				w.mu.Lock()
				if time.Since(w.lastSync) >= time.Second {
					w.writer.Flush()
					w.file.Sync()
					w.lastSync = time.Now()
				}
				w.mu.Unlock()
			}
		}
	}
}

func (w *WAL) encodeRecord(rec *WALRecord) []byte {
	keyBytes := []byte(rec.Key)
	// Format: [type(1)][seq(8)][ts(8)][keyLen(4)][key][valLen(4)][val][ttl(8)][crc(4)]
	total := 1 + 8 + 8 + 4 + len(keyBytes) + 4 + len(rec.Value) + 8 + 4
	buf := make([]byte, total)
	off := 0
	buf[off] = rec.Type
	off++
	binary.BigEndian.PutUint64(buf[off:], rec.Sequence)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], uint64(rec.Timestamp))
	off += 8
	binary.BigEndian.PutUint32(buf[off:], uint32(len(keyBytes)))
	off += 4
	copy(buf[off:], keyBytes)
	off += len(keyBytes)
	binary.BigEndian.PutUint32(buf[off:], uint32(len(rec.Value)))
	off += 4
	copy(buf[off:], rec.Value)
	off += len(rec.Value)
	binary.BigEndian.PutUint64(buf[off:], uint64(rec.TTLNano))
	off += 8

	crc := crc32.ChecksumIEEE(buf[:off])
	binary.BigEndian.PutUint32(buf[off:], crc)
	return buf
}

// EncodeRecord serializes one WAL record for replication transport.
func EncodeRecord(rec *WALRecord) []byte {
	return (&WAL{}).encodeRecord(rec)
}

// maybeSync is obsolete with background flusher, but kept for legacy calls if any.
func (w *WAL) maybeSync() error {
	return nil
}

// Sync forces a sync to disk.
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writer.Flush()
	return w.file.Sync()
}

// ReadAll reads all WAL records (for recovery).
func (w *WAL) ReadAll() ([]*WALRecord, error) {
	path := filepath.Join(w.dir, "wal.log")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	return decodeRecords(f)
}

func decodeRecords(r io.Reader) ([]*WALRecord, error) {
	var records []*WALRecord
	for {
		header := make([]byte, 1+8+8+4) // type + seq + ts + keyLen
		if _, err := io.ReadFull(r, header); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			fmt.Printf("WAL Recovery: Warning: Stopping at sequence %d due to partial read: %v\n", len(records)+1, err)
			break
		}

		recType := header[0]
		seq := binary.BigEndian.Uint64(header[1:9])
		ts := binary.BigEndian.Uint64(header[9:17])
		keyLen := binary.BigEndian.Uint32(header[17:21])

		keyBuf := make([]byte, keyLen)
		if _, err := io.ReadFull(r, keyBuf); err != nil {
			fmt.Printf("WAL Recovery: Warning: Stopping at sequence %d due to incomplete key: %v\n", seq, err)
			break
		}

		valHeader := make([]byte, 4)
		if _, err := io.ReadFull(r, valHeader); err != nil {
			fmt.Printf("WAL Recovery: Warning: Stopping at sequence %d due to incomplete value header: %v\n", seq, err)
			break
		}
		valLen := binary.BigEndian.Uint32(valHeader)

		valBuf := make([]byte, valLen)
		if _, err := io.ReadFull(r, valBuf); err != nil {
			fmt.Printf("WAL Recovery: Warning: Stopping at sequence %d due to incomplete value: %v\n", seq, err)
			break
		}

		footer := make([]byte, 8+4) // ttl + crc
		if _, err := io.ReadFull(r, footer); err != nil {
			fmt.Printf("WAL Recovery: Warning: Stopping at sequence %d due to incomplete footer: %v\n", seq, err)
			break
		}
		ttl := binary.BigEndian.Uint64(footer[:8])
		expectedCrc := binary.BigEndian.Uint32(footer[8:])

		// Reconstruct buffer to verify CRC
		crcBuf := append(header, keyBuf...)
		crcBuf = append(crcBuf, valHeader...)
		crcBuf = append(crcBuf, valBuf...)
		crcBuf = append(crcBuf, footer[:8]...)

		actualCrc := crc32.ChecksumIEEE(crcBuf)
		if actualCrc != expectedCrc {
			fmt.Printf("WAL Recovery: Warning: CRC mismatch at sequence %d (expected %x, got %x). Tail is corrupted, halting replay.\n", seq, expectedCrc, actualCrc)
			break
		}

		records = append(records, &WALRecord{
			Type:      recType,
			Key:       string(keyBuf),
			Value:     valBuf,
			TTLNano:   int64(ttl),
			Timestamp: int64(ts),
			Sequence:  seq,
		})
	}
	return records, nil
}

// Rotate rotates the WAL file (used after snapshot).
func (w *WAL) Rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writer.Flush()
	w.file.Close()
	old := filepath.Join(w.dir, "wal.log")
	archived := filepath.Join(w.dir, fmt.Sprintf("wal-%d.log", time.Now().UnixNano()))
	os.Rename(old, archived)
	f, err := os.OpenFile(old, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	w.file = f
	w.writer = bufio.NewWriterSize(f, 64*1024)
	w.size = 0
	return nil
}

// NeedsRotation returns true if the WAL has exceeded max size.
func (w *WAL) NeedsRotation() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.maxSize > 0 && w.size >= w.maxSize
}

// Close flushes and closes the WAL.
func (w *WAL) Close() error {
	close(w.recordCh)
	<-w.doneCh // Wait for flusher to finish and sync
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// EncodeVAddPayload serializes a docID and vector into a byte slice for the WAL.
func EncodeVAddPayload(docID string, vec []float32) []byte {
	docBytes := []byte(docID)
	docLen := len(docBytes)
	buf := make([]byte, 4+docLen+4*len(vec))
	binary.BigEndian.PutUint32(buf[0:], uint32(docLen))
	copy(buf[4:], docBytes)
	off := 4 + docLen
	for _, v := range vec {
		binary.BigEndian.PutUint32(buf[off:], math.Float32bits(v))
		off += 4
	}
	return buf
}

// DecodeVAddPayload deserializes a docID and vector from a WAL payload.
func DecodeVAddPayload(data []byte) (string, []float32) {
	if len(data) < 4 {
		return "", nil
	}
	docLen := binary.BigEndian.Uint32(data[0:4])
	if len(data) < 4+int(docLen) {
		return "", nil
	}
	docID := string(data[4 : 4+docLen])
	off := 4 + int(docLen)
	vecLen := (len(data) - off) / 4
	vec := make([]float32, vecLen)
	for i := 0; i < vecLen; i++ {
		vec[i] = math.Float32frombits(binary.BigEndian.Uint32(data[off:]))
		off += 4
	}
	return docID, vec
}

// EncodeVAddBatchPayload serializes a batch of vectors into a byte slice.
func EncodeVAddBatchPayload(dim int, ids []string, vecs [][]float32) []byte {
	numVectors := len(ids)
	// Calculate size:
	// dim (4) + numVectors (4)
	// for each vector: id_len (4) + id_bytes + dim * 4 bytes
	size := 8
	for _, id := range ids {
		size += 4 + len(id) + (dim * 4)
	}

	buf := make([]byte, size)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(dim))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(numVectors))

	offset := 8
	for i := 0; i < numVectors; i++ {
		id := ids[i]
		vec := vecs[i]

		binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(len(id)))
		offset += 4
		copy(buf[offset:offset+len(id)], []byte(id))
		offset += len(id)

		for j := 0; j < dim; j++ {
			binary.LittleEndian.PutUint32(buf[offset:offset+4], math.Float32bits(vec[j]))
			offset += 4
		}
	}
	return buf
}

// DecodeVAddBatchPayload deserializes a batch of vectors.
func DecodeVAddBatchPayload(data []byte) (int, []string, [][]float32) {
	if len(data) < 8 {
		return 0, nil, nil
	}
	dim := int(binary.LittleEndian.Uint32(data[0:4]))
	numVectors := int(binary.LittleEndian.Uint32(data[4:8]))

	ids := make([]string, numVectors)
	vecs := make([][]float32, numVectors)

	offset := 8
	for i := 0; i < numVectors; i++ {
		idLen := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += 4

		ids[i] = string(data[offset : offset+idLen])
		offset += idLen

		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			vec[j] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
			offset += 4
		}
		vecs[i] = vec
	}
	return dim, ids, vecs
}
