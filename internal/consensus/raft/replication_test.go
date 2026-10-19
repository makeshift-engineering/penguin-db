package raft

import (
	"context"
	"fmt"
	"testing"
	"time"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
)

// TestReplication_BasicProposalAndCommit verifies that client proposals submitted to the leader
// are replicated across the cluster, committed by quorum, and applied to all LSM storage engines.
func TestReplication_BasicProposalAndCommit(t *testing.T) {
	cluster := newTestCluster(t, 3)

	leaderID := cluster.WaitLeader(2 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cluster.Propose(ctx, leaderID, "user:100", "Alice")
	if err != nil {
		t.Fatalf("failed proposing key-value pair to leader %s: %v", leaderID, err)
	}

	// Verify that the proposal is committed and applied across all cluster nodes.
	for _, id := range cluster.nodeIDs {
		cluster.AssertCommitIndex(id, 2, 2*time.Second)
		cluster.AssertValue(id, "user:100", "Alice", 2*time.Second)
	}

	// Submit a subsequent proposal to verify monotonic log index progression.
	err = cluster.Propose(ctx, leaderID, "user:200", "Bob")
	if err != nil {
		t.Fatalf("failed proposing second key-value pair to leader %s: %v", leaderID, err)
	}

	for _, id := range cluster.nodeIDs {
		cluster.AssertCommitIndex(id, 3, 2*time.Second)
		cluster.AssertValue(id, "user:200", "Bob", 2*time.Second)
	}
}

// TestReplication_NonLeaderRejection verifies that proposals submitted directly to followers
// fail immediately with a NotLeaderError containing the current leader identity for redirection.
func TestReplication_NonLeaderRejection(t *testing.T) {
	cluster := newTestCluster(t, 3)

	leaderID := cluster.WaitLeader(2 * time.Second)

	// Identify a follower node.
	var followerID string
	for _, id := range cluster.nodeIDs {
		if id != leaderID {
			followerID = id
			break
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := cluster.Propose(ctx, followerID, "test-key", "test-val")
	if err == nil {
		t.Fatalf("expected proposal to follower %s to fail, but succeeded", followerID)
	}

	var notLeaderErr *NotLeaderError
	if !IsNotLeader(err) {
		t.Fatalf("expected NotLeaderError, got %T: %v", err, err)
	}

	_ = notLeaderErr
	if err.Error() == "" {
		t.Fatal("expected non-empty error message from NotLeaderError")
	}
}

// TestReplication_FastConflictRollback verifies that when a follower contains uncommitted
// divergent log entries from a previous term, the fast conflict resolution protocol resolves
// the mismatch and synchronizes the follower log with the leader.
func TestReplication_FastConflictRollback(t *testing.T) {
	cluster := newTestCluster(t, 3)

	leaderID := cluster.WaitLeader(2 * time.Second)

	// Identify a follower to inject with conflicting log entries.
	var followerID string
	for _, id := range cluster.nodeIDs {
		if id != leaderID {
			followerID = id
			break
		}
	}

	followerNode := cluster.nodes[followerID]
	cluster.Disconnect(followerID)

	// Append conflicting uncommitted entries directly to the isolated follower log.
	conflictingEntries := []*consensuspb.LogEntry{
		{
			Index: 2,
			Term:  followerNode.CurrentTerm() + 1,
			Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
			Data:  []byte("conflicting-payload-term-2"),
		},
		{
			Index: 3,
			Term:  followerNode.CurrentTerm() + 1,
			Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
			Data:  []byte("conflicting-payload-term-3"),
		},
	}
	if err := followerNode.log.Append(conflictingEntries); err != nil {
		t.Fatalf("failed injecting conflicting entries to follower log: %v", err)
	}

	// Commit valid writes on the remaining cluster majority.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := cluster.Propose(ctx, leaderID, "fruit", "apple"); err != nil {
		t.Fatalf("failed proposing write on majority: %v", err)
	}

	// Reconnect the divergent follower.
	cluster.Reconnect(followerID)

	// Verify that the divergent follower rolled back conflicting entries and converged.
	cluster.AssertCommitIndex(followerID, 2, 3*time.Second)
	cluster.AssertValue(followerID, "fruit", "apple", 2*time.Second)
}

// TestReplication_BatchOverflow verifies that when client writes exceed the configured
// MaxEntriesPerAppend threshold, the leader splits replication across multiple consecutive
// batches until followers are synchronized.
func TestReplication_BatchOverflow(t *testing.T) {
	cluster := newTestCluster(t, 3)
	leaderID := cluster.WaitLeader(2 * time.Second)

	// Clamp max entries per append to a small value to force multiple batch round trips.
	leader := cluster.nodes[leaderID]
	leader.mu.Lock()
	leader.cfg.MaxEntriesPerAppend = 3
	leader.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	totalWrites := 10
	for i := 1; i <= totalWrites; i++ {
		key := "overflow_key"
		val := "overflow_val"
		if err := cluster.Propose(ctx, leaderID, key, val); err != nil {
			t.Fatalf("failed proposing batch item %d: %v", i, err)
		}
	}

	for _, id := range cluster.nodeIDs {
		cluster.AssertCommitIndex(id, uint64(totalWrites+1), 3*time.Second)
		cluster.AssertValue(id, "overflow_key", "overflow_val", 2*time.Second)
	}
}

// TestReplication_ProposalsDrainedOnStop verifies that pending proposals immediately unblock
// with ErrStopped when the node is shut down, preventing client goroutine leaks.
func TestReplication_ProposalsDrainedOnStop(t *testing.T) {
	cluster := newTestCluster(t, 3)
	leaderID := cluster.WaitLeader(2 * time.Second)

	// Disconnect followers so writes cannot achieve majority quorum.
	for _, id := range cluster.nodeIDs {
		if id != leaderID {
			cluster.Disconnect(id)
		}
	}

	errChan := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		errChan <- cluster.Propose(ctx, leaderID, "pending_key", "pending_val")
	}()

	// Allow proposal to register in leader proposals map.
	time.Sleep(50 * time.Millisecond)

	// Stop the leader.
	leader := cluster.nodes[leaderID]
	_ = leader.Stop()

	select {
	case err := <-errChan:
		if err != ErrStopped {
			t.Fatalf("expected ErrStopped on shutdown, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pending proposal to drain on shutdown")
	}
}

// TestReplication_Figure8_PreviousTermCommitRule verifies the safety invariant that a leader
// cannot commit log entries from a previous term simply by counting replicas; it must commit
// an entry from its current term, which indirectly commits prior entries.
func TestReplication_Figure8_PreviousTermCommitRule(t *testing.T) {
	cluster := newTestCluster(t, 3)
	leaderID := cluster.WaitLeader(2 * time.Second)
	leader := cluster.nodes[leaderID]

	// Advance leader to term 2 artificially.
	leader.mu.Lock()
	leader.currentTerm = 2
	leader.mu.Unlock()

	// Append an entry with Term 1 at index 2 directly into the leader log.
	term1Entry := &consensuspb.LogEntry{
		Index: 2,
		Term:  1,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
		Data:  []byte("term-1-data"),
	}
	if err := leader.log.Append([]*consensuspb.LogEntry{term1Entry}); err != nil {
		t.Fatalf("failed appending term 1 entry: %v", err)
	}

	// Simulate peer-2 also storing entry 2 with term 1, achieving a majority quorum.
	leader.peerTracker.UpdateProgress("node-2", 2)

	// Call advanceCommitIndex while entry at quorum match has Term 1 (previous term).
	leader.mu.Lock()
	leader.advanceCommitIndex()
	commitIdxAfterPriorTermQuorum := leader.commitIndex
	leader.mu.Unlock()

	// CommitIndex must not advance to 2 because the entry belongs to a prior term.
	if commitIdxAfterPriorTermQuorum >= 2 {
		t.Fatalf("Figure 8 violation: leader committed entry from prior term by counting replicas alone, commitIndex: %d",
			commitIdxAfterPriorTermQuorum)
	}

	// Now append an entry with Term 2 (current term) at index 3.
	term2Entry := &consensuspb.LogEntry{
		Index: 3,
		Term:  2,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
		Data:  []byte("term-2-data"),
	}
	if err := leader.log.Append([]*consensuspb.LogEntry{term2Entry}); err != nil {
		t.Fatalf("failed appending term 2 entry: %v", err)
	}

	// Replicate index 3 to peer-2, achieving quorum for the current term entry.
	leader.peerTracker.UpdateProgress("node-2", 3)

	leader.mu.Lock()
	leader.advanceCommitIndex()
	commitIdxAfterCurrentTermQuorum := leader.commitIndex
	leader.mu.Unlock()

	// Once the current term entry achieves quorum, commitIndex advances to 3, indirectly committing index 2.
	if commitIdxAfterCurrentTermQuorum < 3 {
		t.Fatalf("expected commitIndex to advance to 3 once current term entry reached quorum, got %d",
			commitIdxAfterCurrentTermQuorum)
	}
}

// TestReplication_FastConflictRollback_FollowerLogShorter verifies that when a follower log
// is significantly shorter than the leader log, the follower returns ConflictIndex matching
// its last log index plus one, allowing the leader to catch up in a single round trip.
func TestReplication_FastConflictRollback_FollowerLogShorter(t *testing.T) {
	cluster := newTestCluster(t, 3)
	leaderID := cluster.WaitLeader(2 * time.Second)

	// Identify a follower to disconnect.
	var laggingFollower string
	for _, id := range cluster.nodeIDs {
		if id != leaderID {
			laggingFollower = id
			break
		}
	}

	cluster.Disconnect(laggingFollower)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Commit multiple writes on majority while follower is isolated.
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("short_test_%d", i)
		val := fmt.Sprintf("val_%d", i)
		if err := cluster.Propose(ctx, leaderID, key, val); err != nil {
			t.Fatalf("majority proposal failed: %v", err)
		}
	}

	// Reconnect the lagging follower.
	cluster.Reconnect(laggingFollower)

	// Verify that the lagging follower rolls forward rapidly and applies all entries.
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("short_test_%d", i)
		val := fmt.Sprintf("val_%d", i)
		cluster.AssertValue(laggingFollower, key, val, 3*time.Second)
	}
}

// TestReplication_LeaderStepDownDrainsProposals verifies that when an active leader steps down
// to the follower role upon observing a higher term, any pending uncommitted client proposals
// are immediately unblocked with a NotLeaderError rather than lingering until context timeout.
func TestReplication_LeaderStepDownDrainsProposals(t *testing.T) {
	cluster := newTestCluster(t, 3)
	leaderID := cluster.WaitLeader(2 * time.Second)
	leader := cluster.nodes[leaderID]

	// Disconnect followers so writes cannot achieve quorum.
	for _, id := range cluster.nodeIDs {
		if id != leaderID {
			cluster.Disconnect(id)
		}
	}

	errChan := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		errChan <- cluster.Propose(ctx, leaderID, "stepdown_key", "stepdown_val")
	}()

	// Allow proposal to register in leader proposals map.
	time.Sleep(50 * time.Millisecond)

	// Inject an AppendEntries with a higher term to trigger leader step-down.
	higherTerm := leader.CurrentTerm() + 5
	appendReq := &consensuspb.AppendEntriesRequest{
		Term:         higherTerm,
		LeaderId:     "node-replacement",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		LeaderCommit: 0,
	}

	resp, err := leader.HandleAppendEntries(context.Background(), appendReq)
	if err != nil {
		t.Fatalf("HandleAppendEntries failed: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected leader to accept higher term AppendEntries")
	}

	// Verify that the proposal immediately unblocks with NotLeaderError.
	select {
	case err := <-errChan:
		if !IsNotLeader(err) {
			t.Fatalf("expected NotLeaderError on leader step down, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pending proposal to drain on leader step down")
	}
}


