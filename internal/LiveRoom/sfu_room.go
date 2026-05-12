package liveRoom

import (
	"sync"

	"github.com/google/uuid"
	"github.com/pion/rtcp"
)

type SFURoom struct {
	RoomID uuid.UUID

	mu sync.RWMutex

	Host    *SFUPeer
	Viewers map[string]*SFUPeer // peerID -> peer

	Forwarders map[string]*SFUForwarder // trackID -> forwarder
}

func NewSFURoom(roomID uuid.UUID) *SFURoom {
	return &SFURoom{
		RoomID:     roomID,
		Viewers:    make(map[string]*SFUPeer),
		Forwarders: make(map[string]*SFUForwarder),
	}
}

func (r *SFURoom) SetHost(p *SFUPeer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Host = p
}

func (r *SFURoom) AddViewer(p *SFUPeer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Viewers[p.PeerID] = p
}

func (r *SFURoom) RemovePeer(peerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Host != nil && r.Host.PeerID == peerID {
		r.Host = nil
	}

	delete(r.Viewers, peerID)

	// remove peer from all forwarders
	for _, f := range r.Forwarders {
		f.RemoveSubscriber(peerID)
	}
}

func (r *SFURoom) GetHost() *SFUPeer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Host
}

func (r *SFURoom) ListViewers() []*SFUPeer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*SFUPeer, 0, len(r.Viewers))
	for _, v := range r.Viewers {
		out = append(out, v)
	}
	return out
}

func (r *SFURoom) UpsertForwarder(trackID string, f *SFUForwarder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Forwarders[trackID] = f
}

func (r *SFURoom) GetForwarders() []*SFUForwarder {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*SFUForwarder, 0, len(r.Forwarders))
	for _, f := range r.Forwarders {
		out = append(out, f)
	}
	return out
}


func (r *SFURoom) RequestKeyframe(trackID string) bool {
    r.mu.RLock()
    host := r.Host
    f := r.Forwarders[trackID]
    r.mu.RUnlock()

    if host == nil || host.PC == nil || f == nil {
        return false
    }

    _ = host.PC.WriteRTCP([]rtcp.Packet{
        &rtcp.PictureLossIndication{MediaSSRC: f.SSRC},
    })
    return true
}