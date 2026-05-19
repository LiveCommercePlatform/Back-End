package liveRoom

import (
	"context"
	"io"
	"log"
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v4"
)

const (
	forwarderBufferSize = 1460
)

type SFUForwarder struct {
	TrackID string
	Source  *webrtc.TrackRemote

	mu sync.RWMutex

	Subscribers map[string]*webrtc.TrackLocalStaticRTP

	closed atomic.Bool

	Ctx    context.Context
	Cancel context.CancelFunc
}

func NewSFUForwarder(
	trackID string,
	src *webrtc.TrackRemote,
) *SFUForwarder {

	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	return &SFUForwarder{
		TrackID:    trackID,
		Source:     src,
		Subscribers: make(
			map[string]*webrtc.TrackLocalStaticRTP,
		),
		Ctx:    ctx,
		Cancel: cancel,
	}
}

func (f *SFUForwarder) AddSubscriber(
	peerID string,
	track *webrtc.TrackLocalStaticRTP,
) {

	if _, exists := f.Subscribers[peerID]; exists {
	return
}
	if track == nil {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed.Load() {
		return
	}

	f.Subscribers[peerID] = track
}

func (f *SFUForwarder) RemoveSubscriber(
	peerID string,
) {

	shouldClose := false

	f.mu.Lock()

	delete(f.Subscribers, peerID)

	if len(f.Subscribers) == 0 {
		shouldClose = true
	}

	f.mu.Unlock()

	if shouldClose {
		f.Close()
	}
}

func (f *SFUForwarder) Empty() bool {

	f.mu.RLock()
	defer f.mu.RUnlock()

	return len(f.Subscribers) == 0
}

func (f *SFUForwarder) Close() {

	if !f.closed.CompareAndSwap(false, true) {
		return
	}

	f.mu.Lock()

	f.Subscribers = make(
		map[string]*webrtc.TrackLocalStaticRTP,
	)

	f.mu.Unlock()

	log.Printf(
	"[SFU_FORWARDER] closed track=%s",
	f.TrackID,
		)

	f.Cancel()
}

func (f *SFUForwarder) IsClosed() bool {
	return f.closed.Load()
}

func (f *SFUForwarder) StartForwarding() {

	if f == nil {
		return
	}

	if f.Source == nil {

		f.Close()

		return
	}

	buf := make(
		[]byte,
		forwarderBufferSize,
	)

	for {

		select {

		case <-f.Ctx.Done():
			return

		default:
		}

		n, _, err := f.Source.Read(buf)

		if err != nil {

			if err != io.EOF {

				log.Printf(
					"[SFU_FORWARDER] track=%s read_error=%v",
					f.TrackID,
					err,
				)
			}

			f.Close()

			return
		}

		if n <= 0 {
			continue
		}

		f.mu.RLock()

		subs := make(
			map[string]*webrtc.TrackLocalStaticRTP,
			len(f.Subscribers),
		)

		for k, v := range f.Subscribers {
			subs[k] = v
		}

		f.mu.RUnlock()

		for peerID, track := range subs {

			if track == nil {

				f.RemoveSubscriber(peerID)

				continue
			}

			_, err := track.Write(buf[:n])

			if err != nil {

				log.Printf(
					"[SFU_FORWARDER] track=%s peer=%s write_error=%v",
					f.TrackID,
					peerID,
					err,
				)

				f.RemoveSubscriber(peerID)
			}
		}
	}
}