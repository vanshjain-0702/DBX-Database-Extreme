package query

import "time"

// RaftNode is the subset of HashiCorp Raft the executor needs.
type RaftNode interface {
	Apply(cmd []byte, timeout time.Duration) RaftFuture
	State() int
	SingleVoter() bool
}

// RaftFuture is a committed log apply.
type RaftFuture interface {
	Error() error
	Response() interface{}
}
