package raft

import (
	"errors"
	"fmt"
)

var (
	ErrStopped          = errors.New("raft node is stopped or closing")
	ErrProposalDropped  = errors.New("proposal dropped because leadership was lost before commit")
	ErrStaleTerm        = errors.New("request term is older than current term")
	ErrSnapshotRequired = errors.New("peer requires snapshot catch-up because requested logs were compacted")
	ErrUnknownPeer      = errors.New("target peer is not registered in cluster topology")
)

type NotLeaderError struct {
	LeaderID   string
	LeaderAddr string
}

func (e *NotLeaderError) Error() string {
	if e.LeaderID == "" {
		return "node is not the leader; no leader is currently known"
	}
	if e.LeaderAddr != "" {
		return fmt.Sprintf("node is not the leader; redirect request to %s (%s)", e.LeaderID, e.LeaderAddr)
	}
	return fmt.Sprintf("node is not the leader; redirect request to %s", e.LeaderID)
}

func IsNotLeader(err error) bool {
	var notLeader *NotLeaderError
	return errors.As(err, &notLeader)
}
