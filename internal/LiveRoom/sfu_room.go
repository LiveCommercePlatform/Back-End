package liveRoom

import (
	"sync"

	"github.com/google/uuid"
	"github.com/pion/rtcp"
)

type SFURoom struct {
	RoomID uuid.UUID

	mu sync.RWMutex

	Host *SFUPeer

	

	Viewers map[string]*SFUPeer

	Forwarders map[string]*SFUForwarder
}

func NewSFURoom(roomID uuid.UUID) *SFURoom {

	return &SFURoom{
		RoomID: roomID,
		Viewers: make(map[string]*SFUPeer),
		Forwarders: make(map[string]*SFUForwarder),
	}
}

func (r *SFURoom) SetHost(peer *SFUPeer) {

	r.mu.Lock()
	defer r.mu.Unlock()

	r.Host = peer
}

func (r *SFURoom) GetHost() *SFUPeer {

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.Host
}

func (r *SFURoom) AddViewer(peer *SFUPeer) {

	r.mu.Lock()
	defer r.mu.Unlock()

	r.Viewers[peer.PeerID] = peer
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

func (r *SFURoom) UpsertForwarder(
	trackID string,
	f *SFUForwarder,
) {

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

func (r *SFURoom) RemovePeer(peerID string) {

	r.mu.Lock()

	if r.Host != nil && r.Host.PeerID == peerID {

		host := r.Host

		r.Host = nil

		viewers := make([]*SFUPeer, 0, len(r.Viewers))

		for _, v := range r.Viewers {
			viewers = append(viewers, v)
		}

		forwarders := make([]*SFUForwarder, 0, len(r.Forwarders))

		for _, f := range r.Forwarders {
			forwarders = append(forwarders, f)
		}

		r.Viewers = make(map[string]*SFUPeer)
		r.Forwarders = make(map[string]*SFUForwarder)

		r.mu.Unlock()

		host.Close()

		for _, viewer := range viewers {
			viewer.Close()
		}

		for _, f := range forwarders {
			f.Close()
		}

		return
	}


	forwarders := make([]*SFUForwarder, 0, len(r.Forwarders))

	for _, f := range r.Forwarders {
		forwarders = append(forwarders, f)
	}

	viewer := r.Viewers[peerID]

	delete(r.Viewers, peerID)

	r.mu.Unlock()

	if viewer != nil {
		viewer.Close()
	}

	for _, f := range forwarders {
		f.RemoveSubscriber(peerID)
	}
}

func (r *SFURoom) RequestKeyframe(trackID string) bool {

	r.mu.RLock()

	host := r.Host
	f := r.Forwarders[trackID]

	r.mu.RUnlock()

	if host == nil || host.PC == nil || f == nil || f.Source == nil {
		return false
	}

	_ = host.PC.WriteRTCP([]rtcp.Packet{
		&rtcp.PictureLossIndication{
			MediaSSRC: uint32(f.Source.SSRC()),
		},
	})

	return true
}
func (r *SFURoom) RemoveForwarder(trackID string) {

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.Forwarders, trackID)
}

func (r *SFURoom) Empty() bool {

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.Host == nil &&
		len(r.Viewers) == 0
}


func (r *SFURoom) GetForwarder(
	trackID string,
) (*SFUForwarder, bool) {

	r.mu.RLock()
	defer r.mu.RUnlock()

	f, ok := r.Forwarders[trackID]

	return f, ok
}