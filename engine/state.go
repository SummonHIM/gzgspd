package engine

// State 表示单个会话的当前状态。
type State int

const (
	StateStarting State = iota
	StateNotLoggedIn
	StateLoggingIn
	StateLoggedIn
	StatePaused
	StateLoggingOut
	StateStopped
)

// String 返回可读状态名。
func (s State) String() string {
	switch s {
	case StateStarting:
		return "Starting"
	case StateNotLoggedIn:
		return "Not logged in"
	case StateLoggingIn:
		return "Logging in"
	case StateLoggedIn:
		return "Logged in"
	case StatePaused:
		return "Paused"
	case StateLoggingOut:
		return "Logging out"
	case StateStopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}
