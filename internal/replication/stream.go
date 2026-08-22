package replication

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/dbx/dbx/internal/persistence"
	"github.com/dbx/dbx/internal/protocol"
)

// PrimaryStream manages sending WAL records to connected replicas.
type PrimaryStream struct {
	mu       sync.RWMutex
	replicas map[uint64]*StreamReplicaConn
	nextID   uint64
	listener net.Listener
	done     chan struct{}
	stopOnce sync.Once
}

const maxReplicationFrameSize = 64 << 20

type StreamReplicaConn struct {
	ID   uint64
	Conn net.Conn
	mu   sync.Mutex
}

func NewPrimaryStream() *PrimaryStream {
	return &PrimaryStream{
		replicas: make(map[uint64]*StreamReplicaConn),
		done:     make(chan struct{}),
	}
}

// Start listens for replicas and bootstraps each connection from the WAL.
func (p *PrimaryStream) Start(addr string, wal *persistence.WAL) error {
	if wal == nil {
		return fmt.Errorf("replication: WAL is required")
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.listener = listener
	p.mu.Unlock()
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				select {
				case <-p.done:
					return
				default:
				}
				continue
			}
			id := p.AddReplica(conn)
			go p.bootstrap(id, conn, wal)
		}
	}()
	return nil
}

// Addr returns the bound replication listener address.
func (p *PrimaryStream) Addr() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

func (p *PrimaryStream) bootstrap(id uint64, conn net.Conn, wal *persistence.WAL) {
	records, err := wal.ReadAll()
	if err != nil {
		p.RemoveReplica(id)
		conn.Close()
		return
	}
	for _, rec := range records {
		replica := p.replica(id)
		if replica == nil {
			return
		}
		replica.mu.Lock()
		if err := writeFrame(conn, persistence.EncodeRecord(rec)); err != nil {
			replica.mu.Unlock()
			p.RemoveReplica(id)
			conn.Close()
			return
		}
		replica.mu.Unlock()
	}
}

func (p *PrimaryStream) replica(id uint64) *StreamReplicaConn {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.replicas[id]
}

// BroadcastRecord sends a durable WAL record to all replicas.
func (p *PrimaryStream) BroadcastRecord(rec *persistence.WALRecord) {
	p.Broadcast(persistence.EncodeRecord(rec))
}

// Stop closes the listener and all replica connections.
func (p *PrimaryStream) Stop() {
	p.stopOnce.Do(func() {
		close(p.done)
		p.mu.Lock()
		if p.listener != nil {
			_ = p.listener.Close()
		}
		for id, replica := range p.replicas {
			_ = replica.Conn.Close()
			delete(p.replicas, id)
		}
		p.mu.Unlock()
	})
}

// AddReplica registers a new replica stream.
func (p *PrimaryStream) AddReplica(conn net.Conn) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextID++
	p.replicas[p.nextID] = &StreamReplicaConn{
		ID:   p.nextID,
		Conn: conn,
	}
	return p.nextID
}

// RemoveReplica unregisters a replica stream.
func (p *PrimaryStream) RemoveReplica(id uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.replicas, id)
}

// Broadcast sends a raw WAL record payload to all connected replicas.
func (p *PrimaryStream) Broadcast(data []byte) {
	if len(data) == 0 || len(data) > maxReplicationFrameSize {
		return
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Length prefixed frame
	frame := make([]byte, 4)
	binary.BigEndian.PutUint32(frame, uint32(len(data)))
	payload := append(frame, data...)

	for id, rep := range p.replicas {
		rep.Conn.SetWriteDeadline(time.Now().Add(time.Second))
		rep.mu.Lock()
		err := writeFull(rep.Conn, payload)
		rep.mu.Unlock()
		if err != nil {
			// If a replica is too slow or disconnected, drop it.
			rep.Conn.Close()
			go p.RemoveReplica(id)
		}
	}
}

func writeFrame(conn net.Conn, data []byte) error {
	if len(data) == 0 || len(data) > maxReplicationFrameSize {
		return io.ErrShortBuffer
	}
	frame := make([]byte, 4)
	binary.BigEndian.PutUint32(frame, uint32(len(data)))
	if err := writeFull(conn, frame); err != nil {
		return err
	}
	return writeFull(conn, data)
}

// ReplicaStream manages receiving WAL records from a primary.
type ReplicaStream struct {
	PrimaryAddr string
	Engine      interface {
		ApplyWALRecord(rec *persistence.WALRecord) error
	}
	done     chan struct{}
	stopOnce sync.Once
}

// NewReplicaStream creates a background consumer connecting to the primary.
func NewReplicaStream(addr string, engine interface {
	ApplyWALRecord(*persistence.WALRecord) error
}) *ReplicaStream {
	return &ReplicaStream{
		PrimaryAddr: addr,
		Engine:      engine,
		done:        make(chan struct{}),
	}
}

func (rs *ReplicaStream) Start() {
	go func() {
		for {
			select {
			case <-rs.done:
				return
			default:
			}

			conn, err := net.Dial("tcp", rs.PrimaryAddr)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}

			// Send SYNC command
			writer := protocol.NewWriter(conn)
			writer.WriteArray(1)
			writer.WriteBulkString([]byte("SYNC"))

			rs.consumeStream(conn)
			conn.Close()
		}
	}()
}

func (rs *ReplicaStream) Stop() {
	rs.stopOnce.Do(func() { close(rs.done) })
}

func (rs *ReplicaStream) consumeStream(conn net.Conn) {
	// Simple length-prefixed frame consumer
	for {
		var length uint32
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			return // reconnect
		}
		if length == 0 || length > maxReplicationFrameSize {
			return
		}

		data := make([]byte, length)
		if _, err := io.ReadFull(conn, data); err != nil {
			return // reconnect
		}

		rec, err := persistence.DecodeRecord(data)
		if err != nil {
			return
		}
		if err := rs.Engine.ApplyWALRecord(rec); err != nil {
			return
		}
	}
}

func writeFull(conn net.Conn, payload []byte) error {
	for len(payload) > 0 {
		n, err := conn.Write(payload)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		payload = payload[n:]
	}
	return nil
}
