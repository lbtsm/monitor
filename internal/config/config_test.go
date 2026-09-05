package config

import "testing"

func TestParseOptConfig_SyncHeightAlarmDefaultEnabled(t *testing.T) {
	cfg, err := ParseOptConfig(&ChainConfig{
		Name:     "klaytn",
		Id:       8217,
		Endpoint: "http://klaytn.local",
		Opts:     map[string]string{},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOptConfig returned error: %v", err)
	}
	if !cfg.SyncHeightAlarm {
		t.Fatal("SyncHeightAlarm = false, want true by default")
	}
}

func TestParseOptConfig_SyncHeightAlarmCanBeDisabled(t *testing.T) {
	cfg, err := ParseOptConfig(&ChainConfig{
		Name:     "klaytn",
		Id:       8217,
		Endpoint: "http://klaytn.local",
		Opts:     map[string]string{SyncHeightAlarm: "false"},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOptConfig returned error: %v", err)
	}
	if cfg.SyncHeightAlarm {
		t.Fatal("SyncHeightAlarm = true, want false when opts.syncHeightAlarm=false")
	}
}

func TestParseOptConfig_InvalidSyncHeightAlarmRejected(t *testing.T) {
	_, err := ParseOptConfig(&ChainConfig{
		Name:     "klaytn",
		Id:       8217,
		Endpoint: "http://klaytn.local",
		Opts:     map[string]string{SyncHeightAlarm: "nope"},
	}, nil, nil, nil)
	if err == nil {
		t.Fatal("ParseOptConfig returned nil error, want invalid syncHeightAlarm rejected")
	}
}
