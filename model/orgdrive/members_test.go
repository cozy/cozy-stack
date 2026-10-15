package orgdrive

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
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

	_ "github.com/cozy/cozy-stack/worker/mails"
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
		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 1, nil, []Group{{ID: "g1", Role: RoleRead}})))
		err := h.Handle(ctx, ensureWith(t, org, "sp", 2, nil, []Group{{ID: "g1", Role: RoleWrite}}))
		require.ErrorIs(t, err, sharing.ErrGroupCannotBeAddedTwice)
		s = onlyDrive(t, org)
		require.Len(t, s.Groups, 1)
		assert.False(t, s.Groups[0].Revoked)
		assert.True(t, s.Groups[0].ReadOnly)
		dave = member(t, s, "dave@example.net")
		assert.True(t, dave.ReadOnly)
		assert.Equal(t, status, dave.Status)

		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 3, nil, nil)))
		s = onlyDrive(t, org)
		assert.Equal(t, sharing.MemberStatusRevoked, member(t, s, "dave@example.net").Status)
		assert.True(t, s.Active)
	})

	for _, directRole := range []string{RoleRead, RoleWrite} {
		t.Run("RejectsConflictingRolesInOneCommand/"+directRole, func(t *testing.T) {
			org := newOrgInstance(t)
			inGroup(t, org, "g1", "dave@example.net")
			groupRole := RoleRead
			if directRole == RoleRead {
				groupRole = RoleWrite
			}
			pub := &fakePublisher{}
			err := NewHandler(pub).Handle(ctx, ensureWith(t, org, "sp", 1,
				[]Member{{Email: "dave@example.net", Role: directRole}},
				[]Group{{ID: "g1", Role: groupRole}}))
			require.ErrorIs(t, err, sharing.ErrMemberAlreadyAdded)
			assert.Empty(t, pub.requests)
			assert.Empty(t, onlyDrive(t, org).Groups)
		})
	}

	t.Run("RejectsGroupConflictingWithExistingDirectMember", func(t *testing.T) {
		org := newOrgInstance(t)
		inGroup(t, org, "g1", "dave@example.net", "erin@example.net")
		pub := &fakePublisher{}
		h := NewHandler(pub)
		dave := []Member{{Email: "dave@example.net", Role: RoleRead}}
		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 1, dave, nil)))
		err := h.Handle(ctx, ensureWith(t, org, "sp", 2, dave, []Group{{ID: "g1", Role: RoleWrite}}))
		require.ErrorIs(t, err, sharing.ErrMemberAlreadyAdded)
		s := onlyDrive(t, org)
		assert.Empty(t, s.Groups)
		assert.True(t, member(t, s, "dave@example.net").ReadOnly)
		assert.Len(t, pub.requests, 1)
	})

	t.Run("RejectsDirectRoleConflictingWithGroupMember", func(t *testing.T) {
		org := newOrgInstance(t)
		inGroup(t, org, "g1", "dave@example.net")
		h := NewHandler(&fakePublisher{})
		groups := []Group{{ID: "g1", Role: RoleRead}}
		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 1, nil, groups)))
		err := h.Handle(ctx, ensureWith(t, org, "sp", 2,
			[]Member{{Email: "dave@example.net", Role: RoleWrite}}, groups))
		require.ErrorIs(t, err, sharing.ErrMemberAlreadyInGroup)
		dave := member(t, onlyDrive(t, org), "dave@example.net")
		assert.True(t, dave.ReadOnly)
		assert.True(t, dave.OnlyInGroups)
	})

	t.Run("RejectsConflictingGroupRoles", func(t *testing.T) {
		org := newOrgInstance(t)
		inGroup(t, org, "readers", "dave@example.net")
		inGroup(t, org, "writers", "dave@example.net")
		h := NewHandler(&fakePublisher{})
		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 1, nil,
			[]Group{{ID: "readers", Role: RoleRead}})))
		err := h.Handle(ctx, ensureWith(t, org, "sp", 2, nil,
			[]Group{{ID: "readers", Role: RoleRead}, {ID: "writers", Role: RoleWrite}}))
		require.ErrorIs(t, err, sharing.ErrMemberAlreadyInGroup)
		assert.Len(t, onlyDrive(t, org).Groups, 1)
	})

	t.Run("DirectRoleUpdatesUseExistingReadOnlyMethods", func(t *testing.T) {
		org := newOrgInstance(t)
		inGroup(t, org, "g1", "dave@example.net", "erin@example.net")
		h := NewHandler(&fakePublisher{})
		groups := []Group{{ID: "g1", Role: RoleWrite}}
		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 1,
			[]Member{{Email: "dave@example.net", Role: RoleWrite}}, groups)))
		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 2,
			[]Member{{Email: "dave@example.net", Role: RoleRead}}, groups)))
		s := onlyDrive(t, org)
		assert.True(t, member(t, s, "dave@example.net").ReadOnly)
		assert.False(t, member(t, s, "erin@example.net").ReadOnly)
		assert.False(t, s.Groups[0].ReadOnly)

		require.NoError(t, h.Handle(ctx, ensureWith(t, org, "sp", 3, nil, groups)))
		dave := member(t, onlyDrive(t, org), "dave@example.net")
		assert.True(t, dave.OnlyInGroups)
		assert.True(t, dave.ReadOnly)
	})

	t.Run("RetriesFailedInvitationBeforePublishingReady", func(t *testing.T) {
		org := newOrgInstance(t)
		inDirectory(t, org, "alice@example.net")
		pub := &fakePublisher{}
		h := NewHandler(pub)
		d := ensureWith(t, org, "retry", 1, []Member{{Email: "alice@example.net", Role: RoleRead}}, nil)

		client := config.CouchClient()
		original := client.Transport
		t.Cleanup(func() { client.Transport = original })
		var failures atomic.Int32
		client.Transport = roundTripper(func(r *http.Request) (*http.Response, error) {
			if r.Method == http.MethodPost && strings.Contains(r.URL.Path, org.DBPrefix()) && strings.Contains(r.URL.Path, "io-cozy-jobs") {
				failures.Add(1)
				return nil, errors.New("job storage unavailable")
			}
			return original.RoundTrip(r)
		})

		err := h.Handle(ctx, d)
		require.Positive(t, failures.Load())
		require.ErrorIs(t, err, sharing.ErrInvitationNotSent)
		assert.Empty(t, pub.requests)
		s := onlyDrive(t, org)
		assert.False(t, s.Reference.Ready)
		assert.Equal(t, sharing.MemberStatusMailNotSent, member(t, s, "alice@example.net").Status)
		state := s.Credentials[0].State

		client.Transport = original
		require.NoError(t, h.Handle(ctx, d))
		retried := onlyDrive(t, org)
		assert.Equal(t, s.SID, retried.SID)
		assert.Len(t, retried.Members, 2)
		assert.Equal(t, state, retried.Credentials[0].State)
		assert.Equal(t, sharing.MemberStatusPendingInvitation, member(t, retried, "alice@example.net").Status)
		assert.True(t, retried.Reference.Ready)
		assert.Len(t, pub.requests, 1)
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
