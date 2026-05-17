package liveRoom

func sendSignal(
	client *WSClient,
	t string,
	data any,
) {

	if client == nil {
		return
	}

	if !client.SafeSend(
		NewSignalMessage(t, data),
	) {
		client.Close()
	}
}