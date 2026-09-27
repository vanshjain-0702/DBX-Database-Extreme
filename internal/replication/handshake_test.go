package replication

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbx/dbx/internal/persistence"
)

const testReplToken = "test-replication-token-0123456789abcdef"

func startSecretPrimary(t *testing.T) *PrimaryStream {
	t.Helper()
	wal, err := persistence.OpenWAL(filepath.Join(t.TempDir(), "wal"), "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { wal.Close() })
	if err := wal.Write(&persistence.WALRecord{Type: persistence.RecordSet, Key: "tenant-secret", Value: []byte("privileged-clause")}); err != nil {
		t.Fatal(err)
	}
	primary := NewPrimaryStream(testReplToken)
	if err := primary.Start("127.0.0.1:0", wal); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(primary.Stop)
	return primary
}

// A local process that just connects must not receive the WAL. Before the
// handshake existed, the primary streamed its whole history on accept.
func TestPrimaryStreamsNothingToUnauthenticatedPeer(t *testing.T) {
	primary := startSecretPrimary(t)
	conn, err := net.DialTimeout("tcp", primary.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout + 2*time.Second))
	got, _ := io.ReadAll(conn)
	if bytes.Contains(got, []byte("privileged-clause")) || bytes.Contains(got, []byte("tenant-secret")) {
		t.Fatalf("primary leaked WAL to an unauthenticated peer: %q", got)
	}
	if primary.ReplicaCount() != 0 {
		t.Fatalf("unauthenticated peer was registered as a replica")
	}
}

func TestReplicaWithWrongTokenReceivesNothing(t *testing.T) {
	primary := startSecretPrimary(t)
	engine := &collectingReplicaEngine{records: make(chan *persistence.WALRecord, 1)}
	replica := NewReplicaStream(primary.Addr(), "wrong-token-wrong-token-wrong-token-00", engine)
	replica.Start()
	defer replica.Stop()
	select {
	case rec := <-engine.records:
		t.Fatalf("replica with the wrong token received %q", rec.Key)
	case <-time.After(1500 * time.Millisecond):
	}
	if primary.ReplicaCount() != 0 {
		t.Fatal("wrong-token replica was registered")
	}
}

// The replica must also refuse a primary that cannot prove the key, so a
// process squatting on a free loopback port cannot inject writes.
func TestReplicaRejectsImpostorPrimary(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	done := make(chan error, 1)
	go func() {
		_, err := clientHandshake(client, testReplToken)
		done <- err
	}()
	if _, err := readExactFrame(server, len(handshakeMagic)+handshakeNonce); err != nil {
		t.Fatal(err)
	}
	forged := make([]byte, handshakeNonce+32)
	if err := writeFrame(server, forged); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("replica accepted a primary that does not hold the token")
	}
}

// A peer that sees the socket bytes but not the session key learns nothing:
// an authenticated replica's own wire must carry only ciphertext.
func TestReplicationFramesAreEncryptedOnTheWire(t *testing.T) {
	primary := startSecretPrimary(t)
	conn, err := net.DialTimeout("tcp", primary.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fc, err := clientHandshake(conn, testReplToken)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		t.Fatal(err)
	}
	sealed := make([]byte, binary.BigEndian.Uint32(header[:]))
	if _, err := io.ReadFull(conn, sealed); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("privileged-clause")) || bytes.Contains(sealed, []byte("tenant-secret")) {
		t.Fatalf("replication frame carried plaintext: %q", sealed)
	}
	plain, err := fc.open(sealed)
	if err != nil {
		t.Fatalf("session key did not open the frame: %v", err)
	}
	rec, err := persistence.DecodeRecord(plain)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Key != "tenant-secret" || string(rec.Value) != "privileged-clause" {
		t.Fatalf("unexpected record %q=%q", rec.Key, rec.Value)
	}
}

func TestSessionKeysDifferPerConnection(t *testing.T) {
	a, err := sessionCipher(testReplToken, bytes.Repeat([]byte{1}, handshakeNonce), bytes.Repeat([]byte{2}, handshakeNonce))
	if err != nil {
		t.Fatal(err)
	}
	b, err := sessionCipher(testReplToken, bytes.Repeat([]byte{3}, handshakeNonce), bytes.Repeat([]byte{2}, handshakeNonce))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.open(a.seal([]byte("frame"))); err == nil {
		t.Fatal("a frame from one session opened under another session's key")
	}
}

func TestPrimaryRefusesToStartWithoutToken(t *testing.T) {
	wal, err := persistence.OpenWAL(filepath.Join(t.TempDir(), "wal"), "always", 64)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	if err := NewPrimaryStream("").Start("127.0.0.1:0", wal); err == nil {
		t.Fatal("primary started an unauthenticated replication listener")
	}
}
