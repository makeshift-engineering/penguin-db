package raft

import "log/slog"

func (r *RaftNode) advanceCommitIndex() {
	if r.role != RoleLeader {
		return
	}

	lastLogIndex, _ := r.log.LastIndexAndTerm()
	quorumMatch := r.peerTracker.QuorumMatchIndex(lastLogIndex)

	if quorumMatch > r.commitIndex {
		entry, err := r.log.GetEntry(quorumMatch)
		if err == nil && entry.Term == r.currentTerm {
			r.commitIndex = quorumMatch
			r.notifyApply()
		}
	}
}

func (r *RaftNode) notifyApply() {
	select {
	case r.applyChan <- struct{}{}:
	default:
	}
}

func (r *RaftNode) applyWorker() {
	defer r.wg.Done()

	for {
		select {
		case <-r.stopChan:
			return

		case <-r.applyChan:
			for {
				r.mu.RLock()
				if r.stopped {
					r.mu.RUnlock()
					return
				}
				nextToApply := r.lastApplied + 1
				commitIndex := r.commitIndex
				r.mu.RUnlock()

				if nextToApply > commitIndex {
					break
				}

				entry, err := r.log.GetEntry(nextToApply)
				if err != nil {
					slog.Error("Failed reading committed log entry to apply", "index", nextToApply, "error", err)
					break
				}

				_, applyErr := r.fsm.Apply(entry)

				r.mu.Lock()
				r.lastApplied = nextToApply
				r.mu.Unlock()

				r.proposalsMu.Lock()
				if ch, exists := r.proposals[nextToApply]; exists {
					ch <- applyErr
					close(ch)
					delete(r.proposals, nextToApply)
				}
				r.proposalsMu.Unlock()
			}
		}
	}
}
