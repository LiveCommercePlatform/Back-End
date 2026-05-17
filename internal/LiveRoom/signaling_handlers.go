package liveRoom

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

func handleJoin(
	session *SignalingSession,
	msg SignalMessage,
) {

	var payload JoinSignalPayload

	if err := mapToStruct(
		msg.Data,
		&payload,
	); err != nil {
		return
	}

	pc, err := createPeerConnection()
	if err != nil {
		return
	}

	peer := NewSFUPeer(
		uuid.NewString(),
		session.Room.RoomID,
		PeerRole(payload.Role),
		pc,
	)

	session.SetPeer(peer, pc)

	if peer.Role == PeerRoleHost {

		session.Room.SetHost(peer)

	} else {

		session.Room.AddViewer(peer)

		attachViewerTracks(
			session.Room,
			peer,
		)
	}

	pc.OnICECandidate(func(
		candidate *webrtc.ICECandidate,
	) {

		if candidate == nil {
			return
		}

		if !session.Client.SafeSend(
			NewSignalMessage(
				"ice_candidate",
				candidate.ToJSON(),
			),
		) {
			session.Client.Close()
		}
	})

	pc.OnTrack(func(
		track *webrtc.TrackRemote,
		receiver *webrtc.RTPReceiver,
	) {

		handleTrack(
			session,
			track,
			receiver,
		)
	})
}

func handleOffer(
	session *SignalingSession,
	msg SignalMessage,
) {

	peer, pc := session.GetPeer()

	if peer == nil || pc == nil {
		return
	}

	var offer webrtc.SessionDescription

	if err := mapToStruct(
		msg.Data,
		&offer,
	); err != nil {
		return
	}

	if err := pc.SetRemoteDescription(offer); err != nil {
		return
	}

	session.RemoteDescriptionSet.Store(true)

	flushPendingICE(session)

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		return
	}

	sendSignal(
		session.Client,
		"answer",
		pc.LocalDescription(),
	)
}

func handleAnswer(
	session *SignalingSession,
	msg SignalMessage,
) {

	_, pc := session.GetPeer()

	if pc == nil {
		return
	}

	var answer webrtc.SessionDescription

	if err := mapToStruct(
		msg.Data,
		&answer,
	); err != nil {
		return
	}

	if err := pc.SetRemoteDescription(answer); err != nil {
		return
	}

	session.RemoteDescriptionSet.Store(true)

	flushPendingICE(session)
}

func handleICECandidate(
	session *SignalingSession,
	msg SignalMessage,
) {

	var candidate webrtc.ICECandidateInit

	if err := mapToStruct(
		msg.Data,
		&candidate,
	); err != nil {
		return
	}

	_ = session.AddICECandidate(candidate)
}

func handleLeave(
	session *SignalingSession,
) {

	session.Cleanup()
}

func mapToStruct(
	in any,
	out any,
) error {

	b, err := json.Marshal(in)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, out)
}