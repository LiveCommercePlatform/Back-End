package liveRoom

import (
	"github.com/pion/webrtc/v4"
)

func createPeerConnection() (
	*webrtc.PeerConnection,
	error,
) {

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
				},
			},
		},
	}

	return webrtc.NewPeerConnection(config)
}