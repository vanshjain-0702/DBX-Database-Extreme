package replication

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
)

const labelSession = "dbx-repl-session"

// sealOverhead is the AES-GCM tag each replication frame carries.
const sealOverhead = 16

const maxSealedFrameSize = maxReplicationFrameSize + sealOverhead

// frameCipher seals replication frames with a key that exists for one
// connection only: it is derived from the group token and both handshake
// nonces. The GCM nonce is a frame counter, so a replayed, dropped, or
// reordered frame fails to open and the replica reconnects.
type frameCipher struct {
	aead    cipher.AEAD
	counter uint64
}

func sessionCipher(token string, replicaNonce, primaryNonce []byte) (*frameCipher, error) {
	block, err := aes.NewCipher(handshakeMAC(token, labelSession, replicaNonce, primaryNonce))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &frameCipher{aead: aead}, nil
}

func (c *frameCipher) nextNonce() []byte {
	nonce := make([]byte, c.aead.NonceSize())
	binary.BigEndian.PutUint64(nonce[len(nonce)-8:], c.counter)
	c.counter++
	return nonce
}

// seal is not safe for concurrent use; callers hold the connection's lock.
func (c *frameCipher) seal(plain []byte) []byte {
	return c.aead.Seal(nil, c.nextNonce(), plain, nil)
}

func (c *frameCipher) open(sealed []byte) ([]byte, error) {
	return c.aead.Open(nil, c.nextNonce(), sealed, nil)
}
