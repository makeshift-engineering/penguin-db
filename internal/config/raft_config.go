package config

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrEmptyNodeID            = errors.New("node ID cannot be empty")
	ErrSelfInPeers            = errors.New("node ID cannot be listed within its own peer cluster map")
	ErrInvalidElectionTimeout = errors.New("electionTimeoutMin must be strictly less than electionTimeoutMax")
	ErrInvalidHeartbeat       = errors.New("heartbeatInterval must be less than electionTimeoutMin")
)

type RaftConfig struct {
	NodeID              string            `json:"node_id"`
	Peers               map[string]string `json:"peers"`
	ElectionTimeoutMin  time.Duration     `json:"election_timeout_min"`
	ElectionTimeoutMax  time.Duration     `json:"election_timeout_max"`
	HeartbeatInterval   time.Duration     `json:"heartbeat_interval"`
	SnapshotThreshold   uint64            `json:"snapshot_threshold"`
	MaxEntriesPerAppend int               `json:"max_entries_per_append"`
	PreVote             bool              `json:"pre_vote"`
}

func DefaultRaftConfig(nodeID string) *RaftConfig {
	return &RaftConfig{
		NodeID:              nodeID,
		Peers:               make(map[string]string),
		ElectionTimeoutMin:  150 * time.Millisecond,
		ElectionTimeoutMax:  300 * time.Millisecond,
		HeartbeatInterval:   50 * time.Millisecond,
		SnapshotThreshold:   50000,
		MaxEntriesPerAppend: 500,
		PreVote:             true,
	}
}

func (c *RaftConfig) Validate() error {
	if c.NodeID == "" {
		return ErrEmptyNodeID
	}
	if _, exists := c.Peers[c.NodeID]; exists {
		return fmt.Errorf("%w: node %s found in peer list", ErrSelfInPeers, c.NodeID)
	}
	if c.ElectionTimeoutMin <= 0 || c.ElectionTimeoutMax <= 0 {
		return errors.New("election timeouts must be strictly positive durations")
	}
	if c.ElectionTimeoutMin >= c.ElectionTimeoutMax {
		return fmt.Errorf("%w: min (%v) >= max (%v)", ErrInvalidElectionTimeout, c.ElectionTimeoutMin, c.ElectionTimeoutMax)
	}
	if c.HeartbeatInterval <= 0 {
		return errors.New("heartbeat interval must be a strictly positive duration")
	}
	if c.HeartbeatInterval >= c.ElectionTimeoutMin {
		return fmt.Errorf("%w: heartbeat (%v) >= election min (%v)", ErrInvalidHeartbeat, c.HeartbeatInterval, c.ElectionTimeoutMin)
	}
	if c.MaxEntriesPerAppend <= 0 {
		return errors.New("max entries per append must be greater than zero")
	}
	return nil
}
