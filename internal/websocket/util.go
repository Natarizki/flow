package websocket

import "encoding/json"

func ParsePayload(raw json.RawMessage, v interface{}) error {
	return json.Unmarshal(raw, v)
}
