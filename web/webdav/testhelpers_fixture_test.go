package webdav

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLargeFixture_Deterministic verifies that two calls to largeFixture with
// the same n produce identical byte streams (same SHA-256 hash).
func TestLargeFixture_Deterministic(t *testing.T) {
	const n = int64(1024)

	hex1, n1, err1 := drainStreaming(largeFixture(n))
	if err1 != nil {
		t.Fatalf("first drainStreaming: %v", err1)
	}

	hex2, n2, err2 := drainStreaming(largeFixture(n))
	if err2 != nil {
		t.Fatalf("second drainStreaming: %v", err2)
	}

	if hex1 != hex2 {
		t.Errorf("largeFixture(%d) is not deterministic: got %q and %q", n, hex1, hex2)
	}
	if n1 != n2 {
		t.Errorf("largeFixture(%d) byte counts differ: %d vs %d", n, n1, n2)
	}
}

// TestLargeFixture_ExactByteCount verifies that largeFixture(n) produces
// exactly n bytes.
func TestLargeFixture_ExactByteCount(t *testing.T) {
	const n = int64(5000)

	_, gotN, err := drainStreaming(largeFixture(n))
	if err != nil {
		t.Fatalf("drainStreaming: %v", err)
	}
	if gotN != n {
		t.Errorf("largeFixture(%d) produced %d bytes; want exactly %d", n, gotN, n)
	}
}

// TestLargeFixture_LargeSize_StreamsWithoutAllocation verifies that largeFixture
// itself is a streaming generator — wrapping it in drainStreaming must not cause
// memory usage to spike by more than 16 MiB above baseline when generating 100 MiB.
func TestLargeFixture_LargeSize_StreamsWithoutAllocation(t *testing.T) {
	const streamSize = int64(100 << 20) // 100 MiB

	baselinePeak := measurePeakHeap(t, func() {})

	drainPeak := measurePeakHeap(t, func() {
		_, _, err := drainStreaming(largeFixture(streamSize))
		if err != nil {
			t.Errorf("drainStreaming(largeFixture(100 MiB)): %v", err)
		}
	})

	const allowedOverhead = uint64(16 << 20)
	if drainPeak > baselinePeak+allowedOverhead {
		t.Errorf(
			"largeFixture appears to accumulate: peak=%d baseline=%d overhead=%d (max allowed %d)",
			drainPeak, baselinePeak, drainPeak-baselinePeak, allowedOverhead,
		)
	}
}

// TestLargeFixture_DifferentSizesDifferentHash verifies that largeFixture with
// different sizes produces different byte streams (the stream is not constant).
func TestLargeFixture_DifferentSizesDifferentHash(t *testing.T) {
	hex100, _, err1 := drainStreaming(largeFixture(100))
	if err1 != nil {
		t.Fatalf("drainStreaming(largeFixture(100)): %v", err1)
	}

	hex200, _, err2 := drainStreaming(largeFixture(200))
	if err2 != nil {
		t.Fatalf("drainStreaming(largeFixture(200)): %v", err2)
	}

	if hex100 == hex200 {
		t.Error("largeFixture(100) and largeFixture(200) produced the same hash — stream is constant (bug)")
	}
}

// TestLargeFixture_NoBinaryFilesInRepo guards against regressions where large
// binary fixtures are accidentally checked into the testdata/ directory.
// Fixtures must always be generated on-the-fly via largeFixture.
func TestLargeFixture_NoBinaryFilesInRepo(t *testing.T) {
	root := "testdata"
	if _, err := os.Stat(root); os.IsNotExist(err) {
		// No testdata dir — perfect.
		return
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Size() > 100*1024 {
			t.Errorf(
				"large binary fixture detected: %s (%d bytes) — fixtures must be generated via largeFixture, not checked in",
				path, info.Size(),
			)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
