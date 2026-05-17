package liveRoom

type SignalMessage struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

func NewSignalMessage(
	t string,
	data any,
) SignalMessage {

	return SignalMessage{
		Type: t,
		Data: data,
	}
}