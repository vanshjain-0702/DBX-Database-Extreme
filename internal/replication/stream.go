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
)

// PrimaryStream manages sending WAL records to connected replicas.
type PrimaryStream struct {
	mu       sync.RWMutex
	replicas map[uint64]*StreamReplicaConn
	nextID   uint64
	listener net.Listener
	token    string
	// pending holds connections still in handshake or bootstrap. Stop closes
	// them and waits, so no bootstrap reads the WAL after the engine closes it.
	pending  map[net.Conn]struct{}
	admitWG  sync.WaitGroup
	done     chan struct{}
	liveCh   chan []byte
	stopOnce sync.Once
}

const liveReplicationBuffer = 4096

const maxReplicationFrameSize = 64 << 20

type StreamReplicaConn struct {
	ID   uint64
	Conn net.Conn
	// mu serializes writes and guards cipher's frame counter.
	mu     sync.Mutex
	cipher *frameCipher
}

// send seals data for this replica and writes it as one frame. The caller
// holds r.mu.
func (r *StreamReplicaConn) send(data []byte) error {
	return writeFrame(r.Conn, r.cipher.seal(data))
}

// NewPrimaryStream creates a primary that streams only to replicas proving
// they hold token.
func NewPrimaryStream(token string) *PrimaryStream {
	return &PrimaryStream{
		replicas: make(map[uint64]*StreamReplicaConn),
		token:    token,
		pending:  make(map[net.Conn]struct{}),
		done:     make(chan struct{}),
		liveCh:   make(chan []byte, liveReplicationBuffer),
	}
}

// Start listens for replicas and bootstraps each authenticated connection
// from the WAL.
func (p *PrimaryStream) Start(addr string, wal *persistence.WAL) error {
	if wal == nil {
		return fmt.Errorf("replication: WAL is required")
	}
	if len(p.token) < MinTokenLength {
		return fmt.Errorf("replication: a token of at least %d characters is required", MinTokenLength)
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
			p.mu.Lock()
			select {
			case <-p.done:
				p.mu.Unlock()
				_ = conn.Close()
				return
			default:
			}
			p.pending[conn] = struct{}{}
			p.admitWG.Add(1)
			p.mu.Unlock()
			go p.admit(conn, wal)
		}
	}()
	return nil
}

// admit registers conn for live records only after the handshake, so an
// unauthenticated peer never receives a single WAL byte.
func (p *PrimaryStream) admit(conn net.Conn, wal *persistence.WAL) {
	defer p.admitWG.Done()
	defer func() {
		p.mu.Lock()
		delete(p.pending, conn)
		p.mu.Unlock()
	}()
	fc, err := serverHandshake(conn, p.token)
	if err != nil {
		_ = conn.Close()
		return
	}
	select {
	case <-p.done:
		_ = conn.Close()
		return
	default:
	}
	id := p.addReplica(conn, fc)
	p.bootstrap(id, conn, wal)
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
		if err := replica.send(persistence.EncodeRecord(rec)); err != nil {
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
		for conn := range p.pending {
			_ = conn.Close()
		}
		p.mu.Unlock()
		p.admitWG.Wait()
	})
}

// addReplica registers an authenticated replica and the cipher for its frames.
func (p *PrimaryStream) addReplica(conn net.Conn, fc *frameCipher) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextID++
	p.replicas[p.nextID] = &StreamReplicaConn{
		ID:     p.nextID,
		Conn:   conn,
		cipher: fc,
	}
	return p.nextID
}

// RemoveReplica unregisters a replica stream.
func (p *PrimaryStream) RemoveReplica(id uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.replicas, id)
}

// Broadcast seals a raw WAL record payload for each connected replica.
func (p *PrimaryStream) Broadcast(data []byte) {
	if len(data) == 0 || len(data) > maxReplicationFrameSize {
		return
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	for id, rep := range p.replicas {
		rep.Conn.SetWriteDeadline(time.Now().Add(time.Second))
		rep.mu.Lock()
		err := rep.send(data)
		rep.mu.Unlock()
		if err != nil {
			// If a replica is too slow or disconnected, drop it.
			rep.Conn.Close()
			go p.RemoveReplica(id)
		}
	}
}

func writeFrame(conn net.Conn, data []byte) error {
	if len(data) == 0 || len(data) > maxSealedFrameSize {
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
	token    string
	mu       sync.Mutex
	conn     net.Conn
	done     chan struct{}
	stopOnce sync.Once
}

// NewReplicaStream creates a background consumer connecting to the primary.
// token must match the primary's; the handshake authenticates both ends.
func NewReplicaStream(addr, token string, engine interface {
	ApplyWALRecord(*persistence.WALRecord) error
}) *ReplicaStream {
	return &ReplicaStream{
		PrimaryAddr: addr,
		Engine:      engine,
		token:       token,
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

			var fc *frameCipher
			conn, err := isolation.DialTimeout(rs.PrimaryAddr, time.Second)
			if err == nil {
				if fc, err = clientHandshake(conn, rs.token); err != nil {
					conn.Close()
				}
			}
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

			rs.consumeStream(conn, fc)
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

// consumeStream applies sealed frames until the connection fails or a frame
// does not authenticate; either way the caller reconnects and re-bootstraps.
func (rs *ReplicaStream) consumeStream(conn net.Conn, fc *frameCipher) {
	for {
		var length uint32
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			return
		}
		if length <= sealOverhead || length > maxSealedFrameSize {
			return
		}

		sealed := make([]byte, length)
		if _, err := io.ReadFull(conn, sealed); err != nil {
			return
		}
		data, err := fc.open(sealed)
		if err != nil {
			return
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
