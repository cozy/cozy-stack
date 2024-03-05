package sharing

import (
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/cozy/cozy-stack/worker/mails"
)

func TestGroups(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance(&lifecycle.Options{
		Email:      "alice@example.net",
		PublicName: "Alice",
	})

	t.Run("RevokeGroup", func(t *testing.T) {
		now := time.Now()
		friends := createGroup(t, inst, "Friends")
		football := createGroup(t, inst, "Football")
		bob := createContactInGroups(t, inst, "Bob", []string{friends.ID()})
		_ = createContactInGroups(t, inst, "Charlie", []string{friends.ID(), football.ID()})
		_ = createContactInGroups(t, inst, "Dave", []string{football.ID()})

		s := &Sharing{
			Active:      true,
			Owner:       true,
			Description: "Just testing groups",
			Members: []Member{
				{Status: MemberStatusOwner, Name: "Alice", Email: "alice@cozy.tools"},
			},
			Rules: []Rule{
				{
					Title:   "Just testing groups",
					DocType: "io.cozy.tests",
					Values:  []string{uuidv7()},
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		require.NoError(t, couchdb.CreateDoc(inst, s))
		require.NoError(t, s.AddContact(inst, bob.ID(), false))
		require.NoError(t, s.AddGroup(inst, friends.ID(), false))
		require.NoError(t, s.AddGroup(inst, football.ID(), false))

		require.Len(t, s.Members, 4)
		require.Equal(t, s.Members[0].Name, "Alice")
		require.Equal(t, s.Members[1].Name, "Bob")
		assert.False(t, s.Members[1].OnlyInGroups)
		assert.Equal(t, s.Members[1].Groups, []int{0})
		require.Equal(t, s.Members[2].Name, "Charlie")
		assert.True(t, s.Members[2].OnlyInGroups)
		assert.Equal(t, s.Members[2].Groups, []int{0, 1})
		require.Equal(t, s.Members[3].Name, "Dave")
		assert.True(t, s.Members[3].OnlyInGroups)
		assert.Equal(t, s.Members[3].Groups, []int{1})

		require.Len(t, s.Groups, 2)
		require.Equal(t, s.Groups[0].Name, "Friends")
		assert.False(t, s.Groups[0].Revoked)
		require.Equal(t, s.Groups[1].Name, "Football")
		assert.False(t, s.Groups[1].Revoked)

		require.NoError(t, s.RevokeGroup(inst, 1)) // Revoke the football group

		require.Len(t, s.Members, 4)
		assert.NotEqual(t, s.Members[1].Status, MemberStatusRevoked)
		assert.Equal(t, s.Members[1].Groups, []int{0})
		assert.NotEqual(t, s.Members[2].Status, MemberStatusRevoked)
		assert.Equal(t, s.Members[2].Groups, []int{0})
		assert.Equal(t, s.Members[3].Status, MemberStatusRevoked)
		assert.Empty(t, s.Members[3].Groups)

		require.Len(t, s.Groups, 2)
		assert.False(t, s.Groups[0].Revoked)
		assert.True(t, s.Groups[1].Revoked)

		require.NoError(t, s.RevokeGroup(inst, 0)) // Revoke the fiends group

		require.Len(t, s.Members, 4)
		assert.NotEqual(t, s.Members[1].Status, MemberStatusRevoked)
		assert.Empty(t, s.Members[1].Groups)
		assert.Equal(t, s.Members[2].Status, MemberStatusRevoked)
		assert.Empty(t, s.Members[2].Groups)
		assert.Equal(t, s.Members[3].Status, MemberStatusRevoked)
		assert.Empty(t, s.Members[3].Groups)

		require.Len(t, s.Groups, 2)
		assert.True(t, s.Groups[0].Revoked)
		assert.True(t, s.Groups[1].Revoked)
	})

	t.Run("UpdateGroups", func(t *testing.T) {
		now := time.Now()
		friends := createGroup(t, inst, "Friends")
		football := createGroup(t, inst, "Football")
		_ = createContactInGroups(t, inst, "Bob", []string{friends.ID()})
		charlie := createContactInGroups(t, inst, "Charlie", []string{football.ID()})
		dave := createContactWithoutEmail(t, inst, "Dave", []string{football.ID()})

		s := &Sharing{
			Active:      true,
			Owner:       true,
			Description: "Just testing groups",
			Members: []Member{
				{Status: MemberStatusOwner, Name: "Alice", Email: "alice@cozy.tools"},
			},
			Rules: []Rule{
				{
					Title:   "Just testing groups",
					DocType: "io.cozy.tests",
					Values:  []string{uuidv7()},
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		require.NoError(t, s.AddGroup(inst, friends.ID(), false))
		require.NoError(t, s.AddGroup(inst, football.ID(), false))
		perms, err := s.Create(inst)
		require.NoError(t, err)
		require.NoError(t, s.SendInvitations(inst, perms))
		sid := s.ID()

		require.Len(t, s.Members, 4)
		require.Equal(t, s.Members[0].Name, "Alice")
		require.Equal(t, s.Members[1].Name, "Bob")
		assert.True(t, s.Members[1].OnlyInGroups)
		assert.Equal(t, s.Members[1].Groups, []int{0})
		assert.Equal(t, s.Members[1].Status, MemberStatusPendingInvitation)
		require.Equal(t, s.Members[2].Name, "Charlie")
		assert.True(t, s.Members[2].OnlyInGroups)
		assert.Equal(t, s.Members[2].Groups, []int{1})
		assert.Equal(t, s.Members[2].Status, MemberStatusPendingInvitation)
		require.Equal(t, s.Members[3].Name, "Dave")
		assert.True(t, s.Members[3].OnlyInGroups)
		assert.Equal(t, s.Members[3].Groups, []int{1})
		assert.Equal(t, s.Members[3].Status, MemberStatusMailNotSent)

		require.Len(t, s.Groups, 2)
		require.Equal(t, s.Groups[0].Name, "Friends")
		assert.False(t, s.Groups[0].Revoked)
		require.Equal(t, s.Groups[1].Name, "Football")
		assert.False(t, s.Groups[1].Revoked)

		// Charlie is added to the friends group
		msg1 := job.ShareGroupMessage{
			ContactID:   charlie.ID(),
			GroupsAdded: []string{friends.ID()},
		}
		require.NoError(t, UpdateGroups(inst, msg1))

		s = &Sharing{}
		require.NoError(t, couchdb.GetDoc(inst, consts.Sharings, sid, s))

		require.Len(t, s.Members, 4)
		require.Equal(t, s.Members[0].Name, "Alice")
		require.Equal(t, s.Members[1].Name, "Bob")
		assert.True(t, s.Members[1].OnlyInGroups)
		assert.Equal(t, s.Members[1].Groups, []int{0})
		require.Equal(t, s.Members[2].Name, "Charlie")
		assert.True(t, s.Members[2].OnlyInGroups)
		assert.Equal(t, s.Members[2].Groups, []int{0, 1})
		require.Equal(t, s.Members[3].Name, "Dave")
		assert.True(t, s.Members[3].OnlyInGroups)
		assert.Equal(t, s.Members[3].Groups, []int{1})

		// Charlie is removed of the football group
		msg2 := job.ShareGroupMessage{
			ContactID:     charlie.ID(),
			GroupsRemoved: []string{football.ID()},
		}
		require.NoError(t, UpdateGroups(inst, msg2))

		s = &Sharing{}
		require.NoError(t, couchdb.GetDoc(inst, consts.Sharings, sid, s))

		require.Len(t, s.Members, 4)
		require.Equal(t, s.Members[0].Name, "Alice")
		require.Equal(t, s.Members[1].Name, "Bob")
		assert.True(t, s.Members[1].OnlyInGroups)
		assert.Equal(t, s.Members[1].Groups, []int{0})
		require.Equal(t, s.Members[2].Name, "Charlie")
		assert.True(t, s.Members[2].OnlyInGroups)
		assert.Equal(t, s.Members[2].Groups, []int{0})
		require.Equal(t, s.Members[3].Name, "Dave")
		assert.True(t, s.Members[3].OnlyInGroups)
		assert.Equal(t, s.Members[3].Groups, []int{1})

		// Email address is added for Dave, and an invitation can now be sent
		addEmailToContact(t, inst, dave)
		msg3 := job.ShareGroupMessage{
			ContactID:       dave.ID(),
			BecomeInvitable: true,
		}
		require.NoError(t, UpdateGroups(inst, msg3))

		s = &Sharing{}
		require.NoError(t, couchdb.GetDoc(inst, consts.Sharings, sid, s))

		require.Len(t, s.Members, 4)
		require.Equal(t, s.Members[0].Name, "Alice")
		require.Equal(t, s.Members[1].Name, "Bob")
		assert.True(t, s.Members[1].OnlyInGroups)
		assert.Equal(t, s.Members[1].Groups, []int{0})
		assert.Equal(t, s.Members[1].Status, MemberStatusPendingInvitation)
		require.Equal(t, s.Members[2].Name, "Charlie")
		assert.True(t, s.Members[2].OnlyInGroups)
		assert.Equal(t, s.Members[2].Groups, []int{0})
		assert.Equal(t, s.Members[1].Status, MemberStatusPendingInvitation)
		require.Equal(t, s.Members[3].Name, "Dave")
		assert.True(t, s.Members[3].OnlyInGroups)
		assert.Equal(t, s.Members[3].Groups, []int{1})
		assert.Equal(t, s.Members[3].Status, MemberStatusPendingInvitation)
	})
}

func createGroup(t *testing.T, inst *instance.Instance, name string) *contact.Group {
	t.Helper()
	g := contact.NewGroup()
	g.M["name"] = name
	require.NoError(t, couchdb.CreateDoc(inst, g))
	return g
}

func createContactInGroups(t *testing.T, inst *instance.Instance, contactName string, groupIDs []string) *contact.Contact {
	t.Helper()
	email := strings.ToLower(contactName) + "@cozy.tools"
	mail := map[string]interface{}{"address": email}

	var groups []interface{}
	for _, id := range groupIDs {
		groups = append(groups, map[string]interface{}{
			"_id":   id,
			"_type": consts.Groups,
		})
	}

	c := contact.New()
	c.M["fullname"] = contactName
	c.M["email"] = []interface{}{mail}
	c.M["relationships"] = map[string]interface{}{
		"groups": map[string]interface{}{"data": groups},
	}
	require.NoError(t, couchdb.CreateDoc(inst, c))
	return c
}

func createContactWithoutEmail(t *testing.T, inst *instance.Instance, contactName string, groupIDs []string) *contact.Contact {
	t.Helper()

	var groups []interface{}
	for _, id := range groupIDs {
		groups = append(groups, map[string]interface{}{
			"_id":   id,
			"_type": consts.Groups,
		})
	}

	c := contact.New()
	c.M["fullname"] = contactName
	c.M["relationships"] = map[string]interface{}{
		"groups": map[string]interface{}{"data": groups},
	}
	require.NoError(t, couchdb.CreateDoc(inst, c))
	return c
}

func addEmailToContact(t *testing.T, inst *instance.Instance, c *contact.Contact) {
	t.Helper()

	email := strings.ToLower(c.PrimaryName()) + "@cozy.tools"
	mail := map[string]interface{}{"address": email}
	c.M["email"] = []interface{}{mail}
	require.NoError(t, couchdb.UpdateDoc(inst, c))
}
