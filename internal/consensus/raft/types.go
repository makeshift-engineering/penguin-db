package raft

import (
	"fmt"
	"sync"
	"time"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
	"github.com/makeshift-engineering/penguin-db/internal/config"
	"github.com/makeshift-engineering/penguin-db/internal/consensus/log"
	"github.com/makeshift-engineering/penguin-db/internal/consensus/statemachine"
)

type Role uint8

const (
	RoleFollower Role = iota
	RolePreCandidate
	RoleCandidate
	RoleLeader
)

func (r Role) String() string {
	switch r {
	case RoleFollower:
		return "Follower"
	case RolePreCandidate:
		return "PreCandidate"
	case RoleCandidate:
		return "Candidate"
	case RoleLeader:
		return "Leader"
	default:
		return fmt.Sprintf("UnknownRole(%d)", r)
	}
}

type electionTimeoutEvent struct{}
type heartbeatTickEvent struct{}

type voteResponseEvent struct {
	from      string
	resp      *consensuspb.RequestVoteResponse
	isPreVote bool
}

type appendResponseEvent struct {
	from        string
	req         *consensuspb.AppendEntriesRequest
	resp        *consensuspb.AppendEntriesResponse
	entriesSent int
}

type clientProposalEvent struct {
	data   []byte
	result chan error
}

type NodeStatus struct {
	NodeID      string
	Role        Role
	CurrentTerm uint64
	LeaderID    string
	CommitIndex uint64
	LastApplied uint64
}

type RaftNode struct {
	mu  sync.RWMutex
	cfg *config.RaftConfig

	currentTerm uint64
	votedFor    string
	log         log.RaftLog

	role        Role
	leaderID    string
	commitIndex uint64
	lastApplied uint64

	fsm         statemachine.StateMachine
	transport   Transport
	peerTracker *PeerTracker

	votesGranted      map[string]bool
	lastHeartbeatTime time.Time
	electionTimer     *time.Timer
	heartbeatTicker   *time.Ticker

	eventChan chan any
	applyChan chan struct{}

	proposalsMu sync.RWMutex
	proposals   map[uint64]chan error

	stopChan chan struct{}
	wg       sync.WaitGroup
	stopped  bool
}
