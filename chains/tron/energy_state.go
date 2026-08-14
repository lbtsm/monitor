package tron

import "time"

type AlertStatus string

const (
	StatusOK      AlertStatus = "OK"
	StatusAlert   AlertStatus = "ALERT"
	StatusUnknown AlertStatus = "UNKNOWN"
)

type AlertAction int

const (
	ActionNone AlertAction = iota
	ActionFirstAlert
	ActionRepeatAlert
	ActionEscalateAlert
	ActionRecovery
	ActionFailureAlert
)

// ExpiryPolicy is the per-address alerting policy (already defaulted).
type ExpiryPolicy struct {
	ProtectedThreshold int64
	RecoveryThreshold  int64
	RepeatInterval     time.Duration
	FailureThreshold   int
}

// AlertState is the persisted alert-dedup state for one address.
// Status only holds the last confirmed OK/ALERT baseline; scan failures are
// tracked via ConsecutiveFails and never overwrite the baseline, so an API
// outage can neither trigger nor clear an energy alert.
type AlertState struct {
	Status            AlertStatus `json:"status,omitempty"`
	FirstAlertAtMs    int64       `json:"firstAlertAtMs,omitempty"`
	LastSentAtMs      int64       `json:"lastSentAtMs,omitempty"`
	LastRecoveredAtMs int64       `json:"lastRecoveredAtMs,omitempty"`
	LastSentProtected int64       `json:"lastSentProtected,omitempty"`
	ConsecutiveFails  int         `json:"consecutiveFails,omitempty"`
	FailureAlerted    bool        `json:"failureAlerted,omitempty"`
}

// NextAlertState advances the state machine for one scan result and returns
// the action to perform. snap may be nil when scanFailed is true.
func NextAlertState(st AlertState, snap *EnergySnapshot, scanFailed bool, pol ExpiryPolicy, now time.Time) (AlertState, AlertAction) {
	if scanFailed {
		st.ConsecutiveFails++
		if st.ConsecutiveFails >= pol.FailureThreshold && !st.FailureAlerted {
			st.FailureAlerted = true
			return st, ActionFailureAlert
		}
		return st, ActionNone
	}

	st.ConsecutiveFails = 0
	st.FailureAlerted = false
	nowMs := now.UnixMilli()

	if snap.ProtectedLookahead < pol.ProtectedThreshold {
		if st.Status != StatusAlert { // first alert (baseline was OK/empty)
			st.Status = StatusAlert
			st.FirstAlertAtMs = nowMs
			st.LastSentAtMs = nowMs
			st.LastSentProtected = snap.ProtectedLookahead
			return st, ActionFirstAlert
		}
		prevGap := pol.ProtectedThreshold - st.LastSentProtected
		curGap := pol.ProtectedThreshold - snap.ProtectedLookahead
		if prevGap > 0 && float64(curGap) > float64(prevGap)*1.2 {
			st.LastSentAtMs = nowMs
			st.LastSentProtected = snap.ProtectedLookahead
			return st, ActionEscalateAlert
		}
		if nowMs-st.LastSentAtMs >= pol.RepeatInterval.Milliseconds() {
			st.LastSentAtMs = nowMs
			st.LastSentProtected = snap.ProtectedLookahead
			return st, ActionRepeatAlert
		}
		return st, ActionNone
	}

	if st.Status == StatusAlert {
		if snap.ProtectedLookahead >= pol.RecoveryThreshold {
			st.Status = StatusOK
			st.LastRecoveredAtMs = nowMs
			return st, ActionRecovery
		}
		return st, ActionNone // recovery buffer: stay ALERT silently
	}
	st.Status = StatusOK
	return st, ActionNone
}
