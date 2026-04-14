package webdav

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// measurePeakHeap runs fn and concurrently samples runtime.MemStats.HeapInuse
// every 100ms. Returns the peak HeapInuse observed during fn's execution.
// The first sample is taken before fn starts; the last after fn returns.
//
// HeapInuse is used because it reflects the arena committed for the heap
// and is the right proxy for "is the body being buffered in memory during
// streaming". See .planning/research/PITFALLS.md §2 for the rationale
// (RSS post-hoc measurement is unreliable due to Go's arena retention;
// concurrent HeapInuse sampling is the reliable alternative).
//
// If fn panics, the panic propagates to the caller. The sampler goroutine
// is always stopped before the function returns (via a close(done) guarded
// by sync.Once semantics inherent in channel close).
func measurePeakHeap(tb testing.TB, fn func()) uint64 {
	tb.Helper()

	var peak uint64
	var samples []uint64 // kept for on-fail debug logging

	// Flush prior-test garbage so the baseline sample reflects only
	// live allocations, not debris from earlier tests. Without this,
	// the peak observed during fn() can be inflated by uncollected
	// objects from a preceding test in the same package run.
	runtime.GC()

	// Take initial sample before fn starts.
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	atomic.StoreUint64(&peak, ms.HeapInuse)
	samples = append(samples, ms.HeapInuse)

	done := make(chan struct{})
	var samplerWg sync.WaitGroup
	samplerWg.Add(1)
	go func() {
		defer samplerWg.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				samples = append(samples, m.HeapInuse)
				for {
					cur := atomic.LoadUint64(&peak)
					if m.HeapInuse <= cur {
						break
					}
					if atomic.CompareAndSwapUint64(&peak, cur, m.HeapInuse) {
						break
					}
				}
			}
		}
	}()

	fn()

	// Stop sampler, take one final sample.
	close(done)
	samplerWg.Wait()
	runtime.ReadMemStats(&ms)
	if ms.HeapInuse > atomic.LoadUint64(&peak) {
		atomic.StoreUint64(&peak, ms.HeapInuse)
	}
	samples = append(samples, ms.HeapInuse)

	// On test failure, log the full sample trail for debug.
	tb.Cleanup(func() {
		if tb.Failed() {
			tb.Logf("measurePeakHeap samples (HeapInuse bytes, 100ms cadence): %v", samples)
		}
	})

	return atomic.LoadUint64(&peak)
}

// drainStreaming reads r fully, computes SHA-256 on the fly via io.TeeReader,
// and returns (checksum_hex, n_bytes_read, err). It never allocates the full
// body as []byte — memory use is bounded by io.Copy's internal 32 KiB buffer.
//
// This is the ONLY sanctioned way to consume large response bodies in WebDAV
// tests. Accumulating helpers (ReadAll variants, httpexpect.Body().Raw(), or
// buffered writers) on bodies > 1 MB defeat the streaming validation by
// holding the entire body in test-process memory. See PITFALLS.md §1.
func drainStreaming(r io.Reader) (string, int64, error) {
	h := sha256.New()
	tee := io.TeeReader(r, h)
	n, err := io.Copy(io.Discard, tee)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// largeFixtureSeed is the deterministic seed for the rand.Source used by
// largeFixture. Hex 0x434F5A59 is the ASCII "COZY" — a mnemonic constant,
// no cryptographic significance. Two callers with the same size n receive
// identical streams, which lets Phase 5 tests compute the SHA-256 of a
// fixture before PUT and compare it against the SHA-256 of the body
// returned by GET — without ever holding the fixture bytes in memory.
const largeFixtureSeed = int64(0x434F5A59)

// largeFixture returns an io.Reader producing n deterministic pseudo-random
// bytes. The seed is hardcoded (largeFixtureSeed) so repeated calls with
// the same n produce identical output. Zero binary fixture files are checked
// into the repository — all test fixtures are generated on-the-fly.
//
// Use this for any test needing a reproducible body of arbitrary size.
// For a 1 GB fixture: largeFixture(1 << 30). Do NOT buffer the result —
// drain it via drainStreaming or pipe it directly into http.Request.Body.
func largeFixture(n int64) io.Reader {
	// rand.New(rand.NewSource(seed)) produces a *rand.Rand whose Read method
	// fills a []byte with deterministic pseudo-random bytes. Wrap in
	// io.LimitReader so it stops at exactly n bytes.
	return io.LimitReader(rand.New(rand.NewSource(largeFixtureSeed)), n)
}
