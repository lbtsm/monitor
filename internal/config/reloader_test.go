package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// helper: minimal valid Config (must contain a "map" chain).
// applyDefaults is run so the seed mirrors production state (where GetConfig
// has already normalised the structure before storing).
func validRawConfig() Config {
	c := Config{
		Chains: []RawChainConfig{
			{Name: "map", Id: "22776", Endpoint: "http://map.local"},
			{Name: "bsc", Id: "56", Endpoint: "http://bsc.local"},
		},
	}
	c.applyDefaults()
	return c
}

func writeJSON(t *testing.T, dir, name string, c Config) string {
	t.Helper()
	p := filepath.Join(dir, name)
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReloadFromFile_HappyPath(t *testing.T) {
	dir := t.TempDir()
	old := validRawConfig()
	store := NewStore(&old)

	updated := validRawConfig()
	updated.Chains[1].Endpoint = "http://bsc.new"
	path := writeJSON(t, dir, "config.json", updated)

	if err := ReloadFromFile(store, path); err != nil {
		t.Fatalf("ReloadFromFile error: %v", err)
	}

	got := store.Load()
	if got.Chains[1].Endpoint != "http://bsc.new" {
		t.Fatalf("after reload, bsc endpoint = %q, want http://bsc.new", got.Chains[1].Endpoint)
	}
}

func TestReloadFromFile_InvalidConfigKeepsOld(t *testing.T) {
	dir := t.TempDir()
	old := validRawConfig()
	store := NewStore(&old)
	prev := store.Load()

	bad := Config{Chains: []RawChainConfig{{Name: "map"}}} // missing endpoint
	path := writeJSON(t, dir, "config.json", bad)

	if err := ReloadFromFile(store, path); err == nil {
		t.Fatal("expected error on invalid config, got nil")
	}
	if got := store.Load(); got != prev {
		t.Fatal("invalid reload mutated the store")
	}
}

func TestReloadFromFile_RejectsImmutableTypeChange(t *testing.T) {
	dir := t.TempDir()
	old := validRawConfig()
	old.Chains[1].Type = "ethereum"
	store := NewStore(&old)
	prev := store.Load()

	updated := validRawConfig()
	updated.Chains[1].Type = "tron" // type changed
	path := writeJSON(t, dir, "config.json", updated)

	err := ReloadFromFile(store, path)
	if err == nil {
		t.Fatal("expected error on type change, got nil")
	}
	if got := store.Load(); got != prev {
		t.Fatal("rejected reload mutated the store")
	}
}

func TestReloadFromFile_RejectsImmutableIdChange(t *testing.T) {
	dir := t.TempDir()
	old := validRawConfig()
	store := NewStore(&old)
	prev := store.Load()

	updated := validRawConfig()
	updated.Chains[1].Id = "999" // id changed
	path := writeJSON(t, dir, "config.json", updated)

	if err := ReloadFromFile(store, path); err == nil {
		t.Fatal("expected error on id change, got nil")
	}
	if got := store.Load(); got != prev {
		t.Fatal("rejected reload mutated the store")
	}
}

func TestReloadFromFile_RejectsKeystorePathChange(t *testing.T) {
	dir := t.TempDir()
	old := validRawConfig()
	old.KeystorePath = "/keys/v1"
	store := NewStore(&old)
	prev := store.Load()

	updated := validRawConfig()
	updated.KeystorePath = "/keys/v2"
	path := writeJSON(t, dir, "config.json", updated)

	if err := ReloadFromFile(store, path); err == nil {
		t.Fatal("expected error on keystorePath change, got nil")
	}
	if got := store.Load(); got != prev {
		t.Fatal("rejected reload mutated the store")
	}
}

func TestReloadFromFile_RejectsCheckHeightCountChange(t *testing.T) {
	dir := t.TempDir()
	old := validRawConfig()
	old.Chains[1].Opts = map[string]string{"checkHeightCount": "100"}
	store := NewStore(&old)

	updated := validRawConfig()
	updated.Chains[1].Opts = map[string]string{"checkHeightCount": "200"}
	path := writeJSON(t, dir, "config.json", updated)

	if err := ReloadFromFile(store, path); err == nil {
		t.Fatal("expected error on checkHeightCount change, got nil")
	}
}

func TestReloadFromFile_RejectsChangeIntervalChange(t *testing.T) {
	dir := t.TempDir()
	old := validRawConfig()
	old.Chains[1].Opts = map[string]string{"changeInterval": "60"}
	store := NewStore(&old)

	updated := validRawConfig()
	updated.Chains[1].Opts = map[string]string{"changeInterval": "120"}
	path := writeJSON(t, dir, "config.json", updated)

	if err := ReloadFromFile(store, path); err == nil {
		t.Fatal("expected error on changeInterval change, got nil")
	}
}

func TestReloadFromFile_RejectsNameRenameWithoutIdChange(t *testing.T) {
	// pure rename (chain.id stays same, only name flips) is rejected
	dir := t.TempDir()
	old := validRawConfig()
	store := NewStore(&old)
	prev := store.Load()

	updated := validRawConfig()
	updated.Chains[1].Name = "bsc-renamed"
	path := writeJSON(t, dir, "config.json", updated)

	if err := ReloadFromFile(store, path); err == nil {
		t.Fatal("expected error on lonely name change, got nil")
	}
	if got := store.Load(); got != prev {
		t.Fatal("rejected reload mutated the store")
	}
}

func TestReloadFromFile_AllowsEndpointChange(t *testing.T) {
	dir := t.TempDir()
	old := validRawConfig()
	store := NewStore(&old)

	updated := validRawConfig()
	updated.Chains[1].Endpoint = "http://bsc.new"
	path := writeJSON(t, dir, "config.json", updated)

	if err := ReloadFromFile(store, path); err != nil {
		t.Fatalf("endpoint change should be allowed, got error: %v", err)
	}
}

func TestReloadFromFile_AllowsAddRemoveChain(t *testing.T) {
	dir := t.TempDir()
	old := validRawConfig()
	store := NewStore(&old)

	// add a new chain
	added := validRawConfig()
	added.Chains = append(added.Chains, RawChainConfig{Name: "eth", Id: "1", Endpoint: "http://eth.local"})
	path := writeJSON(t, dir, "config-add.json", added)
	if err := ReloadFromFile(store, path); err != nil {
		t.Fatalf("adding a chain should be allowed: %v", err)
	}

	// remove the new chain
	removed := validRawConfig()
	path2 := writeJSON(t, dir, "config-rm.json", removed)
	if err := ReloadFromFile(store, path2); err != nil {
		t.Fatalf("removing a chain should be allowed: %v", err)
	}
}

func TestWatchSignals_ReloadsWhenConfigFileChanges(t *testing.T) {
	dir := t.TempDir()
	old := validRawConfig()
	old.Chains[1].Opts = map[string]string{WaterLine: "50"}
	path := writeJSON(t, dir, "config.json", old)

	store := NewStore(&old)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go WatchSignals(ctx, store, path)
	time.Sleep(100 * time.Millisecond)

	updated := validRawConfig()
	updated.Chains[1].Opts = map[string]string{WaterLine: "5"}
	writeJSON(t, dir, "config.json", updated)

	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for file reload, waterLine=%q", store.Load().Chains[1].Opts[WaterLine])
		case <-tick.C:
			if got := store.Load().Chains[1].Opts[WaterLine]; got == "5" {
				return
			}
		}
	}
}
