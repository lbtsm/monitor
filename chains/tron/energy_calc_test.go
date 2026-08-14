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
