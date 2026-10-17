package consensus_server

import (
	"context"
	"errors"
	"io"
	"log/slog"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
	"github.com/makeshift-engineering/penguin-db/internal/consensus/raft"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ConsensusReceiver interface {
	HandleRequestVote(ctx context.Context, req *consensuspb.RequestVoteRequest) (*consensuspb.RequestVoteResponse, error)
	HandleAppendEntries(ctx context.Context, req *consensuspb.AppendEntriesRequest) (*consensuspb.AppendEntriesResponse, error)
	HandleInstallSnapshot(ctx context.Context, stream consensuspb.RaftService_InstallSnapshotServer) error
}

type Server struct {
	consensuspb.UnimplementedRaftServiceServer
	receiver ConsensusReceiver
}

func NewServer(receiver ConsensusReceiver) *Server {
	return &Server{
		receiver: receiver,
	}
}

func (s *Server) Register(grpcServer *grpc.Server) {
	consensuspb.RegisterRaftServiceServer(grpcServer, s)
}

func (s *Server) RequestVote(ctx context.Context, req *consensuspb.RequestVoteRequest) (*consensuspb.RequestVoteResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request vote message cannot be nil")
	}

	resp, err := s.receiver.HandleRequestVote(ctx, req)
	if err != nil {
		return nil, mapConsensusError(err)
	}

	return resp, nil
}

func (s *Server) AppendEntries(ctx context.Context, req *consensuspb.AppendEntriesRequest) (*consensuspb.AppendEntriesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "append entries message cannot be nil")
	}

	resp, err := s.receiver.HandleAppendEntries(ctx, req)
	if err != nil {
		return nil, mapConsensusError(err)
	}

	return resp, nil
}

func (s *Server) HandleInstallSnapshot(stream consensuspb.RaftService_InstallSnapshotServer) error {
	err := s.receiver.HandleInstallSnapshot(stream.Context(), stream)
	if err != nil {
		return mapConsensusError(err)
	}

	return nil
}

func mapConsensusError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request was canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "consensus deadline exceeded")
	case errors.Is(err, raft.ErrStopped):
		return status.Error(codes.Unavailable, "consensus node is shut down")
	case errors.Is(err, io.EOF):
		return status.Error(codes.OutOfRange, "unexpected EOF encountered during stream")
	default:
		slog.Error("Unhandled consensus error", "error", err)
		return status.Error(codes.Internal, "internal consensus error")
	}
}
