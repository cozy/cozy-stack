package sharing

import (
	"testing"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
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

	_, err := permission.CreateShareInteractSet(inst, sharingID, map[string]string{
		aliceEmail: aliceToken,
	}, perms)
	require.NoError(t, err)
	_, err = permission.CreateShareInteractSet(inst, sharingID, map[string]string{
		bobEmail: bobToken,
	}, perms)
	require.NoError(t, err)

	first, err := permission.GetForShareInteract(inst, sharingID)
	require.NoError(t, err)

	targetEmail := bobEmail
	targetToken := bobToken
	if _, ok := first.Codes[bobEmail]; ok {
		targetEmail = aliceEmail
		targetToken = aliceToken
	}

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
