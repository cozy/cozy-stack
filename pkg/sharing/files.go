package sharing

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

// EnsureSharedWithMeDir returns the shared-with-me directory, and create it if
// it doesn't exist
func EnsureSharedWithMeDir(inst *instance.Instance) error {
	fs := inst.VFS()
	dir, _, err := fs.DirOrFileByID(consts.SharedWithMeDirID)
	if err != nil {
		return err
	}

	if dir == nil {
		name := inst.Translate("Tree Shared with me")
		dir, err = vfs.NewDirDocWithPath(name, consts.SharedWithMeDirID, "/", nil)
		if err != nil {
			return err
		}
		return fs.CreateDir(dir)
	}

	if dir.RestorePath != "" {
		_, err = vfs.RestoreDir(fs, dir)
		if err != nil {
			return err
		}
		children, err := fs.DirBatch(dir, &couchdb.SkipCursor{})
		if err != nil {
			return err
		}
		for _, child := range children {
			d, f := child.Refine()
			if d != nil {
				_, err = vfs.TrashDir(fs, d)
			} else {
				_, err = vfs.TrashFile(fs, f)
			}
			if err != nil {
				return err
			}
		}
	}

	return nil
}
