package raft

import (
	"context"
	"log/slog"
	"time"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
)

func (r *RaftNode) startHeartbeatTicker() {
	if r.heartbeatTicker != nil {
		r.heartbeatTicker.Stop()
	}
	r.heartbeatTicker = time.NewTicker(r.cfg.HeartbeatInterval)
	go func() {
		for {
			select {
			case <-r.stopChan:
				return
			case <-r.heartbeatTicker.C:
				r.eventChan <- heartbeatTickEvent{}
			}
		}
	}()
}

func (r *RaftNode) broadcastAppendEntries() {
	if r.role != RoleLeader {
		return
	}

	for _, peerID := range r.peerTracker.PeerIDs() {
		r.replicateToPeer(peerID)
	}
}

func (r *RaftNode) replicateToPeer(peerID string) {
	peer, exists := r.peerTracker.GetPeer(peerID)
	if !exists || peer.Paused || peer.InFlight {
		return
	}

	firstLogIndex := r.log.FirstIndex()
	if peer.NextIndex < firstLogIndex {
		r.sendSnapshotToPeer(peerID)
		return
	}

	prevLogIndex := peer.NextIndex - 1
	var prevLogTerm uint64
	if prevLogIndex > 0 {
		if prevLogIndex >= firstLogIndex {
			prevEntry, err := r.log.GetEntry(prevLogIndex)
			if err != nil {
				return
			}
			prevLogTerm = prevEntry.Term
		} else if prevLogIndex == firstLogIndex-1 {
			prevLogTerm = r.lastSnapshotTerm
		} else {
			r.sendSnapshotToPeer(peerID)
			return
		}
	}

	lastLogIndex, _ := r.log.LastIndexAndTerm()
	var entriesToSend []*consensuspb.LogEntry
	if lastLogIndex >= peer.NextIndex {
		endIndex := peer.NextIndex + uint64(r.cfg.MaxEntriesPerAppend)
		if endIndex > lastLogIndex+1 {
			endIndex = lastLogIndex + 1
		}
		var err error
		entriesToSend, err = r.log.GetEntries(peer.NextIndex, endIndex)
		if err != nil {
			return
		}
	}

	req := &consensuspb.AppendEntriesRequest{
		Term:         r.currentTerm,
		LeaderId:     r.cfg.NodeID,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entriesToSend,
		LeaderCommit: r.commitIndex,
	}

	r.peerTracker.SetInFlight(peerID, true)
	go func(id string, request *consensuspb.AppendEntriesRequest) {
		resp, err := r.transport.SendAppendEntries(context.Background(), id, request)
		if err != nil {
			r.peerTracker.SetInFlight(id, false)
			return
		}

		r.eventChan <- appendResponseEvent{
			from:        id,
			req:         request,
			resp:        resp,
			entriesSent: len(request.Entries),
		}
	}(peerID, req)
}

func (r *RaftNode) handleAppendResponse(event appendResponseEvent) {
	resp := event.resp

	if resp.Term > r.currentTerm {
		r.peerTracker.SetInFlight(event.from, false)
		r.becomeFollower(resp.Term, "")
		return
	}

	if r.role != RoleLeader || resp.Term != r.currentTerm {
		r.peerTracker.SetInFlight(event.from, false)
		return
	}

	if resp.Success {
		r.peerTracker.UpdateProgress(event.from, resp.MatchIndex)
		r.advanceCommitIndex()
		return
	}

	if resp.ConflictTerm > 0 {
		lastIdxOfTerm := r.findLastIndexOfTerm(resp.ConflictTerm)
		if lastIdxOfTerm > 0 {
			r.peerTracker.DecrementNextIndex(event.from, lastIdxOfTerm+1)
		} else {
			r.peerTracker.DecrementNextIndex(event.from, resp.ConflictIndex)
		}
	} else if resp.ConflictIndex > 0 {
		r.peerTracker.DecrementNextIndex(event.from, resp.ConflictIndex)
	} else {
		r.peerTracker.DecrementNextIndex(event.from, 0)
	}

	r.replicateToPeer(event.from)
}

func (r *RaftNode) HandleAppendEntries(ctx context.Context, req *consensuspb.AppendEntriesRequest) (*consensuspb.AppendEntriesResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if req.Term < r.currentTerm {
		return &consensuspb.AppendEntriesResponse{
			Term:    r.currentTerm,
			Success: false,
		}, nil
	}

	if req.Term > r.currentTerm || r.role != RoleFollower {
		r.becomeFollower(req.Term, req.LeaderId)
	}

	r.leaderID = req.LeaderId
	r.resetElectionTimer()

	lastLogIndex, _ := r.log.LastIndexAndTerm()
	firstLogIndex := r.log.FirstIndex()
	if req.PrevLogIndex > 0 {
		if req.PrevLogIndex > lastLogIndex {
			return &consensuspb.AppendEntriesResponse{
				Term:          r.currentTerm,
				Success:       false,
				ConflictIndex: lastLogIndex + 1,
				ConflictTerm:  0,
			}, nil
		}

		if req.PrevLogIndex < firstLogIndex-1 {
			return &consensuspb.AppendEntriesResponse{
				Term:          r.currentTerm,
				Success:       false,
				ConflictIndex: firstLogIndex,
				ConflictTerm:  0,
			}, nil
		}

		if req.PrevLogIndex >= firstLogIndex {
			prevEntry, err := r.log.GetEntry(req.PrevLogIndex)
			if err != nil {
				return &consensuspb.AppendEntriesResponse{
					Term:          r.currentTerm,
					Success:       false,
					ConflictIndex: lastLogIndex + 1,
				}, nil
			}

			if prevEntry.Term != req.PrevLogTerm {
				conflictTerm := prevEntry.Term
				firstIdxOfTerm := r.findFirstIndexOfTerm(conflictTerm)
				return &consensuspb.AppendEntriesResponse{
					Term:          r.currentTerm,
					Success:       false,
					ConflictTerm:  conflictTerm,
					ConflictIndex: firstIdxOfTerm,
				}, nil
			}
		} else if req.PrevLogIndex == firstLogIndex-1 {
			if r.lastSnapshotTerm > 0 && req.PrevLogTerm != r.lastSnapshotTerm {
				return &consensuspb.AppendEntriesResponse{
					Term:          r.currentTerm,
					Success:       false,
					ConflictIndex: firstLogIndex,
				}, nil
			}
		}
	}

	for i, entry := range req.Entries {
		if entry.Index < firstLogIndex {
			continue
		}
		if entry.Index <= lastLogIndex {
			existingEntry, err := r.log.GetEntry(entry.Index)
			if err == nil && existingEntry.Term != entry.Term {
				if err := r.log.TruncateSuffix(entry.Index); err != nil {
					slog.Error("Failed truncating conflicting log suffix", "index", entry.Index, "error", err)
					return nil, err
				}
				if err := r.log.Append(req.Entries[i:]); err != nil {
					return nil, err
				}
				break
			}
		} else {
			if err := r.log.Append(req.Entries[i:]); err != nil {
				return nil, err
			}
			break
		}
	}

	if req.LeaderCommit > r.commitIndex {
		newLastIndex, _ := r.log.LastIndexAndTerm()
		if req.LeaderCommit < newLastIndex {
			r.commitIndex = req.LeaderCommit
		} else {
			r.commitIndex = newLastIndex
		}
		r.notifyApply()
	}

	matchIndex := req.PrevLogIndex + uint64(len(req.Entries))
	return &consensuspb.AppendEntriesResponse{
		Term:       r.currentTerm,
		Success:    true,
		MatchIndex: matchIndex,
	}, nil
}

func (r *RaftNode) findFirstIndexOfTerm(term uint64) uint64 {
	firstIdx := r.log.FirstIndex()
	lastIdx, _ := r.log.LastIndexAndTerm()
	for idx := firstIdx; idx <= lastIdx; idx++ {
		entry, err := r.log.GetEntry(idx)
		if err == nil && entry.Term == term {
			return idx
		}
	}
	return firstIdx
}

func (r *RaftNode) findLastIndexOfTerm(term uint64) uint64 {
	firstIdx := r.log.FirstIndex()
	lastIdx, _ := r.log.LastIndexAndTerm()
	for idx := lastIdx; idx >= firstIdx; idx-- {
		entry, err := r.log.GetEntry(idx)
		if err == nil && entry.Term == term {
			return idx
		}
	}
	return 0
}
