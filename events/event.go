package events

import "time"

type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Level     Level     `json:"level"`
	Category  string    `json:"category"`
	Action    string    `json:"action"`
	Data      any       `json:"data"`
}

func NewEvent(level Level, category, action string, data any) Event {
	if level == "" {
		level = LevelInfo
	}
	return Event{
		Timestamp: time.Now(),
		Level:     level,
		Category:  category,
		Action:    action,
		Data:      data,
	}
}
