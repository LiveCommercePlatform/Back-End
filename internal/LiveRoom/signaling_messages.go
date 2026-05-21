package liveRoom

type SignalMessage struct {
	Type string `json:"type"`
	Payload any    `json:"payload"`
}

func NewSignalMessage(
	t string,
	payload any,
) SignalMessage {

	return SignalMessage{
		Type:    t,
		Payload: payload,
	}
}