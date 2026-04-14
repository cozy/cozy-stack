package webdav

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/studio-b12/gowebdav"
)

// largeFileSize is the body size used by all LARGE tests. 1 GiB is the
// figure asserted in LARGE-01 and LARGE-02 (see .planning/REQUIREMENTS.md).
// Do not turn this into an env var override — reproducibility beats
// iteration speed for this suite. Use `-short` to skip these tests during
// local work.
const largeFileSize int64 = 1 << 30 // 1 GiB

// largeHeapCeiling is the hard upper bound on peak HeapInuse during a
// 1 GiB transfer. 128 MiB is the figure asserted in LARGE-01 and LARGE-02.
const largeHeapCeiling uint64 = 128 << 20 // 128 MiB

// largeClientTimeout bounds the total time a gowebdav client call may take
// for a 1 GiB transfer on loopback. Generous headroom for loaded CI runners.
const largeClientTimeout = 5 * time.Minute

// largeBearerAuth is a minimal gowebdav.Authenticator that sets a Bearer
// token header on every request without any retry buffering. It is used
// with gowebdav.NewPreemptiveAuth so that the gowebdav request loop never
// wraps the PUT body in a bytes.Buffer for auth-retry purposes.
//
// Background: gowebdav's default Authorizer (NewAutoAuth) buffers the
// request body via io.TeeReader into a bytes.Buffer so it can replay the
// body on auth-challenge retry. For a 1 GiB non-seekable reader this would
// accumulate the entire body in memory, defeating the streaming assertion.
// NewPreemptiveAuth passes the body io.Reader through unmodified, which
// preserves the streaming property end-to-end.
type largeBearerAuth struct {
	token string
}

func (a largeBearerAuth) Authorize(_ *http.Client, rq *http.Request, _ string) error {
	rq.Header.Set("Authorization", "Bearer "+a.token)
	return nil
}

func (a largeBearerAuth) Verify(_ *http.Client, rs *http.Response, _ string) (bool, error) {
	return false, nil
}

func (a largeBearerAuth) Close() error { return nil }

func (a largeBearerAuth) Clone() gowebdav.Authenticator { return a }

// newLargeTestClient returns a gowebdav.Client configured against env.TS
// with the 5-minute timeout required for 1 GiB loopback transfers.
//
// Critically, it uses NewPreemptiveAuth + largeBearerAuth instead of the
// default NewAutoAuth. This prevents gowebdav from wrapping the PUT body
// in a bytes.Buffer (which NewAutoAuth does to support auth-challenge
// retry on non-seekable readers). With NewPreemptiveAuth, the body
// io.Reader is passed directly to the HTTP transport — preserving
// end-to-end streaming for the 1 GiB fixture.
func newLargeTestClient(t *testing.T, env *webdavTestEnv) *gowebdav.Client {
	t.Helper()
	auth := gowebdav.NewPreemptiveAuth(largeBearerAuth{token: env.Token})
	c := gowebdav.NewAuthClient(env.TS.URL+"/dav/files", auth)
	c.SetTimeout(largeClientTimeout)
	require.NoError(t, c.Connect(), "gowebdav client must connect")
	return c
}

// putLargeFixture uploads largeFileSize deterministic bytes to path via
// gowebdav using WriteStreamWithLength (the only gowebdav method that
// avoids client-side buffering for non-seekable readers — see
// .planning/phases/05-large-file-streaming-proof/05-RESEARCH.md
// §Critical Finding).
//
// This helper does NOT assert heap bounds. TestPut_LargeFile_Streaming
// is the authoritative proof that the PUT path is streaming; using this
// helper inside TestGet_LargeFile's setup would otherwise duplicate that
// assertion in a hidden form.
func putLargeFixture(t *testing.T, client *gowebdav.Client, path string) {
	t.Helper()
	err := client.WriteStreamWithLength(path, largeFixture(largeFileSize), largeFileSize, 0)
	require.NoError(t, err, "putLargeFixture: WriteStreamWithLength %s", path)
}

// TestPut_LargeFile_Streaming proves LARGE-01: a 1 GiB PUT via gowebdav
// completes successfully and the server's peak HeapInuse during the
// transfer stays below 128 MiB.
//
// This test skips under -short (anticipating Phase 8 CI-03). The speed
// log at the end is informative only — v1.2 makes no assertion on MB/s.
func TestPut_LargeFile_Streaming(t *testing.T) {
	if testing.Short() {
		t.Skip("LARGE test: skipped in -short mode")
	}

	env := newWebdavTestEnv(t, nil)
	client := newLargeTestClient(t, env)

	start := time.Now()
	peak := measurePeakHeap(t, func() {
		err := client.WriteStreamWithLength(
			"/large.bin",
			largeFixture(largeFileSize),
			largeFileSize,
			0,
		)
		require.NoError(t, err, "PUT 1 GiB via WriteStreamWithLength must succeed")
	})
	elapsed := time.Since(start)

	require.Less(t, peak, largeHeapCeiling,
		"PUT peak HeapInuse %d bytes exceeds 128 MiB ceiling", peak)

	mbps := float64(largeFileSize) / float64(1<<20) / elapsed.Seconds()
	t.Logf("PUT LARGE: %.1f MB/s (peak heap %d B)", mbps, peak)
}

// TestGet_LargeFile proves LARGE-02: a 1 GiB GET via gowebdav completes
// successfully, the SHA-256 of the downloaded body matches the uploaded
// fixture, and the server's peak HeapInuse during the download stays
// below 128 MiB.
//
// Setup path: PUT the fixture via gowebdav (full HTTP-in, HTTP-out).
// The setup PUT is NOT heap-measured — TestPut_LargeFile_Streaming is
// the authoritative proof of the PUT streaming path.
func TestGet_LargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("LARGE test: skipped in -short mode")
	}

	env := newWebdavTestEnv(t, nil)
	client := newLargeTestClient(t, env)

	// Setup: upload the 1 GiB fixture. This is NOT the measured test.
	// See TestPut_LargeFile_Streaming for the authoritative PUT proof.
	putLargeFixture(t, client, "/large.bin")

	// Expected SHA-256 is computed from the same deterministic seed as
	// the body uploaded above. drainStreaming bounds memory via
	// io.TeeReader + io.Copy — no full-body buffering.
	expectedSum, expectedN, err := drainStreaming(largeFixture(largeFileSize))
	require.NoError(t, err, "computing expected SHA-256 from largeFixture must succeed")
	require.Equal(t, largeFileSize, expectedN, "fixture size sanity")

	var actualSum string
	var actualN int64

	start := time.Now()
	peak := measurePeakHeap(t, func() {
		rc, rerr := client.ReadStream("/large.bin")
		require.NoError(t, rerr, "ReadStream must open")
		defer rc.Close()

		sum, n, derr := drainStreaming(rc)
		require.NoError(t, derr, "drainStreaming must complete")
		actualSum = sum
		actualN = n
	})
	elapsed := time.Since(start)

	require.Equal(t, largeFileSize, actualN,
		"GET body length %d does not match fixture size %d", actualN, largeFileSize)
	require.Equal(t, expectedSum, actualSum,
		"GET body SHA-256 must match PUT fixture")
	require.Less(t, peak, largeHeapCeiling,
		"GET peak HeapInuse %d bytes exceeds 128 MiB ceiling", peak)

	mbps := float64(largeFileSize) / float64(1<<20) / elapsed.Seconds()
	t.Logf("GET LARGE: %.1f MB/s (peak heap %d B)", mbps, peak)
}
