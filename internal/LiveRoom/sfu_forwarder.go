package liveRoom

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"
)

const (
	forwarderBufferSize = 2048
	rtpWriteTimeout     = 200 * time.Millisecond
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

	ctx, cancel := context.WithCancel(context.Background())

	return &SFUForwarder{
		TrackID:    trackID,
		Source:     src,
		Subscribers: make(map[string]*webrtc.TrackLocalStaticRTP),
		Ctx:        ctx,
		Cancel:     cancel,
	}
}

func (f *SFUForwarder) AddSubscriber(
	peerID string,
	track *webrtc.TrackLocalStaticRTP,
) {

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

func (f *SFUForwarder) RemoveSubscriber(peerID string) {

	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.Subscribers, peerID)
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

	f.Cancel()

	f.mu.Lock()
	defer f.mu.Unlock()

	f.Subscribers =
		make(map[string]*webrtc.TrackLocalStaticRTP)
}

func (f *SFUForwarder) IsClosed() bool {
	return f.closed.Load()
}

func writeRTPWithTimeout(
	ctx context.Context,
	track *webrtc.TrackLocalStaticRTP,
	packet []byte,
) error {

	if track == nil {
		return errors.New("nil_track")
	}

	done := make(chan error, 1)

	go func() {

		_, err := track.Write(packet)

		select {

		case done <- err:

		default:
		}
	}()

	select {

	case err := <-done:
		return err

	case <-ctx.Done():
		return ctx.Err()

	case <-time.After(rtpWriteTimeout):
		return errors.New("rtp_write_timeout")
	}
}

func (f *SFUForwarder) StartForwarding() {

	if f == nil {
		return
	}

	if f.Source == nil {
		f.Close()
		return
	}

	buf := make([]byte, forwarderBufferSize)

	for {

		select {

		case <-f.Ctx.Done():
			return

		default:
		}

		n, _, err := f.Source.Read(buf)

		if err != nil {

			if err != io.EOF {
				// optional logging later
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

			err := writeRTPWithTimeout(
				f.Ctx,
				track,
				buf[:n],
			)

			if err != nil {

				f.RemoveSubscriber(peerID)
			}
		}
	}
}