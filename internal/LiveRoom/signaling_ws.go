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
	Role      string          `json:"role,omitempty"`
	ClientID  string          `json:"client_id,omitempty"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
	TrackID   string          `json:"track_id,omitempty"`
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

func sendSignal(client *WSClient, typ string, data any) {

	client.SafeSend(signalOut{
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

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(m),
	)

	cfg := webrtc.Configuration{
		ICEServers: loadICEServersFromEnv(),
	}

	return api.NewPeerConnection(cfg)
}

func ensureRoomExists(c *gin.Context, roomID uuid.UUID) bool {

	var lr models.LiveRoom

	if err := database.DB.
		Select("id").
		First(&lr, "id = ?", roomID).
		Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {

			c.JSON(http.StatusNotFound, gin.H{
				"error": "live_room_not_found",
			})

			return false
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "db_error",
		})

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
// @Description WebRTC signaling endpoint
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

		// optional auth

		var authedUserID *uuid.UUID

		if v, exists := c.Get("userID"); exists {

			if uid, ok := v.(uuid.UUID); ok {
				authedUserID = &uid
			}
		}

		conn, err := signalingUpgrader.Upgrade(
			c.Writer,
			c.Request,
			nil,
		)

		if err != nil {
			return
		}

		client := NewWSClient(conn)

		go client.WritePump()

		// activity tracking

		var activityMu sync.Mutex

		lastActivity := time.Now()

		touch := func() {

			activityMu.Lock()
			lastActivity = time.Now()
			activityMu.Unlock()
		}

		getLastActivity := func() time.Time {

			activityMu.Lock()
			defer activityMu.Unlock()

			return lastActivity
		}

		room := getOrCreateSFURoom(roomID)

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

					if time.Since(getLastActivity()) > idleLimit {

						if peer != nil {
							room.RemovePeer(peer.PeerID)
						}

						client.Close()

						return
					}

				case <-idleStop:
					return
				}
			}
		}()

		sendSignal(client,
			"signal.welcome",
			welcomeData{
				PeerID: peerID,
				Role:   "",
			},
		)

		client.ReadPump(func(messageType int, data []byte) {

			touch()

			var in signalIn

			if err := json.Unmarshal(data, &in); err != nil {

				sendSignal(client,
					"signal.error",
					errData{
						Error: "bad_json",
					},
				)

				return
			}

			switch in.Type {

			// -------------------------------------------------
			// JOIN
			// -------------------------------------------------

			case "signal.join":

				r := strings.TrimSpace(
					strings.ToLower(in.Role),
				)

				if r != "host" && r != "viewer" {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "invalid_role",
						},
					)

					return
				}

				// reconnect viewer

				if r == "viewer" {

					if cid := normalizeID(in.ClientID); cid != "" {
						peerID = cid
					}
				}

				// host auth

				if r == "host" && authedUserID == nil {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "host_requires_auth",
						},
					)

					return
				}

				// single host

				if r == "host" && room.GetHost() != nil {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "host_already_exists",
						},
					)

					return
				}

				var err error

				pc, err = newPC()

				if err != nil {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "pc_create_failed",
						},
					)

					return
				}

				// ICE

				pc.OnICECandidate(func(cand *webrtc.ICECandidate) {

					if cand == nil {
						return
					}

					sendSignal(client,
						"signal.ice",
						cand.ToJSON(),
					)
				})

				// viewer recvonly

				if r == "viewer" {

					_, _ = pc.AddTransceiverFromKind(
						webrtc.RTPCodecTypeVideo,
						webrtc.RTPTransceiverInit{
							Direction: webrtc.RTPTransceiverDirectionRecvonly,
						},
					)

					_, _ = pc.AddTransceiverFromKind(
						webrtc.RTPCodecTypeAudio,
						webrtc.RTPTransceiverInit{
							Direction: webrtc.RTPTransceiverDirectionRecvonly,
						},
					)
				}

				role = PeerRole(r)

				peer = NewSFUPeer(
					peerID,
					roomID,
					role,
					pc,
				)

				peer.UserID = authedUserID

				pc.OnConnectionStateChange(
					func(st webrtc.PeerConnectionState) {

						if peer == nil ||
							peer.Role != PeerRoleHost {

							return
						}

						switch st {

						case webrtc.PeerConnectionStateFailed,
							webrtc.PeerConnectionStateDisconnected,
							webrtc.PeerConnectionStateClosed:

							endLiveRoomFromSFU(roomID)
						}
					},
				)

				// reconnect cleanup

				if role == PeerRoleViewer {

					room.mu.RLock()

					old := room.Viewers[peerID]

					room.mu.RUnlock()

					if old != nil {
						room.RemovePeer(old.PeerID)
					}
				}

				// -------------------------------------------------
				// HOST TRACKS
				// -------------------------------------------------

				pc.OnTrack(func(
					tr *webrtc.TrackRemote,
					recv *webrtc.RTPReceiver,
				) {

					if peer == nil ||
						peer.Role != PeerRoleHost {

						sendSignal(client,
							"signal.error",
							errData{
								Error: "viewer_cannot_publish",
							},
						)

						_ = pc.Close()

						return
					}

					trackID := tr.StreamID() + "_" + tr.ID()

					f := NewSFUForwarder(
						trackID,
						tr,
					)

					room.UpsertForwarder(
						trackID,
						f,
					)

					// attach to existing viewers

					for _, v := range room.ListViewers() {

						if v == nil || v.PC == nil {
							continue
						}

						local, err :=
							webrtc.NewTrackLocalStaticRTP(
								tr.Codec().RTPCodecCapability,
								tr.ID(),
								tr.StreamID(),
							)

						if err != nil {
							continue
						}

						sender, err :=
							v.PC.AddTrack(local)

						if err != nil {
							continue
						}

						v.SetSender(trackID, sender)

						f.AddSubscriber(
							v.PeerID,
							local,
						)

						drainRTCP(sender)

						_ = v.PC.WriteRTCP([]rtcp.Packet{
							&rtcp.PictureLossIndication{
								MediaSSRC: uint32(tr.SSRC()),
							},
						})
					}

					go f.StartForwarding()
				})

				// -------------------------------------------------
				// REGISTER PEER
				// -------------------------------------------------

				if role == PeerRoleHost {

					room.SetHost(peer)

				} else {

					room.AddViewer(peer)

					// attach existing tracks

					for _, f := range room.GetForwarders() {

						src := f.Source

						if src == nil {
							continue
						}

						local, err :=
							webrtc.NewTrackLocalStaticRTP(
								src.Codec().RTPCodecCapability,
								src.ID(),
								src.StreamID(),
							)

						if err != nil {
							continue
						}

						sender, err :=
							peer.PC.AddTrack(local)

						if err != nil {
							continue
						}

						peer.SetSender(
							f.TrackID,
							sender,
						)

						f.AddSubscriber(
							peer.PeerID,
							local,
						)

						drainRTCP(sender)

						_ = peer.PC.WriteRTCP([]rtcp.Packet{
							&rtcp.PictureLossIndication{
								MediaSSRC: uint32(src.SSRC()),
							},
						})
					}
				}

				sendSignal(client,
					"signal.joined",
					welcomeData{
						PeerID: peerID,
						Role:   string(role),
					},
				)

			// -------------------------------------------------
			// OFFER
			// -------------------------------------------------

			case "signal.offer":

				if pc == nil {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "join_first",
						},
					)

					return
				}

				if strings.TrimSpace(in.SDP) == "" {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "missing_sdp",
						},
					)

					return
				}

				offer := webrtc.SessionDescription{
					Type: webrtc.SDPTypeOffer,
					SDP:  in.SDP,
				}

				if err := pc.SetRemoteDescription(offer); err != nil {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "set_remote_failed",
						},
					)

					return
				}

				answer, err := pc.CreateAnswer(nil)

				if err != nil {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "create_answer_failed",
						},
					)

					return
				}

				if err := pc.SetLocalDescription(answer); err != nil {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "set_local_failed",
						},
					)

					return
				}

				sendSignal(
					client,
					"signal.answer",
					pc.LocalDescription(),
				)

			// -------------------------------------------------
			// ICE
			// -------------------------------------------------

			case "signal.ice":

				if pc == nil {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "join_first",
						},
					)

					return
				}

				var cand webrtc.ICECandidateInit

				if err := json.Unmarshal(
					in.Candidate,
					&cand,
				); err != nil {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "bad_candidate",
						},
					)

					return
				}

				_ = pc.AddICECandidate(cand)

			// -------------------------------------------------
			// LEAVE
			// -------------------------------------------------

			case "signal.leave":

				if peer != nil {
					room.RemovePeer(peer.PeerID)
				}

				sendSignal(client,
					"signal.left",
					gin.H{"ok": true},
				)

				client.Close()

				return

			// -------------------------------------------------
			// KEYFRAME
			// -------------------------------------------------

			case "signal.request_keyframe":

				if pc == nil || peer == nil {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "join_first",
						},
					)

					return
				}

				tid := strings.TrimSpace(in.TrackID)

				if tid == "" {

					for _, f := range room.GetForwarders() {
						_ = room.RequestKeyframe(f.TrackID)
					}

					sendSignal(client,
						"signal.keyframe_requested",
						gin.H{"ok": true},
					)

					return
				}

				ok := room.RequestKeyframe(tid)

				if !ok {

					sendSignal(client,
						"signal.error",
						errData{
							Error: "track_not_found_or_no_host",
						},
					)

					return
				}

				sendSignal(client,
					"signal.keyframe_requested",
					gin.H{
						"ok":       true,
						"track_id": tid,
					},
				)

			// -------------------------------------------------
			// UNKNOWN
			// -------------------------------------------------

			default:

				sendSignal(client,
					"signal.error",
					errData{
						Error: "unsupported_type",
					},
				)
			}
		})

		if peer != nil {
			room.RemovePeer(peer.PeerID)
		}

		client.Close()
	}
}