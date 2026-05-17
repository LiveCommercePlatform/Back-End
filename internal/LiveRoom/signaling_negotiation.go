// signaling_negotiation.go
package liveRoom

import (
	"time"

	"github.com/pion/webrtc/v4"
)

func triggerNegotiation(
	client *WSClient,
	peer *SFUPeer,
) {

	if peer == nil || peer.PC == nil {
		return
	}

	peer.NegotiationMu.Lock()
	defer peer.NegotiationMu.Unlock()

	if peer.MakingOffer.Load() {
		return
	}

	peer.MakingOffer.Store(true)
	defer peer.MakingOffer.Store(false)

	if peer.PC.SignalingState() !=
		webrtc.SignalingStateStable {
		return
	}

	offer, err := peer.PC.CreateOffer(nil)
	if err != nil {
		return
	}

	if err := peer.PC.SetLocalDescription(
		offer,
	); err != nil {
		return
	}

	sendSignal(
		client,
		"renegotiate",
		peer.PC.LocalDescription(),
	)
}

func startNegotiationLoop(
	session *SignalingSession,
) {

	t := time.NewTicker(2 * time.Second)

	go func() {

		defer t.Stop()

		for {

			select {

			case <-session.Ctx.Done():
				return

			case <-t.C:

				peer, _ := session.GetPeer()

				if peer == nil {
					continue
				}

				if !peer.NeedsNegotiation.
					CompareAndSwap(true, false) {
					continue
				}

				triggerNegotiation(
					session.Client,
					peer,
				)
			}
		}
	}()
}