package engine

import "time"

// Event 是一次状态变更或失败通知，供 GUI 消费。
type Event struct {
	Key     string
	Time    time.Time
	State   State
	Message string
	Err     error
}
