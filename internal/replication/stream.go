package replication

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/dbx/dbx/internal/isolation"
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
	liveCh   chan []byte
	stopOnce sync.Once
}

const liveReplicationBuffer = 4096

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
		liveCh:   make(chan []byte, liveReplicationBuffer),
	}
}

// Start listens for replicas and bootstraps each connection from the WAL.
func (p *PrimaryStream) Start(addr string, wal *persistence.WAL) error {
	if wal == nil {
		return fmt.Errorf("replication: WAL is required")
	}
	listener, err := isolation.Listen(addr, nil)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.listener = listener
	p.mu.Unlock()
	go p.relay()
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

// ReplicaCount returns the number of currently registered replica connections.
func (p *PrimaryStream) ReplicaCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.replicas)
}

func (p *PrimaryStream) bootstrap(id uint64, conn net.Conn, wal *persistence.WAL) {
	// everysec acknowledges after the frame is in the process buffer, not after
	// fsync. Flush so a replica that connected in that window still catch-up
	// bootstraps the records it missed on the live channel.
	if err := wal.Sync(); err != nil {
		p.RemoveReplica(id)
		conn.Close()
		return
	}
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

// BroadcastRecord enqueues a WAL record for replicas. It never blocks the
// caller: a full buffer drops the frame and the replica catches up on the
// next reconnect bootstrap.
func (p *PrimaryStream) BroadcastRecord(rec *persistence.WALRecord) {
	if rec == nil {
		return
	}
	data := persistence.EncodeRecord(rec)
	select {
	case <-p.done:
		return
	case p.liveCh <- data:
	default:
	}
}

func (p *PrimaryStream) relay() {
	for {
		select {
		case <-p.done:
			return
		case data := <-p.liveCh:
			p.Broadcast(data)
		}
	}
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

const (
	replicaReconnectMin = 50 * time.Millisecond
	replicaReconnectMax = 2 * time.Second
)

// ReplicaStream manages receiving WAL records from a primary.
type ReplicaStream struct {
	PrimaryAddr string
	Engine      interface {
		ApplyWALRecord(rec *persistence.WALRecord) error
	}
	mu       sync.Mutex
	conn     net.Conn
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
		backoff := replicaReconnectMin
		for {
			select {
			case <-rs.done:
				return
			default:
			}

			conn, err := isolation.DialTimeout(rs.PrimaryAddr, time.Second)
			if err != nil {
				select {
				case <-rs.done:
					return
				case <-time.After(backoff):
				}
				if backoff < replicaReconnectMax {
					backoff *= 2
					if backoff > replicaReconnectMax {
						backoff = replicaReconnectMax
					}
				}
				continue
			}
			backoff = replicaReconnectMin
			rs.setConn(conn)
			select {
			case <-rs.done:
				rs.setConn(nil)
				conn.Close()
				return
			default:
			}

			writer := protocol.NewWriter(conn)
			_ = writer.WriteArray(1)
			_ = writer.WriteBulkString([]byte("SYNC"))
			_ = writer.Flush()

			rs.consumeStream(conn)
			rs.setConn(nil)
			conn.Close()
		}
	}()
}

func (rs *ReplicaStream) setConn(conn net.Conn) {
	rs.mu.Lock()
	rs.conn = conn
	rs.mu.Unlock()
}

func (rs *ReplicaStream) Stop() {
	rs.stopOnce.Do(func() {
		close(rs.done)
		rs.mu.Lock()
		conn := rs.conn
		rs.conn = nil
		rs.mu.Unlock()
		if conn != nil {
			_ = conn.Close()
		}
	})
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
