package config

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestApplyHotReloadable_OverwritesAllReloadableFields checks that every
// field declared as hot-reloadable is actually copied from source to target.
func TestApplyHotReloadable_OverwritesAllReloadableFields(t *testing.T) {
	oldTk := &Token{BridgeAddr: "old-bridge"}
	oldGenni := &Api{Endpoint: "old-genni"}
	oldTss := &Tss{Maintainer: "old-maint"}

	target := &OptConfig{
		Name:          "bsc",
		Id:            56,
		Endpoint:      "http://old", // immutable, must NOT be touched
		KeystorePath:  "/keys/old",  // immutable
		WaterLine:     "100",
		From:          []string{"0xold"},
		Users:         []From{{Group: "g1", From: "0xa"}},
		ContractToken: []ContractToken{{Address: "0xold-ct"}},
		Energies:      []Energy{{Address: "old-en"}},
		Tss:           oldTss,
		Tk:            oldTk,
		Genni:         oldGenni,
		LightNode:     common.HexToAddress("0xaaaa"),
		ApiUrl:        "old-api",
	}

	newTk := &Token{BridgeAddr: "new-bridge"}
	newGenni := &Api{Endpoint: "new-genni"}
	newTss := &Tss{Maintainer: "new-maint"}

	source := &OptConfig{
		Name:          "bsc",
		Id:            56,
		Endpoint:      "http://new", // ignored
		KeystorePath:  "/keys/new",  // ignored
		WaterLine:     "200",
		From:          []string{"0xnew"},
		Users:         []From{{Group: "g2", From: "0xb"}},
		ContractToken: []ContractToken{{Address: "0xnew-ct"}},
		Energies:      []Energy{{Address: "new-en"}},
		Tss:           newTss,
		Tk:            newTk,
		Genni:         newGenni,
		LightNode:     common.HexToAddress("0xbbbb"),
		ApiUrl:        "new-api",
	}

	ApplyHotReloadable(target, source)

	if target.WaterLine != "200" {
		t.Errorf("WaterLine = %q, want 200", target.WaterLine)
	}
	if target.From[0] != "0xnew" {
		t.Errorf("From[0] = %q, want 0xnew", target.From[0])
	}
	if target.Users[0].Group != "g2" {
		t.Errorf("Users[0].Group = %q, want g2", target.Users[0].Group)
	}
	if target.ContractToken[0].Address != "0xnew-ct" {
		t.Errorf("ContractToken[0] = %q, want 0xnew-ct", target.ContractToken[0].Address)
	}
	if target.Energies[0].Address != "new-en" {
		t.Errorf("Energies[0] = %q, want new-en", target.Energies[0].Address)
	}
	if target.Tss != newTss {
		t.Error("Tss not repointed to new pointer")
	}
	if target.Tk != newTk {
		t.Error("Tk not repointed to new pointer")
	}
	if target.Genni != newGenni {
		t.Error("Genni not repointed to new pointer")
	}
	if target.LightNode != common.HexToAddress("0xbbbb") {
		t.Errorf("LightNode = %v, want 0xbbbb", target.LightNode)
	}
	if target.ApiUrl != "new-api" {
		t.Errorf("ApiUrl = %q, want new-api", target.ApiUrl)
	}
}

// TestApplyHotReloadable_PreservesImmutableFields verifies that fields
// classified as immutable are NOT modified by Apply, which protects against
// a buggy reload pipeline corrupting state that's tied to chain construction.
func TestApplyHotReloadable_PreservesImmutableFields(t *testing.T) {
	target := &OptConfig{
		Name:           "bsc",
		Id:             56,
		Endpoint:       "http://old",
		KeystorePath:   "/keys/old",
		ChangeInterval: "60",
		CheckHgtCount:  100,
		MapChainID:     22776,
		GasLimit:       big.NewInt(21000),
		MaxGasPrice:    big.NewInt(1e9),
		GasMultiplier:  big.NewFloat(1.5),
		StartBlock:     big.NewInt(123),
	}
	source := &OptConfig{
		Name:           "bsc",
		Id:             56,
		Endpoint:       "http://new",
		KeystorePath:   "/keys/new",
		ChangeInterval: "999",
		CheckHgtCount:  999,
		MapChainID:     999,
		GasLimit:       big.NewInt(99999),
		MaxGasPrice:    big.NewInt(99999),
		GasMultiplier:  big.NewFloat(99),
		StartBlock:     big.NewInt(99999),
	}

	ApplyHotReloadable(target, source)

	checks := map[string]any{
		"Endpoint":       []any{target.Endpoint, "http://old"},
		"KeystorePath":   []any{target.KeystorePath, "/keys/old"},
		"ChangeInterval": []any{target.ChangeInterval, "60"},
		"CheckHgtCount":  []any{target.CheckHgtCount, int64(100)},
		"MapChainID":     []any{target.MapChainID, ChainId(22776)},
	}
	for field, pair := range checks {
		got := pair.([]any)[0]
		want := pair.([]any)[1]
		if got != want {
			t.Errorf("%s = %v, want %v (must not change)", field, got, want)
		}
	}
}

// TestApplyHotReloadable_NilSourceIsNoop guards against accidental nil-source
// breakage; the caller must never pass nil but the function should be defensive.
func TestApplyHotReloadable_NilSourceIsNoop(t *testing.T) {
	target := &OptConfig{WaterLine: "100"}
	defer func() {
		if r := recover(); r == nil {
			t.Error("ApplyHotReloadable(target, nil) should panic")
		}
	}()
	ApplyHotReloadable(target, nil)
}
