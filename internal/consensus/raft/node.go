package raft

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
	"github.com/makeshift-engineering/penguin-db/internal/config"
	"github.com/makeshift-engineering/penguin-db/internal/consensus/log"
	"github.com/makeshift-engineering/penguin-db/internal/consensus/statemachine"
)

func NewRaftNode(
	cfg *config.RaftConfig,
	raftLog log.RaftLog,
	fsm statemachine.StateMachine,
	transport Transport,
) (*RaftNode, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid raft configuration: %w", err)
	}
	initialTerm := raftLog.CurrentTerm()
	initialVote := raftLog.VotedFor()
	node := &RaftNode{
		cfg:               cfg,
		log:               raftLog,
		fsm:               fsm,
		transport:         transport,
		peerTracker:       NewPeerTracker(cfg.NodeID, cfg.Peers),
		currentTerm:       initialTerm,
		votedFor:          initialVote,
		role:              RoleFollower,
		leaderID:          "",
		commitIndex:       0,
		lastApplied:       0,
		votesGranted:      make(map[string]bool),
		lastHeartbeatTime: time.Now(),
		eventChan:         make(chan any, 1024),
		applyChan:         make(chan struct{}, 1),
		proposals:         make(map[uint64]chan error),
		stopChan:          make(chan struct{}),
	}
	return node, nil
}

func (r *RaftNode) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopped {
		return ErrStopped
	}

	slog.Info("Starting Raft node", "nodeID", r.cfg.NodeID, "term", r.currentTerm)

	r.wg.Add(1)
	go r.run()

	r.wg.Add(1)
	go r.applyWorker()

	r.resetElectionTimer()
	return nil
}

func (r *RaftNode) Stop() error {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return nil
	}
	r.stopped = true
	close(r.stopChan)

	if r.electionTimer != nil {
		r.electionTimer.Stop()
	}
	if r.heartbeatTicker != nil {
		r.heartbeatTicker.Stop()
	}
	r.mu.Unlock()
	r.wg.Wait()

	r.proposalsMu.Lock()
	for index, ch := range r.proposals {
		ch <- ErrStopped
		close(ch)
		delete(r.proposals, index)
	}
	r.proposalsMu.Unlock()

	_ = r.transport.Close()
	_ = r.log.Close()
	_ = r.fsm.Close()

	slog.Info("Raft node stopped cleanly", "nodeID", r.cfg.NodeID)
	return nil
}

func (r *RaftNode) run() {
	defer r.wg.Done()
	for {
		select {
		case <-r.stopChan:
			return
		case event := <-r.eventChan:
			r.handleEvent(event)
		}
	}
}

func (r *RaftNode) handleEvent(event any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopped {
		return
	}

	switch e := event.(type) {
	case electionTimeoutEvent:
		if r.role != RoleLeader {
			slog.Warn("Election timeout triggered", "node", r.cfg.NodeID, "role", r.role.String(), "term", r.currentTerm)
			r.startElection()
		}

	case heartbeatTickEvent:
		if r.role == RoleLeader {
			r.broadcastAppendEntries()
		}
	case voteResponseEvent:
		r.handleVoteResponse(e)
	case appendResponseEvent:
		r.handleAppendResponse(e)
	case clientProposalEvent:
		r.handleClientProposal(e)
	default:
		slog.Error("Unknown event received in event loop", "type", fmt.Sprintf("%T", event))
	}
}

func (r *RaftNode) Propose(ctx context.Context, data []byte) error {
	r.mu.RLock()
	if r.stopped {
		r.mu.RUnlock()
		return ErrStopped
	}
	if r.role != RoleLeader {
		leaderID := r.leaderID
		var leaderAddr string
		if peer, exists := r.peerTracker.GetPeer(leaderID); exists {
			leaderAddr = peer.Address
		}
		r.mu.RUnlock()
		return &NotLeaderError{LeaderID: leaderID, LeaderAddr: leaderAddr}
	}
	r.mu.RUnlock()

	resultChan := make(chan error, 1)
	proposal := clientProposalEvent{
		data:   data,
		result: resultChan,
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.stopChan:
		return ErrStopped
	case r.eventChan <- proposal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.stopChan:
		return ErrStopped
	case err := <-resultChan:
		return err
	}
}

func (r *RaftNode) handleClientProposal(event clientProposalEvent) {
	if r.role != RoleLeader {
		leaderID := r.leaderID
		var leaderAddr string
		if peer, exists := r.peerTracker.GetPeer(leaderID); exists {
			leaderAddr = peer.Address
		}
		event.result <- &NotLeaderError{LeaderID: leaderID, LeaderAddr: leaderAddr}
		return
	}

	lastLogIndex, _ := r.log.LastIndexAndTerm()
	newIndex := lastLogIndex + 1

	entry := &consensuspb.LogEntry{
		Index: newIndex,
		Term:  r.currentTerm,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
		Data:  event.data,
	}

	if err := r.log.Append([]*consensuspb.LogEntry{entry}); err != nil {
		slog.Error("Failed appending client proposal to local log", "index", newIndex, "error", err)
		event.result <- err
		return
	}

	r.proposalsMu.Lock()
	r.proposals[newIndex] = event.result
	r.proposalsMu.Unlock()

	if len(r.peerTracker.PeerIDs()) == 0 {
		r.commitIndex = newIndex
		r.notifyApply()
		return
	}

	r.broadcastAppendEntries()
}

func (r *RaftNode) Status() NodeStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return NodeStatus{
		NodeID:      r.cfg.NodeID,
		Role:        r.role,
		CurrentTerm: r.currentTerm,
		LeaderID:    r.leaderID,
		CommitIndex: r.commitIndex,
		LastApplied: r.lastApplied,
	}
}

func (r *RaftNode) IsLeader() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.role == RoleLeader
}
func (r *RaftNode) LeaderID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.leaderID
}
func (r *RaftNode) CurrentTerm() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentTerm
}
