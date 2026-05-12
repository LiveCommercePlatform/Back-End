package liveRoom

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"gorm.io/gorm"
)

// --------------------
// SFU rooms registry
// --------------------
var (
	sfuMu    sync.Mutex
	sfuRooms = make(map[uuid.UUID]*SFURoom)
)

func getOrCreateSFURoom(roomID uuid.UUID) *SFURoom {
	sfuMu.Lock()
	defer sfuMu.Unlock()

	if r, ok := sfuRooms[roomID]; ok {
		return r
	}
	r := NewSFURoom(roomID)
	sfuRooms[roomID] = r
	return r
}

// --------------------
// WebSocket upgrader
// --------------------
var signalingUpgrader = websocket.Upgrader{
	CheckOrigin: allowWSOrigin,
}

// --------------------
// Signaling protocol
// --------------------
type signalIn struct {
	Type      string          `json:"type"`
	Role      string          `json:"role,omitempty"`      // join
	ClientID  string          `json:"client_id,omitempty"` // viewer reconnect id
	SDP       string          `json:"sdp,omitempty"`       // offer
	Candidate json.RawMessage `json:"candidate,omitempty"` // ice
	TrackID   string `json:"track_id,omitempty"`
}

type signalOut struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
	TS   int64       `json:"ts"`
}

type welcomeData struct {
	PeerID string `json:"peer_id"`
	Role   string `json:"role"`
}

type errData struct {
	Error string `json:"error"`
}

func sendJSON(conn *websocket.Conn, typ string, data any) {
	_ = conn.WriteJSON(signalOut{
		Type: typ,
		Data: data,
		TS:   time.Now().Unix(),
	})
}

// --------------------
// WebRTC
// --------------------
func newPC() (*webrtc.PeerConnection, error) {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))

	cfg := webrtc.Configuration{
		ICEServers: loadICEServersFromEnv(),
	}
	return api.NewPeerConnection(cfg)
}

func ensureRoomExists(c *gin.Context, roomID uuid.UUID) bool {
	var lr models.LiveRoom
	if err := database.DB.Select("id").First(&lr, "id = ?", roomID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return false
	}
	return true
}

func normalizeID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return s
}

// WSSignaling godoc
// @Summary LiveRoom signaling websocket (WebRTC/SFU)
// @Description WebRTC signaling endpoint. Viewer can be anonymous; Host must be authenticated (ws cookie).
// @Tags liveroom
// @Param id path string true "LiveRoom ID (uuid)"
// @Success 101 {string} string "Switching Protocols"
// @Router /ws/live-rooms/{id}/signaling [get]
func WSSignaling() gin.HandlerFunc {
	return func(c *gin.Context) {
		roomID, ok := parseUUIDParam(c, "id")
		if !ok {
			return
		}
		if !ensureRoomExists(c, roomID) {
			return
		}

		// optional auth (WSOptionalAuthMiddleware should set these if cookie exists)
		var authedUserID *uuid.UUID
		if v, exists := c.Get("userID"); exists {
			if uid, ok := v.(uuid.UUID); ok {
				authedUserID = &uid
			}
		}

		conn, err := signalingUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		// activity tracking for idle timeout
		lastActivity := time.Now()
		touch := func() { lastActivity = time.Now() }

		// ws timeouts
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetReadLimit(1 << 20)
		conn.SetPongHandler(func(string) error {
			touch()
			return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		})

		// ping loop
		go func() {
			t := time.NewTicker(25 * time.Second)
			defer t.Stop()
			for range t.C {
				_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			}
		}()

		room := getOrCreateSFURoom(roomID)

		// state
		peerID := uuid.NewString()
		var role PeerRole
		var peer *SFUPeer
		var pc *webrtc.PeerConnection

		// idle watcher
		idleStop := make(chan struct{})
		defer close(idleStop)

		go func() {
			t := time.NewTicker(10 * time.Second)
			defer t.Stop()

			const idleLimit = 90 * time.Second
			for {
				select {
				case <-t.C:
					if time.Since(lastActivity) > idleLimit {
						if peer != nil {
							room.RemovePeer(peer.PeerID)
							if peer.PC != nil {
								_ = peer.PC.Close()
							}
						}
						_ = conn.Close()
						return
					}
				case <-idleStop:
					return
				}
			}
		}()

		// candidate buffering until remote description set
		var remoteSet bool
		var pendingCandidates []webrtc.ICECandidateInit

		sendJSON(conn, "signal.welcome", welcomeData{PeerID: peerID, Role: ""})

		for {
			var in signalIn
			if err := conn.ReadJSON(&in); err != nil {
				// cleanup
				if peer != nil {
					room.RemovePeer(peer.PeerID)
					if peer.PC != nil {
						_ = peer.PC.Close()
					}
				}
				return
			}
			touch()

			switch in.Type {

			case "signal.join":
				r := strings.TrimSpace(strings.ToLower(in.Role))
				if r != "host" && r != "viewer" {
					sendJSON(conn, "signal.error", errData{Error: "invalid_role"})
					continue
				}

				// viewer reconnect: use client_id as peerID if provided
				if r == "viewer" {
					if cid := normalizeID(in.ClientID); cid != "" {
						peerID = cid
					}
				}

				// host must be authenticated
				if r == "host" && authedUserID == nil {
					sendJSON(conn, "signal.error", errData{Error: "host_requires_auth"})
					continue
				}

				// only one host
				if r == "host" && room.GetHost() != nil {
					sendJSON(conn, "signal.error", errData{Error: "host_already_exists"})
					continue
				}

				var err error
				pc, err = newPC()
				if err != nil {
					sendJSON(conn, "signal.error", errData{Error: "pc_create_failed"})
					continue
				}

				// ICE -> send to client
				pc.OnICECandidate(func(cand *webrtc.ICECandidate) {
					if cand == nil {
						return
					}
					sendJSON(conn, "signal.ice", cand.ToJSON())
				})

				// viewer recvonly hints (still enforced on server by publish rules)
				if r == "viewer" {
					_, _ = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
						Direction: webrtc.RTPTransceiverDirectionRecvonly,
					})
					_, _ = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
						Direction: webrtc.RTPTransceiverDirectionRecvonly,
					})
				}

				role = PeerRole(r)
				peer = NewSFUPeer(peerID, roomID, role, pc)
				peer.UserID = authedUserID


				pc.OnConnectionStateChange(func(st webrtc.PeerConnectionState) {
					// فقط host کنترل کننده است
					if peer == nil || peer.Role != PeerRoleHost {
						return
					}

					switch st {
					case webrtc.PeerConnectionStateFailed,
						webrtc.PeerConnectionStateDisconnected,
						webrtc.PeerConnectionStateClosed:

						// auto-end live
						endLiveRoomFromSFU(roomID)
					}
				})

				// reconnect cleanup (viewer only)
				if role == PeerRoleViewer {
					room.mu.RLock()
					old := room.Viewers[peerID]
					room.mu.RUnlock()
					if old != nil {
						room.RemovePeer(old.PeerID)
						if old.PC != nil {
							_ = old.PC.Close()
						}
					}
				}

				// publish enforcement: only host can publish tracks
				pc.OnTrack(func(tr *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
					if peer == nil || peer.Role != PeerRoleHost {
						sendJSON(conn, "signal.error", errData{Error: "viewer_cannot_publish"})
						_ = pc.Close()
						return
					}

					trackID := tr.ID()
					f := NewSFUForwarder(trackID, tr)
					room.UpsertForwarder(trackID, f)

					// attach to existing viewers
					for _, v := range room.ListViewers() {
						if v == nil || v.PC == nil {
							continue
						}
						local, err := webrtc.NewTrackLocalStaticRTP(
							tr.Codec().RTPCodecCapability,
							tr.ID(),
							tr.StreamID(),
						)
						if err != nil {
							continue
						}
						sender, err := v.PC.AddTrack(local)
						if err == nil {
							v.SetSender(trackID, sender)
							f.AddSubscriber(v.PeerID, local)
							drainRTCP(sender)

							_ = v.PC.WriteRTCP([]rtcp.Packet{
								&rtcp.PictureLossIndication{MediaSSRC: uint32(tr.SSRC())},
							})
						}
					}

					go f.StartForwarding()
				})

				// register peer into room
				if role == PeerRoleHost {
					room.SetHost(peer)
				} else {
					room.AddViewer(peer)

					// attach existing tracks to this viewer
					for _, f := range room.GetForwarders() {
						src := f.Source
						if src == nil {
							continue
						}
						local, err := webrtc.NewTrackLocalStaticRTP(
							src.Codec().RTPCodecCapability,
							src.ID(),
							src.StreamID(),
						)
						if err != nil {
							continue
						}
						sender, err := peer.PC.AddTrack(local)
						if err == nil {
							peer.SetSender(f.TrackID, sender)
							f.AddSubscriber(peer.PeerID, local)
							drainRTCP(sender)

							_ = peer.PC.WriteRTCP([]rtcp.Packet{
								&rtcp.PictureLossIndication{MediaSSRC: uint32(src.SSRC())},
							})
						}
					}
				}

				sendJSON(conn, "signal.joined", welcomeData{PeerID: peerID, Role: string(role)})

			case "signal.offer":
				if pc == nil {
					sendJSON(conn, "signal.error", errData{Error: "join_first"})
					continue
				}
				if strings.TrimSpace(in.SDP) == "" {
					sendJSON(conn, "signal.error", errData{Error: "missing_sdp"})
					continue
				}

				offer := webrtc.SessionDescription{
					Type: webrtc.SDPTypeOffer,
					SDP:  in.SDP,
				}

				if err := pc.SetRemoteDescription(offer); err != nil {
					sendJSON(conn, "signal.error", errData{Error: "set_remote_failed"})
					continue
				}
				remoteSet = true

				// flush pending candidates
				for _, cand := range pendingCandidates {
					_ = pc.AddICECandidate(cand)
				}
				pendingCandidates = nil

				answer, err := pc.CreateAnswer(nil)
				if err != nil {
					sendJSON(conn, "signal.error", errData{Error: "create_answer_failed"})
					continue
				}
				if err := pc.SetLocalDescription(answer); err != nil {
					sendJSON(conn, "signal.error", errData{Error: "set_local_failed"})
					continue
				}

				sendJSON(conn, "signal.answer", pc.LocalDescription())

			case "signal.ice":
				if pc == nil {
					sendJSON(conn, "signal.error", errData{Error: "join_first"})
					continue
				}

				var cand webrtc.ICECandidateInit
				if err := json.Unmarshal(in.Candidate, &cand); err != nil {
					sendJSON(conn, "signal.error", errData{Error: "bad_candidate"})
					continue
				}

				if !remoteSet {
					pendingCandidates = append(pendingCandidates, cand)
					continue
				}

				_ = pc.AddICECandidate(cand)

			case "signal.leave":
				if peer != nil {
					room.RemovePeer(peer.PeerID)
					if peer.PC != nil {
						_ = peer.PC.Close()
					}
				}
				sendJSON(conn, "signal.left", gin.H{"ok": true})
				return

			case "signal.request_keyframe":
				if pc == nil || peer == nil {
					sendJSON(conn, "signal.error", errData{Error: "join_first"})
					continue
				}

				// viewer هم می‌تونه درخواست بده، مشکلی نیست
				tid := strings.TrimSpace(in.TrackID)
				if tid == "" {
					// اگر track_id نفرستاد، روی همه forwarderها PLI بزن
					for _, f := range room.GetForwarders() {
						_ = room.RequestKeyframe(f.TrackID)
					}
					sendJSON(conn, "signal.keyframe_requested", gin.H{"ok": true})
					continue
				}

				ok := room.RequestKeyframe(tid)
				if !ok {
					sendJSON(conn, "signal.error", errData{Error: "track_not_found_or_no_host"})
					continue
				}

				sendJSON(conn, "signal.keyframe_requested", gin.H{"ok": true, "track_id": tid})
			default:
				sendJSON(conn, "signal.error", errData{Error: "unsupported_type"})
			}
		}
	}
}