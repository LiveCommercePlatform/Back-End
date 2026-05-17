package liveRoom

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type PeerRole string

const (
	PeerRoleHost   PeerRole = "host"
	PeerRoleViewer PeerRole = "viewer"
)

type SFUPeer struct {
	PeerID string
	RoomID uuid.UUID

	UserID *uuid.UUID

	Role PeerRole

	PC *webrtc.PeerConnection

	mu sync.RWMutex

	Senders map[string]*webrtc.RTPSender

	Ctx    context.Context
	Cancel context.CancelFunc

	NeedsNegotiation atomic.Bool
	NegotiationMu sync.Mutex
	MakingOffer  atomic.Bool
}

func NewSFUPeer(
	peerID string,
	roomID uuid.UUID,
	role PeerRole,
	pc *webrtc.PeerConnection,
) *SFUPeer {

	ctx, cancel := context.WithCancel(context.Background())

	return &SFUPeer{
		PeerID: peerID,
		RoomID: roomID,
		Role:   role,
		PC:     pc,
		Senders: make(map[string]*webrtc.RTPSender),
		Ctx:    ctx,
		Cancel: cancel,
	}
}

func (p *SFUPeer) SetSender(
	trackID string,
	sender *webrtc.RTPSender,
) {

	p.mu.Lock()
	defer p.mu.Unlock()

	p.Senders[trackID] = sender
}

func (p *SFUPeer) RemoveSender(trackID string) {

	p.mu.Lock()
	defer p.mu.Unlock()

	sender, ok := p.Senders[trackID]

	if ok && sender != nil {
		_ = sender.Stop()
	}

	delete(p.Senders, trackID)
}

func (p *SFUPeer) Close() {

	p.Cancel()

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, sender := range p.Senders {

		if sender != nil {
			_ = sender.Stop()
		}
	}

	p.Senders = make(map[string]*webrtc.RTPSender)

	if p.PC != nil {
		_ = p.PC.Close()
	}
}