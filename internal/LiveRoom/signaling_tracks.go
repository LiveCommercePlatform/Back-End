// signaling_tracks.go
package liveRoom

import (
	"github.com/pion/webrtc/v4"
)

func handleTrack(
	session *SignalingSession,
	track *webrtc.TrackRemote,
	receiver *webrtc.RTPReceiver,
) {

	room := session.Room

	trackID := track.ID()

	forwarder := NewSFUForwarder(
		trackID,
		track,
	)

	room.UpsertForwarder(
		trackID,
		forwarder,
	)

	defer func() {

		room.RemoveForwarder(trackID)

		forwarder.Close()
	}()

	viewers := room.ListViewers()

	for _, viewer := range viewers {

		if viewer == nil || viewer.PC == nil {
			continue
		}

		localTrack, err := webrtc.NewTrackLocalStaticRTP(
			track.Codec().RTPCodecCapability,
			track.ID(),
			track.StreamID(),
		)

		if err != nil {
			continue
		}

		sender, err := viewer.PC.AddTrack(localTrack)
		if err != nil {
			continue
		}

		viewer.SetSender(
			track.ID(),
			sender,
		)

		forwarder.AddSubscriber(
			viewer.PeerID,
			localTrack,
		)

		viewer.NeedsNegotiation.Store(true)
	}

	forwarder.StartForwarding()
}

func attachViewerTracks(
	room *SFURoom,
	viewer *SFUPeer,
) {

	forwarders := room.GetForwarders()

	for _, f := range forwarders {

		if f == nil || f.Source == nil {
			continue
		}

		localTrack, err := webrtc.NewTrackLocalStaticRTP(
			f.Source.Codec().RTPCodecCapability,
			f.Source.ID(),
			f.Source.StreamID(),
		)

		if err != nil {
			continue
		}

		sender, err := viewer.PC.AddTrack(localTrack)
		if err != nil {
			continue
		}

		viewer.SetSender(
			f.Source.ID(),
			sender,
		)

		f.AddSubscriber(
			viewer.PeerID,
			localTrack,
		)
	}

	viewer.NeedsNegotiation.Store(true)
}