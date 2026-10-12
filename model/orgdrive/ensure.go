// Package orgdrive applies the commands other apps send to keep shared drives
// on an organization instance in sync with their own objects. It knows the
// apps only by the reference they give.
package orgdrive

import (
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// Result is what applying a command left: the drive, and whether the command
// changed anything. A command with the revision already applied changes
// nothing, but its reply is published again.
type Result struct {
	Drive   *sharing.Sharing
	Applied bool
}

// Ensure makes the drive of the reference match a created command. It returns
// a nil drive when the command is older than the last one applied.
func Ensure(inst *instance.Instance, cmd *Command) (*Result, error) {
	all, err := sharing.FindByReference(inst, cmd.Reference.Source, cmd.Reference.ID)
	if err != nil {
		return nil, err
	}
	var last int64
	var drive *sharing.Sharing
	for _, s := range all {
		if s.Reference.Revision > last {
			last = s.Reference.Revision
		}
		if drive == nil {
			if ok, err := alive(inst, s); err != nil {
				return nil, err
			} else if ok {
				drive = s
			}
		}
	}
	switch {
	case cmd.Revision < last:
		return &Result{}, nil
	case cmd.Revision == last && drive != nil:
		return &Result{Drive: drive}, nil
	}

	if drive == nil {
		drive, err = create(inst, cmd)
		if err != nil {
			return nil, err
		}
		return &Result{Drive: drive, Applied: true}, nil
	}

	drive.Reference.Revision = cmd.Revision
	if drive.Description != cmd.Name {
		err = rename(inst, drive, cmd.Name)
	} else {
		err = couchdb.UpdateDoc(inst, drive)
	}
	if err != nil {
		return nil, err
	}
	return &Result{Drive: drive, Applied: true}, nil
}

// alive is false for a drive that was revoked, or whose folder was trashed or
// deleted. A new drive replaces it.
func alive(inst *instance.Instance, s *sharing.Sharing) (bool, error) {
	if !s.Owner || !s.Drive || !s.Active {
		return false, nil
	}
	rule := s.FirstFilesRule()
	if rule == nil || len(rule.Values) == 0 {
		return false, nil
	}
	dir, err := inst.VFS().DirByID(rule.Values[0])
	if couchdb.IsNotFoundError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !inTrash(dir), nil
}

func inTrash(dir *vfs.DirDoc) bool {
	return dir.Fullpath == vfs.TrashDirName || strings.HasPrefix(dir.Fullpath, vfs.TrashDirName+"/")
}

func create(inst *instance.Instance, cmd *Command) (*sharing.Sharing, error) {
	fs := inst.VFS()
	dir, err := vfs.NewDirDocWithPath(freeName(fs, folderName(cmd.Name)), consts.RootDirID, "/", nil)
	if err != nil {
		return nil, err
	}
	if err := fs.CreateDir(dir); err != nil {
		return nil, err
	}
	s, err := sharing.CreateDrive(inst, dir.ID(), cmd.Name, "")
	if err != nil {
		return nil, err
	}
	s.OrgDrive = inst.IsOrganizationInstance()
	s.Reference = &sharing.Reference{
		Source:   cmd.Reference.Source,
		ID:       cmd.Reference.ID,
		Revision: cmd.Revision,
	}
	if _, err := s.Create(inst); err != nil {
		return nil, err
	}
	return s, nil
}

func rename(inst *instance.Instance, s *sharing.Sharing, name string) error {
	fs := inst.VFS()
	dir, err := fs.DirByID(s.FirstFilesRule().Values[0])
	if err != nil {
		return err
	}
	if want := folderName(name); dir.DocName != want {
		newName := freeName(fs, want)
		if _, err := vfs.ModifyDirMetadata(fs, dir, &vfs.DocPatch{Name: &newName}); err != nil {
			return err
		}
	}
	return s.PatchDescription(inst, name)
}

func folderName(name string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if strings.ContainsRune(vfs.ForbiddenFilenameChars, r) {
			return '-'
		}
		return r
	}, name))
	if name == "" || name == "." || name == ".." {
		return "Drive"
	}
	return name
}

func freeName(fs vfs.VFS, name string) string {
	if exists, err := fs.GetIndexer().DirChildExists(consts.RootDirID, name); err == nil && exists {
		return vfs.ConflictName(fs, consts.RootDirID, name, false)
	}
	return name
}
