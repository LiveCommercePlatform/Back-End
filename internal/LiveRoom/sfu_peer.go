package liveRoom

import (
	"sync"

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

	// اگر authenticated باشه
	UserID *uuid.UUID
	RoleStr string // user/admin (برای خودت)

	PC *webrtc.PeerConnection

	mu      sync.Mutex
	sendMap map[string]*webrtc.RTPSender // trackID -> sender
}

func NewSFUPeer(peerID string, roomID uuid.UUID, role PeerRole, pc *webrtc.PeerConnection) *SFUPeer {
	return &SFUPeer{
		PeerID: peerID,
		RoomID: roomID,
		Role:   role,
		PC:     pc,
		sendMap: make(map[string]*webrtc.RTPSender),
	}
}

func (p *SFUPeer) SetSender(trackID string, sender *webrtc.RTPSender) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sendMap[trackID] = sender
}