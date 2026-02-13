package events

import (
	"time"

	strutils "github.com/yusing/goutils/strings"
)

type Event struct {
	ID        string    `json:"uuid"`
	Timestamp time.Time `json:"timestamp"`
	Level     Level     `json:"level"`
	Category  string    `json:"category"`
	Action    string    `json:"action"`
	Data      any       `json:"data"`
} // @name Event

func NewEvent(level Level, category, action string, data any) Event {
	if level == "" {
		level = LevelInfo
	}
	return Event{
		ID:        strutils.NewUUIDv7(),
		Timestamp: time.Now(),
		Level:     level,
		Category:  category,
		Action:    action,
		Data:      data,
	}
}
