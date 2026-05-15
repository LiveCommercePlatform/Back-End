package liveRoom

import (
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v4"
)

type SFUForwarder struct {
	TrackID string
	Source  *webrtc.TrackRemote

	mu sync.RWMutex

	Subscribers map[string]*webrtc.TrackLocalStaticRTP

	closed atomic.Bool
}

func NewSFUForwarder(
	trackID string,
	src *webrtc.TrackRemote,
) *SFUForwarder {

	return &SFUForwarder{
		TrackID: trackID,
		Source:  src,

		Subscribers: make(
			map[string]*webrtc.TrackLocalStaticRTP,
		),
	}
}
func (f *SFUForwarder) AddSubscriber(peerID string, local *webrtc.TrackLocalStaticRTP) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Subscribers[peerID] = local
}

func (f *SFUForwarder) RemoveSubscriber(peerID string) {

	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.Subscribers, peerID)
}

func (f *SFUForwarder) StartForwarding() {

	buf := make([]byte, 1500)

	for {

		n, _, err := f.Source.Read(buf)

		if err != nil {

			f.Close()

			return
		}

		f.mu.RLock()

		for peerID, track := range f.Subscribers {

			if track == nil {
				continue
			}

			if _, err := track.Write(buf[:n]); err != nil {

				f.mu.RUnlock()

				f.RemoveSubscriber(peerID)

				f.mu.RLock()
			}
		}

		f.mu.RUnlock()
	}
}

func (f *SFUForwarder) Close() {

	if !f.closed.CompareAndSwap(false, true) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.Subscribers = map[string]*webrtc.TrackLocalStaticRTP{}
}

func (f *SFUForwarder) Empty() bool {

	f.mu.RLock()
	defer f.mu.RUnlock()

	return len(f.Subscribers) == 0
}