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
