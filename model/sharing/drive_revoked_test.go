package sharing

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAcceptedDrive creates a shared drive on the owner instance, accepted
// by the recipient instance (with a valid drive token and a shortcut).
func setupAcceptedDrive(t *testing.T, ownerInst, recipientInst *instance.Instance) (*Sharing, *Sharing) {
	t.Helper()

	rootDir, err := vfs.Mkdir(ownerInst.VFS(), "/Projects", nil)
	require.NoError(t, err)

	now := time.Now().UTC()
	members := func() []Member {
		return []Member{
			{Status: MemberStatusOwner, Instance: "https://" + ownerInst.Domain},
			{Status: MemberStatusReady, Instance: "https://" + recipientInst.Domain},
		}
	}

	ownerSharing := &Sharing{
		SID:           uuidv7(),
		Drive:         true,
		DriveRootType: DriveRootTypeDirectory,
		Owner:         true,
		Active:        true,
		CreatedAt:     now,
		UpdatedAt:     now,
		Rules:         []Rule{{Title: "Projects", DocType: consts.Files, Values: []string{rootDir.DocID}}},
		Members:       members(),
		Credentials:   []Credentials{{State: "state-1", XorKey: MakeXorKey()}},
	}
	require.NoError(t, couchdb.CreateNamedDoc(ownerInst, ownerSharing))

	driveToken, err := ownerSharing.GetInteractCode(ownerInst, &ownerSharing.Members[1], 1)
	require.NoError(t, err)

	recipientSharing := &Sharing{
		SID:           ownerSharing.SID,
		Drive:         true,
		DriveRootType: DriveRootTypeDirectory,
		Active:        true,
		CreatedAt:     now,
		UpdatedAt:     now,
		Rules: []Rule{{
			Title:   "Projects",
			DocType: consts.Files,
			Values:  []string{XorID(rootDir.DocID, ownerSharing.Credentials[0].XorKey)},
		}},
		Members:     members(),
		Credentials: []Credentials{{DriveToken: driveToken, State: "state-1"}},
	}
	require.NoError(t, recipientSharing.CreateDriveShortcut(recipientInst, true))
	require.NoError(t, couchdb.CreateNamedDoc(recipientInst, recipientSharing))

	return ownerSharing, recipientSharing
}

func TestDriveRevokedByOwner(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(t *testing.T, ownerInst, recipientInst *instance.Instance, owner, recipient *Sharing)
		wantRevoked bool
	}{
		{
			name:        "member still active on owner",
			mutate:      func(*testing.T, *instance.Instance, *instance.Instance, *Sharing, *Sharing) {},
			wantRevoked: false,
		},
		{
			name: "member revoked on owner without notification",
			mutate: func(t *testing.T, ownerInst, _ *instance.Instance, owner, _ *Sharing) {
				require.NoError(t, owner.RemoveInteractPermissionsForAMember(ownerInst, &owner.Members[1], 1))
				owner.Members[1].Status = MemberStatusRevoked
				require.NoError(t, couchdb.UpdateDoc(ownerInst, owner))
			},
			wantRevoked: true,
		},
		{
			name: "member revoked on owner with drive token kept",
			mutate: func(t *testing.T, ownerInst, _ *instance.Instance, owner, _ *Sharing) {
				owner.Members[1].Status = MemberStatusRevoked
				require.NoError(t, couchdb.UpdateDoc(ownerInst, owner))
			},
			wantRevoked: true,
		},
		{
			name: "sharing inactive on owner",
			mutate: func(t *testing.T, ownerInst, _ *instance.Instance, owner, _ *Sharing) {
				owner.Active = false
				require.NoError(t, couchdb.UpdateDoc(ownerInst, owner))
			},
			wantRevoked: true,
		},
		{
			name: "sharing deleted on owner",
			mutate: func(t *testing.T, ownerInst, _ *instance.Instance, owner, _ *Sharing) {
				require.NoError(t, couchdb.DeleteDoc(ownerInst, owner))
			},
			wantRevoked: true,
		},
		{
			name: "owner on another stack",
			mutate: func(t *testing.T, ownerInst, recipientInst *instance.Instance, owner, recipient *Sharing) {
				require.NoError(t, couchdb.DeleteDoc(ownerInst, owner))
				recipient.Members[0].Instance = "https://unknown.cozy.example.net"
				require.NoError(t, couchdb.UpdateDoc(recipientInst, recipient))
			},
			wantRevoked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ownerInst, recipientInst := setupTwoTestInstances(t)
			owner, recipient := setupAcceptedDrive(t, ownerInst, recipientInst)
			tt.mutate(t, ownerInst, recipientInst, owner, recipient)

			checks, err := CheckSharings(recipientInst, true)
			require.NoError(t, err)
			wantCheck := map[string]interface{}{
				"id":    recipient.SID,
				"type":  "drive_revoked_by_owner",
				"owner": recipient.Members[0].Instance,
			}
			if tt.wantRevoked {
				assert.Contains(t, checks, wantCheck)
			} else {
				assert.NotContains(t, checks, wantCheck)
			}

			fixed, err := FixSharings(recipientInst)
			require.NoError(t, err)
			stored, err := FindSharing(recipientInst, recipient.SID)
			require.NoError(t, err)
			if !tt.wantRevoked {
				assert.NotContains(t, fixed, wantCheck)
				assert.True(t, stored.Active)
				return
			}
			assert.Contains(t, fixed, wantCheck)
			assert.False(t, stored.Active)
			assert.Empty(t, stored.Credentials)
			_, err = recipientInst.VFS().FileByID(recipient.ShortcutID)
			assert.Error(t, err, "the drive shortcut should be removed")

			checks, err = CheckSharings(recipientInst, true)
			require.NoError(t, err)
			assert.NotContains(t, checks, wantCheck)
		})
	}
}
