package vfsswift

import (
	"bytes"
	"encoding/hex"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/swift"
)

func (sfs *swiftVFS) Fsck(accumulate func(log *vfs.FsckLog)) error {
	entries := make(map[string]*vfs.TreeFile, 1024)
	tree, err := sfs.BuildTree(func(f *vfs.TreeFile) {
		if !f.IsOrphan {
			entries[f.Fullpath] = f
		}
	})
	if err != nil {
		return err
	}
	if err = sfs.CheckTreeIntegrity(tree, accumulate); err != nil {
		return err
	}
	return sfs.checkFiles(entries, accumulate)
}

func (sfs *swiftVFS) CheckFilesConsistency(accumulate func(log *vfs.FsckLog)) error {
	entries := make(map[string]*vfs.TreeFile, 1024)
	_, err := sfs.BuildTree(func(f *vfs.TreeFile) {
		if !f.IsOrphan {
			entries[f.Fullpath] = f
		}
	})
	if err != nil {
		return err
	}
	return sfs.checkFiles(entries, accumulate)
}

func (sfs *swiftVFS) checkFiles(entries map[string]*vfs.TreeFile, accumulate func(log *vfs.FsckLog)) (err error) {
	var orphansObjs []swift.Object

	err = sfs.c.ObjectsWalk(sfs.container, nil, func(opts *swift.ObjectsOpts) (interface{}, error) {
		var objs []swift.Object
		objs, err = sfs.c.Objects(sfs.container, opts)
		if err != nil {
			return nil, err
		}
		for _, obj := range objs {
			f, ok := entries[obj.Name]
			if !ok {
				orphansObjs = append(orphansObjs, obj)
			} else if f.IsDir != (obj.ContentType == dirContentType) {
				if f.IsDir {
					accumulate(&vfs.FsckLog{
						Type:    vfs.TypeMismatch,
						IsFile:  true,
						FileDoc: f,
						DirDoc:  objectToFileDocV1(sfs.container, obj),
					})
				} else {
					accumulate(&vfs.FsckLog{
						Type:    vfs.TypeMismatch,
						IsFile:  false,
						DirDoc:  f,
						FileDoc: objectToFileDocV1(sfs.container, obj),
					})
				}
			} else if !f.IsDir {
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
				}
				delete(entries, obj.Name)
			}
		}
		return objs, err
	})

	for _, f := range entries {
		accumulate(&vfs.FsckLog{
			Type:    vfs.FSMissing,
			IsFile:  true,
			FileDoc: f,
		})
	}

	for _, obj := range orphansObjs {
		if obj.ContentType == dirContentType {
			accumulate(&vfs.FsckLog{
				Type:   vfs.IndexMissing,
				IsFile: false,
				DirDoc: objectToFileDocV1(sfs.container, obj),
			})
		} else {
			accumulate(&vfs.FsckLog{
				Type:    vfs.IndexMissing,
				IsFile:  true,
				FileDoc: objectToFileDocV1(sfs.container, obj),
			})
		}
	}

	return
}

func objectToFileDocV1(container string, object swift.Object) *vfs.TreeFile {
	var dirID, name string
	if dirIDAndName := strings.SplitN(object.Name, "/", 2); len(dirIDAndName) == 2 {
		dirID = dirIDAndName[0]
		name = dirIDAndName[0]
	}
	docType := consts.FileType
	if object.ContentType == dirContentType {
		docType = consts.DirType
	}
	md5sum, _ := hex.DecodeString(object.Hash)
	mime, class := vfs.ExtractMimeAndClass(object.ContentType)
	return &vfs.TreeFile{
		DirOrFileDoc: vfs.DirOrFileDoc{
			DirDoc: &vfs.DirDoc{
				Type:      docType,
				DocID:     makeDocID(object.Name),
				DocName:   name,
				DirID:     dirID,
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
