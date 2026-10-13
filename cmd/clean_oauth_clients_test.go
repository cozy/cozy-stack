package cmd

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/stack"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanOAuthClients(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	if job.SystemUnsafe() == nil {
		_, _, err := stack.Start()
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		flagCleanOAuthClientsDomain = ""
		flagCleanOAuthClientsDryRun = false
	})

	d1 := fmt.Sprintf("clean-oauth-1-%d.example", time.Now().UnixNano())
	d2 := fmt.Sprintf("clean-oauth-2-%d.example", time.Now().UnixNano())

	inst1, err := lifecycle.Create(&lifecycle.Options{
		Domain: d1,
		Email:  "user1@" + d1,
		Locale: "en",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lifecycle.Destroy(d1) })

	inst2, err := lifecycle.Create(&lifecycle.Options{
		Domain: d2,
		Email:  "user2@" + d2,
		Locale: "en",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lifecycle.Destroy(d2) })

	// Create clients on inst1
	c1 := &oauth.Client{ClientName: "c1", RedirectURIs: []string{"http://localhost"}, SoftwareID: "target-app"}
	require.Nil(t, c1.Create(inst1))
	c2 := &oauth.Client{ClientName: "c2", RedirectURIs: []string{"http://localhost"}, SoftwareID: "target-app"}
	require.Nil(t, c2.Create(inst1))
	cOther := &oauth.Client{ClientName: "cOther", RedirectURIs: []string{"http://localhost"}, SoftwareID: "other-app"}
	require.Nil(t, cOther.Create(inst1))

	// Create client on inst2
	c3 := &oauth.Client{ClientName: "c3", RedirectURIs: []string{"http://localhost"}, SoftwareID: "target-app"}
	require.Nil(t, c3.Create(inst2))

	// 1. Dry run targeting only inst1
	var out bytes.Buffer
	err = cleanOAuthClientsWithWriter(&out, "target-app", d1, true)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "user1@"+d1)
	assert.Contains(t, out.String(), "2 clients")
	assert.Contains(t, out.String(), "total: 2 clients would delete")
	assert.NotContains(t, out.String(), d2)

	// Verify nothing was deleted
	found1, err := findOAuthClientsBySoftwareID(inst1, "target-app")
	require.NoError(t, err)
	assert.Len(t, found1, 2)

	found2, err := findOAuthClientsBySoftwareID(inst2, "target-app")
	require.NoError(t, err)
	assert.Len(t, found2, 1)

	// 2. Real deletion targeting only inst1
	out.Reset()
	err = cleanOAuthClientsWithWriter(&out, "target-app", d1, false)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "user1@"+d1)
	assert.Contains(t, out.String(), "total: 2 clients deleted")

	// Verify target clients deleted on inst1, but other-app remains
	found1, err = findOAuthClientsBySoftwareID(inst1, "target-app")
	require.NoError(t, err)
	assert.Empty(t, found1)

	otherFound, err := findOAuthClientsBySoftwareID(inst1, "other-app")
	require.NoError(t, err)
	assert.Len(t, otherFound, 1)

	// Verify inst2 target client is untouched
	found2, err = findOAuthClientsBySoftwareID(inst2, "target-app")
	require.NoError(t, err)
	assert.Len(t, found2, 1)

	// 3. Deletion for all instances
	out.Reset()
	err = cleanOAuthClientsWithWriter(&out, "target-app", "", false)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "user2@"+d2)
	assert.Contains(t, out.String(), "total: 1 clients deleted")

	found2, err = findOAuthClientsBySoftwareID(inst2, "target-app")
	require.NoError(t, err)
	assert.Empty(t, found2)

	// 4. Non-existent domain error
	err = cleanOAuthClientsWithWriter(&out, "target-app", "nonexistent.example", true)
	assert.Error(t, err)

	// 5. Test CLI command execution with --domain and --dry-run
	cleanOAuthClientsCmd.SetArgs([]string{"other-app", "--domain", d1, "--dry-run"})
	err = cleanOAuthClientsCmd.Execute()
	assert.NoError(t, err)

	otherFound, err = findOAuthClientsBySoftwareID(inst1, "other-app")
	require.NoError(t, err)
	assert.Len(t, otherFound, 1)

	// 6. Test CLI command invalid args
	cleanOAuthClientsCmd.SetArgs([]string{})
	err = cleanOAuthClientsCmd.Execute()
	assert.NoError(t, err) // usage prints and returns nil
}
