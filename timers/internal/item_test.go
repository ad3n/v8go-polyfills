package internal

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestTimeoutRunsOnceAndClears(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	cleared := make(chan struct{})
	item := &Item{
		ID:    1,
		Delay: 10,
		FunctionCB: func() {
			calls.Add(1)
		},
		ClearCB: func(int32) {
			close(cleared)
		},
	}
	item.Start()

	select {
	case <-cleared:
	case <-time.After(time.Second):
		t.Fatal("timeout did not finish")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("callback calls = %d, want 1", got)
	}
}

func TestClearBeforeStart(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	item := &Item{Delay: 10, FunctionCB: func() { calls.Add(1) }}
	item.Clear()
	item.Start()
	time.Sleep(30 * time.Millisecond)

	if got := calls.Load(); got != 0 {
		t.Fatalf("callback calls = %d, want 0", got)
	}
}
