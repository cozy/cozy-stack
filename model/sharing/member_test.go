package sharing

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/require"
)

func TestFindMemberByInteractCodeWithDuplicatePermissions(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance()
	require.NoError(t, couchdb.ResetDB(inst, consts.Permissions))

	const sharingID = "sharing-duplicate-interact-permissions"
	const aliceEmail = "alice@example.test"
	const bobEmail = "bob@example.test"

	aliceToken := "alice-interact-token"
	bobToken := "bob-interact-token"
	perms := permission.Permission{
		Permissions: permission.Set{{
			Title:  "Shared drive",
			Type:   consts.Files,
			Values: []string{"shared-drive-root"},
			Verbs:  permission.ALL,
		}},
	}

	err := couchdb.CreateDoc(inst, &permission.Permission{
		Type:        permission.TypeShareInteract,
		Permissions: perms.Permissions,
		Codes: map[string]string{
			aliceEmail: aliceToken,
		},
		SourceID: consts.Sharings + "/" + sharingID,
	})
	require.NoError(t, err)
	err = couchdb.CreateDoc(inst, &permission.Permission{
		Type:        permission.TypeShareInteract,
		Permissions: perms.Permissions,
		Codes: map[string]string{
			bobEmail: bobToken,
		},
		SourceID: consts.Sharings + "/" + sharingID,
	})
	require.NoError(t, err)

	targetEmail := bobEmail
	targetToken := bobToken

	s := Sharing{
		SID: sharingID,
		Members: []Member{
			{Email: "owner@example.test", Status: MemberStatusOwner},
			{Email: aliceEmail, Status: MemberStatusReady},
			{Email: bobEmail, Status: MemberStatusReady},
		},
	}
	member, err := s.FindMemberByInteractCode(inst, targetToken)
	require.NoError(t, err)
	require.Equal(t, targetEmail, member.Email)
}

func TestGetInteractCodeConcurrentCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance()
	require.NoError(t, couchdb.ResetDB(inst, consts.Permissions))

	calls := 100
	if os.Getenv("COZY_STRESS_TESTS") == "1" {
		calls = 1000
	}
	sharingID := "sharing-concurrent-interact-permissions"
	s := Sharing{
		SID:     sharingID,
		AppSlug: "drive",
		Rules: []Rule{{
			Title:   "Shared drive",
			DocType: consts.Files,
			Values:  []string{"shared-drive-root"},
		}},
	}

	members := make([]Member, calls)
	codes := make([]string, calls)
	errs := make(chan error, calls)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(calls)

	for i := 0; i < calls; i++ {
		members[i] = Member{Email: fmt.Sprintf("member-%04d@example.test", i)}
		go func(i int) {
			defer wg.Done()
			<-start
			code, err := s.GetInteractCode(inst, &members[i], i+1)
			if err != nil {
				errs <- err
				return
			}
			codes[i] = code
		}(i)
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	interact, err := permission.GetForShareInteract(inst, sharingID)
	require.NoError(t, err)
	require.Len(t, interact.Codes, calls)
	for i, member := range members {
		require.NotEmpty(t, codes[i])
		require.Equal(t, codes[i], interact.Codes[member.Email])
	}

	var perms []permission.Permission
	req := couchdb.FindRequest{
		UseIndex: "by-source-and-type",
		Selector: mango.And(
			mango.Equal("type", permission.TypeShareInteract),
			mango.Equal("source_id", consts.Sharings+"/"+sharingID),
		),
		Limit: calls,
	}
	require.NoError(t, couchdb.FindDocs(inst, consts.Permissions, &req, &perms))
	require.Len(t, perms, 1)
	require.Equal(t, permission.ShareInteractPermissionID(sharingID), perms[0].ID())
}
