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
