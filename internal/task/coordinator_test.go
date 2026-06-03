package task

import (
	"testing"
	"time"
)

func TestConcurrentStartStaggerDisabledForSerial(t *testing.T) {
	if got := concurrentStartStagger(0, 1); got != 0 {
		t.Fatalf("serial stagger should be 0, got %s", got)
	}
}

func TestConcurrentStartStaggerSpreadsInitialConcurrencyWindow(t *testing.T) {
	cases := []struct {
		idx      int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{idx: 0, minDelay: 0, maxDelay: concurrentStartStaggerJitterMax},
		{idx: 1, minDelay: 100 * time.Millisecond, maxDelay: 100*time.Millisecond + concurrentStartStaggerJitterMax},
		{idx: 9, minDelay: 900 * time.Millisecond, maxDelay: 900*time.Millisecond + concurrentStartStaggerJitterMax},
		{idx: 10, minDelay: 0, maxDelay: concurrentStartStaggerJitterMax},
	}

	for _, tc := range cases {
		got := concurrentStartStagger(tc.idx, 10)
		if got < tc.minDelay || got > tc.maxDelay {
			t.Fatalf("idx %d stagger = %s, want within [%s, %s]", tc.idx, got, tc.minDelay, tc.maxDelay)
		}
	}
}
