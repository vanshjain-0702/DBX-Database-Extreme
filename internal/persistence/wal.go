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
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dbx/dbx/internal/security"
)

// DecodeRecord decodes one length-framed replication payload.
func DecodeRecord(data []byte) (*WALRecord, error) {
	records, err := decodeFrames(bytes.NewReader(data), false, nil)
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
	RecordSet         = byte(1)
	RecordDelete      = byte(2)
	RecordExpire      = byte(3)
	RecordVAdd        = byte(4)
	RecordVAddBatch   = byte(5)
	RecordVTombstone  = byte(6)
	RecordDeleteIndex = byte(7)
)

var walV2Magic = []byte("DBXWAL2\n")
var walEncMagic = []byte("DBXWALe2")

const maxWALFrameSize = 64 << 20

// WALEffect is an idempotent final state installed by a transaction.
// ExpiresAt is an absolute Unix-nanosecond deadline; zero means no expiry.
type WALEffect struct {
	Type      byte
	Key       string
	Value     []byte
	ExpiresAt int64
}

// WALRecord is a single write-ahead log entry.
type WALRecord struct {
	Type      byte
	Key       string
	Value     []byte
	TTLNano   int64
	Timestamp int64
	Sequence  uint64
	Effects   []WALEffect
	done      chan struct{}
	err       error
}

// WAL is a write-ahead log for durability.
type WAL struct {
	mu        sync.Mutex
	file      *os.File
	writer    *bufio.Writer
	dir       string
	syncMode  string // "always", "everysec", "no"
	seq       atomic.Uint64
	lastSync  time.Time
	size      int64
	maxSize   int64
	failedErr error

	recordCh chan *WALRecord
	doneCh   chan struct{}
	subsMu   sync.RWMutex
	subs     []func(*WALRecord)
	enc      *security.Encryptor
}

// Subscribe registers a callback invoked after a record is accepted by the WAL.
// Callbacks run on the flusher goroutine and must not block: the primary write
// path never waits on replica TCP.
func (w *WAL) Subscribe(callback func(*WALRecord)) {
	if callback == nil {
		return
	}
	w.subsMu.Lock()
	w.subs = append(w.subs, callback)
	w.subsMu.Unlock()
}

func (w *WAL) notifySubs(rec *WALRecord) {
	if rec == nil {
		return
	}
	w.subsMu.RLock()
	if len(w.subs) == 0 {
		w.subsMu.RUnlock()
		return
	}
	subs := append([]func(*WALRecord){}, w.subs...)
	w.subsMu.RUnlock()
	recordCopy := *rec
	recordCopy.Value = append([]byte(nil), rec.Value...)
	if len(rec.Effects) > 0 {
		recordCopy.Effects = make([]WALEffect, len(rec.Effects))
		for i, effect := range rec.Effects {
			recordCopy.Effects[i] = effect
			recordCopy.Effects[i].Value = append([]byte(nil), effect.Value...)
		}
	}
	for _, callback := range subs {
		callback(&recordCopy)
	}
}

// OpenWAL opens or creates a WAL in dir.
func OpenWAL(dir, syncMode string, maxSizeMB int) (*WAL, error) {
	return OpenWALEncrypted(dir, syncMode, maxSizeMB, nil)
}

// OpenWALEncrypted opens a WAL and, when enc is set, stores frames as AES-256-GCM.
func OpenWALEncrypted(dir, syncMode string, maxSizeMB int, enc *security.Encryptor) (*WAL, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("wal: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "wal.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("wal: open %s: %w", path, err)
	}
	info, _ := f.Stat()
	header := walHeader(enc)
	if info.Size() == 0 {
		if _, err := f.Write(header); err != nil {
			f.Close()
			return nil, fmt.Errorf("wal: write v2 header: %w", err)
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, fmt.Errorf("wal: sync v2 header: %w", err)
		}
		info, _ = f.Stat()
	} else {
		if err := repairWALTail(f, enc); err != nil {
			f.Close()
			return nil, fmt.Errorf("wal: open v2 log: %w (legacy WALs require offline reset/migration)", err)
		}
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			f.Close()
			return nil, err
		}
	}
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
		enc:      enc,
	}
	records, readErr := w.ReadAll()
	if readErr != nil {
		f.Close()
		return nil, fmt.Errorf("wal: validate segments: %w", readErr)
	}
	for _, rec := range records {
		if rec.Sequence > w.seq.Load() {
			w.seq.Store(rec.Sequence)
		}
	}
	go w.flusher()
	return w, nil
}

func walHeader(enc *security.Encryptor) []byte {
	if enc != nil {
		return walEncMagic
	}
	return walV2Magic
}

func repairWALTail(f *os.File, enc *security.Encryptor) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	header := make([]byte, len(walV2Magic))
	if _, err := io.ReadFull(f, header); err != nil {
		return fmt.Errorf("unsupported WAL format")
	}
	if enc != nil {
		if !bytes.Equal(header, walEncMagic) {
			return fmt.Errorf("unsupported WAL format")
		}
	} else if !bytes.Equal(header, walV2Magic) {
		return fmt.Errorf("unsupported WAL format")
	}
	validOffset := int64(len(header))
	for {
		var length [4]byte
		if _, err := io.ReadFull(f, length[:]); err != nil {
			if err == io.EOF {
				return nil
			}
			if err == io.ErrUnexpectedEOF {
				return f.Truncate(validOffset)
			}
			return err
		}
		frameLen := int(binary.BigEndian.Uint32(length[:]))
		minLen := 25
		if enc != nil {
			minLen = 12 + 16
		}
		if frameLen < minLen || frameLen > maxWALFrameSize {
			return fmt.Errorf("invalid WAL frame length %d", frameLen)
		}
		frame := make([]byte, frameLen)
		if _, err := io.ReadFull(f, frame); err != nil {
			if err == io.ErrUnexpectedEOF {
				return f.Truncate(validOffset)
			}
			return err
		}
		encoded := append(length[:], frame...)
		if _, err := decodeFrames(bytes.NewReader(encoded), false, enc); err != nil {
			return err
		}
		validOffset += int64(4 + frameLen)
	}
}

func ackWAL(rec *WALRecord, err error) {
	if rec == nil || rec.done == nil {
		return
	}
	rec.err = err
	close(rec.done)
}

// Write queues a record to the WAL.
// sync=always waits for fsync. everysec/no return after the record is queued
// (Redis-compatible: SET does not wait on the 1s fsync).
func (w *WAL) Write(rec *WALRecord) error {
	if err := w.Failure(); err != nil {
		return fmt.Errorf("wal is write-stopped: %w", err)
	}
	rec.Sequence = w.seq.Add(1)
	rec.Timestamp = time.Now().UnixNano()
	if len(rec.Effects) == 0 {
		expiresAt := rec.TTLNano
		if expiresAt > 0 && rec.Type != RecordExpire {
			expiresAt = rec.Timestamp + expiresAt
		}
		if rec.Type == RecordExpire && expiresAt > 0 && expiresAt < rec.Timestamp {
			// Compatibility for callers that still supply a relative EXPIRE.
			expiresAt = rec.Timestamp + expiresAt
		}
		rec.Effects = []WALEffect{{
			Type:      rec.Type,
			Key:       rec.Key,
			Value:     append([]byte(nil), rec.Value...),
			ExpiresAt: expiresAt,
		}}
	}

	rec.done = make(chan struct{})
	w.recordCh <- rec
	<-rec.done
	return rec.err
}

// WriteTransaction appends a set of final-state effects as one indivisible WAL
// frame. Recovery either observes every effect in the frame or none of them.
func (w *WAL) WriteTransaction(effects []WALEffect) (uint64, error) {
	if len(effects) == 0 {
		return w.seq.Load(), nil
	}
	rec := &WALRecord{Effects: make([]WALEffect, len(effects))}
	for i := range effects {
		rec.Effects[i] = effects[i]
		rec.Effects[i].Value = append([]byte(nil), effects[i].Value...)
	}
	err := w.Write(rec)
	return rec.Sequence, err
}

// Sequence returns the last sequence assigned to this WAL.
func (w *WAL) Sequence() uint64 { return w.seq.Load() }

// Dir returns the WAL segment directory.
func (w *WAL) Dir() string { return w.dir }

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
		if err != nil && w.failedErr == nil {
			w.failedErr = err
		}
		w.lastSync = time.Now()
		w.mu.Unlock()
		for _, rec := range pending {
			if rec == nil {
				continue
			}
			ackWAL(rec, err)
			if err == nil {
				w.notifySubs(rec)
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
			data, encErr := w.encodeRecord(rec)
			if encErr != nil {
				ackWAL(rec, encErr)
				continue
			}
			w.mu.Lock()
			n, err := w.writer.Write(data)
			w.size += int64(n)
			if err != nil && w.failedErr == nil {
				w.failedErr = err
			}
			w.mu.Unlock()
			if err != nil {
				ackWAL(rec, err)
				continue
			}

			if w.syncMode == "always" {
				pending = append(pending, rec)
				syncAndNotify()
			} else {
				// everysec acknowledges after the complete frame reaches the
				// process buffer; the one-second sync loop defines the stated
				// crash-loss window and latches later storage errors.
				ackWAL(rec, nil)
				w.notifySubs(rec)
			}
		case <-ticker.C:
			if w.syncMode == "always" {
				syncAndNotify()
				continue
			}
			if len(pending) == 0 {
				if w.syncMode == "everysec" {
					w.mu.Lock()
					if time.Since(w.lastSync) >= time.Second {
						err := w.writer.Flush()
						if err == nil {
							err = w.file.Sync()
						}
						if err != nil && w.failedErr == nil {
							w.failedErr = err
						}
						w.lastSync = time.Now()
					}
					w.mu.Unlock()
				}
				continue
			}
			w.mu.Lock()
			flushErr := w.writer.Flush()
			if w.syncMode != "no" && time.Since(w.lastSync) >= time.Second {
				if flushErr == nil {
					flushErr = w.file.Sync()
				}
				w.lastSync = time.Now()
			}
			if flushErr != nil && w.failedErr == nil {
				w.failedErr = flushErr
			}
			w.mu.Unlock()
			for _, rec := range pending {
				ackWAL(rec, flushErr)
			}
			pending = pending[:0]
		}
	}
}

func encodeRecordPlain(rec *WALRecord) []byte {
	effects := rec.Effects
	if len(effects) == 0 {
		effects = []WALEffect{{Type: rec.Type, Key: rec.Key, Value: rec.Value, ExpiresAt: rec.TTLNano}}
	}
	bodyLen := 1 + 8 + 8 + 4
	for _, effect := range effects {
		bodyLen += 1 + 4 + 4 + 8 + len(effect.Key) + len(effect.Value)
	}
	frameLen := bodyLen + 4
	buf := make([]byte, 4+frameLen)
	binary.BigEndian.PutUint32(buf[:4], uint32(frameLen))
	off := 4
	buf[off] = 2
	off++
	binary.BigEndian.PutUint64(buf[off:], rec.Sequence)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], uint64(rec.Timestamp))
	off += 8
	binary.BigEndian.PutUint32(buf[off:], uint32(len(effects)))
	off += 4
	for _, effect := range effects {
		buf[off] = effect.Type
		off++
		binary.BigEndian.PutUint32(buf[off:], uint32(len(effect.Key)))
		off += 4
		binary.BigEndian.PutUint32(buf[off:], uint32(len(effect.Value)))
		off += 4
		binary.BigEndian.PutUint64(buf[off:], uint64(effect.ExpiresAt))
		off += 8
		copy(buf[off:], effect.Key)
		off += len(effect.Key)
		copy(buf[off:], effect.Value)
		off += len(effect.Value)
	}
	crc := crc32.ChecksumIEEE(buf[4:off])
	binary.BigEndian.PutUint32(buf[off:], crc)
	return buf
}

func (w *WAL) encodeRecord(rec *WALRecord) ([]byte, error) {
	plain := encodeRecordPlain(rec)
	if w == nil || w.enc == nil {
		return plain, nil
	}
	sealed, err := w.enc.Encrypt(plain[4:])
	if err != nil {
		return nil, err
	}
	out := make([]byte, 4+len(sealed))
	binary.BigEndian.PutUint32(out[:4], uint32(len(sealed)))
	copy(out[4:], sealed)
	return out, nil
}

// EncodeRecord serializes one WAL record for replication transport.
func EncodeRecord(rec *WALRecord) []byte {
	return encodeRecordPlain(rec)
}

// Sync forces a sync to disk.
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.writer.Flush()
	if err == nil {
		err = w.file.Sync()
	}
	if err != nil && w.failedErr == nil {
		w.failedErr = err
	}
	return err
}

// Failure returns the latched storage failure that stopped future writes.
func (w *WAL) Failure() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.failedErr
}

// ReadAll reads all WAL records (for recovery).
func (w *WAL) ReadAll() ([]*WALRecord, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		if w.file == nil {
			return nil, err
		}
		if _, seekErr := w.file.Seek(0, io.SeekStart); seekErr != nil {
			return nil, err
		}
		records, readErr := decodeRecords(w.file, w.enc)
		_, _ = w.file.Seek(0, io.SeekEnd)
		if readErr != nil {
			return nil, readErr
		}
		return records, nil
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "wal.log" || (strings.HasPrefix(name, "wal-") && strings.HasSuffix(name, ".log")) {
			paths = append(paths, filepath.Join(w.dir, name))
		}
	}
	sort.Strings(paths)
	var all []*WALRecord
	for _, path := range paths {
		f, openErr := os.Open(path)
		if openErr != nil {
			return nil, openErr
		}
		var records []*WALRecord
		var readErr error
		if filepath.Base(path) == "wal.log" {
			records, readErr = decodeRecords(f, w.enc)
		} else {
			records, readErr = decodeRecordsMode(f, false, w.enc)
		}
		f.Close()
		if readErr != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), readErr)
		}
		all = append(all, records...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Sequence < all[j].Sequence })
	return all, nil
}

func decodeRecords(r io.Reader, enc *security.Encryptor) ([]*WALRecord, error) {
	return decodeRecordsMode(r, true, enc)
}

func decodeRecordsMode(r io.Reader, allowPartialTail bool, enc *security.Encryptor) ([]*WALRecord, error) {
	header := make([]byte, len(walV2Magic))
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("missing WAL v2 header: %w", err)
	}
	if enc != nil {
		if !bytes.Equal(header, walEncMagic) {
			return nil, fmt.Errorf("unsupported WAL format")
		}
	} else if !bytes.Equal(header, walV2Magic) {
		return nil, fmt.Errorf("unsupported WAL format")
	}
	return decodeFrames(r, allowPartialTail, enc)
}

func decodeFrames(r io.Reader, allowPartialTail bool, enc *security.Encryptor) ([]*WALRecord, error) {
	var records []*WALRecord
	for {
		var lengthBuf [4]byte
		if _, err := io.ReadFull(r, lengthBuf[:]); err != nil {
			if err == io.EOF {
				return records, nil
			}
			if err == io.ErrUnexpectedEOF && allowPartialTail {
				return records, nil
			}
			return nil, fmt.Errorf("WAL frame length: %w", err)
		}
		frameLen := int(binary.BigEndian.Uint32(lengthBuf[:]))
		minLen := 1 + 8 + 8 + 4 + 4
		if enc != nil {
			minLen = 12 + 16
		}
		if frameLen < minLen || frameLen > maxWALFrameSize {
			return nil, fmt.Errorf("invalid WAL frame length %d", frameLen)
		}
		frame := make([]byte, frameLen)
		if _, err := io.ReadFull(r, frame); err != nil {
			if err == io.ErrUnexpectedEOF && allowPartialTail {
				return records, nil
			}
			return nil, fmt.Errorf("WAL frame payload: %w", err)
		}
		if enc != nil {
			plain, err := enc.Decrypt(frame)
			if err != nil {
				return nil, fmt.Errorf("WAL decrypt: %w", err)
			}
			frame = plain
			if len(frame) < 1+8+8+4+4 {
				return nil, fmt.Errorf("invalid WAL frame length %d", len(frame))
			}
		}
		body := frame[:len(frame)-4]
		expectedCRC := binary.BigEndian.Uint32(frame[len(frame)-4:])
		if actual := crc32.ChecksumIEEE(body); actual != expectedCRC {
			return nil, fmt.Errorf("WAL CRC mismatch (expected %x, got %x)", expectedCRC, actual)
		}
		off := 0
		if body[off] != 2 {
			return nil, fmt.Errorf("unsupported WAL record version %d", body[off])
		}
		off++
		if off+20 > len(body) {
			return nil, fmt.Errorf("truncated WAL transaction header")
		}
		rec := &WALRecord{
			Sequence:  binary.BigEndian.Uint64(body[off:]),
			Timestamp: int64(binary.BigEndian.Uint64(body[off+8:])),
		}
		off += 16
		effectCount := int(binary.BigEndian.Uint32(body[off:]))
		off += 4
		if effectCount <= 0 || effectCount > 1_000_000 {
			return nil, fmt.Errorf("invalid WAL effect count %d", effectCount)
		}
		rec.Effects = make([]WALEffect, 0, effectCount)
		for i := 0; i < effectCount; i++ {
			if off+17 > len(body) {
				return nil, fmt.Errorf("truncated WAL effect header")
			}
			effectType := body[off]
			keyLen := int(binary.BigEndian.Uint32(body[off+1:]))
			valueLen := int(binary.BigEndian.Uint32(body[off+5:]))
			expiresAt := int64(binary.BigEndian.Uint64(body[off+9:]))
			off += 17
			if keyLen < 0 || valueLen < 0 || off+keyLen+valueLen > len(body) {
				return nil, fmt.Errorf("invalid WAL effect lengths")
			}
			effect := WALEffect{
				Type:      effectType,
				Key:       string(body[off : off+keyLen]),
				Value:     append([]byte(nil), body[off+keyLen:off+keyLen+valueLen]...),
				ExpiresAt: expiresAt,
			}
			off += keyLen + valueLen
			rec.Effects = append(rec.Effects, effect)
		}
		if off != len(body) {
			return nil, fmt.Errorf("WAL frame has %d trailing bytes", len(body)-off)
		}
		first := rec.Effects[0]
		rec.Type, rec.Key, rec.Value, rec.TTLNano = first.Type, first.Key, first.Value, first.ExpiresAt
		records = append(records, rec)
	}
}

// Rotate rotates the WAL file (used after snapshot).
func (w *WAL) Rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failedErr != nil {
		return fmt.Errorf("wal is write-stopped: %w", w.failedErr)
	}
	if err := w.writer.Flush(); err != nil {
		w.failedErr = err
		return err
	}
	if err := w.file.Sync(); err != nil {
		w.failedErr = err
		return err
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	old := filepath.Join(w.dir, "wal.log")
	archived := filepath.Join(w.dir, fmt.Sprintf("wal-%020d.log", w.seq.Load()))
	if err := os.Rename(old, archived); err != nil {
		return err
	}
	f, err := os.OpenFile(old, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	w.file = f
	w.writer = bufio.NewWriterSize(f, 64*1024)
	if _, err := w.writer.Write(walHeader(w.enc)); err != nil {
		return err
	}
	if err := w.writer.Flush(); err != nil {
		return err
	}
	if err := w.file.Sync(); err != nil {
		return err
	}
	w.size = int64(len(walHeader(w.enc)))
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
