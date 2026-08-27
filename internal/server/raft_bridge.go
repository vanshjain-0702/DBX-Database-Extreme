package server

import (
	"time"

	"github.com/dbx/dbx/internal/query"
	"github.com/hashicorp/raft"
)

type raftBridge struct {
	node *raft.Raft
}

func (b *raftBridge) Apply(cmd []byte, timeout time.Duration) query.RaftFuture {
	return b.node.Apply(cmd, timeout)
}

func (b *raftBridge) State() int {
	return int(b.node.State())
}

func (b *raftBridge) SingleVoter() bool {
	future := b.node.GetConfiguration()
	if err := future.Error(); err != nil {
		return true
	}
	voters := 0
	for _, srv := range future.Configuration().Servers {
		if srv.Suffrage == raft.Voter {
			voters++
		}
	}
	return voters <= 1
}
