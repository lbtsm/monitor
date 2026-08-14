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
	// 两轮扫描（首报一轮 + 重启后去重一轮）都应留下快照记录
	st, err := LoadEnergyState(filepath.Join(c.dir, "energy_state_TTest1.json"), "TTest1")
	if err != nil || st.Alert.Status != StatusAlert || len(st.Snapshots) != 2 {
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
