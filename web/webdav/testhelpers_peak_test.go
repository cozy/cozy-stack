package webdav

import (
	"runtime"
	"testing"
	"time"
)

// TestMeasurePeakHeap_ReturnsPeakDuringAllocation verifies that measurePeakHeap
// captures the peak HeapInuse while an allocation is live, not the final state
// after it may have been released.
func TestMeasurePeakHeap_ReturnsPeakDuringAllocation(t *testing.T) {
	const allocSize = 64 << 20 // 64 MiB

	peak := measurePeakHeap(t, func() {
		// Allocate 64 MiB and hold it for 500ms so the sampler has time to observe it.
		buf := make([]byte, allocSize)
		buf[0] = 1 // write to force the page to be committed
		time.Sleep(500 * time.Millisecond)
		// runtime.KeepAlive prevents GC from collecting buf before the sleep ends.
		// Without this, the Go compiler may determine buf is "unreachable" after
		// the write and collect it before any sampler tick fires.
		runtime.KeepAlive(buf)
	})

	// Allow generous slack: we expect at least 32 MiB observed (half the allocation).
	const minExpected = uint64(32 << 20)
	if peak < minExpected {
		t.Errorf("measurePeakHeap returned %d bytes; want >= %d bytes (32 MiB)", peak, minExpected)
	}
}

// TestMeasurePeakHeap_ReturnsNonZeroForEmptyFn verifies that even for an empty
// function, the sampler fires at least once and observes the baseline HeapInuse
// of the running test process (which is always > 0).
func TestMeasurePeakHeap_ReturnsNonZeroForEmptyFn(t *testing.T) {
	peak := measurePeakHeap(t, func() {})
	if peak == 0 {
		t.Error("measurePeakHeap returned 0 for an empty fn; want > 0 (baseline HeapInuse)")
	}
}

// TestMeasurePeakHeap_MonotonicPeak verifies that the helper captures the peak
// during fn execution, not the final value after GC may have collected the alloc.
func TestMeasurePeakHeap_MonotonicPeak(t *testing.T) {
	const allocSize = 32 << 20 // 32 MiB

	var peakDuringFn uint64

	peak := measurePeakHeap(t, func() {
		buf := make([]byte, allocSize)
		buf[0] = 1 // force page commitment
		// Hold allocation for 300ms so sampler fires during allocation.
		time.Sleep(300 * time.Millisecond)
		// Record a rough expectation: allocation is live now, so heap is large.
		peakDuringFn = uint64(allocSize) // lower bound: we know at least allocSize is live
		// KeepAlive ensures buf is live through the sleep, so the sampler can observe it.
		runtime.KeepAlive(buf)
	})

	// After fn returns, GC may have run. But the reported peak should reflect
	// what was observed during fn, which should be at least the lower bound.
	const minExpected = uint64(16 << 20) // generous: at least 16 MiB observed
	if peak < minExpected {
		t.Errorf("measurePeakHeap peak=%d bytes; want >= %d bytes; peakDuringFn lower bound=%d",
			peak, minExpected, peakDuringFn)
	}
}
