package liveRoom

import (
	"github.com/pion/webrtc/v4"
)

func triggerNegotiation(
	client *WSClient,
	peer *SFUPeer,
) {

	if peer == nil || peer.PC == nil {
		return
	}

	if !peer.NeedsNegotiation.
		CompareAndSwap(true, false) {
		return
	}

	peer.NegotiationMu.Lock()
	defer peer.NegotiationMu.Unlock()

	if peer.MakingOffer.Load() {
		return
	}

	if peer.PC.SignalingState() !=
		webrtc.SignalingStateStable {
		return
	}

	peer.MakingOffer.Store(true)
	defer peer.MakingOffer.Store(false)

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