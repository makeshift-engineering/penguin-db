package raft

import (
	"context"
	"fmt"
	"testing"
	"time"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
)

// TestSnapshot_LocalCompaction verifies that capturing a point-in-time snapshot
// truncates the persistent consensus log prefix and advances the earliest retained log index.
func TestSnapshot_LocalCompaction(t *testing.T) {
	cluster := newTestCluster(t, 1, func(opts *ClusterConfigOptions) {
		opts.SnapshotThreshold = 5
	})

	leaderID := cluster.WaitLeader(2 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Propose multiple writes to build log entries.
	for i := 1; i <= 8; i++ {
		key := fmt.Sprintf("metric_%d", i)
		val := fmt.Sprintf("val_%d", i)
		if err := cluster.Propose(ctx, leaderID, key, val); err != nil {
			t.Fatalf("failed proposing write %d: %v", i, err)
		}
	}

	leaderNode := cluster.nodes[leaderID]
	cluster.AssertCommitIndex(leaderID, 9, 2*time.Second)

	// Explicitly trigger snapshot compaction.
	if err := leaderNode.TakeSnapshot(); err != nil {
		t.Fatalf("TakeSnapshot failed: %v", err)
	}

	// Verify that the log prefix was truncated and FirstIndex advanced past 1.
	firstIndex := leaderNode.log.FirstIndex()
	if firstIndex <= 1 {
		t.Fatalf("expected log FirstIndex to advance past 1 after snapshot compaction, got %d", firstIndex)
	}

	// Verify all data remains readable in the underlying LSM storage engine.
	for i := 1; i <= 8; i++ {
		key := fmt.Sprintf("metric_%d", i)
		val := fmt.Sprintf("val_%d", i)
		cluster.AssertValue(leaderID, key, val, 2*time.Second)
	}
}

// TestSnapshot_StreamingToLaggingFollower verifies that when a follower falls behind
// the leader's earliest retained log entry, the leader streams an InstallSnapshot RPC,
// and the follower restores its state machine and resumes normal replication.
func TestSnapshot_StreamingToLaggingFollower(t *testing.T) {
	cluster := newTestCluster(t, 3, func(opts *ClusterConfigOptions) {
		opts.SnapshotThreshold = 5
	})

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Propose enough entries on the remaining majority to trigger snapshot compaction on the leader.
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("item_%d", i)
		val := fmt.Sprintf("value_%d", i)
		if err := cluster.Propose(ctx, leaderID, key, val); err != nil {
			t.Fatalf("failed proposing key %d on majority: %v", i, err)
		}
	}

	leaderNode := cluster.nodes[leaderID]
	if err := leaderNode.TakeSnapshot(); err != nil {
		t.Fatalf("leader TakeSnapshot failed: %v", err)
	}

	firstIndex := leaderNode.log.FirstIndex()
	if firstIndex <= 1 {
		t.Fatalf("expected leader log FirstIndex to advance after snapshot compaction, got %d", firstIndex)
	}

	// Reconnect the lagging follower whose nextIndex is now behind the leader's earliest entry.
	cluster.Reconnect(laggingFollower)

	// Propose another write to stimulate replication and trigger InstallSnapshot streaming.
	_ = cluster.Propose(ctx, leaderID, "post_snapshot_key", "post_val")

	// Verify that the lagging follower installs the snapshot and synchronizes all keys.
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("item_%d", i)
		val := fmt.Sprintf("value_%d", i)
		cluster.AssertValue(laggingFollower, key, val, 5*time.Second)
	}

	cluster.AssertValue(laggingFollower, "post_snapshot_key", "post_val", 5*time.Second)
}

// TestSnapshot_StaleTermRejection verifies that incoming snapshots bearing a term older
// than the receiver current term are rejected without altering the log, commit index, or state machine.
func TestSnapshot_StaleTermRejection(t *testing.T) {
	cluster := newTestCluster(t, 2)
	cluster.WaitLeader(2 * time.Second)

	follower := cluster.nodes["node-2"]
	currentTerm := follower.CurrentTerm()
	if currentTerm == 0 {
		currentTerm = 1
	}

	stream := &mockSnapshotServerStream{
		reqChan:  make(chan *consensuspb.InstallSnapshotRequest, 2),
		respChan: make(chan *consensuspb.InstallSnapshotResponse, 1),
		ctx:      context.Background(),
	}

	// Send an InstallSnapshot request carrying a stale term.
	stream.reqChan <- &consensuspb.InstallSnapshotRequest{
		Term:              currentTerm - 1,
		LeaderId:          "node-stale",
		LastIncludedIndex: 50,
		LastIncludedTerm:  currentTerm - 1,
		Data:              []byte("dummy snapshot data"),
		Done:              true,
	}

	err := follower.HandleInstallSnapshot(context.Background(), stream)
	if err != nil {
		t.Fatalf("HandleInstallSnapshot returned unexpected error: %v", err)
	}

	select {
	case resp := <-stream.respChan:
		if resp.Term < currentTerm {
			t.Fatalf("expected response term >= %d, got %d", currentTerm, resp.Term)
		}
	default:
		t.Fatal("expected InstallSnapshotResponse on stream, got none")
	}

	// Verify that the follower status did not update its commit index to 50.
	status := follower.Status()
	if status.CommitIndex >= 50 {
		t.Fatalf("follower improperly accepted stale snapshot, commitIndex: %d", status.CommitIndex)
	}
}

// TestSnapshot_MultipleCompactionCycles verifies that continuous proposals across multiple
// threshold cycles repeatedly advance log prefix truncation while maintaining full state machine integrity.
func TestSnapshot_MultipleCompactionCycles(t *testing.T) {
	cluster := newTestCluster(t, 1, func(opts *ClusterConfigOptions) {
		opts.SnapshotThreshold = 4
	})

	leaderID := cluster.WaitLeader(2 * time.Second)
	leader := cluster.nodes[leaderID]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Propose entries across multiple compaction cycles.
	for i := 1; i <= 15; i++ {
		key := fmt.Sprintf("multi_%d", i)
		val := fmt.Sprintf("val_%d", i)
		if err := cluster.Propose(ctx, leaderID, key, val); err != nil {
			t.Fatalf("failed proposing write %d: %v", i, err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	// Explicitly invoke snapshot to compact all up to current applied.
	_ = leader.TakeSnapshot()

	firstIndex := leader.log.FirstIndex()
	if firstIndex <= 1 {
		t.Fatalf("expected log FirstIndex to advance past 1 across multiple compaction cycles, got %d", firstIndex)
	}

	// Verify that all historical keys remain readable from the state machine.
	for i := 1; i <= 15; i++ {
		key := fmt.Sprintf("multi_%d", i)
		val := fmt.Sprintf("val_%d", i)
		cluster.AssertValue(leaderID, key, val, 2*time.Second)
	}
}

