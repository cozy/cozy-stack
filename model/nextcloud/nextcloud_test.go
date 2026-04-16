package nextcloud

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/webdav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchUserIDWithCredentials(t *testing.T) {
	// safehttp refuses loopback hosts outside dev mode.
	oldBuildMode := build.BuildMode
	build.BuildMode = build.ModeDev
	t.Cleanup(func() { build.BuildMode = oldBuildMode })

	ctx := logger.WithContext(context.Background(), logger.WithNamespace("nextcloud-test"))

	t.Run("resolves user ID against OCS cloud/user on a Nextcloud without user_status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/ocs/v2.php/cloud/user" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"ocs":{"meta":{"status":"ok","statuscode":200},"data":{"id":"alice-webdav"}}}`)
				return
			}
			// Any other path (including user_status) returns 404, matching
			// managed Nextcloud hosts that strip optional OCS apps.
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)

		userID, err := FetchUserIDWithCredentials(ctx, srv.URL+"/", "alice", "app-password")
		require.NoError(t, err)
		assert.Equal(t, "alice-webdav", userID)
	})

	t.Run("returns ErrInvalidAuth on 401", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(srv.Close)

		_, err := FetchUserIDWithCredentials(ctx, srv.URL+"/", "alice", "bad")
		assert.ErrorIs(t, err, webdav.ErrInvalidAuth)
	})

	t.Run("returns ErrInvalidAuth on 403", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(srv.Close)

		_, err := FetchUserIDWithCredentials(ctx, srv.URL+"/", "alice", "bad")
		assert.ErrorIs(t, err, webdav.ErrInvalidAuth)
	})

	t.Run("does not conflate a 500 with an auth failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		_, err := FetchUserIDWithCredentials(ctx, srv.URL+"/", "alice", "app-password")
		require.Error(t, err)
		assert.False(t, errors.Is(err, webdav.ErrInvalidAuth),
			"a 500 from Nextcloud must not be reported as invalid credentials")
	})

	t.Run("honors a sub-path install when the nextcloud URL has a base path", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only serve the sub-path. A probe that strips /nextcloud from
			// the base URL would land on /ocs/... and get a 404 here.
			if r.URL.Path == "/nextcloud/ocs/v2.php/cloud/user" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"ocs":{"meta":{"status":"ok","statuscode":200},"data":{"id":"subpath-alice"}}}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)

		userID, err := FetchUserIDWithCredentials(ctx, srv.URL+"/nextcloud/", "alice", "app-password")
		require.NoError(t, err)
		assert.Equal(t, "subpath-alice", userID)
	})

	t.Run("normalizes missing and trailing slashes in the base URL path", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/nextcloud/ocs/v2.php/cloud/user" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"ocs":{"meta":{"status":"ok","statuscode":200},"data":{"id":"norm-alice"}}}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)

		// Both with and without the trailing slash must resolve to the
		// same probe URL.
		for _, base := range []string{srv.URL + "/nextcloud", srv.URL + "/nextcloud/"} {
			userID, err := FetchUserIDWithCredentials(ctx, base, "alice", "app-password")
			require.NoError(t, err, "base=%q", base)
			assert.Equal(t, "norm-alice", userID, "base=%q", base)
		}
	})
}
