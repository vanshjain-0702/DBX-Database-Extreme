package replication

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"
)

// The replication handshake is a mutual HMAC challenge over the group token.
// Neither side sends the token, and the primary writes no WAL byte until the
// replica has proven it holds the key. The replica also verifies the primary,
// so a process squatting on a free loopback port cannot feed it writes. Both
// sides then derive a per-connection key, and every WAL frame is sealed.
const (
	handshakeMagic   = "DBXREPL1"
	handshakeNonce   = 32
	handshakeTimeout = 5 * time.Second

	labelPrimary = "dbx-repl-primary"
	labelReplica = "dbx-repl-replica"
)

// MinTokenLength matches config.MinReplicationTokenLength.
const MinTokenLength = 32

var errHandshake = errors.New("replication: handshake failed")

func handshakeMAC(token, label string, first, second []byte) []byte {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(label))
	mac.Write(first)
	mac.Write(second)
	return mac.Sum(nil)
}

// readExactFrame reads one length-prefixed frame that must be exactly want
// bytes. An unauthenticated peer cannot make us allocate more than that.
func readExactFrame(conn net.Conn, want int) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}
	if int(binary.BigEndian.Uint32(header[:])) != want {
		return nil, errHandshake
	}
	buf := make([]byte, want)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func newNonce() ([]byte, error) {
	nonce := make([]byte, handshakeNonce)
	_, err := rand.Read(nonce)
	return nonce, err
}

// serverHandshake authenticates a replica and returns the cipher that seals
// every frame sent to it on this connection.
func serverHandshake(conn net.Conn, token string) (*frameCipher, error) {
	if len(token) < MinTokenLength {
		return nil, errHandshake
	}
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	hello, err := readExactFrame(conn, len(handshakeMagic)+handshakeNonce)
	if err != nil {
		return nil, err
	}
	if string(hello[:len(handshakeMagic)]) != handshakeMagic {
		return nil, errHandshake
	}
	replicaNonce := hello[len(handshakeMagic):]
	primaryNonce, err := newNonce()
	if err != nil {
		return nil, err
	}
	challenge := append(append([]byte{}, primaryNonce...), handshakeMAC(token, labelPrimary, replicaNonce, primaryNonce)...)
	if err := writeFrame(conn, challenge); err != nil {
		return nil, err
	}
	proof, err := readExactFrame(conn, sha256.Size)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal(proof, handshakeMAC(token, labelReplica, primaryNonce, replicaNonce)) {
		return nil, errHandshake
	}
	return sessionCipher(token, replicaNonce, primaryNonce)
}

// clientHandshake authenticates the primary and returns the cipher that opens
// the frames it sends on this connection.
func clientHandshake(conn net.Conn, token string) (*frameCipher, error) {
	if len(token) < MinTokenLength {
		return nil, errHandshake
	}
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	replicaNonce, err := newNonce()
	if err != nil {
		return nil, err
	}
	if err := writeFrame(conn, append([]byte(handshakeMagic), replicaNonce...)); err != nil {
		return nil, err
	}
	challenge, err := readExactFrame(conn, handshakeNonce+sha256.Size)
	if err != nil {
		return nil, err
	}
	primaryNonce := challenge[:handshakeNonce]
	if !hmac.Equal(challenge[handshakeNonce:], handshakeMAC(token, labelPrimary, replicaNonce, primaryNonce)) {
		return nil, errHandshake
	}
	if err := writeFrame(conn, handshakeMAC(token, labelReplica, primaryNonce, replicaNonce)); err != nil {
		return nil, err
	}
	return sessionCipher(token, replicaNonce, primaryNonce)
}
