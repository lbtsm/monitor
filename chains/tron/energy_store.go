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
