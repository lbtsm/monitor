package tron

import (
	"errors"
	"testing"
	"time"
)

func TestWithRetry(t *testing.T) {
	t.Run("succeeds after transient failures", func(t *testing.T) {
		calls := 0
		err := withRetry(3, time.Millisecond, func() error {
			calls++
			if calls < 3 {
				return errors.New("transient")
			}
			return nil
		})
		if err != nil || calls != 3 {
			t.Fatalf("err=%v calls=%d, want nil/3", err, calls)
		}
	})

	t.Run("returns last error when exhausted", func(t *testing.T) {
		calls := 0
		err := withRetry(3, time.Millisecond, func() error {
			calls++
			return errors.New("boom")
		})
		if err == nil || calls != 3 {
			t.Fatalf("err=%v calls=%d, want error/3", err, calls)
		}
	})
}
