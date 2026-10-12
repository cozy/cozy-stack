package orgdrive

import (
	"context"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/orgdirectory"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/tests/testutils"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMembers(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}
	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	ctx := context.Background()

	t.Run("AddsTheDirectoryMembersAndSkipsTheOthers", func(t *testing.T) {
		org := newOrgInstance(t)
		inDirectory(t, org, "alice@example.net")
		pub := &fakePublisher{}

		require.NoError(t, NewHandler(pub).Handle(ctx, ensureWith(t, org, "sp", 1, []Member{
			{Email: "alice@example.net", Role: RoleWrite},
			{Email: "bob@example.net", Role: RoleRead},
		}, nil)))

		s := onlyDrive(t, org)
		alice := member(t, s, "alice@example.net")
		assert.False(t, alice.ReadOnly)
		assert.False(t, alice.OnlyInGroups)
		assert.Equal(t, []Skipped{{Email: "bob@example.net", Reason: ReasonNotInDirectory}}, pub.ready(t, 0).Skipped)
	})

	t.Run("ChangesRolesAndRevokesTheMembersLeft", func(t *testing.T) {
		org := newOrgInstance(t)
		inDirectory(t, org, "alice@example.net")
		inDirectory(t, org, "carol@example.net")
		h := NewHandler(&fakePublisher{})

		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 1, []Member{
			{Email: "alice@example.net", Role: RoleWrite},
			{Email: "carol@example.net", Role: RoleWrite},
		}, nil)))
		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 2, []Member{
			{Email: "alice@example.net", Role: RoleRead},
		}, nil)))

		s := onlyDrive(t, org)
		assert.True(t, member(t, s, "alice@example.net").ReadOnly)
		assert.Equal(t, sharing.MemberStatusRevoked, member(t, s, "carol@example.net").Status)
		assert.True(t, s.Active)
	})

	t.Run("SharesWithGroups", func(t *testing.T) {
		org := newOrgInstance(t)
		inGroup(t, org, "g1", "dave@example.net", "erin@example.net")
		pub := &fakePublisher{}
		h := NewHandler(pub)

		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 1, nil, []Group{
			{ID: "g1", Role: RoleRead},
			{ID: "unknown", Role: RoleRead},
		})))

		s := onlyDrive(t, org)
		require.Len(t, s.Groups, 1)
		assert.Equal(t, orgdirectory.GroupDocID(org.OrgID, "g1"), s.Groups[0].ID)
		assert.True(t, s.Groups[0].ReadOnly)
		dave := member(t, s, "dave@example.net")
		assert.True(t, dave.OnlyInGroups)
		assert.True(t, dave.ReadOnly)
		assert.Equal(t, []Skipped{{GroupID: "unknown", Reason: ReasonNotInDirectory}}, pub.ready(t, 0).Skipped)

		status := member(t, s, "dave@example.net").Status
		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 2, nil, []Group{{ID: "g1", Role: RoleWrite}})))
		s = onlyDrive(t, org)
		require.Len(t, s.Groups, 1)
		assert.False(t, s.Groups[0].Revoked)
		assert.False(t, s.Groups[0].ReadOnly)
		dave = member(t, s, "dave@example.net")
		assert.False(t, dave.ReadOnly)
		assert.Equal(t, status, dave.Status)

		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 3, nil, nil)))
		s = onlyDrive(t, org)
		assert.Equal(t, sharing.MemberStatusRevoked, member(t, s, "dave@example.net").Status)
		assert.True(t, s.Active)
	})

	t.Run("DirectRoleWinsOverAReadGroup", func(t *testing.T) {
		org := newOrgInstance(t)
		inGroup(t, org, "g1", "dave@example.net", "erin@example.net")
		pub := &fakePublisher{}
		h := NewHandler(pub)

		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 1,
			[]Member{{Email: "dave@example.net", Role: RoleWrite}},
			[]Group{{ID: "g1", Role: RoleRead}})))

		s := onlyDrive(t, org)
		dave := member(t, s, "dave@example.net")
		assert.False(t, dave.ReadOnly)
		assert.False(t, dave.OnlyInGroups)
		assert.True(t, member(t, s, "erin@example.net").ReadOnly)
		assert.Empty(t, pub.ready(t, 0).Skipped)

		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 2, nil, []Group{{ID: "g1", Role: RoleRead}})))
		dave = member(t, onlyDrive(t, org), "dave@example.net")
		assert.True(t, dave.OnlyInGroups)
		assert.True(t, dave.ReadOnly)
		assert.NotEqual(t, sharing.MemberStatusRevoked, dave.Status)
	})

	t.Run("GroupJoinsADirectMemberWithAnotherRole", func(t *testing.T) {
		org := newOrgInstance(t)
		inGroup(t, org, "g1", "dave@example.net", "erin@example.net")
		pub := &fakePublisher{}
		h := NewHandler(pub)
		dave := []Member{{Email: "dave@example.net", Role: RoleWrite}}

		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 1, dave, nil)))
		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 2, dave, []Group{{ID: "g1", Role: RoleRead}})))

		s := onlyDrive(t, org)
		require.Len(t, s.Groups, 1)
		assert.False(t, member(t, s, "dave@example.net").ReadOnly)
		assert.True(t, member(t, s, "erin@example.net").ReadOnly)
		assert.Empty(t, pub.ready(t, 1).Skipped)
	})

	t.Run("WriteGroupWinsOverADirectReader", func(t *testing.T) {
		org := newOrgInstance(t)
		inGroup(t, org, "g1", "dave@example.net", "erin@example.net")
		h := NewHandler(&fakePublisher{})
		dave := []Member{{Email: "dave@example.net", Role: RoleRead}}

		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 1, dave, []Group{{ID: "g1", Role: RoleRead}})))
		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 2, dave, []Group{{ID: "g1", Role: RoleWrite}})))

		s := onlyDrive(t, org)
		assert.False(t, s.Groups[0].Revoked)
		assert.False(t, member(t, s, "dave@example.net").ReadOnly)
		erin := member(t, s, "erin@example.net")
		assert.False(t, erin.ReadOnly)
		assert.NotEqual(t, sharing.MemberStatusRevoked, erin.Status)

		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 3, dave, []Group{{ID: "g1", Role: RoleRead}})))
		assert.True(t, member(t, onlyDrive(t, org), "dave@example.net").ReadOnly)
	})

	t.Run("RedeliveryDoesNotAddTwice", func(t *testing.T) {
		org := newOrgInstance(t)
		inDirectory(t, org, "alice@example.net")
		h := NewHandler(&fakePublisher{})
		d := ensureWith(t, org, "sp", 1, []Member{{Email: "alice@example.net", Role: RoleWrite}}, nil)

		require.NoError(t, h.Handle(ctx, d))
		require.NoError(t, h.Handle(ctx, d))

		assert.Len(t, onlyDrive(t, org).Members, 2)
	})
}

func ensureWith(t *testing.T, org *instance.Instance, ref string, revision int64, members []Member, groups []Group) amqp.Delivery {
	t.Helper()
	return amqp.Delivery{RoutingKey: rabbitmq.RoutingKeyOrgDriveEnsure, Body: mustJSON(t, map[string]any{
		"organizationId": org.OrgID,
		"reference":      map[string]any{"source": "twake-space", "id": ref},
		"revision":       revision,
		"state":          "created",
		"name":           "Space",
		"members":        members,
		"groups":         groups,
	})}
}

func inDirectory(t *testing.T, org *instance.Instance, email string) {
	t.Helper()
	_, err := orgdirectory.UpsertManagedContact(org, orgdirectory.ContactPatch{
		OrganizationID: org.OrgID,
		Username:       utils.RandomString(8),
		Email:          email,
	})
	require.NoError(t, err)
}

func inGroup(t *testing.T, org *instance.Instance, id string, emails ...string) {
	t.Helper()
	msg := orgdirectory.GroupCreatedMessage{OrganizationID: org.OrgID, ID: id, Name: "Group " + id}
	for _, email := range emails {
		msg.Members = append(msg.Members, orgdirectory.GroupMember{Username: utils.RandomString(8), Email: email})
	}
	require.NoError(t, orgdirectory.SyncGroupCreated(context.Background(), msg))
}

func member(t *testing.T, s *sharing.Sharing, email string) sharing.Member {
	t.Helper()
	for _, m := range s.Members[1:] {
		if m.Email == email {
			return m
		}
	}
	require.Failf(t, "member not found", "%s in %v", email, s.Members)
	return sharing.Member{}
}
