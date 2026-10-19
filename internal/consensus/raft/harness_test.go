package raft

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
	"github.com/makeshift-engineering/penguin-db/internal/config"
	"github.com/makeshift-engineering/penguin-db/internal/consensus/log"
	"github.com/makeshift-engineering/penguin-db/internal/consensus/statemachine"
	"github.com/makeshift-engineering/penguin-db/internal/storage"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// mockSnapshotClientStream implements the client streaming handle for InstallSnapshot RPCs.
type mockSnapshotClientStream struct {
	reqChan  chan *consensuspb.InstallSnapshotRequest
	respChan chan *consensuspb.InstallSnapshotResponse
	errChan  chan error
	ctx      context.Context
}

func (s *mockSnapshotClientStream) Send(req *consensuspb.InstallSnapshotRequest) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.reqChan <- req:
		return nil
	}
}

func (s *mockSnapshotClientStream) CloseAndRecv() (*consensuspb.InstallSnapshotResponse, error) {
	close(s.reqChan)
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case err := <-s.errChan:
		return nil, err
	case resp := <-s.respChan:
		return resp, nil
	}
}

func (s *mockSnapshotClientStream) Header() (metadata.MD, error) { return nil, nil }
func (s *mockSnapshotClientStream) Trailer() metadata.MD         { return nil }
func (s *mockSnapshotClientStream) CloseSend() error             { return nil }
func (s *mockSnapshotClientStream) Context() context.Context     { return s.ctx }
func (s *mockSnapshotClientStream) SendMsg(m any) error          { return nil }
func (s *mockSnapshotClientStream) RecvMsg(m any) error          { return nil }

// mockSnapshotServerStream implements the server streaming handle for InstallSnapshot RPCs.
type mockSnapshotServerStream struct {
	reqChan  chan *consensuspb.InstallSnapshotRequest
	respChan chan *consensuspb.InstallSnapshotResponse
	ctx      context.Context
}

func (s *mockSnapshotServerStream) Recv() (*consensuspb.InstallSnapshotRequest, error) {
	req, ok := <-s.reqChan
	if !ok {
		return nil, io.EOF
	}
	return req, nil
}

func (s *mockSnapshotServerStream) SendAndClose(resp *consensuspb.InstallSnapshotResponse) error {
	s.respChan <- resp
	return nil
}

func (s *mockSnapshotServerStream) SetHeader(metadata.MD) error  { return nil }
func (s *mockSnapshotServerStream) SendHeader(metadata.MD) error { return nil }
func (s *mockSnapshotServerStream) SetTrailer(metadata.MD)       {}
func (s *mockSnapshotServerStream) Context() context.Context     { return s.ctx }
func (s *mockSnapshotServerStream) SendMsg(m any) error          { return nil }
func (s *mockSnapshotServerStream) RecvMsg(m any) error          { return nil }

// mockNetwork simulates a fully connected or partitioned in-memory mesh network.
type mockNetwork struct {
	mu           sync.RWMutex
	nodes        map[string]*RaftNode
	disconnected map[string]bool
	partitions   map[string]map[string]bool
}

func newMockNetwork() *mockNetwork {
	return &mockNetwork{
		nodes:        make(map[string]*RaftNode),
		disconnected: make(map[string]bool),
		partitions:   make(map[string]map[string]bool),
	}
}

func (n *mockNetwork) registerNode(nodeID string, node *RaftNode) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.nodes[nodeID] = node
}

func (n *mockNetwork) isBlocked(from, to string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.disconnected[from] || n.disconnected[to] {
		return true
	}
	if blockedPeers, exists := n.partitions[from]; exists && blockedPeers[to] {
		return true
	}
	return false
}

// mockTransport implements the Transport interface routed directly through the in-memory mock network.
type mockTransport struct {
	localID string
	network *mockNetwork
}

func (t *mockTransport) SendRequestVote(ctx context.Context, to string, req *consensuspb.RequestVoteRequest) (*consensuspb.RequestVoteResponse, error) {
	if t.network.isBlocked(t.localID, to) {
		return nil, errors.New("network link unreachable: destination partitioned")
	}

	t.network.mu.RLock()
	targetNode, exists := t.network.nodes[to]
	t.network.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnknownPeer, to)
	}

	resp, err := targetNode.HandleRequestVote(ctx, req)
	if err != nil {
		return nil, err
	}

	if t.network.isBlocked(to, t.localID) {
		return nil, errors.New("network link unreachable: return path partitioned")
	}
	return resp, nil
}

func (t *mockTransport) SendAppendEntries(ctx context.Context, to string, req *consensuspb.AppendEntriesRequest) (*consensuspb.AppendEntriesResponse, error) {
	if t.network.isBlocked(t.localID, to) {
		return nil, errors.New("network link unreachable: destination partitioned")
	}

	t.network.mu.RLock()
	targetNode, exists := t.network.nodes[to]
	t.network.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnknownPeer, to)
	}

	resp, err := targetNode.HandleAppendEntries(ctx, req)
	if err != nil {
		return nil, err
	}

	if t.network.isBlocked(to, t.localID) {
		return nil, errors.New("network link unreachable: return path partitioned")
	}
	return resp, nil
}

func (t *mockTransport) SendInstallSnapshot(ctx context.Context, to string, streamHandler func(consensuspb.RaftService_InstallSnapshotClient) error) (*consensuspb.InstallSnapshotResponse, error) {
	if t.network.isBlocked(t.localID, to) {
		return nil, errors.New("network link unreachable: destination partitioned")
	}

	t.network.mu.RLock()
	targetNode, exists := t.network.nodes[to]
	t.network.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnknownPeer, to)
	}

	reqChan := make(chan *consensuspb.InstallSnapshotRequest, 16)
	respChan := make(chan *consensuspb.InstallSnapshotResponse, 1)
	errChan := make(chan error, 1)

	clientStream := &mockSnapshotClientStream{
		reqChan:  reqChan,
		respChan: respChan,
		errChan:  errChan,
		ctx:      ctx,
	}

	serverStream := &mockSnapshotServerStream{
		reqChan:  reqChan,
		respChan: respChan,
		ctx:      ctx,
	}

	go func() {
		if serverErr := targetNode.HandleInstallSnapshot(ctx, serverStream); serverErr != nil {
			errChan <- serverErr
		}
	}()

	if handlerErr := streamHandler(clientStream); handlerErr != nil {
		return nil, handlerErr
	}

	resp, err := clientStream.CloseAndRecv()
	if err != nil {
		return nil, err
	}

	if t.network.isBlocked(to, t.localID) {
		return nil, errors.New("network link unreachable: return path partitioned")
	}
	return resp, nil
}

func (t *mockTransport) Close() error {
	return nil
}

// TestCluster provides a self-contained multi-node virtual Raft cluster for deterministic integration testing.
type TestCluster struct {
	t          *testing.T
	network    *mockNetwork
	nodes      map[string]*RaftNode
	engines    map[string]storage.Engine
	logs       map[string]log.RaftLog
	cfgs       map[string]*config.RaftConfig
	engineDirs map[string]string
	logDirs    map[string]string
	nodeIDs    []string
}

// ClusterConfigOptions permits tailoring node parameters for specific test scenarios.
type ClusterConfigOptions struct {
	SnapshotThreshold uint64
	ElectionMin       time.Duration
	ElectionMax       time.Duration
	HeartbeatInterval time.Duration
	DisablePreVote    bool
}

// newTestCluster constructs, wires, and starts a virtual Raft cluster of the specified size.
func newTestCluster(t *testing.T, size int, optFns ...func(*ClusterConfigOptions)) *TestCluster {
	t.Helper()

	opts := ClusterConfigOptions{
		SnapshotThreshold: 50,
		ElectionMin:       60 * time.Millisecond,
		ElectionMax:       120 * time.Millisecond,
		HeartbeatInterval: 20 * time.Millisecond,
		DisablePreVote:    false,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	network := newMockNetwork()
	cluster := &TestCluster{
		t:          t,
		network:    network,
		nodes:      make(map[string]*RaftNode),
		engines:    make(map[string]storage.Engine),
		logs:       make(map[string]log.RaftLog),
		cfgs:       make(map[string]*config.RaftConfig),
		engineDirs: make(map[string]string),
		logDirs:    make(map[string]string),
		nodeIDs:    make([]string, size),
	}

	peerAddresses := make(map[string]string, size)
	for i := 0; i < size; i++ {
		id := fmt.Sprintf("node-%d", i+1)
		cluster.nodeIDs[i] = id
		peerAddresses[id] = fmt.Sprintf("inproc://%s", id)
	}

	for _, id := range cluster.nodeIDs {
		nodePeers := make(map[string]string)
		for peerID, addr := range peerAddresses {
			if peerID != id {
				nodePeers[peerID] = addr
			}
		}

		nodeCfg := &config.RaftConfig{
			NodeID:              id,
			Peers:               nodePeers,
			ElectionTimeoutMin:  opts.ElectionMin,
			ElectionTimeoutMax:  opts.ElectionMax,
			HeartbeatInterval:   opts.HeartbeatInterval,
			SnapshotThreshold:   opts.SnapshotThreshold,
			MaxEntriesPerAppend: 50,
			PreVote:             !opts.DisablePreVote,
		}

		engineDir := t.TempDir()
		engine, err := storage.NewEngine(engineDir, storage.DefaultOptions())
		if err != nil {
			t.Fatalf("failed instantiating test storage engine for %s: %v", id, err)
		}
		cluster.engines[id] = engine

		fsm := statemachine.NewLSMStateMachine(engine)

		logDir := t.TempDir()
		persistentLog, err := log.OpenPersistentLog(logDir)
		if err != nil {
			t.Fatalf("failed instantiating persistent log for %s: %v", id, err)
		}
		cluster.logs[id] = persistentLog
		cluster.cfgs[id] = nodeCfg
		cluster.engineDirs[id] = engineDir
		cluster.logDirs[id] = logDir

		transport := &mockTransport{localID: id, network: network}

		node, err := NewRaftNode(nodeCfg, persistentLog, fsm, transport)
		if err != nil {
			t.Fatalf("failed instantiating raft node for %s: %v", id, err)
		}
		cluster.nodes[id] = node
		network.registerNode(id, node)
	}

	for _, node := range cluster.nodes {
		if err := node.Start(); err != nil {
			t.Fatalf("failed starting raft node %s: %v", node.cfg.NodeID, err)
		}
	}

	t.Cleanup(func() {
		cluster.Close()
	})

	return cluster
}

// WaitLeader polls the virtual cluster until exactly one node has established leadership, returning its ID.
func (c *TestCluster) WaitLeader(timeout time.Duration) string {
	c.t.Helper()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var currentLeader string
		leadersCount := 0

		for _, id := range c.nodeIDs {
			node := c.nodes[id]
			if node.IsLeader() {
				currentLeader = id
				leadersCount++
			}
		}

		if leadersCount == 1 {
			return currentLeader
		}

		time.Sleep(10 * time.Millisecond)
	}

	c.t.Fatalf("timed out waiting for stable leader within %v (nodes: %+v)", timeout, c.dumpStatus())
	return ""
}

// AssertNoLeader verifies that no node claims leadership during the given inspection window.
func (c *TestCluster) AssertNoLeader(duration time.Duration) {
	c.t.Helper()
	deadline := time.Now().Add(duration)

	for time.Now().Before(deadline) {
		for _, id := range c.nodeIDs {
			if c.nodes[id].IsLeader() {
				c.t.Fatalf("expected no leader, but %s is leader (status: %+v)", id, c.nodes[id].Status())
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Partition severs all communication between two disjoint node groupings.
func (c *TestCluster) Partition(groupA, groupB []string) {
	c.network.mu.Lock()
	defer c.network.mu.Unlock()

	for _, a := range groupA {
		if c.network.partitions[a] == nil {
			c.network.partitions[a] = make(map[string]bool)
		}
		for _, b := range groupB {
			c.network.partitions[a][b] = true
			if c.network.partitions[b] == nil {
				c.network.partitions[b] = make(map[string]bool)
			}
			c.network.partitions[b][a] = true
		}
	}
}

// Heal restores all network partitions across the entire mock network.
func (c *TestCluster) Heal() {
	c.network.mu.Lock()
	defer c.network.mu.Unlock()
	c.network.partitions = make(map[string]map[string]bool)
	c.network.disconnected = make(map[string]bool)
}

// Disconnect completely isolates a single node from all cluster peers.
func (c *TestCluster) Disconnect(nodeID string) {
	c.network.mu.Lock()
	defer c.network.mu.Unlock()
	c.network.disconnected[nodeID] = true
}

// Reconnect restores network connectivity for an isolated node.
func (c *TestCluster) Reconnect(nodeID string) {
	c.network.mu.Lock()
	defer c.network.mu.Unlock()
	delete(c.network.disconnected, nodeID)
}

// Propose serializes a key-value write operation into a protobuf WriteBatchRequest and submits it to the target node.
func (c *TestCluster) Propose(ctx context.Context, targetID string, key, value string) error {
	node, exists := c.nodes[targetID]
	if !exists {
		return fmt.Errorf("unknown target node: %s", targetID)
	}

	batch := &storagepb.WriteBatchRequest{
		Operations: []*storagepb.Op{
			{
				Type:  storagepb.OpType_OP_TYPE_PUT,
				Key:   []byte(key),
				Value: []byte(value),
			},
		},
	}

	data, err := proto.Marshal(batch)
	if err != nil {
		return fmt.Errorf("failed marshaling write batch: %w", err)
	}

	return node.Propose(ctx, data)
}

// AssertCommitIndex waits until the specified node reaches at least the expected commit index.
func (c *TestCluster) AssertCommitIndex(nodeID string, expectedMinIndex uint64, timeout time.Duration) {
	c.t.Helper()
	deadline := time.Now().Add(timeout)
	node := c.nodes[nodeID]

	for time.Now().Before(deadline) {
		status := node.Status()
		if status.CommitIndex >= expectedMinIndex {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	c.t.Fatalf("node %s failed reaching commit index %d within %v (current status: %+v)",
		nodeID, expectedMinIndex, timeout, node.Status())
}

// AssertValue verifies that the underlying LSM storage engine of the specified node contains the expected key-value pair.
func (c *TestCluster) AssertValue(nodeID string, key, expectedValue string, timeout time.Duration) {
	c.t.Helper()
	deadline := time.Now().Add(timeout)
	engine := c.engines[nodeID]

	for time.Now().Before(deadline) {
		val, err := engine.Get([]byte(key))
		if err == nil && string(val) == expectedValue {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	val, err := engine.Get([]byte(key))
	if err != nil {
		c.t.Fatalf("failed reading key %q from %s within %v: %v", key, nodeID, timeout, err)
	}
	if string(val) != expectedValue {
		c.t.Fatalf("value mismatch on %s for key %q: expected %q, got %q", nodeID, key, expectedValue, string(val))
	}
}

// dumpStatus collects diagnostic summaries of all nodes for failure reporting.
func (c *TestCluster) dumpStatus() map[string]NodeStatus {
	statuses := make(map[string]NodeStatus, len(c.nodes))
	for id, node := range c.nodes {
		statuses[id] = node.Status()
	}
	return statuses
}

// Close gracefully stops all virtual Raft nodes and cleans up resources.
func (c *TestCluster) Close() {
	for _, node := range c.nodes {
		_ = node.Stop()
	}
	for _, engine := range c.engines {
		_ = engine.Close()
	}
}

// RestartNode simulates a crash and reboot of the specified node, reopening its persistent storage.
func (c *TestCluster) RestartNode(nodeID string) {
	c.t.Helper()

	oldNode := c.nodes[nodeID]
	_ = oldNode.Stop()

	oldEngine := c.engines[nodeID]
	_ = oldEngine.Close()

	time.Sleep(50 * time.Millisecond)

	newEngine, err := storage.NewEngine(c.engineDirs[nodeID], storage.DefaultOptions())
	if err != nil {
		c.t.Fatalf("failed re-opening storage engine for %s: %v", nodeID, err)
	}
	c.engines[nodeID] = newEngine

	fsm := statemachine.NewLSMStateMachine(newEngine)

	newLog, err := log.OpenPersistentLog(c.logDirs[nodeID])
	if err != nil {
		c.t.Fatalf("failed re-opening persistent log for %s: %v", nodeID, err)
	}
	c.logs[nodeID] = newLog

	transport := &mockTransport{localID: nodeID, network: c.network}

	newNode, err := NewRaftNode(c.cfgs[nodeID], newLog, fsm, transport)
	if err != nil {
		c.t.Fatalf("failed re-instantiating raft node for %s: %v", nodeID, err)
	}

	c.nodes[nodeID] = newNode
	c.network.registerNode(nodeID, newNode)

	if err := newNode.Start(); err != nil {
		c.t.Fatalf("failed restarting raft node %s: %v", nodeID, err)
	}
}
