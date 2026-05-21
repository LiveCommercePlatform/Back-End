package liveRoom

import (
	"net/http"
	"os"
	"strings"
)

func allowWSOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))

	// اگر Origin خالیه (مثلاً server-to-server) بذار عبور کن
	if origin == "" {
		return true
	}

	allowed := strings.TrimSpace(os.Getenv("WS_ALLOWED_ORIGINS"))
	// مثال:
	// WS_ALLOWED_ORIGINS="http://localhost:3000,https://yourdomain.com"
	if allowed == "" {
		// dev fallback
		return origin == "http://localhost:3000"
	}

	for _, o := range strings.Split(allowed, ",") {
		if strings.TrimSpace(o) == origin {
			return true
		}
	}
	return false
}