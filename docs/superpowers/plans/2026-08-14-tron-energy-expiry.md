# TRON Energy 到期监控实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 chains/tron 中新增独立的 Energy 到期检查器：每小时查询目标地址的入站 Energy 代理，计算 lookahead（默认 72h）后仍受锁定保护的 Energy，低于阈值经 Slack 报警，支持去重/持续提醒/恢复通知/监控异常报警，状态持久化到 JSON 文件。

**Architecture:** 查询层（`con.go`，gRPC 入站代理封装）→ 计算层（`energy_calc.go`，纯函数）→ 状态机（`energy_state.go`，纯逻辑）→ 持久化（`energy_store.go`，JSON 原子写）→ 检查器（`energy.go`，独立 goroutine 每分钟 tick，按各地址周期调度）。配置扩展 `config.Energy`，`protectedThreshold==0` 时不启用（向后兼容）。

**Tech Stack:** Go、gotron-sdk（gRPC，已有依赖）、log15、util.Alarm（Slack）。无新增第三方依赖。

**Spec:** `docs/superpowers/specs/2026-08-14-tron-energy-expiry-design.md`

**测试约定:** 表驱动测试（仿 `internal/config/waterline_test.go`）。测试命令 `go test ./chains/tron/... ./internal/config/... -v -run <Name>`。

---

### Task 1: 扩展 config.Energy 结构与默认值

**Files:**
- Modify: `internal/config/config.go`（Energy 结构体，约第 71 行）
- Test: `internal/config/energy_test.go`（新建）

- [ ] **Step 1: 写失败的测试**

创建 `internal/config/energy_test.go`：

```go
package config

import "testing"

func TestEnergyApplyExpiryDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   Energy
		want Energy
	}{
		{
			name: "disabled when protectedThreshold is zero",
			in:   Energy{Address: "T1", Waterline: 100},
			want: Energy{Address: "T1", Waterline: 100},
		},
		{
			name: "fills all defaults",
			in:   Energy{Address: "T1", ProtectedThreshold: 10000000},
			want: Energy{
				Address:              "T1",
				ProtectedThreshold:   10000000,
				RecoveryThreshold:    10500000, // ×1.05 向上取整
				LookaheadHours:       72,
				CheckIntervalMinutes: 60,
				RepeatIntervalHours:  12,
			},
		},
		{
			name: "recovery rounds up",
			in:   Energy{Address: "T1", ProtectedThreshold: 3},
			want: Energy{
				Address:              "T1",
				ProtectedThreshold:   3,
				RecoveryThreshold:    4, // 3*1.05=3.15 → ceil 4
				LookaheadHours:       72,
				CheckIntervalMinutes: 60,
				RepeatIntervalHours:  12,
			},
		},
		{
			name: "explicit values are kept",
			in: Energy{
				Address:              "T1",
				ProtectedThreshold:   100,
				RecoveryThreshold:    120,
				LookaheadHours:       24,
				CheckIntervalMinutes: 30,
				RepeatIntervalHours:  6,
			},
			want: Energy{
				Address:              "T1",
				ProtectedThreshold:   100,
				RecoveryThreshold:    120,
				LookaheadHours:       24,
				CheckIntervalMinutes: 30,
				RepeatIntervalHours:  6,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in
			got.ApplyExpiryDefaults()
			if got != tt.want {
				t.Fatalf("ApplyExpiryDefaults()=%+v, want %+v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config/ -run TestEnergyApplyExpiryDefaults -v`
Expected: FAIL（编译错误：Energy 无 ProtectedThreshold 字段 / ApplyExpiryDefaults 未定义）

- [ ] **Step 3: 实现**

在 `internal/config/config.go` 中把 Energy 结构体（原第 71-74 行）替换为：

```go
type Energy struct {
	Address   string `json:"address"`
	Waterline int64  `json:"waterline"`
	// Expiry monitoring (Stake 2.0 delegation lock). Disabled when
	// ProtectedThreshold <= 0; remaining fields are defaulted by
	// ApplyExpiryDefaults.
	ProtectedThreshold   int64 `json:"protectedThreshold"`
	RecoveryThreshold    int64 `json:"recoveryThreshold"`
	LookaheadHours       int64 `json:"lookaheadHours"`
	CheckIntervalMinutes int64 `json:"checkIntervalMinutes"`
	RepeatIntervalHours  int64 `json:"repeatIntervalHours"`
}

// ApplyExpiryDefaults fills unset expiry-monitoring fields. No-op when the
// feature is disabled (ProtectedThreshold <= 0).
func (e *Energy) ApplyExpiryDefaults() {
	if e.ProtectedThreshold <= 0 {
		return
	}
	if e.RecoveryThreshold <= 0 {
		// protected × 1.05, rounded up
		e.RecoveryThreshold = e.ProtectedThreshold + (e.ProtectedThreshold+19)/20
	}
	if e.LookaheadHours <= 0 {
		e.LookaheadHours = 72
	}
	if e.CheckIntervalMinutes <= 0 {
		e.CheckIntervalMinutes = 60
	}
	if e.RepeatIntervalHours <= 0 {
		e.RepeatIntervalHours = 12
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/config/ -run TestEnergyApplyExpiryDefaults -v`
Expected: PASS。再跑 `go test ./internal/config/` 确认没破坏现有测试（注意：仓库里 internal/config 有用户未提交的改动，若原本就有失败先记录，不要归因到本任务）。

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/energy_test.go
git commit -m "feat(config): add tron energy expiry monitoring fields to Energy"
```

---

### Task 2: 计算层 energy_calc.go（纯函数）

**Files:**
- Create: `chains/tron/energy_calc.go`
- Test: `chains/tron/energy_calc_test.go`

- [ ] **Step 1: 写失败的测试**

创建 `chains/tron/energy_calc_test.go`：

```go
package tron

import (
	"testing"
	"time"
)

func TestComputeEnergySnapshot(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	lookahead := 72 * time.Hour
	hourMs := int64(3600 * 1000)
	// energyPerTRX = TotalEnergyLimit/TotalEnergyWeight = 180e9/6e9 = 30
	res := ResourceParams{
		EnergyLimit:       500_000,
		EnergyUsed:        120_000,
		TotalEnergyLimit:  180_000_000_000,
		TotalEnergyWeight: 6_000_000_000,
	}
	// 1000 TRX = 1_000_000_000 sun → 30_000 energy
	trx1000 := int64(1_000_000_000)

	dels := []DelegationDetail{
		{From: "A", FrozenBalanceSun: trx1000, ExpireTimeMs: now.UnixMilli() + 200*hourMs}, // 8天后到期：全部窗口受保护
		{From: "B", FrozenBalanceSun: trx1000, ExpireTimeMs: now.UnixMilli() + 48*hourMs},  // 48h 后到期：只受 24h 保护
		{From: "C", FrozenBalanceSun: trx1000, ExpireTimeMs: now.UnixMilli() - hourMs},     // 已到期
		{From: "D", FrozenBalanceSun: trx1000, ExpireTimeMs: 0},                            // 无锁定
		{From: "E", FrozenBalanceSun: trx1000, ExpireTimeMs: now.Add(lookahead).UnixMilli()}, // 恰好=cutoff：不计入（严格晚于）
	}

	snap, err := ComputeEnergySnapshot("T1", dels, res, now, lookahead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.TotalInboundEnergy != 150_000 {
		t.Errorf("TotalInboundEnergy=%d, want 150000", snap.TotalInboundEnergy)
	}
	if snap.Protected24H != 90_000 { // A + B + E（E 到期于 72h > 24h 窗口）
		t.Errorf("Protected24H=%d, want 90000", snap.Protected24H)
	}
	if snap.ProtectedLookahead != 30_000 { // 仅 A
		t.Errorf("ProtectedLookahead=%d, want 30000", snap.ProtectedLookahead)
	}
	if snap.Protected7D != 30_000 { // 仅 A（200h > 168h）
		t.Errorf("Protected7D=%d, want 30000", snap.Protected7D)
	}
	// lookahead 窗口内到期：B(48h) 和 E(恰好 72h)，C 已到期、D 无锁定不算
	if snap.ExpiringCount != 2 || snap.ExpiringEnergy != 60_000 {
		t.Errorf("Expiring=%d/%d, want 2/60000", snap.ExpiringCount, snap.ExpiringEnergy)
	}
	if snap.NearestExpiryMs != now.UnixMilli()+48*hourMs {
		t.Errorf("NearestExpiryMs=%d, want %d", snap.NearestExpiryMs, now.UnixMilli()+48*hourMs)
	}
	if snap.EnergyLimit != 500_000 || snap.EnergyUsed != 120_000 {
		t.Errorf("limit/used=%d/%d, want 500000/120000", snap.EnergyLimit, snap.EnergyUsed)
	}
}

func TestComputeEnergySnapshotZeroWeight(t *testing.T) {
	_, err := ComputeEnergySnapshot("T1", nil,
		ResourceParams{TotalEnergyLimit: 1, TotalEnergyWeight: 0},
		time.UnixMilli(0), 72*time.Hour)
	if err == nil {
		t.Fatal("expected error on TotalEnergyWeight=0, got nil")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./chains/tron/ -run TestComputeEnergySnapshot -v`
Expected: FAIL（编译错误：类型未定义）

- [ ] **Step 3: 实现**

创建 `chains/tron/energy_calc.go`：

```go
package tron

import (
	"fmt"
	"time"
)

// DelegationDetail is one inbound energy delegation as read from chain.
type DelegationDetail struct {
	From             string
	FrozenBalanceSun int64
	ExpireTimeMs     int64 // lock expiry, unix ms; 0 = no lock (revocable anytime)
}

// ResourceParams holds the account/network resource numbers used for the
// sun→energy conversion, from GetAccountResource.
type ResourceParams struct {
	EnergyLimit       int64
	EnergyUsed        int64
	TotalEnergyLimit  int64
	TotalEnergyWeight int64
}

// EnergySnapshot is the result of one scan of one address.
type EnergySnapshot struct {
	Address            string `json:"address"`
	QueriedAtMs        int64  `json:"queriedAtMs"`
	EnergyLimit        int64  `json:"energyLimit"`
	EnergyUsed         int64  `json:"energyUsed"`
	TotalInboundEnergy int64  `json:"totalInboundEnergy"`
	Protected24H       int64  `json:"protected24h"`
	ProtectedLookahead int64  `json:"protectedLookahead"`
	Protected7D        int64  `json:"protected7d"`
	ExpiringCount      int    `json:"expiringCount"`
	ExpiringEnergy     int64  `json:"expiringEnergy"`
	NearestExpiryMs    int64  `json:"nearestExpiryMs"`
}

// ComputeEnergySnapshot converts inbound delegations into protected-energy
// metrics. Protected = lock expiry strictly later than the window cutoff;
// expired, expiring-in-window and no-lock delegations are excluded.
func ComputeEnergySnapshot(addr string, dels []DelegationDetail, res ResourceParams, now time.Time, lookahead time.Duration) (*EnergySnapshot, error) {
	if res.TotalEnergyWeight <= 0 {
		return nil, fmt.Errorf("invalid TotalEnergyWeight %d", res.TotalEnergyWeight)
	}
	energyPerSun := float64(res.TotalEnergyLimit) / float64(res.TotalEnergyWeight) / 1_000_000

	nowMs := now.UnixMilli()
	cut24 := now.Add(24 * time.Hour).UnixMilli()
	cutLook := now.Add(lookahead).UnixMilli()
	cut7d := now.Add(7 * 24 * time.Hour).UnixMilli()

	snap := &EnergySnapshot{
		Address:     addr,
		QueriedAtMs: nowMs,
		EnergyLimit: res.EnergyLimit,
		EnergyUsed:  res.EnergyUsed,
	}
	for _, d := range dels {
		est := int64(float64(d.FrozenBalanceSun) * energyPerSun)
		snap.TotalInboundEnergy += est
		if d.ExpireTimeMs > cut24 {
			snap.Protected24H += est
		}
		if d.ExpireTimeMs > cutLook {
			snap.ProtectedLookahead += est
		}
		if d.ExpireTimeMs > cut7d {
			snap.Protected7D += est
		}
		if d.ExpireTimeMs > nowMs && d.ExpireTimeMs <= cutLook {
			snap.ExpiringCount++
			snap.ExpiringEnergy += est
		}
		if d.ExpireTimeMs > nowMs && (snap.NearestExpiryMs == 0 || d.ExpireTimeMs < snap.NearestExpiryMs) {
			snap.NearestExpiryMs = d.ExpireTimeMs
		}
	}
	return snap, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./chains/tron/ -run TestComputeEnergySnapshot -v`
Expected: PASS（两个测试都过）

- [ ] **Step 5: Commit**

```bash
git add chains/tron/energy_calc.go chains/tron/energy_calc_test.go
git commit -m "feat(tron): add protected-energy snapshot computation"
```

---

### Task 3: 状态机 energy_state.go（纯逻辑）

**Files:**
- Create: `chains/tron/energy_state.go`
- Test: `chains/tron/energy_state_test.go`

- [ ] **Step 1: 写失败的测试**

创建 `chains/tron/energy_state_test.go`：

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./chains/tron/ -run TestNextAlertState -v`
Expected: FAIL（编译错误：类型未定义）

- [ ] **Step 3: 实现**

创建 `chains/tron/energy_state.go`：

```go
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
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./chains/tron/ -run TestNextAlertState -v`
Expected: PASS（全部子测试）

- [ ] **Step 5: Commit**

```bash
git add chains/tron/energy_state.go chains/tron/energy_state_test.go
git commit -m "feat(tron): add energy expiry alert state machine"
```

---

### Task 4: 持久化 energy_store.go

**Files:**
- Create: `chains/tron/energy_store.go`
- Test: `chains/tron/energy_store_test.go`

- [ ] **Step 1: 写失败的测试**

创建 `chains/tron/energy_store_test.go`：

```go
package tron

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnergyStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "energy_state_T1.json")

	st, err := LoadEnergyState(path, "T1")
	if err != nil {
		t.Fatalf("load missing file: %v", err)
	}
	if st.Address != "T1" || len(st.Snapshots) != 0 {
		t.Fatalf("fresh state=%+v", st)
	}

	st.Alert = AlertState{Status: StatusAlert, LastSentProtected: 123}
	st.Append(SnapshotEntry{AtMs: 1, Status: StatusAlert, Snap: &EnergySnapshot{Address: "T1"}})
	if err := SaveEnergyState(path, st); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := LoadEnergyState(path, "T1")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Alert.Status != StatusAlert || got.Alert.LastSentProtected != 123 || len(got.Snapshots) != 1 {
		t.Fatalf("reloaded=%+v", got)
	}
}

func TestEnergyStateHistoryCap(t *testing.T) {
	st := &StoredEnergyState{Address: "T1"}
	for i := 0; i < maxSnapshotHistory+10; i++ {
		st.Append(SnapshotEntry{AtMs: int64(i)})
	}
	if len(st.Snapshots) != maxSnapshotHistory {
		t.Fatalf("len=%d, want %d", len(st.Snapshots), maxSnapshotHistory)
	}
	if st.Snapshots[0].AtMs != 10 {
		t.Fatalf("oldest=%d, want 10 (ring dropped head)", st.Snapshots[0].AtMs)
	}
}

func TestLoadEnergyStateCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "energy_state_T1.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := LoadEnergyState(path, "T1")
	if err == nil {
		t.Fatal("expected error for corrupt file")
	}
	if st == nil || st.Address != "T1" {
		t.Fatalf("must still return usable fresh state, got %+v", st)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./chains/tron/ -run TestEnergyState -v`
Expected: FAIL（编译错误）

- [ ] **Step 3: 实现**

创建 `chains/tron/energy_store.go`：

```go
package tron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// maxSnapshotHistory bounds the persisted scan history: 168 hourly scans ≈ 7 days.
const maxSnapshotHistory = 168

// SnapshotEntry is one scan result kept for audit.
type SnapshotEntry struct {
	AtMs   int64           `json:"atMs"`
	Status AlertStatus     `json:"status"`
	Err    string          `json:"err,omitempty"`
	Snap   *EnergySnapshot `json:"snapshot,omitempty"`
}

// StoredEnergyState is the on-disk state for one monitored address.
type StoredEnergyState struct {
	Address   string          `json:"address"`
	Alert     AlertState      `json:"alert"`
	Snapshots []SnapshotEntry `json:"snapshots"`
}

// Append adds an entry, dropping the oldest beyond maxSnapshotHistory.
func (s *StoredEnergyState) Append(e SnapshotEntry) {
	s.Snapshots = append(s.Snapshots, e)
	if n := len(s.Snapshots) - maxSnapshotHistory; n > 0 {
		s.Snapshots = append(s.Snapshots[:0:0], s.Snapshots[n:]...)
	}
}

// LoadEnergyState reads state from path. A missing file yields a fresh state
// and nil error; a corrupt file yields a fresh state AND the error, so the
// caller can log/alarm but keep monitoring.
func LoadEnergyState(path, address string) (*StoredEnergyState, error) {
	fresh := &StoredEnergyState{Address: address}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fresh, nil
		}
		return fresh, err
	}
	st := &StoredEnergyState{}
	if err := json.Unmarshal(data, st); err != nil {
		return fresh, fmt.Errorf("corrupt energy state %s: %w", path, err)
	}
	st.Address = address
	return st, nil
}

// SaveEnergyState writes state atomically (tmp file + rename), creating the
// parent directory when needed.
func SaveEnergyState(path string, st *StoredEnergyState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./chains/tron/ -run TestEnergyState -v && go test ./chains/tron/ -run TestLoadEnergyStateCorrupt -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add chains/tron/energy_store.go chains/tron/energy_store_test.go
git commit -m "feat(tron): add JSON persistence for energy expiry state"
```

---

### Task 5: 查询层 — con.go 入站代理封装

**Files:**
- Modify: `chains/tron/con.go`
- Test: `chains/tron/con_test.go`（新建，仅测 withRetry）

**背景（务必理解再动手）：** SDK 现成的 `GetDelegatedResourcesV2` 遍历的是 `ToAccounts`（出站方向，查"该地址代理给了谁"），与需求相反。必须绕过它直接用 raw stub `c.cli.Client`（类型 `api.WalletClient`）：先 `GetDelegatedResourceAccountIndexV2` 取 `FromAccounts`，再对每个 from 调 `GetDelegatedResourceV2(from→target)`。

- [ ] **Step 1: 写失败的测试（重试助手）**

创建 `chains/tron/con_test.go`：

```go
package tron

import (
	"errors"
	"testing"
	"time"
)

func TestWithRetry(t *testing.T) {
	t.Run("succeeds after transient failures", func(t *testing.T) {
		calls := 0
		err := withRetry(3, time.Millisecond, func() error {
			calls++
			if calls < 3 {
				return errors.New("transient")
			}
			return nil
		})
		if err != nil || calls != 3 {
			t.Fatalf("err=%v calls=%d, want nil/3", err, calls)
		}
	})

	t.Run("returns last error when exhausted", func(t *testing.T) {
		calls := 0
		err := withRetry(3, time.Millisecond, func() error {
			calls++
			return errors.New("boom")
		})
		if err == nil || calls != 3 {
			t.Fatalf("err=%v calls=%d, want error/3", err, calls)
		}
	})
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./chains/tron/ -run TestWithRetry -v`
Expected: FAIL（withRetry 未定义）

- [ ] **Step 3: 实现**

在 `chains/tron/con.go` 追加（import 需增加 `context`、`github.com/lbtsm/gotron-sdk/pkg/client`、`github.com/lbtsm/gotron-sdk/pkg/common`、`github.com/lbtsm/gotron-sdk/pkg/proto/api`、`github.com/pkg/errors`；注意 `errors` 用 pkg/errors，与项目其他文件一致）：

```go
const (
	delegationRequestGap = 500 * time.Millisecond
	queryMaxRetries      = 3
	queryRetryBackoff    = time.Second
	queryTimeout         = 15 * time.Second
)

// withRetry runs do up to attempts times with exponential backoff.
func withRetry(attempts int, backoff time.Duration, do func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = do(); err == nil {
			return nil
		}
		if i < attempts-1 {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return err
}

// InboundDelegations lists every energy delegation TO target (Stake 2.0).
// Any final failure returns an error — callers must treat the whole scan as
// UNKNOWN rather than compute from partial data.
func (c *Connection) InboundDelegations(target string) ([]DelegationDetail, error) {
	targetBytes, err := common.DecodeCheck(target)
	if err != nil {
		return nil, errors.Wrapf(err, "decode address %s", target)
	}

	index, err := c.delegationIndex(targetBytes)
	if err != nil {
		return nil, errors.Wrap(err, "GetDelegatedResourceAccountIndexV2")
	}

	details := make([]DelegationDetail, 0, len(index.GetFromAccounts()))
	for i, from := range index.GetFromAccounts() {
		if i > 0 {
			time.Sleep(delegationRequestGap)
		}
		list, err := c.delegationDetail(from, targetBytes)
		if err != nil {
			return nil, errors.Wrapf(err, "GetDelegatedResourceV2 from %s", common.EncodeCheck(from))
		}
		for _, d := range list.GetDelegatedResource() {
			if d.GetFrozenBalanceForEnergy() <= 0 {
				continue // bandwidth-only delegation
			}
			details = append(details, DelegationDetail{
				From:             common.EncodeCheck(d.GetFrom()),
				FrozenBalanceSun: d.GetFrozenBalanceForEnergy(),
				ExpireTimeMs:     d.GetExpireTimeForEnergy(),
			})
		}
	}
	return details, nil
}

func (c *Connection) delegationIndex(target []byte) (*core.DelegatedResourceAccountIndex, error) {
	var index *core.DelegatedResourceAccountIndex
	err := withRetry(queryMaxRetries, queryRetryBackoff, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		defer cancel()
		var e error
		index, e = c.cli.Client.GetDelegatedResourceAccountIndexV2(ctx, client.GetMessageBytes(target))
		return e
	})
	return index, err
}

func (c *Connection) delegationDetail(from, to []byte) (*api.DelegatedResourceList, error) {
	var list *api.DelegatedResourceList
	err := withRetry(queryMaxRetries, queryRetryBackoff, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		defer cancel()
		var e error
		list, e = c.cli.Client.GetDelegatedResourceV2(ctx, &api.DelegatedResourceMessage{
			FromAddress: from,
			ToAddress:   to,
		})
		return e
	})
	return list, err
}

// EnergyResourceParams reads the numbers needed for sun→energy conversion.
func (c *Connection) EnergyResourceParams(target string) (ResourceParams, error) {
	var params ResourceParams
	err := withRetry(queryMaxRetries, queryRetryBackoff, func() error {
		res, e := c.cli.GetAccountResource(target)
		if e != nil {
			return e
		}
		params = ResourceParams{
			EnergyLimit:       res.GetEnergyLimit(),
			EnergyUsed:        res.GetEnergyUsed(),
			TotalEnergyLimit:  res.GetTotalEnergyLimit(),
			TotalEnergyWeight: res.GetTotalEnergyWeight(),
		}
		return nil
	})
	return params, err
}
```

import 块还需 `github.com/lbtsm/gotron-sdk/pkg/proto/core`。注意 `core.DelegatedResourceAccountIndex` 是 stub 返回类型（在 `Tron.pb.go`，有 `GetFromAccounts() [][]byte` 方法）——如编译报类型不匹配，以 `api.WalletClient` 接口中 `GetDelegatedResourceAccountIndexV2` 的实际返回类型为准修正。

- [ ] **Step 4: 运行测试与编译**

Run: `go test ./chains/tron/ -run TestWithRetry -v && go build ./chains/tron/`
Expected: 测试 PASS，编译通过

- [ ] **Step 5: Commit**

```bash
git add chains/tron/con.go chains/tron/con_test.go
git commit -m "feat(tron): add inbound delegation gRPC queries with retry"
```

---

### Task 6: 检查器 energy.go（调度、文案、报警）

**Files:**
- Create: `chains/tron/energy.go`
- Test: `chains/tron/energy_test.go`

- [ ] **Step 1: 写失败的测试**

创建 `chains/tron/energy_test.go`：

```go
package tron

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChainSafe/log15"
	"github.com/mapprotocol/monitor/internal/config"
)

type fakeQuerier struct {
	dels []DelegationDetail
	res  ResourceParams
	err  error
}

func (f *fakeQuerier) InboundDelegations(string) ([]DelegationDetail, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.dels, nil
}

func (f *fakeQuerier) EnergyResourceParams(string) (ResourceParams, error) {
	if f.err != nil {
		return ResourceParams{}, f.err
	}
	return f.res, nil
}

func testEnergyCfg() config.Energy {
	en := config.Energy{Address: "TTest1", ProtectedThreshold: 100_000}
	en.ApplyExpiryDefaults()
	return en
}

func newTestChecker(t *testing.T, q energyQuerier) (*expiryChecker, *[]string) {
	t.Helper()
	var sent []string
	c := newExpiryChecker(log15.New(), q, "tron", t.TempDir(),
		func(_ context.Context, msg string) { sent = append(sent, msg) })
	return c, &sent
}

func TestRunOnceFirstAlertAndPersist(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	q := &fakeQuerier{
		// 30 TRX 无锁定 → protected 0 < 100_000 阈值
		dels: []DelegationDetail{{From: "A", FrozenBalanceSun: 30_000_000, ExpireTimeMs: 0}},
		res:  ResourceParams{TotalEnergyLimit: 180_000_000_000, TotalEnergyWeight: 6_000_000_000},
	}
	c, sent := newTestChecker(t, q)
	en := testEnergyCfg()

	c.runOnce(en, now)

	if len(*sent) != 1 || !strings.Contains((*sent)[0], "TRON Energy 预警") {
		t.Fatalf("sent=%v, want one first alert", *sent)
	}
	// 状态已持久化，重启（新 checker）后同样低于阈值不再首报
	c2, sent2 := newTestChecker(t, q)
	c2.dir = c.dir
	c2.runOnce(en, now.Add(time.Hour))
	if len(*sent2) != 0 {
		t.Fatalf("sent after restart=%v, want dedup (no resend within 12h)", *sent2)
	}
	st, err := LoadEnergyState(filepath.Join(c.dir, "energy_state_TTest1.json"), "TTest1")
	if err != nil || st.Alert.Status != StatusAlert || len(st.Snapshots) != 1 {
		t.Fatalf("persisted state=%+v err=%v", st, err)
	}
}

func TestRunOnceFailureIsUnknownNotZero(t *testing.T) {
	q := &fakeQuerier{err: errors.New("rpc down")}
	c, sent := newTestChecker(t, q)
	en := testEnergyCfg()
	now := time.UnixMilli(1_700_000_000_000)

	c.runOnce(en, now)
	c.runOnce(en, now.Add(time.Hour))
	if len(*sent) != 0 {
		t.Fatalf("sent=%v, want none before 3rd failure", *sent)
	}
	c.runOnce(en, now.Add(2*time.Hour))
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "监控异常") {
		t.Fatalf("sent=%v, want one failure alert", *sent)
	}
	if strings.Contains((*sent)[0], "预警]") {
		t.Fatalf("failure alert must not look like an energy alert: %v", *sent)
	}
}

func TestRunOnceRecovery(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	low := &fakeQuerier{
		dels: []DelegationDetail{{From: "A", FrozenBalanceSun: 30_000_000, ExpireTimeMs: 0}},
		res:  ResourceParams{TotalEnergyLimit: 180_000_000_000, TotalEnergyWeight: 6_000_000_000},
	}
	c, sent := newTestChecker(t, low)
	en := testEnergyCfg()
	c.runOnce(en, now) // first alert

	// 换成充足数据：10000 TRX 锁到 30 天后 → 300_000 > recovery 105_000
	c.q = &fakeQuerier{
		dels: []DelegationDetail{{From: "A", FrozenBalanceSun: 10_000_000_000,
			ExpireTimeMs: now.Add(30 * 24 * time.Hour).UnixMilli()}},
		res: ResourceParams{TotalEnergyLimit: 180_000_000_000, TotalEnergyWeight: 6_000_000_000},
	}
	c.runOnce(en, now.Add(time.Hour))
	if len(*sent) != 2 || !strings.Contains((*sent)[1], "恢复") {
		t.Fatalf("sent=%v, want alert then recovery", *sent)
	}
}

func TestFmtEnergyInt(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{{0, "0"}, {999, "999"}, {1000, "1,000"}, {10500000, "10,500,000"}, {-1234, "-1,234"}}
	for _, tt := range tests {
		if got := fmtEnergyInt(tt.in); got != tt.want {
			t.Errorf("fmtEnergyInt(%d)=%q, want %q", tt.in, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./chains/tron/ -run 'TestRunOnce|TestFmtEnergyInt' -v`
Expected: FAIL（编译错误）

- [ ] **Step 3: 实现**

创建 `chains/tron/energy.go`：

```go
package tron

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ChainSafe/log15"
	"github.com/mapprotocol/monitor/internal/config"
)

// energyQuerier is the chain-access surface the checker needs; *Connection
// implements it, tests use a fake.
type energyQuerier interface {
	InboundDelegations(target string) ([]DelegationDetail, error)
	EnergyResourceParams(target string) (ResourceParams, error)
}

const energyFailureThreshold = 3

// displayLoc is the timezone used in alert messages (storage stays UTC ms).
var displayLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// expiryChecker runs protected-energy scans and drives alerting for the
// addresses configured with a protectedThreshold.
type expiryChecker struct {
	log    log15.Logger
	q      energyQuerier
	chain  string
	dir    string // state-file directory
	states map[string]*StoredEnergyState
	alarm  func(ctx context.Context, msg string)
}

func newExpiryChecker(log log15.Logger, q energyQuerier, chainName, dir string,
	alarm func(ctx context.Context, msg string)) *expiryChecker {
	return &expiryChecker{
		log:    log,
		q:      q,
		chain:  chainName,
		dir:    dir,
		states: make(map[string]*StoredEnergyState),
		alarm:  alarm,
	}
}

func (c *expiryChecker) statePath(addr string) string {
	return filepath.Join(c.dir, "energy_state_"+addr+".json")
}

func (c *expiryChecker) state(addr string) *StoredEnergyState {
	if st, ok := c.states[addr]; ok {
		return st
	}
	st, err := LoadEnergyState(c.statePath(addr), addr)
	if err != nil {
		c.log.Error("EnergyExpiry state file unreadable, starting fresh", "addr", addr, "err", err)
	}
	c.states[addr] = st
	return st
}

// runOnce performs one scan+judge+persist+alert cycle for one address.
// en must already have ApplyExpiryDefaults applied.
func (c *expiryChecker) runOnce(en config.Energy, now time.Time) {
	st := c.state(en.Address)
	pol := ExpiryPolicy{
		ProtectedThreshold: en.ProtectedThreshold,
		RecoveryThreshold:  en.RecoveryThreshold,
		RepeatInterval:     time.Duration(en.RepeatIntervalHours) * time.Hour,
		FailureThreshold:   energyFailureThreshold,
	}
	lookahead := time.Duration(en.LookaheadHours) * time.Hour

	snap, scanErr := c.scan(en.Address, now, lookahead)
	newAlert, action := NextAlertState(st.Alert, snap, scanErr != nil, pol, now)

	entry := SnapshotEntry{AtMs: now.UnixMilli(), Snap: snap}
	if scanErr != nil {
		entry.Status = StatusUnknown
		entry.Err = scanErr.Error()
		c.log.Error("EnergyExpiry scan failed", "addr", en.Address,
			"consecutiveFails", newAlert.ConsecutiveFails, "err", scanErr)
	} else {
		entry.Status = newAlert.Status
		c.log.Info("EnergyExpiry scan", "addr", en.Address,
			"protected", snap.ProtectedLookahead, "threshold", en.ProtectedThreshold,
			"expiring", snap.ExpiringCount, "status", newAlert.Status)
	}
	st.Alert = newAlert
	st.Append(entry)
	if err := SaveEnergyState(c.statePath(en.Address), st); err != nil {
		c.log.Error("EnergyExpiry save state failed", "addr", en.Address, "err", err)
	}

	switch action {
	case ActionFirstAlert, ActionRepeatAlert, ActionEscalateAlert:
		c.alarm(context.Background(), formatEnergyAlert(c.chain, en, snap, now, lookahead))
	case ActionRecovery:
		c.alarm(context.Background(), formatEnergyRecovery(c.chain, en, snap))
	case ActionFailureAlert:
		c.alarm(context.Background(), formatEnergyFailure(c.chain, en.Address, newAlert.ConsecutiveFails, scanErr))
	}
}

func (c *expiryChecker) scan(addr string, now time.Time, lookahead time.Duration) (*EnergySnapshot, error) {
	dels, err := c.q.InboundDelegations(addr)
	if err != nil {
		return nil, err
	}
	res, err := c.q.EnergyResourceParams(addr)
	if err != nil {
		return nil, err
	}
	return ComputeEnergySnapshot(addr, dels, res, now, lookahead)
}

func fmtEnergyTime(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return time.UnixMilli(ms).In(displayLoc).Format("2006-01-02 15:04 MST")
}

func fmtEnergyInt(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func formatEnergyAlert(chainName string, en config.Energy, snap *EnergySnapshot, now time.Time, lookahead time.Duration) string {
	return fmt.Sprintf(`[TRON Energy 预警]
链：%s
地址：%s
预测时间：%s
%d 小时后受保护 Energy：%s
配置阈值：%s
缺口：%s
未来 %d 小时到期：%d 笔，共约 %s Energy
最近到期：%s
查询时间：%s`,
		chainName, en.Address,
		fmtEnergyTime(now.Add(lookahead).UnixMilli()),
		en.LookaheadHours, fmtEnergyInt(snap.ProtectedLookahead),
		fmtEnergyInt(en.ProtectedThreshold),
		fmtEnergyInt(en.ProtectedThreshold-snap.ProtectedLookahead),
		en.LookaheadHours, snap.ExpiringCount, fmtEnergyInt(snap.ExpiringEnergy),
		fmtEnergyTime(snap.NearestExpiryMs),
		fmtEnergyTime(snap.QueriedAtMs))
}

func formatEnergyRecovery(chainName string, en config.Energy, snap *EnergySnapshot) string {
	return fmt.Sprintf(`[TRON Energy 恢复]
链：%s
地址：%s
%d 小时后受保护 Energy：%s（≥ 恢复阈值 %s）
查询时间：%s`,
		chainName, en.Address,
		en.LookaheadHours, fmtEnergyInt(snap.ProtectedLookahead),
		fmtEnergyInt(en.RecoveryThreshold),
		fmtEnergyTime(snap.QueriedAtMs))
}

func formatEnergyFailure(chainName, addr string, fails int, scanErr error) string {
	return fmt.Sprintf(`[TRON Energy 监控异常]
链：%s
地址：%s
连续 %d 次查询失败，受保护 Energy 状态未知（不代表 Energy 为 0）
最近错误：%v`,
		chainName, addr, fails, scanErr)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./chains/tron/ -run 'TestRunOnce|TestFmtEnergyInt' -v`
Expected: PASS（4 个测试全过）

- [ ] **Step 5: Commit**

```bash
git add chains/tron/energy.go chains/tron/energy_test.go
git commit -m "feat(tron): add energy expiry checker with dedup alerting"
```

---

### Task 7: 接入 Monitor.Sync 并全量验证

**Files:**
- Modify: `chains/tron/monitor.go`（Sync 方法，约第 40 行）
- Modify: `chains/tron/energy.go`（追加 energyExpirySync 方法）

- [ ] **Step 1: 在 energy.go 追加调度循环**

在 `chains/tron/energy.go` 末尾追加（import 增加 `github.com/mapprotocol/monitor/internal/chain`、`github.com/mapprotocol/monitor/pkg/util`、`github.com/pkg/errors`；注意 fmt 的 errors 冲突——本文件用 pkg/errors 只在此函数）：

```go
// energyExpiryTick is how often the scheduler wakes to check per-address due
// times; actual scan cadence is each address's CheckIntervalMinutes.
const energyExpiryTick = time.Minute

// energyExpirySync schedules expiry scans for all enabled energy entries.
// It re-reads the config snapshot every tick, so hot-reloaded thresholds and
// newly added addresses are picked up without restart.
func (m *Monitor) energyExpirySync() error {
	checker := newExpiryChecker(m.Log, m.conn, m.Cfg.Name, m.Cfg.KeystorePath, util.Alarm)
	nextRun := make(map[string]time.Time)
	for {
		select {
		case <-m.Stop:
			return errors.New("energy expiry polling terminated")
		default:
			snap := m.Snapshot()
			for _, en := range snap.Energies {
				if en.ProtectedThreshold <= 0 {
					continue
				}
				en.ApplyExpiryDefaults()
				now := time.Now()
				if now.Before(nextRun[en.Address]) {
					continue
				}
				checker.runOnce(en, now)
				nextRun[en.Address] = now.Add(time.Duration(en.CheckIntervalMinutes) * time.Minute)
			}
			if !chain.SleepWithStop(m.Stop, energyExpiryTick) {
				return errors.New("energy expiry polling terminated")
			}
		}
	}
}
```

注意：`energy.go` 顶部 import 已有标准库 `fmt`；`errors` 引入的是 `github.com/pkg/errors`（项目惯例）。

- [ ] **Step 2: 修改 monitor.go 的 Sync 启动检查器**

`chains/tron/monitor.go` 的 `Sync()`（原第 40-51 行）改为：

```go
func (m *Monitor) Sync() error {
	m.Log.Debug("Starting listener...")
	m.Wg.Add(1)
	go func() {
		defer m.Wg.Done()
		if err := m.sync(); err != nil {
			m.Log.Error("Polling Account balance failed", "err", err)
		}
	}()

	m.Wg.Add(1)
	go func() {
		defer m.Wg.Done()
		if err := m.energyExpirySync(); err != nil {
			m.Log.Error("Energy expiry polling stopped", "err", err)
		}
	}()

	return nil
}
```

- [ ] **Step 3: 编译与全量测试**

Run: `go build ./... && go test ./chains/tron/... ./internal/config/...`
Expected: 编译通过；chains/tron 全部 PASS；internal/config 中若有与本改动无关的既有失败（该目录有用户未提交改动），记录但不阻塞。

- [ ] **Step 4: Commit**

```bash
git add chains/tron/energy.go chains/tron/monitor.go
git commit -m "feat(tron): wire energy expiry checker into monitor sync"
```

- [ ] **Step 5: 配置样例说明（写入 README）**

在 `README.md` 的 Options 代码块后追加：

```markdown
## TRON Energy Expiry Monitoring

Add `protectedThreshold` to a tron chain's `energy` entry to enable
Stake 2.0 delegation-expiry alerting (protected energy = inbound delegations
whose lock expires strictly after now+lookahead):

```shell
"energy": [{
  "address": "TT6GDYkpHPVk24w9he9pavbagtzqBRS3XP",
  "waterline": 100000,             // existing: current remaining-energy alarm
  "protectedThreshold": 10000000,  // alert when protected energy drops below
  "recoveryThreshold": 10500000,   // optional, default = protected × 1.05
  "lookaheadHours": 72,            // optional, default 72
  "checkIntervalMinutes": 60,      // optional, default 60
  "repeatIntervalHours": 12        // optional, default 12
}]
```

State files are written to `<keystorePath>/energy_state_<address>.json`.
Scan failures alarm separately as "监控异常" and never count as zero energy.
```

- [ ] **Step 6: Commit**

```bash
git add README.md
git commit -m "docs: document tron energy expiry monitoring config"
```

---

## 验收对照（需求第 11 章 → 实现位置)

| 验收项 | 实现 |
|---|---|
| 1 列出全部入站代理 | Task 5 `InboundDelegations` |
| 2 明细含代理方/质押/估算/到期 | Task 2 `DelegationDetail`/`EnergySnapshot` |
| 3 排除 72h 内到期与无锁定 | Task 2 `ComputeEnergySnapshot`（严格晚于 cutoff） |
| 4 一个周期内报警 | Task 7 调度 + Task 3 `ActionFirstAlert` |
| 5 不重复轰炸 | Task 3 `RepeatInterval` 去重 |
| 6 恢复通知 | Task 3 `ActionRecovery`（恢复阈值缓冲） |
| 7 API 失败不误报 0 | Task 3 scanFailed 分支 + Task 5 整轮报错 + Task 6 UNKNOWN 文案 |
| 8 历史可追溯 | Task 4 快照环形缓冲（168 轮） |
| 9 凭据不落盘 | 无新增凭据；沿用 hooks 环境变量 |
