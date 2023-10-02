package office

import (
	"bytes"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
)

func GetFileByKey(inst *instance.Instance, key string) (*vfs.FileDoc, error) {
	detector, err := GetStore().GetDoc(inst, key)
	if err != nil {
		return nil, err
	}
	if detector == nil || detector.ID == "" || detector.Rev == "" {
		return nil, ErrInvalidKey
	}

	fs := inst.VFS()
	file, err := fs.FileByID(detector.ID)
	if err != nil {
		return nil, err
	}

	if file.Rev() == detector.Rev || bytes.Equal(file.MD5Sum, detector.MD5Sum) {
		return file, nil
	}

	// Manage the conflict
	conflictName := vfs.ConflictName(fs, file.DirID, file.DocName, true)
	newfile := vfs.CreateFileDocCopy(file, file.DirID, conflictName)
	newfile.CozyMetadata = vfs.NewCozyMetadata(inst.PageURL("/", nil))
	newfile.CozyMetadata.UpdatedAt = newfile.UpdatedAt
	newfile.CozyMetadata.UploadedAt = &newfile.UpdatedAt
	newfile.CozyMetadata.UploadedBy = &vfs.UploadedByEntry{Slug: OOSlug}
	if err := fs.CopyFile(file, newfile); err != nil {
		return nil, err
	}

	updated := conflictDetector{ID: newfile.ID(), Rev: newfile.Rev(), MD5Sum: newfile.MD5Sum}
	_ = GetStore().UpdateSecret(inst, key, file.ID(), newfile.ID())
	_ = GetStore().UpdateDoc(inst, key, updated)
	return newfile, nil
}
