package config

import "testing"

func TestEnergyApplyExpiryDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   Energy
		want Energy
	}{
		{
			name: "disabled when protectedThreshold is zero",
			in:   Energy{Address: "T1", Waterline: 100},
			want: Energy{Address: "T1", Waterline: 100},
		},
		{
			name: "fills all defaults",
			in:   Energy{Address: "T1", ProtectedThreshold: 10000000},
			want: Energy{
				Address:              "T1",
				ProtectedThreshold:   10000000,
				RecoveryThreshold:    10500000, // ×1.05 向上取整
				LookaheadHours:       72,
				CheckIntervalMinutes: 60,
				RepeatIntervalHours:  12,
			},
		},
		{
			name: "recovery rounds up",
			in:   Energy{Address: "T1", ProtectedThreshold: 3},
			want: Energy{
				Address:              "T1",
				ProtectedThreshold:   3,
				RecoveryThreshold:    4, // 3*1.05=3.15 → ceil 4
				LookaheadHours:       72,
				CheckIntervalMinutes: 60,
				RepeatIntervalHours:  12,
			},
		},
		{
			name: "explicit values are kept",
			in: Energy{
				Address:              "T1",
				ProtectedThreshold:   100,
				RecoveryThreshold:    120,
				LookaheadHours:       24,
				CheckIntervalMinutes: 30,
				RepeatIntervalHours:  6,
			},
			want: Energy{
				Address:              "T1",
				ProtectedThreshold:   100,
				RecoveryThreshold:    120,
				LookaheadHours:       24,
				CheckIntervalMinutes: 30,
				RepeatIntervalHours:  6,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in
			got.ApplyExpiryDefaults()
			if got != tt.want {
				t.Fatalf("ApplyExpiryDefaults()=%+v, want %+v", got, tt.want)
			}
		})
	}
}
