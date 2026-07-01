package chain

import (
	"testing"
	"time"
)

func TestSleepWithStopReturnsFalseWhenStopped(t *testing.T) {
	stop := make(chan int)
	close(stop)

	start := time.Now()
	if SleepWithStop(stop, time.Second) {
		t.Fatal("SleepWithStop should return false when stop is closed")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("SleepWithStop waited too long after stop: %s", elapsed)
	}
}

func TestSleepWithStopReturnsTrueAfterDuration(t *testing.T) {
	stop := make(chan int)

	if !SleepWithStop(stop, time.Millisecond) {
		t.Fatal("SleepWithStop should return true when duration elapses")
	}
}
