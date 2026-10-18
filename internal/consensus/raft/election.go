package raft

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
)

func (r *RaftNode) randomElectionTimeout() time.Duration {
	delta := r.cfg.ElectionTimeoutMax - r.cfg.ElectionTimeoutMin
	jitter := time.Duration(rand.Int63n(int64(delta)))
	return r.cfg.ElectionTimeoutMin + jitter
}

func (r *RaftNode) resetElectionTimer() {
	if r.electionTimer != nil {
		r.electionTimer.Stop()
	}
	timeout := r.randomElectionTimeout()
	r.electionTimer = time.AfterFunc(timeout, func() {
		r.eventChan <- electionTimeoutEvent{}
	})
	r.lastHeartbeatTime = time.Now()
}

func (r *RaftNode) stopElectionTimer() {
	if r.electionTimer != nil {
		r.electionTimer.Stop()
	}
}

func (r *RaftNode) becomeFollower(term uint64, leaderID string) {
	prevRole := r.role
	r.role = RoleFollower
	r.leaderID = leaderID

	if term > r.currentTerm {
		r.currentTerm = term
		r.votedFor = ""
		if err := r.log.SetMeta(r.currentTerm, r.votedFor); err != nil {
			slog.Error("Failed persisting metadata during follower step-down", "node", r.cfg.NodeID, "error", err)
		}
	}

	if r.heartbeatTicker != nil {
		r.heartbeatTicker.Stop()
	}

	r.resetElectionTimer()
	slog.Info("Transitioned to Follower", "node", r.cfg.NodeID, "term", r.currentTerm, "leader", leaderID, "prevRole", prevRole.String())
}

func (r *RaftNode) becomePreCandidate() {
	r.role = RolePreCandidate
	r.leaderID = ""
	r.votesGranted = make(map[string]bool)
	r.votesGranted[r.cfg.NodeID] = true

	r.resetElectionTimer()
	slog.Info("Transitioned to PreCandidate", "node", r.cfg.NodeID, "projectedTerm", r.currentTerm+1)
}

func (r *RaftNode) becomeCandidate() {
	r.role = RoleCandidate
	r.leaderID = ""
	r.currentTerm++
	r.votedFor = r.cfg.NodeID

	if err := r.log.SetMeta(r.currentTerm, r.votedFor); err != nil {
		slog.Error("Failed persisting candidate vote", "node", r.cfg.NodeID, "term", r.currentTerm, "error", err)
	}

	r.votesGranted = make(map[string]bool)
	r.votesGranted[r.cfg.NodeID] = true
	r.resetElectionTimer()
	slog.Info("Transitioned to Candidate", "node", r.cfg.NodeID, "term", r.currentTerm)
}

func (r *RaftNode) becomeLeader() {
	if r.role != RoleCandidate {
		return
	}

	r.role = RoleLeader
	r.leaderID = r.cfg.NodeID

	lastLogIndex, _ := r.log.LastIndexAndTerm()
	r.peerTracker.InitLeaderState(lastLogIndex)

	noOpIndex := lastLogIndex + 1
	noOpEntry := &consensuspb.LogEntry{
		Index: noOpIndex,
		Term:  r.currentTerm,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NOOP,
		Data:  nil,
	}

	if err := r.log.Append([]*consensuspb.LogEntry{noOpEntry}); err != nil {
		slog.Error("Failed appending leader No-Op entry", "node", r.cfg.NodeID, "error", err)
		r.becomeFollower(r.currentTerm, "")
		return
	}

	r.stopElectionTimer()
	slog.Info("Won election and transitioned to Leader", "node", r.cfg.NodeID, "term", r.currentTerm, "noOpIndex", noOpIndex)
}

func (r *RaftNode) startElection() {
	if r.cfg.PreVote {
		r.becomePreCandidate()
		r.broadcastRequestVote(true)
	} else {
		r.becomeCandidate()
		r.broadcastRequestVote(false)
	}
}

func (r *RaftNode) broadcastRequestVote(isPreVote bool) {
	lastLogIndex, lastLogTerm := r.log.LastIndexAndTerm()
	termToSend := r.currentTerm
	if isPreVote {
		termToSend = r.currentTerm + 1
	}

	peerIDs := r.peerTracker.PeerIDs()
	if len(peerIDs) == 0 {
		if isPreVote {
			r.becomeCandidate()
			r.broadcastRequestVote(false)
		} else {
			r.becomeLeader()
		}
		return
	}

	for _, peerID := range peerIDs {
		go func(id string) {
			req := &consensuspb.RequestVoteRequest{
				Term:         termToSend,
				CandidateId:  r.cfg.NodeID,
				LastLogIndex: lastLogIndex,
				LastLogTerm:  lastLogTerm,
				IsPreVote:    isPreVote,
			}

			resp, err := r.transport.SendRequestVote(context.Background(), id, req)
			if err != nil {
				return
			}

			r.eventChan <- voteResponseEvent{
				from:      id,
				resp:      resp,
				isPreVote: isPreVote,
			}
		}(peerID)
	}
}

func (r *RaftNode) handleVoteResponse(event voteResponseEvent) {
	resp := event.resp

	if resp.Term > r.currentTerm {
		r.becomeFollower(resp.Term, "")
		return
	}

	if event.isPreVote {
		if r.role != RolePreCandidate || resp.Term != r.currentTerm+1 {
			return
		}
		if resp.VoteGranted {
			r.votesGranted[event.from] = true
			if r.peerTracker.IsQuorum(len(r.votesGranted)) {
				slog.Info("Pre-Vote quorum achieved, advancing to formal election", "node", r.cfg.NodeID)
				r.becomeCandidate()
				r.broadcastRequestVote(false)
			}
		}
		return
	}

	if r.role != RoleCandidate || resp.Term != r.currentTerm {
		return
	}

	if resp.VoteGranted {
		r.votesGranted[event.from] = true
		if r.peerTracker.IsQuorum(len(r.votesGranted)) {
			r.becomeLeader()
			r.eventChan <- heartbeatTickEvent{}
		}
	}
}

func (r *RaftNode) HandleRequestVote(ctx context.Context, req *consensuspb.RequestVoteRequest) (*consensuspb.RequestVoteResponse, error) {
	r.mu.Lock()
	defer r.mu.Lock()

	if req.Term < r.currentTerm {
		return &consensuspb.RequestVoteResponse{
			Term:        r.currentTerm,
			VoteGranted: false,
		}, nil
	}

	if req.Term > r.currentTerm && !req.IsPreVote {
		r.becomeFollower(req.Term, "")
	}

	if req.IsPreVote {
		if time.Since(r.lastHeartbeatTime) < r.cfg.ElectionTimeoutMin {
			return &consensuspb.RequestVoteResponse{
				Term:        r.currentTerm,
				VoteGranted: false,
			}, nil
		}
	}

	canVote := (r.votedFor == "" || r.votedFor == req.CandidateId) || req.IsPreVote
	lastLogIndex, lastLogTerm := r.log.LastIndexAndTerm()
	isLogUpToDate := false
	if req.LastLogTerm != lastLogTerm {
		isLogUpToDate = req.LastLogTerm > lastLogTerm
	} else {
		isLogUpToDate = req.LastLogIndex >= lastLogIndex
	}

	if canVote && isLogUpToDate {
		if !req.IsPreVote {
			r.votedFor = req.CandidateId
			if err := r.log.SetMeta(r.currentTerm, r.votedFor); err != nil {
				slog.Error("Failed persisting vote grant", "candidate", req.CandidateId, "error", err)
				return nil, err
			}
			r.resetElectionTimer()
		}
		return &consensuspb.RequestVoteResponse{
			Term:        r.currentTerm,
			VoteGranted: true,
		}, nil
	}

	return &consensuspb.RequestVoteResponse{
		Term:        r.currentTerm,
		VoteGranted: false,
	}, nil
}
