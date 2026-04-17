package remote_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/nextcloud"
	"github.com/cozy/cozy-stack/model/permission"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	"github.com/cozy/cozy-stack/tests/testutils"
	weberrors "github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/remote"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type spyRabbitMQ struct {
	mu       sync.Mutex
	messages []rabbitmq.PublishRequest
	err      error
}

func (s *spyRabbitMQ) StartManagers() ([]*rabbitmq.RabbitMQManager, error) {
	return nil, nil
}

func (s *spyRabbitMQ) Publish(_ context.Context, req rabbitmq.PublishRequest) error {
	if s.err != nil {
		return s.err
	}
	if req.Payload != nil {
		payload, err := json.Marshal(req.Payload)
		if err != nil {
			return err
		}
		req.Payload = payload
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, req)
	return nil
}

func (s *spyRabbitMQ) last() rabbitmq.PublishRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messages[len(s.messages)-1]
}

func (s *spyRabbitMQ) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

type nextcloudMockOptions struct {
	authStatus int
	userID     string
}

func startMockNextcloud(t *testing.T, opts nextcloudMockOptions) (url string, calls *int32) {
	t.Helper()
	if opts.authStatus == 0 {
		opts.authStatus = http.StatusOK
	}
	if opts.userID == "" {
		opts.userID = "alice"
	}
	var counter int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ocs/v2.php/cloud/user" {
			atomic.AddInt32(&counter, 1)
			if opts.authStatus != http.StatusOK {
				w.WriteHeader(opts.authStatus)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"ocs":{"data":{"id":%q}}}`, opts.userID)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/", &counter
}

func migrationPermission() *permission.Permission {
	return &permission.Permission{
		Type:     permission.TypeWebapp,
		SourceID: consts.Apps + "/" + consts.SettingsSlug,
		Permissions: permission.Set{
			permission.Rule{
				Type:  consts.NextcloudMigrations,
				Verbs: permission.Verbs(permission.POST),
			},
		},
	}
}

func setupMigrationRouter(t *testing.T, inst *instance.Instance, pdoc *permission.Permission, rmq rabbitmq.Service) *httptest.Server {
	t.Helper()

	handler := echo.New()
	handler.HTTPErrorHandler = weberrors.ErrorHandler
	group := handler.Group("/remote", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("instance", inst)
			if pdoc != nil {
				c.Set("permissions_doc", pdoc)
			}
			return next(c)
		}
	})

	remote.NewHTTPHandler(rmq).Register(group)

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func migrationRequestBody(url string, extra map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"nextcloud_url":          url,
		"nextcloud_login":        "alice",
		"nextcloud_app_password": "app-password-xxx",
		"source_path":            "/",
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func TestPostNextcloudMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	// safehttp refuses loopback hosts outside dev mode; flip the flag so
	// httptest.NewServer URLs are reachable.
	oldBuildMode := build.BuildMode
	build.BuildMode = build.ModeDev
	t.Cleanup(func() { build.BuildMode = oldBuildMode })

	t.Run("HappyPath", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-happy")
		inst := setup.GetTestInstance()

		ncURL, probeCalls := startMockNextcloud(t, nextcloudMockOptions{userID: "alice-webdav"})
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, nil)).
			Expect().Status(http.StatusCreated).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.Value("type").String().IsEqual(consts.NextcloudMigrations)
		migrationID := data.Value("id").String().NotEmpty().Raw()
		attrs := data.Value("attributes").Object()
		attrs.Value("status").String().IsEqual(nextcloud.MigrationStatusPending)
		attrs.Value("target_dir").String().IsEqual(nextcloud.DefaultMigrationTargetDir)
		attrs.Value("errors").Array().IsEmpty()
		attrs.Value("skipped").Array().IsEmpty()
		progress := attrs.Value("progress").Object()
		progress.Value("files_imported").Number().IsEqual(0)
		progress.Value("files_total").Number().IsEqual(0)
		progress.Value("bytes_imported").Number().IsEqual(0)
		progress.Value("bytes_total").Number().IsEqual(0)

		require.Equal(t, int32(1), atomic.LoadInt32(probeCalls), "probe should hit the OCS endpoint once")
		require.Equal(t, 1, spy.count(), "expected one RabbitMQ publish")
		pub := spy.last()
		assert.Equal(t, rabbitmq.ExchangeMigration, pub.Exchange)
		assert.Equal(t, rabbitmq.RoutingKeyNextcloudMigrationRequested, pub.RoutingKey)
		assert.Equal(t, migrationID, pub.MessageID)

		var payload rabbitmq.NextcloudMigrationRequestedMessage
		require.NoError(t, json.Unmarshal(pub.Payload.([]byte), &payload))
		assert.Equal(t, migrationID, payload.MigrationID)
		assert.Equal(t, inst.Domain, payload.WorkplaceFqdn)
		assert.NotEmpty(t, payload.AccountID)
		assert.Equal(t, "/", payload.SourcePath)
		assert.NotZero(t, payload.Timestamp)

		var stored nextcloud.Migration
		require.NoError(t, couchdb.GetDoc(inst, consts.NextcloudMigrations, migrationID, &stored))
		assert.Equal(t, nextcloud.MigrationStatusPending, stored.Status)

		var accDoc couchdb.JSONDoc
		require.NoError(t, couchdb.GetDoc(inst, consts.Accounts, payload.AccountID, &accDoc))
		assert.Equal(t, "nextcloud", accDoc.M["account_type"])
		assert.Equal(t, "alice-webdav", accDoc.M["webdav_user_id"],
			"probe-resolved userID must be cached on the account")
		auth, ok := accDoc.M["auth"].(map[string]interface{})
		require.True(t, ok, "auth should be a map")
		assert.Equal(t, "alice", auth["login"])
		assert.Nil(t, auth["password"], "plaintext password must not be persisted")
		assert.NotEmpty(t, auth["credentials_encrypted"], "credentials should be encrypted at rest")
	})

	t.Run("CustomTargetDirIsPersisted", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-target-dir")
		inst := setup.GetTestInstance()

		ncURL, _ := startMockNextcloud(t, nextcloudMockOptions{userID: "alice-webdav"})
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, map[string]interface{}{
				"target_dir": "/Imports/From Nextcloud",
			})).
			Expect().Status(http.StatusCreated).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Value("data").Object().
			Value("attributes").Object().
			Value("target_dir").String().IsEqual("/Imports/From Nextcloud")
	})

	t.Run("InvalidTargetDirRejected", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-target-dir-invalid")
		inst := setup.GetTestInstance()

		ncURL, _ := startMockNextcloud(t, nextcloudMockOptions{userID: "alice-webdav"})
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		cases := []string{"Imports", "/foo/../bar", "/foo//bar", "/./bar"}
		for _, td := range cases {
			e.POST("/remote/nextcloud/migration").
				WithHeader("Accept", "application/vnd.api+json").
				WithJSON(migrationRequestBody(ncURL, map[string]interface{}{
					"target_dir": td,
				})).
				Expect().Status(http.StatusBadRequest)
		}
		assert.Equal(t, 0, spy.count(), "no publish for invalid target_dir")
	})

	t.Run("WrongCredentialsReturn401", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-wrong-creds")
		inst := setup.GetTestInstance()

		ncURL, probeCalls := startMockNextcloud(t, nextcloudMockOptions{authStatus: http.StatusUnauthorized})
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, nil)).
			Expect().Status(http.StatusUnauthorized)

		assert.Equal(t, int32(1), atomic.LoadInt32(probeCalls))
		assert.Equal(t, 0, spy.count(), "no publish when credentials are invalid")

		var docs []*nextcloud.Migration
		req := &couchdb.AllDocsRequest{Limit: 10}
		err := couchdb.GetAllDocs(inst, consts.NextcloudMigrations, req, &docs)
		if err != nil && !couchdb.IsNoDatabaseError(err) {
			t.Fatalf("unexpected error listing migrations: %s", err)
		}
		assert.Empty(t, docs, "no tracking doc should be created on auth failure")

		var accounts []*couchdb.JSONDoc
		accReq := &couchdb.AllDocsRequest{Limit: 10}
		err = couchdb.GetAllDocs(inst, consts.Accounts, accReq, &accounts)
		if err != nil && !couchdb.IsNoDatabaseError(err) {
			t.Fatalf("unexpected error listing accounts: %s", err)
		}
		for _, a := range accounts {
			if a.M["account_type"] == "nextcloud" {
				t.Fatalf("no nextcloud account should be created on auth failure, got %s", a.ID())
			}
		}
	})

	t.Run("NextcloudUnreachableReturns502", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-unreachable")
		inst := setup.GetTestInstance()

		// Close the server immediately so the URL points at a dead listener.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		deadURL := srv.URL + "/"
		srv.Close()

		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(deadURL, nil)).
			Expect().Status(http.StatusBadGateway)

		assert.Equal(t, 0, spy.count())
	})

	t.Run("ConcurrentTriggersSerializeUnderTheLock", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-concurrent")
		inst := setup.GetTestInstance()

		ncURL, _ := startMockNextcloud(t, nextcloudMockOptions{})
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)

		body, err := json.Marshal(migrationRequestBody(ncURL, nil))
		require.NoError(t, err)

		const parallel = 5
		statuses := make([]int, parallel)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < parallel; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-start
				req, rerr := http.NewRequest(http.MethodPost, ts.URL+"/remote/nextcloud/migration", bytes.NewReader(body))
				require.NoError(t, rerr)
				req.Header.Set("Accept", "application/vnd.api+json")
				req.Header.Set("Content-Type", "application/json")
				resp, rerr := http.DefaultClient.Do(req)
				require.NoError(t, rerr)
				resp.Body.Close()
				statuses[idx] = resp.StatusCode
			}(i)
		}
		close(start)
		wg.Wait()

		var created, conflict int
		for _, s := range statuses {
			switch s {
			case http.StatusCreated:
				created++
			case http.StatusConflict:
				conflict++
			default:
				t.Errorf("unexpected status %d", s)
			}
		}
		assert.Equal(t, 1, created, "exactly one concurrent trigger must win")
		assert.Equal(t, parallel-1, conflict, "the rest must see a 409 from the lock-protected re-check")

		// And there must be exactly one pending/running tracking doc on
		// disk, not one per parallel request.
		var docs []*nextcloud.Migration
		require.NoError(t, couchdb.GetAllDocs(inst, consts.NextcloudMigrations, &couchdb.AllDocsRequest{Limit: 20}, &docs))
		assert.Len(t, docs, 1, "one tracking doc, not %d", len(docs))
		assert.Equal(t, 1, spy.count(), "one RabbitMQ publish, not %d", spy.count())
	})

	t.Run("ConflictWhenMigrationAlreadyRunning", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-conflict")
		inst := setup.GetTestInstance()

		existing := nextcloud.NewPendingMigration("")
		existing.Status = nextcloud.MigrationStatusRunning
		require.NoError(t, couchdb.CreateDoc(inst, existing))

		ncURL, _ := startMockNextcloud(t, nextcloudMockOptions{})
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, nil)).
			Expect().Status(http.StatusConflict)

		assert.Equal(t, 0, spy.count(), "no message should be published on conflict")
	})

	t.Run("PublishFailureMarksMigrationFailed", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-publish-fail")
		inst := setup.GetTestInstance()

		ncURL, _ := startMockNextcloud(t, nextcloudMockOptions{})
		spy := &spyRabbitMQ{err: fmt.Errorf("broker unreachable")}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, nil)).
			Expect().Status(http.StatusServiceUnavailable)

		active, err := nextcloud.FindActiveMigration(inst)
		require.NoError(t, err)
		assert.Nil(t, active, "failed migrations must not block new ones")

		var docs []*nextcloud.Migration
		req := &couchdb.AllDocsRequest{Limit: 10}
		require.NoError(t, couchdb.GetAllDocs(inst, consts.NextcloudMigrations, req, &docs))
		require.Len(t, docs, 1)
		assert.Equal(t, nextcloud.MigrationStatusFailed, docs[0].Status)
		require.NotEmpty(t, docs[0].Errors)
		assert.Contains(t, docs[0].Errors[0].Message, "broker unreachable")
	})

	t.Run("AccountIsReusedOnSecondMigration", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-account-reuse")
		inst := setup.GetTestInstance()

		ncURL, _ := startMockNextcloud(t, nextcloudMockOptions{})
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		first := e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, nil)).
			Expect().Status(http.StatusCreated).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		firstID := first.Value("data").Object().Value("id").String().Raw()
		require.Equal(t, 1, spy.count())
		firstAccountID := decodeAccountID(t, spy.last().Payload.([]byte))

		// Flip the first migration to completed so the conflict check lets
		// the second one through.
		var doc nextcloud.Migration
		require.NoError(t, couchdb.GetDoc(inst, consts.NextcloudMigrations, firstID, &doc))
		doc.Status = nextcloud.MigrationStatusCompleted
		require.NoError(t, couchdb.UpdateDoc(inst, &doc))

		e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, nil)).
			Expect().Status(http.StatusCreated)
		require.Equal(t, 2, spy.count())
		secondAccountID := decodeAccountID(t, spy.last().Payload.([]byte))

		assert.Equal(t, firstAccountID, secondAccountID, "account should be reused across migrations")

		var accounts []*couchdb.JSONDoc
		req := &couchdb.AllDocsRequest{Limit: 10}
		require.NoError(t, couchdb.GetAllDocs(inst, consts.Accounts, req, &accounts))
		var nextcloudAccountIDs []string
		for _, a := range accounts {
			if a.M["account_type"] == "nextcloud" {
				nextcloudAccountIDs = append(nextcloudAccountIDs, a.ID())
			}
		}
		assert.Len(t, nextcloudAccountIDs, 1)
	})

	t.Run("SecondMigrationWithDifferentLoginReusesTheSameAccount", func(t *testing.T) {
		// Policy: keyed-by-type-only. Triggering the endpoint with a
		// different login rewrites the existing nextcloud account in
		// place instead of creating a ghost doc the Settings UI cannot
		// surface. The downside (no multi-account support, and losing a
		// previously valid account on a typo retry) is accepted.
		setup := testutils.NewSetup(t, "ncmigration-account-rekey")
		inst := setup.GetTestInstance()

		ncURL, _ := startMockNextcloud(t, nextcloudMockOptions{userID: "first"})
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		// First migration with login "alice".
		firstResp := e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, nil)).
			Expect().Status(http.StatusCreated).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		firstMigrationID := firstResp.Value("data").Object().Value("id").String().Raw()
		require.Equal(t, 1, spy.count())
		firstAccountID := decodeAccountID(t, spy.last().Payload.([]byte))

		// Unblock the next trigger by marking the first migration failed
		// (failed does not count as active).
		var firstDoc nextcloud.Migration
		require.NoError(t, couchdb.GetDoc(inst, consts.NextcloudMigrations, firstMigrationID, &firstDoc))
		firstDoc.Status = nextcloud.MigrationStatusFailed
		require.NoError(t, couchdb.UpdateDoc(inst, &firstDoc))

		// Second migration with a different login ("bob").
		e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, map[string]interface{}{
				"nextcloud_login": "bob",
			})).
			Expect().Status(http.StatusCreated)
		require.Equal(t, 2, spy.count())
		secondAccountID := decodeAccountID(t, spy.last().Payload.([]byte))

		assert.Equal(t, firstAccountID, secondAccountID,
			"the single nextcloud account must be rewritten, not orphaned")

		// Exactly one nextcloud account on disk, with login now "bob".
		var accounts []*couchdb.JSONDoc
		require.NoError(t, couchdb.GetAllDocs(inst, consts.Accounts, &couchdb.AllDocsRequest{Limit: 10}, &accounts))
		var ncAccounts []*couchdb.JSONDoc
		for _, a := range accounts {
			if a.M["account_type"] == "nextcloud" {
				ncAccounts = append(ncAccounts, a)
			}
		}
		require.Len(t, ncAccounts, 1)
		auth, ok := ncAccounts[0].M["auth"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "bob", auth["login"], "login on the rewritten account")
	})

	t.Run("AccountReuseRefreshesStoredPassword", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-refresh-password")
		inst := setup.GetTestInstance()

		ncURL, _ := startMockNextcloud(t, nextcloudMockOptions{})
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		stale := &couchdb.JSONDoc{
			Type: consts.Accounts,
			M: map[string]interface{}{
				"account_type":   "nextcloud",
				"webdav_user_id": "stale-userid",
				"auth": map[string]interface{}{
					"url":      ncURL,
					"login":    "alice",
					"password": "old-wrong-pass",
				},
			},
		}
		require.NoError(t, couchdb.CreateDoc(inst, stale))
		staleID := stale.ID()

		e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, map[string]interface{}{
				"nextcloud_app_password": "fresh-correct-pass",
			})).
			Expect().Status(http.StatusCreated)

		require.Equal(t, 1, spy.count())
		reusedID := decodeAccountID(t, spy.last().Payload.([]byte))
		assert.Equal(t, staleID, reusedID, "existing account should be reused, not duplicated")

		var refreshed couchdb.JSONDoc
		require.NoError(t, couchdb.GetDoc(inst, consts.Accounts, staleID, &refreshed))
		assert.Equal(t, "alice", refreshed.M["webdav_user_id"],
			"webdav_user_id should be refreshed from the probe")
		auth, ok := refreshed.M["auth"].(map[string]interface{})
		require.True(t, ok)
		assert.NotEmpty(t, auth["credentials_encrypted"])
		assert.Nil(t, auth["password"])
	})

	t.Run("RejectsMissingCredentials", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-reject-missing")
		inst := setup.GetTestInstance()

		ncURL, _ := startMockNextcloud(t, nextcloudMockOptions{})
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, map[string]interface{}{
				"nextcloud_app_password": "",
			})).
			Expect().Status(http.StatusBadRequest)

		assert.Equal(t, 0, spy.count())
	})

	t.Run("RejectsWithoutPermission", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-reject-nomore")
		inst := setup.GetTestInstance()

		ncURL, _ := startMockNextcloud(t, nextcloudMockOptions{})
		pdoc := &permission.Permission{
			Type:     permission.TypeWebapp,
			SourceID: consts.Apps + "/" + consts.SettingsSlug,
		}
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, pdoc, spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/remote/nextcloud/migration").
			WithHeader("Accept", "application/vnd.api+json").
			WithJSON(migrationRequestBody(ncURL, nil)).
			Expect().Status(http.StatusForbidden)

		assert.Equal(t, 0, spy.count())
	})
}

func decodeAccountID(t *testing.T, payload []byte) string {
	t.Helper()
	var msg rabbitmq.NextcloudMigrationRequestedMessage
	require.NoError(t, json.Unmarshal(payload, &msg))
	return msg.AccountID
}

func TestPostNextcloudMigrationCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	oldBuildMode := build.BuildMode
	build.BuildMode = build.ModeDev
	t.Cleanup(func() { build.BuildMode = oldBuildMode })

	t.Run("PublishesAndReturns202", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-cancel-happy")
		inst := setup.GetTestInstance()

		doc := nextcloud.NewPendingMigration("")
		doc.Status = nextcloud.MigrationStatusRunning
		require.NoError(t, couchdb.CreateDoc(inst, doc))

		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/remote/nextcloud/migration/" + doc.ID() + "/cancel").
			WithHost(inst.Domain).
			Expect().Status(http.StatusAccepted)

		require.Equal(t, 1, spy.count(), "expected a single publish")
		pub := spy.last()
		assert.Equal(t, rabbitmq.ExchangeMigration, pub.Exchange)
		assert.Equal(t, rabbitmq.RoutingKeyNextcloudMigrationCanceled, pub.RoutingKey)
		assert.Equal(t, doc.ID(), pub.MessageID)

		var msg rabbitmq.NextcloudMigrationCanceledMessage
		require.NoError(t, json.Unmarshal(pub.Payload.([]byte), &msg))
		assert.Equal(t, doc.ID(), msg.MigrationID)
		assert.Equal(t, inst.Domain, msg.WorkplaceFqdn)
		assert.NotZero(t, msg.Timestamp)

		// The Stack must not mutate the tracking doc — that transition is
		// owned by the migration service (single-writer invariant for the
		// terminal state).
		var stored nextcloud.Migration
		require.NoError(t, couchdb.GetDoc(inst, consts.NextcloudMigrations, doc.ID(), &stored))
		assert.Equal(t, nextcloud.MigrationStatusRunning, stored.Status)
		assert.False(t, stored.CancelRequested)
	})

	t.Run("UnknownMigrationReturns404", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-cancel-unknown")
		inst := setup.GetTestInstance()

		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/remote/nextcloud/migration/does-not-exist/cancel").
			WithHost(inst.Domain).
			Expect().Status(http.StatusNotFound)

		assert.Equal(t, 0, spy.count(), "must not publish for unknown migrations")
	})

	for _, status := range []string{
		nextcloud.MigrationStatusCompleted,
		nextcloud.MigrationStatusFailed,
		nextcloud.MigrationStatusCanceled,
	} {
		status := status
		t.Run("TerminalStatus_"+status, func(t *testing.T) {
			setup := testutils.NewSetup(t, "ncmigration-cancel-"+status)
			inst := setup.GetTestInstance()

			doc := nextcloud.NewPendingMigration("")
			doc.Status = status
			require.NoError(t, couchdb.CreateDoc(inst, doc))

			spy := &spyRabbitMQ{}
			ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
			e := testutils.CreateTestClient(t, ts.URL)

			e.POST("/remote/nextcloud/migration/" + doc.ID() + "/cancel").
				WithHost(inst.Domain).
				Expect().Status(http.StatusConflict)

			assert.Equal(t, 0, spy.count(), "terminal migrations must not publish")
		})
	}

	t.Run("PublishFailureReturns503", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-cancel-publishfail")
		inst := setup.GetTestInstance()

		doc := nextcloud.NewPendingMigration("")
		doc.Status = nextcloud.MigrationStatusRunning
		require.NoError(t, couchdb.CreateDoc(inst, doc))

		spy := &spyRabbitMQ{err: fmt.Errorf("broker down")}
		ts := setupMigrationRouter(t, inst, migrationPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/remote/nextcloud/migration/" + doc.ID() + "/cancel").
			WithHost(inst.Domain).
			Expect().Status(http.StatusServiceUnavailable)

		// Unlike trigger, a cancel publish failure must not flip the
		// tracking doc to failed. The migration is still running.
		var stored nextcloud.Migration
		require.NoError(t, couchdb.GetDoc(inst, consts.NextcloudMigrations, doc.ID(), &stored))
		assert.Equal(t, nextcloud.MigrationStatusRunning, stored.Status)
	})

	t.Run("RejectsWithoutPermission", func(t *testing.T) {
		setup := testutils.NewSetup(t, "ncmigration-cancel-forbidden")
		inst := setup.GetTestInstance()

		doc := nextcloud.NewPendingMigration("")
		doc.Status = nextcloud.MigrationStatusRunning
		require.NoError(t, couchdb.CreateDoc(inst, doc))

		pdoc := &permission.Permission{
			Type:     permission.TypeWebapp,
			SourceID: consts.Apps + "/" + consts.SettingsSlug,
		}
		spy := &spyRabbitMQ{}
		ts := setupMigrationRouter(t, inst, pdoc, spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/remote/nextcloud/migration/" + doc.ID() + "/cancel").
			WithHost(inst.Domain).
			Expect().Status(http.StatusForbidden)

		assert.Equal(t, 0, spy.count())
	})
}
