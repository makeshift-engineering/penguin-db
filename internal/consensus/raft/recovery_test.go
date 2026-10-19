package raft

import (
	"context"
	"fmt"
	"testing"
	"time"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
)

// TestRecovery_FollowerCrashAndCatchUp verifies that when an active follower crashes,
// the surviving majority continues committing operations, and upon restarting, the crashed
// follower reloads its persisted consensus state and catches up to the latest log entries.
func TestRecovery_FollowerCrashAndCatchUp(t *testing.T) {
	cluster := newTestCluster(t, 3)

	leaderID := cluster.WaitLeader(2 * time.Second)

	// Identify a follower to crash.
	var crashedFollower string
	for _, id := range cluster.nodeIDs {
		if id != leaderID {
			crashedFollower = id
			break
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Propose initial entries before crash.
	if err := cluster.Propose(ctx, leaderID, "pre_crash_key", "pre_val"); err != nil {
		t.Fatalf("failed proposing initial write: %v", err)
	}

	for _, id := range cluster.nodeIDs {
		cluster.AssertValue(id, "pre_crash_key", "pre_val", 2*time.Second)
	}

	// Crash the follower by shutting down its node and closing its underlying storage engine.
	crashedNode := cluster.nodes[crashedFollower]
	_ = crashedNode.Stop()
	_ = cluster.engines[crashedFollower].Close()

	// While follower is down, propose additional entries to the remaining majority.
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("during_crash_%d", i)
		val := fmt.Sprintf("val_%d", i)
		if err := cluster.Propose(ctx, leaderID, key, val); err != nil {
			t.Fatalf("failed proposing write during follower crash: %v", err)
		}
	}

	// Restart the crashed follower, reopening its persistent log and LSM storage engine.
	cluster.RestartNode(crashedFollower)

	// Verify that the restarted follower re-synchronizes and contains both pre-crash and post-crash data.
	cluster.AssertValue(crashedFollower, "pre_crash_key", "pre_val", 3*time.Second)
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("during_crash_%d", i)
		val := fmt.Sprintf("val_%d", i)
		cluster.AssertValue(crashedFollower, key, val, 3*time.Second)
	}
}

// TestRecovery_LeaderCrashAndReElection verifies that when the active leader abruptly crashes,
// the remaining nodes elect a new leader, commit subsequent writes, and upon restart, the old
// leader rejoins as a follower and synchronizes with the new leader.
func TestRecovery_LeaderCrashAndReElection(t *testing.T) {
	cluster := newTestCluster(t, 3)

	oldLeader := cluster.WaitLeader(2 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Propose initial entries committed by the original leader.
	if err := cluster.Propose(ctx, oldLeader, "phase", "phase_1"); err != nil {
		t.Fatalf("failed proposing initial write to leader %s: %v", oldLeader, err)
	}

	for _, id := range cluster.nodeIDs {
		cluster.AssertValue(id, "phase", "phase_1", 2*time.Second)
	}

	// Crash the original leader.
	_ = cluster.nodes[oldLeader].Stop()
	_ = cluster.engines[oldLeader].Close()

	// Identify surviving nodes.
	var survivors []string
	for _, id := range cluster.nodeIDs {
		if id != oldLeader {
			survivors = append(survivors, id)
		}
	}

	// Wait for the surviving quorum to elect a replacement leader.
	var newLeader string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, id := range survivors {
			if cluster.nodes[id].IsLeader() {
				newLeader = id
				break
			}
		}
		if newLeader != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if newLeader == "" {
		t.Fatal("surviving nodes failed electing a replacement leader")
	}

	// Propose writes through the newly elected leader.
	if err := cluster.Propose(ctx, newLeader, "phase", "phase_2"); err != nil {
		t.Fatalf("failed proposing write to new leader %s: %v", newLeader, err)
	}

	for _, id := range survivors {
		cluster.AssertValue(id, "phase", "phase_2", 2*time.Second)
	}

	// Restart the original leader node.
	cluster.RestartNode(oldLeader)

	// Verify that the resurrected leader transitions to follower and catches up to phase 2.
	time.Sleep(100 * time.Millisecond)
	oldLeaderStatus := cluster.nodes[oldLeader].Status()
	if oldLeaderStatus.Role != RoleFollower {
		t.Fatalf("expected resurrected leader %s to adopt Follower role, got %s", oldLeader, oldLeaderStatus.Role)
	}

	cluster.AssertValue(oldLeader, "phase", "phase_2", 3*time.Second)
}

// TestRecovery_EntireClusterReboot verifies that when every node in the cluster shuts down,
// persistent log records and LSM state survive, a leader is re-elected, and new writes succeed.
func TestRecovery_EntireClusterReboot(t *testing.T) {
	cluster := newTestCluster(t, 3)

	leaderID := cluster.WaitLeader(2 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Commit entries prior to shutdown.
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("reboot_key_%d", i)
		val := fmt.Sprintf("reboot_val_%d", i)
		if err := cluster.Propose(ctx, leaderID, key, val); err != nil {
			t.Fatalf("failed proposing write %d before cluster reboot: %v", i, err)
		}
	}

	for _, id := range cluster.nodeIDs {
		for i := 1; i <= 3; i++ {
			key := fmt.Sprintf("reboot_key_%d", i)
			val := fmt.Sprintf("reboot_val_%d", i)
			cluster.AssertValue(id, key, val, 2*time.Second)
		}
	}

	// Shut down all nodes to simulate complete cluster power loss.
	for _, id := range cluster.nodeIDs {
		_ = cluster.nodes[id].Stop()
		_ = cluster.engines[id].Close()
	}

	time.Sleep(100 * time.Millisecond)

	// Re-start every node in sequence.
	for _, id := range cluster.nodeIDs {
		cluster.RestartNode(id)
	}

	// Wait for a leader to emerge in the rebooted cluster.
	newLeaderID := cluster.WaitLeader(3 * time.Second)
	if newLeaderID == "" {
		t.Fatal("cluster failed electing a leader following full reboot")
	}

	// Verify historical data survived and remains accessible on all nodes.
	for _, id := range cluster.nodeIDs {
		for i := 1; i <= 3; i++ {
			key := fmt.Sprintf("reboot_key_%d", i)
			val := fmt.Sprintf("reboot_val_%d", i)
			cluster.AssertValue(id, key, val, 2*time.Second)
		}
	}

	// Propose new writes to ensure write replication functions properly after reboot.
	postCtx, postCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer postCancel()

	if err := cluster.Propose(postCtx, newLeaderID, "post_reboot_key", "post_val"); err != nil {
		t.Fatalf("failed proposing write after reboot: %v", err)
	}

	for _, id := range cluster.nodeIDs {
		cluster.AssertValue(id, "post_reboot_key", "post_val", 2*time.Second)
	}
}

// TestRecovery_ConflictingUncommittedLogOverwrite verifies that uncommitted entries written
// by a crashed leader are overwritten after the node reboots and reconnects to a new leader.
func TestRecovery_ConflictingUncommittedLogOverwrite(t *testing.T) {
	cluster := newTestCluster(t, 3)

	initialLeader := cluster.WaitLeader(2 * time.Second)

	// Isolate the leader to prevent replication of the uncommitted write.
	cluster.Disconnect(initialLeader)

	// Inject an uncommitted entry directly into the isolated leader log.
	uncommittedEntry := &consensuspb.LogEntry{
		Index: 2,
		Term:  cluster.nodes[initialLeader].CurrentTerm(),
		Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
		Data:  []byte("divergent-abandoned-payload"),
	}
	if err := cluster.nodes[initialLeader].log.Append([]*consensuspb.LogEntry{uncommittedEntry}); err != nil {
		t.Fatalf("failed injecting uncommitted entry: %v", err)
	}

	// Stop the isolated leader.
	_ = cluster.nodes[initialLeader].Stop()
	_ = cluster.engines[initialLeader].Close()

	// Re-enable network connectivity before restarting so it can communicate once up.
	cluster.Reconnect(initialLeader)

	// Identify remaining nodes and elect a new leader.
	var survivors []string
	for _, id := range cluster.nodeIDs {
		if id != initialLeader {
			survivors = append(survivors, id)
		}
	}

	var newLeader string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, id := range survivors {
			if cluster.nodes[id].IsLeader() {
				newLeader = id
				break
			}
		}
		if newLeader != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if newLeader == "" {
		t.Fatal("failed electing new leader among survivors")
	}

	// Commit a legitimate write through the new leader.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := cluster.Propose(ctx, newLeader, "committed_key", "valid_payload"); err != nil {
		t.Fatalf("failed proposing write to new leader: %v", err)
	}

	for _, id := range survivors {
		cluster.AssertValue(id, "committed_key", "valid_payload", 2*time.Second)
	}

	// Restart the old leader.
	cluster.RestartNode(initialLeader)

	// Verify that the resurrected old leader rolls back its conflicting entry and applies the valid entry.
	cluster.AssertValue(initialLeader, "committed_key", "valid_payload", 3*time.Second)
}
