package raft

import (
	"context"
	"testing"
	"time"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
)

// TestElection_SingleNode verifies that a cluster initialized with a single node
// immediately transitions to leader and commits its initial no-op entry.
func TestElection_SingleNode(t *testing.T) {
	cluster := newTestCluster(t, 1)

	leaderID := cluster.WaitLeader(1 * time.Second)
	if leaderID != "node-1" {
		t.Fatalf("expected node-1 to be elected leader, got %s", leaderID)
	}

	leader := cluster.nodes[leaderID]
	status := leader.Status()

	if status.Role != RoleLeader {
		t.Fatalf("expected role Leader, got %s", status.Role)
	}
	if status.CurrentTerm < 1 {
		t.Fatalf("expected current term >= 1, got %d", status.CurrentTerm)
	}
	if status.CommitIndex < 1 {
		t.Fatalf("expected commit index >= 1 after leader no-op, got %d", status.CommitIndex)
	}
}

// TestElection_ThreeNodes verifies that a standard three-node cluster successfully
// elects exactly one leader while keeping all remaining peers in the follower role.
func TestElection_ThreeNodes(t *testing.T) {
	cluster := newTestCluster(t, 3)

	leaderID := cluster.WaitLeader(2 * time.Second)
	if leaderID == "" {
		t.Fatal("failed electing a leader within deadline")
	}

	time.Sleep(60 * time.Millisecond)

	leaderNode := cluster.nodes[leaderID]
	leaderTerm := leaderNode.CurrentTerm()

	followersCount := 0
	for id, node := range cluster.nodes {
		if id == leaderID {
			continue
		}
		status := node.Status()
		if status.Role != RoleFollower {
			t.Fatalf("expected peer %s to be Follower, got %s", id, status.Role)
		}
		if status.LeaderID != leaderID {
			t.Fatalf("expected peer %s to recognize leader %s, got %s", id, leaderID, status.LeaderID)
		}
		if status.CurrentTerm != leaderTerm {
			t.Fatalf("expected peer %s term %d to match leader term %d", id, status.CurrentTerm, leaderTerm)
		}
		followersCount++
	}

	if followersCount != 2 {
		t.Fatalf("expected 2 followers, got %d", followersCount)
	}
}

// TestElection_PreVote_PreventDisruption verifies that a partitioned follower isolated
// from the cluster does not advance its term, preventing disruption upon reconnection.
func TestElection_PreVote_PreventDisruption(t *testing.T) {
	cluster := newTestCluster(t, 3)

	initialLeader := cluster.WaitLeader(2 * time.Second)
	initialTerm := cluster.nodes[initialLeader].CurrentTerm()

	// Identify a follower to isolate.
	var isolatedFollower string
	for _, id := range cluster.nodeIDs {
		if id != initialLeader {
			isolatedFollower = id
			break
		}
	}

	cluster.Disconnect(isolatedFollower)

	// Allow several election timeouts to elapse while the follower is isolated.
	time.Sleep(300 * time.Millisecond)

	isolatedNode := cluster.nodes[isolatedFollower]
	isolatedStatus := isolatedNode.Status()

	// With Pre-Vote enabled, the isolated node should remain at the initial term
	// because it never secured a pre-vote quorum to advance to candidate.
	if isolatedStatus.CurrentTerm > initialTerm {
		t.Fatalf("expected isolated node term %d not to exceed cluster term %d",
			isolatedStatus.CurrentTerm, initialTerm)
	}

	// Reconnect the isolated follower.
	cluster.Reconnect(isolatedFollower)

	time.Sleep(150 * time.Millisecond)

	reconnectedStatus := isolatedNode.Status()
	if reconnectedStatus.Role != RoleFollower {
		t.Fatalf("expected reconnected node to settle as Follower, got %s", reconnectedStatus.Role)
	}
	if cluster.nodes[initialLeader].Status().Role != RoleLeader {
		t.Fatalf("initial leader %s was unexpectedly deposed after reconnection", initialLeader)
	}
}

// TestElection_LogUpToDateRule verifies that voters reject candidate vote requests
// if the candidate log term is older or its log index is shorter at equal terms.
func TestElection_LogUpToDateRule(t *testing.T) {
	cluster := newTestCluster(t, 2)
	leaderID := cluster.WaitLeader(2 * time.Second)

	var voterID, candidateID string
	for _, id := range cluster.nodeIDs {
		if id != leaderID {
			voterID = id
		} else {
			candidateID = id
		}
	}

	// Ensure voter has received the leader initial entry so its lastLogTerm is at least 1.
	cluster.AssertCommitIndex(voterID, 1, 2*time.Second)

	voter := cluster.nodes[voterID]

	// Candidate with an older last log term must be rejected.
	staleTermReq := &consensuspb.RequestVoteRequest{
		Term:         voter.CurrentTerm() + 1,
		CandidateId:  candidateID,
		LastLogIndex: 100,
		LastLogTerm:  0,
		IsPreVote:    false,
	}

	resp, err := voter.HandleRequestVote(context.Background(), staleTermReq)
	if err != nil {
		t.Fatalf("HandleRequestVote failed: %v", err)
	}
	if resp.VoteGranted {
		t.Fatal("expected voter to reject candidate with older last log term")
	}

	// Candidate with shorter index at the same term must be rejected.
	lastLogIndex, lastLogTerm := voter.log.LastIndexAndTerm()
	if lastLogIndex > 0 {
		shorterIndexReq := &consensuspb.RequestVoteRequest{
			Term:         voter.CurrentTerm() + 1,
			CandidateId:  candidateID,
			LastLogIndex: lastLogIndex - 1,
			LastLogTerm:  lastLogTerm,
			IsPreVote:    false,
		}

		resp, err = voter.HandleRequestVote(context.Background(), shorterIndexReq)
		if err != nil {
			t.Fatalf("HandleRequestVote failed: %v", err)
		}
		if resp.VoteGranted {
			t.Fatal("expected voter to reject candidate with shorter log index at equal term")
		}
	}
}

// TestElection_DiscoverHigherTerm verifies that any node immediately steps down
// to the follower role upon observing an RPC request carrying a higher term.
func TestElection_DiscoverHigherTerm(t *testing.T) {
	cluster := newTestCluster(t, 3)

	leaderID := cluster.WaitLeader(2 * time.Second)
	leader := cluster.nodes[leaderID]

	higherTerm := leader.CurrentTerm() + 5
	appendReq := &consensuspb.AppendEntriesRequest{
		Term:         higherTerm,
		LeaderId:     "node-peer",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      nil,
		LeaderCommit: 0,
	}

	resp, err := leader.HandleAppendEntries(context.Background(), appendReq)
	if err != nil {
		t.Fatalf("HandleAppendEntries failed: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected AppendEntries with higher term to be accepted")
	}

	status := leader.Status()
	if status.Role != RoleFollower {
		t.Fatalf("expected former leader to transition to Follower, got %s", status.Role)
	}
	if status.CurrentTerm != higherTerm {
		t.Fatalf("expected node to adopt higher term %d, got %d", higherTerm, status.CurrentTerm)
	}
}

// TestElection_VoteGrantOncePerTermAndIdempotency verifies that a voter grants at most one vote per term,
// rejects other candidates in that term, but idempotently re-grants the vote on duplicate RPC retries.
func TestElection_VoteGrantOncePerTermAndIdempotency(t *testing.T) {
	cluster := newTestCluster(t, 3)
	cluster.WaitLeader(2 * time.Second)

	voter := cluster.nodes["node-3"]
	targetTerm := voter.CurrentTerm() + 10

	firstVoteReq := &consensuspb.RequestVoteRequest{
		Term:         targetTerm,
		CandidateId:  "node-1",
		LastLogIndex: 100,
		LastLogTerm:  targetTerm - 1,
		IsPreVote:    false,
	}

	// First vote request must be granted.
	resp, err := voter.HandleRequestVote(context.Background(), firstVoteReq)
	if err != nil {
		t.Fatalf("HandleRequestVote failed: %v", err)
	}
	if !resp.VoteGranted {
		t.Fatal("expected first vote request to be granted")
	}

	// Second vote request from a different candidate in the same term must be rejected.
	competingVoteReq := &consensuspb.RequestVoteRequest{
		Term:         targetTerm,
		CandidateId:  "node-2",
		LastLogIndex: 100,
		LastLogTerm:  targetTerm - 1,
		IsPreVote:    false,
	}

	resp, err = voter.HandleRequestVote(context.Background(), competingVoteReq)
	if err != nil {
		t.Fatalf("HandleRequestVote failed: %v", err)
	}
	if resp.VoteGranted {
		t.Fatal("expected voter to reject competing candidate in the same term")
	}

	// Retransmitted vote request from the original candidate must be granted idempotently.
	retryResp, err := voter.HandleRequestVote(context.Background(), firstVoteReq)
	if err != nil {
		t.Fatalf("HandleRequestVote failed: %v", err)
	}
	if !retryResp.VoteGranted {
		t.Fatal("expected retry vote request to be granted idempotently")
	}
}

// TestElection_CandidateReceivesAppendEntries verifies that a candidate immediately reverts to follower
// upon receiving an AppendEntries RPC from a legitimate leader in the same term, but rejects stale leaders.
func TestElection_CandidateReceivesAppendEntries(t *testing.T) {
	cluster := newTestCluster(t, 2)
	cluster.WaitLeader(2 * time.Second)

	node := cluster.nodes["node-2"]

	// Force node into candidate role.
	node.mu.Lock()
	node.becomeCandidate()
	candidateTerm := node.currentTerm
	node.mu.Unlock()

	if !node.IsLeader() && node.Status().Role != RoleCandidate {
		t.Fatalf("expected role Candidate, got %s", node.Status().Role)
	}

	// Stale AppendEntries with older term must be rejected and not cause step-down.
	staleReq := &consensuspb.AppendEntriesRequest{
		Term:         candidateTerm - 1,
		LeaderId:     "node-1",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		LeaderCommit: 0,
	}
	staleResp, err := node.HandleAppendEntries(context.Background(), staleReq)
	if err != nil {
		t.Fatalf("HandleAppendEntries failed: %v", err)
	}
	if staleResp.Success {
		t.Fatal("expected candidate to reject stale AppendEntries")
	}
	if node.Status().Role != RoleCandidate {
		t.Fatalf("candidate unexpectedly stepped down on stale RPC, role: %s", node.Status().Role)
	}

	// Valid AppendEntries from the legitimate leader in the current term must cause immediate step-down.
	validReq := &consensuspb.AppendEntriesRequest{
		Term:         candidateTerm,
		LeaderId:     "node-1",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		LeaderCommit: 0,
	}
	validResp, err := node.HandleAppendEntries(context.Background(), validReq)
	if err != nil {
		t.Fatalf("HandleAppendEntries failed: %v", err)
	}
	if !validResp.Success {
		t.Fatal("expected candidate to accept valid leader AppendEntries")
	}
	if node.Status().Role != RoleFollower {
		t.Fatalf("expected candidate to step down to Follower, got %s", node.Status().Role)
	}
}

// TestElection_PreVote_LeaderLeaseProtection verifies that voters reject pre-vote requests
// if they have heard from a valid leader within the minimum election timeout.
func TestElection_PreVote_LeaderLeaseProtection(t *testing.T) {
	cluster := newTestCluster(t, 2)
	cluster.WaitLeader(2 * time.Second)

	voter := cluster.nodes["node-2"]

	// Send heartbeat to establish fresh lease.
	heartbeat := &consensuspb.AppendEntriesRequest{
		Term:         voter.CurrentTerm(),
		LeaderId:     "node-1",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		LeaderCommit: 0,
	}
	_, err := voter.HandleAppendEntries(context.Background(), heartbeat)
	if err != nil {
		t.Fatalf("heartbeat delivery failed: %v", err)
	}

	// Immediately probe with pre-vote.
	preVoteReq := &consensuspb.RequestVoteRequest{
		Term:         voter.CurrentTerm() + 1,
		CandidateId:  "node-disruptor",
		LastLogIndex: 100,
		LastLogTerm:  voter.CurrentTerm(),
		IsPreVote:    true,
	}

	resp, err := voter.HandleRequestVote(context.Background(), preVoteReq)
	if err != nil {
		t.Fatalf("HandleRequestVote failed: %v", err)
	}
	if resp.VoteGranted {
		t.Fatal("expected pre-vote to be rejected under active leader lease")
	}
}

// TestElection_StaleRequestVoteRejection verifies that a voter immediately rejects RequestVote
// RPCs carrying a term older than its currentTerm, returning its own higher term.
func TestElection_StaleRequestVoteRejection(t *testing.T) {
	cluster := newTestCluster(t, 2)
	cluster.WaitLeader(2 * time.Second)

	voter := cluster.nodes["node-2"]
	currentTerm := voter.CurrentTerm()

	staleVoteReq := &consensuspb.RequestVoteRequest{
		Term:         currentTerm - 1,
		CandidateId:  "node-stale",
		LastLogIndex: 100,
		LastLogTerm:  currentTerm - 1,
		IsPreVote:    false,
	}

	resp, err := voter.HandleRequestVote(context.Background(), staleVoteReq)
	if err != nil {
		t.Fatalf("HandleRequestVote failed: %v", err)
	}
	if resp.VoteGranted {
		t.Fatal("expected voter to reject stale RequestVote")
	}
	if resp.Term < currentTerm {
		t.Fatalf("expected response term >= %d, got %d", currentTerm, resp.Term)
	}
}

// TestElection_CandidateElectionTimeoutRetry verifies that when an isolated node fails to
// acquire quorum, its election timers fire cleanly and repeatedly, and upon reconnection,
// the cluster converges to a stable leader without deadlock.
func TestElection_CandidateElectionTimeoutRetry(t *testing.T) {
	cluster := newTestCluster(t, 3)

	// Isolate node-1 completely so it cannot communicate with peers.
	cluster.Disconnect("node-1")

	node := cluster.nodes["node-1"]

	// Allow multiple election timeouts to elapse while isolated.
	time.Sleep(300 * time.Millisecond)

	status := node.Status()
	if status.Role == RoleLeader {
		t.Fatal("isolated node unexpectedly became leader without quorum")
	}

	// Reconnect node-1 and verify cluster stabilizes with a valid leader.
	cluster.Reconnect("node-1")

	leaderID := cluster.WaitLeader(2 * time.Second)
	if leaderID == "" {
		t.Fatal("cluster failed electing a leader after reconnection")
	}
}


