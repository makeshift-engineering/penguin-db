package raft

import (
	"context"
	"testing"
	"time"
)

// TestPartition_MajorityContinuesMinorityBlocks verifies that when a cluster is partitioned,
// the majority partition continues committing operations while the minority partition is blocked.
func TestPartition_MajorityContinuesMinorityBlocks(t *testing.T) {
	cluster := newTestCluster(t, 3)

	leaderID := cluster.WaitLeader(2 * time.Second)

	// Determine nodes for majority partition (leader + one peer) and minority partition (isolated peer).
	var majorityPeer, minorityPeer string
	for _, id := range cluster.nodeIDs {
		if id != leaderID {
			if majorityPeer == "" {
				majorityPeer = id
			} else {
				minorityPeer = id
			}
		}
	}

	// Sever network links between the minority node and the majority partition.
	cluster.Partition([]string{minorityPeer}, []string{leaderID, majorityPeer})

	// Propose a write to the leader residing within the majority partition.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cluster.Propose(ctx, leaderID, "region", "us-east")
	if err != nil {
		t.Fatalf("write to majority partition failed: %v", err)
	}

	// Verify that the majority partition successfully committed and applied the entry.
	cluster.AssertCommitIndex(leaderID, 2, 2*time.Second)
	cluster.AssertValue(leaderID, "region", "us-east", 2*time.Second)
	cluster.AssertCommitIndex(majorityPeer, 2, 2*time.Second)
	cluster.AssertValue(majorityPeer, "region", "us-east", 2*time.Second)

	// Attempting to write directly to the isolated minority node must fail.
	err = cluster.Propose(ctx, minorityPeer, "region", "us-west")
	if err == nil {
		t.Fatalf("expected write to minority node %s to fail, but succeeded", minorityPeer)
	}

	// Reconnect the minority node and verify that it synchronizes with the majority log.
	cluster.Heal()

	cluster.AssertCommitIndex(minorityPeer, 2, 3*time.Second)
	cluster.AssertValue(minorityPeer, "region", "us-east", 2*time.Second)
}

// TestPartition_LeaderIsolationAndReelection verifies that when the active leader is isolated
// into a minority partition, the remaining majority elects a new leader, and the deposed leader
// steps down upon partition healing.
func TestPartition_LeaderIsolationAndReelection(t *testing.T) {
	cluster := newTestCluster(t, 3)

	oldLeader := cluster.WaitLeader(2 * time.Second)

	// Identify the remaining two nodes that form the majority partition.
	var majorityNodes []string
	for _, id := range cluster.nodeIDs {
		if id != oldLeader {
			majorityNodes = append(majorityNodes, id)
		}
	}

	// Isolate the old leader from the majority.
	cluster.Partition([]string{oldLeader}, majorityNodes)

	// Allow the majority partition to experience an election timeout and elect a new leader.
	time.Sleep(200 * time.Millisecond)

	var newLeader string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, id := range majorityNodes {
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
		t.Fatal("majority partition failed electing a replacement leader")
	}
	if newLeader == oldLeader {
		t.Fatalf("new leader unexpectedly equals isolated old leader %s", oldLeader)
	}

	// Commit a write through the new leader on the majority partition.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cluster.Propose(ctx, newLeader, "active_leader", newLeader)
	if err != nil {
		t.Fatalf("failed proposing write to new leader %s: %v", newLeader, err)
	}

	for _, id := range majorityNodes {
		cluster.AssertValue(id, "active_leader", newLeader, 2*time.Second)
	}

	// Heal the partition so the old leader reconnects with the cluster.
	cluster.Heal()

	time.Sleep(150 * time.Millisecond)

	// Verify that the old leader stepped down to follower and converged on the new state.
	oldLeaderStatus := cluster.nodes[oldLeader].Status()
	if oldLeaderStatus.Role != RoleFollower {
		t.Fatalf("expected old leader %s to step down to Follower, got %s", oldLeader, oldLeaderStatus.Role)
	}

	cluster.AssertValue(oldLeader, "active_leader", newLeader, 2*time.Second)
}

// TestPartition_SplitBrainResistance verifies that across an asymmetric partition,
// a two-node minority partition cannot elect a leader or commit entries.
func TestPartition_SplitBrainResistance(t *testing.T) {
	cluster := newTestCluster(t, 5)

	initialLeader := cluster.WaitLeader(2 * time.Second)

	// Partition into minority of two nodes and majority of three nodes.
	var minorityGroup, majorityGroup []string
	for _, id := range cluster.nodeIDs {
		if len(minorityGroup) < 2 && id != initialLeader {
			minorityGroup = append(minorityGroup, id)
		} else {
			majorityGroup = append(majorityGroup, id)
		}
	}

	cluster.Partition(minorityGroup, majorityGroup)

	// Sleep past multiple election timeouts.
	time.Sleep(250 * time.Millisecond)

	// Verify that neither of the nodes in the minority partition claims to be leader.
	for _, id := range minorityGroup {
		if cluster.nodes[id].IsLeader() {
			t.Fatalf("minority partition node %s claimed leadership, violating split-brain safety", id)
		}
	}

	// Verify that the majority partition remains operational and capable of committing.
	var activeLeader string
	for _, id := range majorityGroup {
		if cluster.nodes[id].IsLeader() {
			activeLeader = id
			break
		}
	}

	if activeLeader == "" {
		t.Fatal("expected majority partition to maintain an active leader")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cluster.Propose(ctx, activeLeader, "quorum_test", "passed")
	if err != nil {
		t.Fatalf("majority partition proposal failed: %v", err)
	}

	for _, id := range majorityGroup {
		cluster.AssertValue(id, "quorum_test", "passed", 2*time.Second)
	}

	// Heal the entire cluster and verify full five-node convergence.
	cluster.Heal()

	for _, id := range cluster.nodeIDs {
		cluster.AssertValue(id, "quorum_test", "passed", 3*time.Second)
	}
}
