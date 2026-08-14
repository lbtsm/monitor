package tron

import (
	"testing"
	"time"
)

func pol() ExpiryPolicy {
	return ExpiryPolicy{
		ProtectedThreshold: 10_000_000,
		RecoveryThreshold:  10_500_000,
		RepeatInterval:     12 * time.Hour,
		FailureThreshold:   3,
	}
}

func snapWith(protected int64) *EnergySnapshot {
	return &EnergySnapshot{ProtectedLookahead: protected}
}

func TestNextAlertState(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)

	t.Run("first alert on drop below threshold", func(t *testing.T) {
		st, act := NextAlertState(AlertState{}, snapWith(8_000_000), false, pol(), base)
		if act != ActionFirstAlert {
			t.Fatalf("action=%v, want ActionFirstAlert", act)
		}
		if st.Status != StatusAlert || st.FirstAlertAtMs != base.UnixMilli() ||
			st.LastSentAtMs != base.UnixMilli() || st.LastSentProtected != 8_000_000 {
			t.Fatalf("state=%+v", st)
		}
	})

	t.Run("no repeat within interval", func(t *testing.T) {
		prev := AlertState{Status: StatusAlert, LastSentAtMs: base.UnixMilli(), LastSentProtected: 8_000_000}
		_, act := NextAlertState(prev, snapWith(8_100_000), false, pol(), base.Add(1*time.Hour))
		if act != ActionNone {
			t.Fatalf("action=%v, want ActionNone", act)
		}
	})

	t.Run("repeat after interval", func(t *testing.T) {
		prev := AlertState{Status: StatusAlert, LastSentAtMs: base.UnixMilli(), LastSentProtected: 8_000_000}
		st, act := NextAlertState(prev, snapWith(8_000_000), false, pol(), base.Add(12*time.Hour))
		if act != ActionRepeatAlert {
			t.Fatalf("action=%v, want ActionRepeatAlert", act)
		}
		if st.LastSentAtMs != base.Add(12*time.Hour).UnixMilli() {
			t.Fatalf("LastSentAtMs not updated: %+v", st)
		}
	})

	t.Run("escalate when gap grows over 20 percent", func(t *testing.T) {
		// prev gap = 10M-8M = 2M；current 7.5M → gap 2.5M = +25%
		prev := AlertState{Status: StatusAlert, LastSentAtMs: base.UnixMilli(), LastSentProtected: 8_000_000}
		st, act := NextAlertState(prev, snapWith(7_500_000), false, pol(), base.Add(1*time.Hour))
		if act != ActionEscalateAlert {
			t.Fatalf("action=%v, want ActionEscalateAlert", act)
		}
		if st.LastSentProtected != 7_500_000 {
			t.Fatalf("LastSentProtected not updated: %+v", st)
		}
	})

	t.Run("gap growth under 20 percent does not escalate", func(t *testing.T) {
		// prev gap 2M；current 7.7M → gap 2.3M = +15%
		prev := AlertState{Status: StatusAlert, LastSentAtMs: base.UnixMilli(), LastSentProtected: 8_000_000}
		_, act := NextAlertState(prev, snapWith(7_700_000), false, pol(), base.Add(1*time.Hour))
		if act != ActionNone {
			t.Fatalf("action=%v, want ActionNone", act)
		}
	})

	t.Run("recovery buffer holds alert", func(t *testing.T) {
		// 10.2M：≥阈值但 <恢复阈值 → 维持 ALERT 不发
		prev := AlertState{Status: StatusAlert, LastSentAtMs: base.UnixMilli(), LastSentProtected: 8_000_000}
		st, act := NextAlertState(prev, snapWith(10_200_000), false, pol(), base.Add(1*time.Hour))
		if act != ActionNone || st.Status != StatusAlert {
			t.Fatalf("action=%v status=%v, want ActionNone/ALERT", act, st.Status)
		}
	})

	t.Run("recovery above recovery threshold", func(t *testing.T) {
		prev := AlertState{Status: StatusAlert, LastSentAtMs: base.UnixMilli(), LastSentProtected: 8_000_000}
		st, act := NextAlertState(prev, snapWith(10_600_000), false, pol(), base.Add(1*time.Hour))
		if act != ActionRecovery || st.Status != StatusOK {
			t.Fatalf("action=%v status=%v, want ActionRecovery/OK", act, st.Status)
		}
		if st.LastRecoveredAtMs != base.Add(1*time.Hour).UnixMilli() {
			t.Fatalf("LastRecoveredAtMs not set: %+v", st)
		}
	})

	t.Run("ok stays ok", func(t *testing.T) {
		st, act := NextAlertState(AlertState{}, snapWith(20_000_000), false, pol(), base)
		if act != ActionNone || st.Status != StatusOK {
			t.Fatalf("action=%v status=%v, want ActionNone/OK", act, st.Status)
		}
	})

	t.Run("scan failure never triggers energy alert", func(t *testing.T) {
		st, act := NextAlertState(AlertState{Status: StatusOK}, nil, true, pol(), base)
		if act != ActionNone {
			t.Fatalf("action=%v, want ActionNone on 1st failure", act)
		}
		if st.ConsecutiveFails != 1 || st.Status != StatusOK {
			t.Fatalf("state=%+v, want fails=1 and baseline kept", st)
		}
	})

	t.Run("failure alert on 3rd consecutive failure, only once", func(t *testing.T) {
		st := AlertState{Status: StatusAlert, ConsecutiveFails: 2}
		st, act := NextAlertState(st, nil, true, pol(), base)
		if act != ActionFailureAlert || !st.FailureAlerted {
			t.Fatalf("action=%v state=%+v, want ActionFailureAlert", act, st)
		}
		if st.Status != StatusAlert {
			t.Fatalf("baseline lost: %+v", st)
		}
		_, act = NextAlertState(st, nil, true, pol(), base.Add(time.Hour))
		if act != ActionNone {
			t.Fatalf("action=%v, want ActionNone on 4th failure (already alerted)", act)
		}
	})

	t.Run("success after unknown resets fails and rejudges against baseline", func(t *testing.T) {
		// 之前是 ALERT，UNKNOWN 若干轮后恢复查询且低于阈值 → 不是首报（基线还是 ALERT），
		// 距上次发送超 12h → 重复提醒
		st := AlertState{Status: StatusAlert, ConsecutiveFails: 5, FailureAlerted: true,
			LastSentAtMs: base.UnixMilli(), LastSentProtected: 8_000_000}
		st, act := NextAlertState(st, snapWith(8_000_000), false, pol(), base.Add(13*time.Hour))
		if act != ActionRepeatAlert {
			t.Fatalf("action=%v, want ActionRepeatAlert", act)
		}
		if st.ConsecutiveFails != 0 || st.FailureAlerted {
			t.Fatalf("failure tracking not reset: %+v", st)
		}
	})
}
