package monitor

import (
	"testing"

	"github.com/mapprotocol/monitor/internal/chain"
	"github.com/mapprotocol/monitor/internal/config"
)

// TestPrepareTick_ReadsLatestWaterLine: each call to prepareTick should
// observe whatever WaterLine is currently in the OptConfig, even when the
// value was mutated through UpdateCfg between calls. This is the key
// property that makes WaterLine hot-reloadable without restarting sync().
func TestPrepareTick_ReadsLatestWaterLine(t *testing.T) {
	cfg := &config.OptConfig{Name: "t", WaterLine: "100"}
	cs := chain.NewCommonSync(nil, cfg, nil, nil, nil)
	m := New(cs)

	_, wl, ok := m.prepareTick()
	if !ok || wl.String() != "100000000000000000000" {
		t.Fatalf("first tick: ok=%v wl=%v, want ok=true wl=100000000000000000000", ok, wl)
	}

	cs.UpdateCfg(func(o *config.OptConfig) { o.WaterLine = "200" })

	_, wl, ok = m.prepareTick()
	if !ok || wl.String() != "200000000000000000000" {
		t.Fatalf("second tick: ok=%v wl=%v, want ok=true wl=200000000000000000000", ok, wl)
	}
}

// TestPrepareTick_InvalidWaterLineReturnsNotOk: a malformed WaterLine should
// surface as ok=false so the polling loop can shut the chain down with a
// SysErr — same behaviour as the pre-refactor code on first parse.
func TestPrepareTick_InvalidWaterLineReturnsNotOk(t *testing.T) {
	cfg := &config.OptConfig{Name: "t", WaterLine: "not-a-number"}
	cs := chain.NewCommonSync(nil, cfg, nil, nil, nil)
	m := New(cs)

	_, _, ok := m.prepareTick()
	if ok {
		t.Fatal("expected ok=false for malformed WaterLine")
	}
}

// TestPrepareTick_SnapshotIsIndependent: the returned snapshot is a value
// copy; subsequent UpdateCfg writes do not retroactively mutate it.
func TestPrepareTick_SnapshotIsIndependent(t *testing.T) {
	cfg := &config.OptConfig{Name: "t", WaterLine: "100"}
	cs := chain.NewCommonSync(nil, cfg, nil, nil, nil)
	m := New(cs)

	snap, _, _ := m.prepareTick()
	cs.UpdateCfg(func(o *config.OptConfig) { o.WaterLine = "999" })

	if snap.WaterLine != "100" {
		t.Fatalf("snapshot mutated retroactively, WaterLine=%q", snap.WaterLine)
	}
}
