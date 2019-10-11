package vfsswift

import (
	"bytes"
	"encoding/hex"
	"path"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/ncw/swift"
)

func (sfs *swiftVFSV2) Fsck(accumulate func(log *vfs.FsckLog), failFast bool) error {
	entries := make(map[string]*vfs.TreeFile, 1024)
	tree, err := sfs.BuildTree(func(f *vfs.TreeFile) {
		if !f.IsDir {
			entries[f.DocID] = f
		}
	})
	if err != nil {
		return err
	}
	if err = sfs.CheckTreeIntegrity(tree, accumulate, failFast); err != nil {
		if err == vfs.ErrFsckFailFail {
			return nil
		}
		return err
	}
	return sfs.checkFiles(entries, accumulate, failFast)
}

func (sfs *swiftVFSV2) CheckFilesConsistency(accumulate func(log *vfs.FsckLog), failFast bool) error {
	entries := make(map[string]*vfs.TreeFile, 1024)
	_, err := sfs.BuildTree(func(f *vfs.TreeFile) {
		if !f.IsDir {
			entries[f.DirID+"/"+f.DocName] = f
		}
	})
	if err != nil {
		return err
	}
	return sfs.checkFiles(entries, accumulate, failFast)
}

func (sfs *swiftVFSV2) checkFiles(
	entries map[string]*vfs.TreeFile,
	accumulate func(log *vfs.FsckLog),
	failFast bool,
) error {
	err := sfs.c.ObjectsWalk(sfs.container, nil, func(opts *swift.ObjectsOpts) (interface{}, error) {
		objs, err := sfs.c.Objects(sfs.container, opts)
		if err != nil {
			return nil, err
		}
		for _, obj := range objs {
			docID := makeDocID(obj.Name)
			f, ok := entries[docID]
			if !ok {
				accumulate(&vfs.FsckLog{
					Type:    vfs.IndexMissing,
					IsFile:  true,
					FileDoc: objectToFileDocV2(sfs.container, obj),
				})
				if failFast {
					return nil, errFailFast
				}
			} else {
				var md5sum []byte
				md5sum, err = hex.DecodeString(obj.Hash)
				if err != nil {
					return nil, err
				}
				if !bytes.Equal(md5sum, f.MD5Sum) || f.ByteSize != obj.Bytes {
					accumulate(&vfs.FsckLog{
						Type:    vfs.ContentMismatch,
						IsFile:  true,
						FileDoc: f,
						ContentMismatch: &vfs.FsckContentMismatch{
							SizeFile:    obj.Bytes,
							SizeIndex:   f.ByteSize,
							MD5SumFile:  md5sum,
							MD5SumIndex: f.MD5Sum,
						},
					})
					if failFast {
						return nil, errFailFast
					}
				}
				delete(entries, docID)
			}
		}
		return objs, err
	})
	if err != nil {
		if err == errFailFast {
			return nil
		}
		return err
	}

	// entries should contain only data that does not contain an associated
	// index.
	for _, f := range entries {
		accumulate(&vfs.FsckLog{
			Type:    vfs.FSMissing,
			IsFile:  true,
			FileDoc: f,
		})
		if err == errFailFast {
			return nil
		}
	}

	return nil
}

func objectToFileDocV2(container string, object swift.Object) *vfs.TreeFile {
	md5sum, _ := hex.DecodeString(object.Hash)
	name := "unknown"
	mime, class := vfs.ExtractMimeAndClass(object.ContentType)
	return &vfs.TreeFile{
		DirOrFileDoc: vfs.DirOrFileDoc{
			DirDoc: &vfs.DirDoc{
				Type:      consts.FileType,
				DocID:     makeDocID(object.Name),
				DocName:   name,
				DirID:     "",
				CreatedAt: object.LastModified,
				UpdatedAt: object.LastModified,
				Fullpath:  path.Join(vfs.OrphansDirName, name),
			},
			ByteSize:   object.Bytes,
			Mime:       mime,
			Class:      class,
			Executable: false,
			MD5Sum:     md5sum,
		},
	}
}
