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

	existingPeer, _ := session.GetPeer()

	if existingPeer != nil {
		return
	}

	

	var payload JoinSignalPayload

	if err := mapToStruct(
		msg.Payload,
		&payload,
	); err != nil {
		return
	}

	pc, err := createPeerConnection()
	if err != nil {
		return
	}

	role, ok := ParsePeerRole(
		payload.Role,
	)

	if !ok {
		_ = pc.Close()
		return
	}

	peer := NewSFUPeer(
		uuid.NewString(),
		session.Room.RoomID,
		role,
		pc,
		session.Client,
	)

	session.SetPeer(peer, pc)

	if peer.Role == PeerRoleHost {

		session.Room.SetHost(peer)
		    pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
        if state == webrtc.PeerConnectionStateFailed ||
            state == webrtc.PeerConnectionStateDisconnected ||
            state == webrtc.PeerConnectionStateClosed {

            go endLiveRoomFromSFU(session.Room.RoomID)
        }
    })

	} else {

		session.Room.AddViewer(peer)

		attachViewerTracks(
			session,
			peer,
		)
	}

	pc.OnICECandidate(func(
		candidate *webrtc.ICECandidate,
	) {

		if candidate == nil {
			return
		}

		sendSignal(
			session.Client,
			"ice_candidate",
			candidate.ToJSON(),
		)
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
		msg.Payload,
		&offer,
	); err != nil {
		return
	}

	state := pc.SignalingState()

	if state != webrtc.SignalingStateStable &&
		state != webrtc.SignalingStateHaveLocalOffer {
		return
	}

if pc.SignalingState() != webrtc.SignalingStateStable {

	if err := pc.SetLocalDescription(
		webrtc.SessionDescription{
			Type: webrtc.SDPTypeRollback,
		},
	); err != nil {

		session.Cleanup()
		return
	}
}

	if err := pc.SetRemoteDescription(offer); err != nil {

		session.Cleanup()

		return
	}

	session.RemoteDescriptionSet.Store(true)

	flushPendingICE(session)

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		session.Cleanup()
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
		msg.Payload,
		&answer,
	); err != nil {
		return
	}

	if pc.SignalingState() !=
	webrtc.SignalingStateHaveLocalOffer {
	return
}


	if err := pc.SetRemoteDescription(answer); err != nil {

		session.Cleanup()

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
		msg.Payload,
		&candidate,
	); err != nil {
		return
	}

	if err := session.AddICECandidate(candidate); err != nil {

	session.Cleanup()

	return
}
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