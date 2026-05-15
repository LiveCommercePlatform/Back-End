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
	Role   PeerRole

	UserID *uuid.UUID

	PC *webrtc.PeerConnection

	mu      sync.RWMutex
	Senders map[string]*webrtc.RTPSender

	ctx    context.Context
	cancel context.CancelFunc

	closed atomic.Bool
	NeedsNegotiation atomic.Bool
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

		ctx:    ctx,
		cancel: cancel,
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


func (p *SFUPeer) Close() {

	if !p.closed.CompareAndSwap(false, true) {
		return
	}

	p.cancel()

	p.mu.Lock()

	for trackID, sender := range p.Senders {

		if sender != nil {
			_ = sender.Stop()
		}

		delete(p.Senders, trackID)
	}

	p.mu.Unlock()

	if p.PC != nil {
		_ = p.PC.Close()
	}
}

func (p *SFUPeer) RemoveSender(trackID string) {

	p.mu.Lock()
	defer p.mu.Unlock()

	sender, ok := p.Senders[trackID]

	if ok {

		if sender != nil {
			_ = sender.Stop()
		}

		delete(p.Senders, trackID)
	}
}