package raft

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
)

const (
	snapshotChunkSize = 64 * 1024
)

func (r *RaftNode) TakeSnapshot() error {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return ErrStopped
	}

	lastApplied := r.lastApplied
	if lastApplied == 0 {
		r.mu.Unlock()
		return nil
	}

	firstIndex := r.log.FirstIndex()
	if lastApplied < firstIndex {
		r.mu.Unlock()
		return nil
	}

	var lastTerm uint64
	entry, err := r.log.GetEntry(lastApplied)
	if err != nil {
		slog.Error("Failed retrieving last applied entry for snapshot compaction", "index", lastApplied, "error", err)
		r.mu.Unlock()
		return fmt.Errorf("failed retrieving log entry at index %d: %w", lastApplied, err)
	}
	lastTerm = entry.Term
	r.mu.Unlock()

	reader, err := r.fsm.Snapshot()
	if err != nil {
		slog.Error("Failed capturing state machine snapshot", "error", err)
		return fmt.Errorf("state machine snapshot creation failed: %w", err)
	}
	defer reader.Close()

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.log.TruncatePrefix(lastApplied); err != nil {
		slog.Error("Failed truncating log prefix after snapshot", "lastApplied", lastApplied, "error", err)
		return fmt.Errorf("log prefix truncation failed: %w", err)
	}

	slog.Info("Completed snapshot log compaction",
		"node", r.cfg.NodeID,
		"lastIncludedIndex", lastApplied,
		"lastIncludedTerm", lastTerm,
		"snapshotSizeBytes", reader.Size(),
	)

	return nil
}

func (r *RaftNode) maybeTriggerSnapshot() {
	if r.cfg.SnapshotThreshold == 0 {
		return
	}

	r.mu.RLock()
	lastApplied := r.lastApplied
	firstIndex := r.log.FirstIndex()
	r.mu.RUnlock()

	if lastApplied >= firstIndex && (lastApplied-firstIndex+1) >= r.cfg.SnapshotThreshold {
		go func() {
			if err := r.TakeSnapshot(); err != nil {
				slog.Warn("Background snapshot compaction failed", "error", err)
			}
		}()
	}
}

func (r *RaftNode) sendSnapshotToPeer(peerID string) {
	peer, exists := r.peerTracker.GetPeer(peerID)
	if !exists || peer.Paused || peer.InFlight {
		return
	}

	r.peerTracker.SetInFlight(peerID, true)

	go func(targetID string) {
		r.mu.RLock()
		if r.role != RoleLeader || r.stopped {
			r.mu.RLock()
			r.peerTracker.SetInFlight(targetID, false)
			return
		}

		currentTerm := r.currentTerm
		leaderID := r.cfg.NodeID
		lastApplied := r.lastApplied
		r.mu.Unlock()

		if lastApplied == 0 {
			r.peerTracker.SetInFlight(targetID, false)
			return
		}

		var lastIncludedTerm uint64
		entry, err := r.log.GetEntry(lastApplied)
		if err == nil {
			lastIncludedTerm = entry.Term
		}

		reader, err := r.fsm.Snapshot()
		if err != nil {
			slog.Error("Failed capturing snapshot for peer transmission", "peer", targetID, "error", err)
			r.peerTracker.SetInFlight(targetID, false)
			return
		}
		defer reader.Close()

		streamHandler := func(stream consensuspb.RaftService_InstallSnapshotClient) error {
			buf := make([]byte, snapshotChunkSize)
			var offset uint64

			for {
				n, readErr := reader.Read(buf)
				if n > 0 {
					isDone := errors.Is(readErr, io.EOF)
					req := &consensuspb.InstallSnapshotRequest{
						Term:              currentTerm,
						LeaderId:          leaderID,
						LastIncludedIndex: lastApplied,
						LastIncludedTerm:  lastIncludedTerm,
						Offset:            offset,
						Data:              buf[:n],
						Done:              isDone,
					}

					if sendErr := stream.Send(req); sendErr != nil {
						return fmt.Errorf("failed sending snapshot chunk to %s: %w", targetID, sendErr)
					}
					offset += uint64(n)
				}
				if errors.Is(readErr, io.EOF) {
					break
				}
				if readErr != nil {
					return fmt.Errorf("error reading snapshot payload: %w", readErr)
				}
			}
			return nil
		}

		resp, err := r.transport.SendInstallSnapshot(context.Background(), targetID, streamHandler)
		if err != nil {
			slog.Warn("Failed streaming snapshot to peer", "peer", targetID, "error", err)
			r.peerTracker.SetInFlight(targetID, false)
			return
		}
		r.handleSnapshotResponse(targetID, lastApplied, resp)
	}(peerID)
}

func (r *RaftNode) handleSnapshotResponse(peerID string, lastIncludedIndex uint64, resp *consensuspb.InstallSnapshotResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if resp.Term > r.currentTerm {
		r.becomeFollower(resp.Term, "")
		return
	}

	if r.role != RoleLeader || resp.Term != r.currentTerm {
		r.peerTracker.SetInFlight(peerID, false)
		return
	}

	r.peerTracker.UpdateProgress(peerID, lastIncludedIndex)
	r.advanceCommitIndex()
	r.replicateToPeer(peerID)
}

func (r *RaftNode) HandleInstallSnapshot(ctx context.Context, stream consensuspb.RaftService_InstallSnapshotServer) error {
	firstReq, err := stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("failed receiving initial snapshot chunk: %w", err)
	}

	r.mu.Lock()
	if firstReq.Term < r.currentTerm {
		term := r.currentTerm
		r.mu.Unlock()
		return stream.SendAndClose(&consensuspb.InstallSnapshotResponse{Term: term})
	}

	if firstReq.Term > r.currentTerm || r.role != RoleFollower {
		r.becomeFollower(firstReq.Term, firstReq.LeaderId)
	}

	r.leaderID = firstReq.LeaderId
	r.resetElectionTimer()
	r.mu.Unlock()

	pipeReader, pipeWriter := io.Pipe()

	go func() {
		if len(firstReq.Data) > 0 {
			if _, writeErr := pipeWriter.Write(firstReq.Data); writeErr != nil {
				_ = pipeWriter.CloseWithError(writeErr)
				return
			}
		}

		if firstReq.Done {
			_ = pipeWriter.Close()
			return
		}

		for {
			req, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				_ = pipeWriter.Close()
				return
			}
			if recvErr != nil {
				_ = pipeWriter.CloseWithError(recvErr)
				return
			}

			if len(req.Data) > 0 {
				if _, writeErr := pipeWriter.Write(req.Data); writeErr != nil {
					_ = pipeWriter.CloseWithError(writeErr)
					return
				}
			}

			if req.Done {
				_ = pipeWriter.Close()
				return
			}
		}
	}()

	if restoreErr := r.fsm.Restore(pipeReader); restoreErr != nil {
		slog.Error("Failed restoring state machine from snapshot stream", "error", restoreErr)
		return fmt.Errorf("state machine restore failed: %w", restoreErr)
	}

	r.mu.Lock()
	lastIncludedIndex := firstReq.LastIncludedIndex
	if lastIncludedIndex > r.commitIndex {
		r.commitIndex = lastIncludedIndex
	}
	if lastIncludedIndex > r.lastApplied {
		r.lastApplied = lastIncludedIndex
	}

	if err := r.log.TruncatePrefix(lastIncludedIndex); err != nil {
		slog.Error("Failed truncating log prefix after snapshot install", "index", lastIncludedIndex, "error", err)
	}

	currentTerm := r.currentTerm
	r.mu.Unlock()

	slog.Info("Successfully installed snapshot from leader",
		"node", r.cfg.NodeID,
		"leader", firstReq.LeaderId,
		"lastIncludedIndex", lastIncludedIndex,
		"lastIncludedTerm", firstReq.LastIncludedTerm,
	)

	return stream.SendAndClose(&consensuspb.InstallSnapshotResponse{Term: currentTerm})
}
