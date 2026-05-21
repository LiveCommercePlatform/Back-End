// signaling_ice.go
package liveRoom

func flushPendingICE(
	session *SignalingSession,
) {

	peer, _ := session.GetPeer()

	if peer == nil || peer.PC == nil {
		return
	}

	session.CandidateMu.Lock()

	pending := session.PendingCandidates

	session.PendingCandidates = nil

	session.CandidateMu.Unlock()

	for _, c := range pending {

		_ = peer.PC.AddICECandidate(c)
	}
}