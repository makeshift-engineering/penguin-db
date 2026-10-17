package raft

import (
	"context"
	"fmt"
	"sync"
	"time"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Transport interface {
	SendRequestVote(ctx context.Context, to string, req *consensuspb.RequestVoteRequest) (*consensuspb.RequestVoteResponse, error)
	SendAppendEntries(ctx context.Context, to string, req *consensuspb.AppendEntriesRequest) (*consensuspb.AppendEntriesResponse, error)
	SendInstallSnapshot(ctx context.Context, to string, streamHandler func(consensuspb.RaftService_InstallSnapshotClient) error) (*consensuspb.InstallSnapshotResponse, error)
	Close() error
}

type GRPCOutboundTransport struct {
	mu      sync.RWMutex
	conns   map[string]*grpc.ClientConn
	clients map[string]consensuspb.RaftServiceClient
	addrs   map[string]string
	timeout time.Duration
	closed  bool
}

func NewGRPCOutboundTransport(peerAddresses map[string]string, rpcTimeout time.Duration) *GRPCOutboundTransport {
	if rpcTimeout <= 0 {
		rpcTimeout = 200 * time.Millisecond
	}

	return &GRPCOutboundTransport{
		conns:   make(map[string]*grpc.ClientConn),
		clients: make(map[string]consensuspb.RaftServiceClient),
		addrs:   peerAddresses,
		timeout: rpcTimeout,
	}
}

func (t *GRPCOutboundTransport) getClient(peerID string) (consensuspb.RaftServiceClient, error) {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return nil, ErrStopped
	}
	client, exists := t.clients[peerID]
	t.mu.RUnlock()
	if exists {
		return client, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, ErrStopped
	}

	if client, exists = t.clients[peerID]; exists {
		return client, nil
	}

	addr, registered := t.addrs[peerID]
	if !registered {
		return nil, fmt.Errorf("%w: %s", ErrUnknownPeer, peerID)
	}

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial peer %s at %s: %w", peerID, addr, err)
	}

	client = consensuspb.NewRaftServiceClient(conn)
	t.conns[peerID] = conn
	t.clients[peerID] = client
	return client, nil
}

func (t *GRPCOutboundTransport) SendRequestVote(ctx context.Context, to string, req *consensuspb.RequestVoteRequest) (*consensuspb.RequestVoteResponse, error) {
	client, err := t.getClient(to)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	return client.RequestVote(callCtx, req)
}

func (t *GRPCOutboundTransport) SendAppendEntries(ctx context.Context, to string, req *consensuspb.AppendEntriesRequest) (*consensuspb.AppendEntriesResponse, error) {
	client, err := t.getClient(to)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	return client.AppendEntries(callCtx, req)
}

func (t *GRPCOutboundTransport) SendInstallSnapshot(ctx context.Context, to string, streamHandler func(consensuspb.RaftService_InstallSnapshotClient) error) (*consensuspb.InstallSnapshotResponse, error) {
	client, err := t.getClient(to)
	if err != nil {
		return nil, err
	}

	stream, err := client.InstallSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed opening snapshot stream to %s: %w", to, err)
	}

	if err := streamHandler(stream); err != nil {
		return nil, fmt.Errorf("streaming snapshot to %s failed: %w", to, err)
	}
	return stream.CloseAndRecv()
}

func (t *GRPCOutboundTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true
	var closeErr error
	for id, conn := range t.conns {
		if err := conn.Close(); err != nil && closeErr == nil {
			closeErr = fmt.Errorf("failed closing connection to %s: %w", id, err)
		}
	}
	t.conns = make(map[string]*grpc.ClientConn)
	t.clients = make(map[string]consensuspb.RaftServiceClient)
	return closeErr
}
