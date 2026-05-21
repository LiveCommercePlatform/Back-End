package liveRoom

import (
	"os"
	"strings"

	"github.com/pion/webrtc/v4"
)

// ENV پیشنهادی:
//
// ICE_URLS="stun:stun.l.google.com:19302,turn:turn.example.com:3478?transport=udp"
// TURN_USERNAME="user"
// TURN_CREDENTIAL="pass"
//
// نکته: اگر TURN_USERNAME/CREDENTIAL خالی باشه، فقط URLها استفاده میشن.
func loadICEServersFromEnv() []webrtc.ICEServer {
	raw := strings.TrimSpace(os.Getenv("ICE_URLS"))
	if raw == "" {
		// fallback dev
		return []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		}
	}

	urls := splitCSV(raw)

	user := strings.TrimSpace(os.Getenv("TURN_USERNAME"))
	cred := strings.TrimSpace(os.Getenv("TURN_CREDENTIAL"))

	s := webrtc.ICEServer{URLs: urls}
	if user != "" && cred != "" {
		s.Username = user
		s.Credential = cred
	}
	return []webrtc.ICEServer{s}
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}