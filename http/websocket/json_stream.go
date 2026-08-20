package websocket

import (
	"encoding/json/jsontext"
	"errors"
	"io"
	"time"
)

// CopyJSONStream copies each JSON value from r to a separate WebSocket text message.
func (cm *Manager) CopyJSONStream(r io.Reader) error {
	dec := jsontext.NewDecoder(r)
	for {
		value, err := dec.ReadValue()
		switch {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		}

		if err := cm.WriteData(TextMessage, value, 10*time.Second); err != nil {
			return err
		}
	}
}
