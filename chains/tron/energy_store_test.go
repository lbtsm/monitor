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
