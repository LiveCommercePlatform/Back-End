package liveRoom

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type SignalingSession struct {

	RoomID uuid.UUID   
    UserID *uuid.UUID
	HostID uuid.UUID
	
	mu sync.RWMutex

	Client *WSClient
	Room   *SFURoom

	Peer *SFUPeer
	PC   *webrtc.PeerConnection



	LastActivity time.Time

	Ctx    context.Context
	Cancel context.CancelFunc

	Closed atomic.Bool

	PendingCandidates []webrtc.ICECandidateInit

	RemoteDescriptionSet atomic.Bool

	CandidateMu sync.Mutex
}

func NewSignalingSession(
	client *WSClient,
	room *SFURoom,
) *SignalingSession {

	ctx, cancel := context.WithCancel(context.Background())

	return &SignalingSession{
		Client:       client,
		Room:         room,
		LastActivity: time.Now(),
		Ctx:          ctx,
		Cancel:       cancel,
	}
}

func (s *SignalingSession) Touch() {

	s.mu.Lock()
	defer s.mu.Unlock()

	s.LastActivity = time.Now()
}

func (s *SignalingSession) GetLastActivity() time.Time {

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.LastActivity
}

func (s *SignalingSession) SetPeer(
	peer *SFUPeer,
	pc *webrtc.PeerConnection,
) {

	s.mu.Lock()
	defer s.mu.Unlock()

	s.Peer = peer
	s.PC = pc
}

func (s *SignalingSession) GetPeer() (
	*SFUPeer,
	*webrtc.PeerConnection,
) {

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Peer, s.PC
}

func (s *SignalingSession) Cleanup() {

	if !s.Closed.CompareAndSwap(false, true) {
		return
	}

	s.Cancel()

	s.mu.Lock()

	peer := s.Peer
	room := s.Room
	client := s.Client

	s.mu.Unlock()

	if peer != nil && room != nil {
		room.RemovePeer(peer.PeerID)
	}

	if client != nil {
		client.Close()
	}
}

func (s *SignalingSession) AddICECandidate(
	c webrtc.ICECandidateInit,
) error {

	s.CandidateMu.Lock()
	defer s.CandidateMu.Unlock()

	if !s.RemoteDescriptionSet.Load() {

		s.PendingCandidates =
			append(s.PendingCandidates, c)

		return nil
	}

	peer, _ := s.GetPeer()

	if peer == nil || peer.PC == nil {
		return nil
	}

return peer.PC.AddICECandidate(c)
}