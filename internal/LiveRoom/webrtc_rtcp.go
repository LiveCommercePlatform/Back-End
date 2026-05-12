package liveRoom

import (
	"time"

	"github.com/pion/webrtc/v4"
)

func drainRTCP(sender *webrtc.RTPSender) {
	if sender == nil {
		return
	}
	go func() {
		buf := make([]byte, 1500)
		for {
			_ = sender.SetReadDeadline(time.Now().Add(30 * time.Second))
			if _, _, err := sender.Read(buf); err != nil {
				return
			}
		}
	}()
}