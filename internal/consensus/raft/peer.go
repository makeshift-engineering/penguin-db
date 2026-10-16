package raft

import (
	"sort"
	"sync"
	"time"
)

type Peer struct {
	ID           string
	Address      string
	NextIndex    uint64
	MatchIndex   uint64
	Paused       bool
	InFlight     bool
	LastActivity time.Time
}

type PeerTracker struct {
	mu      sync.RWMutex
	localID string
	peers   map[string]*Peer
}

func NewPeerTracker(localID string, peerAddresses map[string]string) *PeerTracker {
	tracker := &PeerTracker{
		localID: localID,
		peers:   make(map[string]*Peer, len(peerAddresses)),
	}

	for id, addr := range peerAddresses {
		tracker.peers[id] = &Peer{
			ID:           id,
			Address:      addr,
			NextIndex:    1,
			MatchIndex:   0,
			LastActivity: time.Now(),
		}
	}

	return tracker
}

func (pt *PeerTracker) InitLeaderState(lastlogIndex uint64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	for _, peer := range pt.peers {
		peer.NextIndex = lastlogIndex + 1
		peer.MatchIndex = 0
		peer.InFlight = false
		peer.Paused = false
	}
}

func (pt *PeerTracker) UpdateProgress(peerID string, matchIndex uint64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	peer, exists := pt.peers[peerID]
	if !exists {
		return
	}

	if matchIndex > peer.MatchIndex {
		peer.MatchIndex = matchIndex
		peer.NextIndex = matchIndex + 1
	}

	peer.InFlight = false
	peer.LastActivity = time.Now()
}

func (pt *PeerTracker) SetInFlight(peerID string, inFlight bool) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if peer, exists := pt.peers[peerID]; exists {
		peer.InFlight = inFlight
	}
}

func (pt *PeerTracker) DecrementNextIndex(peerID string, conflictIndex uint64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	peer, exists := pt.peers[peerID]
	if !exists {
		return
	}

	if conflictIndex > 0 && conflictIndex <= peer.NextIndex {
		peer.NextIndex = conflictIndex
	} else if peer.NextIndex > 1 {
		peer.NextIndex--
	}
	peer.InFlight = false
}

func (pt *PeerTracker) QuorumMatchIndex(leaderLastIndex uint64) uint64 {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	matches := make([]uint64, 0, len(pt.peers)+1)
	matches = append(matches, leaderLastIndex)

	for _, peer := range pt.peers {
		matches = append(matches, peer.MatchIndex)
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i] > matches[j]
	})

	quorumIndex := (len(matches) - 1) / 2
	return matches[quorumIndex]
}

func (pt *PeerTracker) IsQuorum(grantedVotes int) bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	totalNodes := len(pt.peers) + 1
	requiredQuorum := (totalNodes / 2) + 1
	return grantedVotes >= requiredQuorum
}

func (pt *PeerTracker) GetPeer(peerID string) (Peer, bool) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	peer, exists := pt.peers[peerID]
	if !exists {
		return Peer{}, false
	}
	return *peer, true
}

func (pt *PeerTracker) PeerIDs() []string {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	ids := make([]string, 0, len(pt.peers))
	for id := range pt.peers {
		ids = append(ids, id)
	}
	return ids
}
