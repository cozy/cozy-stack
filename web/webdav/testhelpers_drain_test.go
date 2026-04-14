package webdav

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"strings"
	"testing"
	"testing/iotest"
)

// TestDrainStreaming_ReturnsCorrectHash verifies that drainStreaming computes
// the correct SHA-256 hex and byte count for a known string.
func TestDrainStreaming_ReturnsCorrectHash(t *testing.T) {
	// SHA-256("hello world") precomputed:
	const wantHex = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	const wantN = int64(11)

	r := strings.NewReader("hello world")
	gotHex, gotN, err := drainStreaming(r)

	if err != nil {
		t.Fatalf("drainStreaming returned unexpected error: %v", err)
	}
	if gotHex != wantHex {
		t.Errorf("SHA-256 mismatch: got %q, want %q", gotHex, wantHex)
	}
	if gotN != wantN {
		t.Errorf("byte count mismatch: got %d, want %d", gotN, wantN)
	}
}

// TestDrainStreaming_LargeStream_NoAccumulation verifies that drainStreaming
// does not accumulate the body in memory. Uses measurePeakHeap to assert
// that peak HeapInuse stays within 16 MiB of baseline when draining 100 MiB.
func TestDrainStreaming_LargeStream_NoAccumulation(t *testing.T) {
	const streamSize = int64(100 << 20) // 100 MiB

	// Capture baseline before the drain.
	baselinePeak := measurePeakHeap(t, func() {})

	// Measure peak during the drain.
	drainPeak := measurePeakHeap(t, func() {
		// Use a deterministic reader (math/rand) rather than largeFixture to
		// avoid an explicit dependency on Task 3 ordering. The fixture size is
		// identical.
		r := io.LimitReader(rand.New(rand.NewSource(42)), streamSize)
		_, _, err := drainStreaming(r)
		if err != nil {
			t.Errorf("drainStreaming returned error: %v", err)
		}
	})

	// Allow up to 16 MiB above baseline. If drainStreaming was accumulating the
	// full 100 MiB body, drainPeak would be ~100 MiB above baseline.
	const allowedOverhead = uint64(16 << 20)
	if drainPeak > baselinePeak+allowedOverhead {
		t.Errorf(
			"drainStreaming appears to be accumulating: peak=%d baseline=%d overhead=%d (max allowed %d)",
			drainPeak, baselinePeak, drainPeak-baselinePeak, allowedOverhead,
		)
	}
}

// TestDrainStreaming_EmptyReader verifies that drainStreaming handles an empty
// reader correctly: returns 0 bytes, SHA-256 of empty input, and nil error.
func TestDrainStreaming_EmptyReader(t *testing.T) {
	// SHA-256("") precomputed:
	const wantHex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	r := bytes.NewReader(nil)
	gotHex, gotN, err := drainStreaming(r)

	if err != nil {
		t.Fatalf("drainStreaming returned unexpected error for empty reader: %v", err)
	}
	if gotN != 0 {
		t.Errorf("byte count mismatch: got %d, want 0", gotN)
	}
	if gotHex != wantHex {
		t.Errorf("SHA-256 mismatch for empty input: got %q, want %q", gotHex, wantHex)
	}
}

// TestDrainStreaming_ReadError verifies that drainStreaming surfaces a reader
// error and returns a non-nil error.
func TestDrainStreaming_ReadError(t *testing.T) {
	sentinel := errors.New("sentinel read error")

	// iotest.ErrReader returns a reader that always returns the given error.
	r := iotest.ErrReader(sentinel)
	_, _, err := drainStreaming(r)

	if err == nil {
		t.Fatal("drainStreaming returned nil error; want non-nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("drainStreaming returned error %v; want sentinel %v", err, sentinel)
	}
}
