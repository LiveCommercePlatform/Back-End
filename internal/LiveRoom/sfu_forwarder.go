package liveRoom

import (
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type SFUForwarder struct {
    TrackID string
    Source  *webrtc.TrackRemote
    SSRC    uint32

    mu     sync.RWMutex
    locals map[string]*webrtc.TrackLocalStaticRTP
}

func NewSFUForwarder(trackID string, src *webrtc.TrackRemote) *SFUForwarder {
    return &SFUForwarder{
        TrackID: trackID,
        Source:  src,
        SSRC:    uint32(src.SSRC()),
        locals:  make(map[string]*webrtc.TrackLocalStaticRTP),
    }
}
func (f *SFUForwarder) AddSubscriber(peerID string, local *webrtc.TrackLocalStaticRTP) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.locals[peerID] = local
}

func (f *SFUForwarder) RemoveSubscriber(peerID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.locals, peerID)
}

// StartForwarding باید یکبار برای هر forwarder اجرا بشه
func (f *SFUForwarder) StartForwarding() {
	buf := make([]byte, 1500)

	for {
		n, _, err := f.Source.Read(buf)
		if err != nil {
			return
		}

		pkt := &rtp.Packet{}
		if err := pkt.Unmarshal(buf[:n]); err != nil {
			continue
		}

		f.mu.RLock()
		for _, local := range f.locals {
			_ = local.WriteRTP(pkt)
		}
		f.mu.RUnlock()
	}
}