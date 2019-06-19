package vfsafero

import (
	"bytes"
	"crypto/md5"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
)

func (afs *aferoVFS) Fsck(accumulate func(log *vfs.FsckLog)) (err error) {
	entries := make(map[string]*vfs.TreeFile, 1024)
	_, err = afs.BuildTree(func(f *vfs.TreeFile) {
		if !f.IsOrphan {
			entries[f.Fullpath] = f
		}
	})
	if err != nil {
		return
	}

	err = afero.Walk(afs.fs, "/", func(fullpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fullpath == vfs.WebappsDirName ||
			fullpath == vfs.KonnectorsDirName ||
			fullpath == vfs.ThumbsDirName {
			return filepath.SkipDir
		}

		f, ok := entries[fullpath]
		if !ok {
			accumulate(&vfs.FsckLog{
				Type:    vfs.IndexMissing,
				IsFile:  true,
				FileDoc: fileInfosToFileDoc(fullpath, info),
			})
		} else if f.IsDir != info.IsDir() {
			if f.IsDir {
				accumulate(&vfs.FsckLog{
					Type:    vfs.TypeMismatch,
					IsFile:  true,
					FileDoc: f,
					DirDoc:  fileInfosToDirDoc(fullpath, info),
				})
			} else {
				accumulate(&vfs.FsckLog{
					Type:    vfs.TypeMismatch,
					IsFile:  false,
					DirDoc:  f,
					FileDoc: fileInfosToFileDoc(fullpath, info),
				})
			}
		} else if !f.IsDir {
			var fd afero.File
			fd, err = afs.fs.Open(fullpath)
			if err != nil {
				return err
			}
			h := md5.New()
			if _, err = io.Copy(h, fd); err != nil {
				fd.Close()
				return err
			}
			if err = fd.Close(); err != nil {
				return err
			}
			md5sum := h.Sum(nil)
			if !bytes.Equal(md5sum, f.MD5Sum) || f.ByteSize != info.Size() {
				accumulate(&vfs.FsckLog{
					Type:    vfs.ContentMismatch,
					IsFile:  true,
					FileDoc: f,
					ContentMismatch: &vfs.FsckContentMismatch{
						SizeFile:    info.Size(),
						SizeIndex:   f.ByteSize,
						MD5SumFile:  md5sum,
						MD5SumIndex: f.MD5Sum,
					},
				})
			}
		}
		delete(entries, fullpath)
		return nil
	})
	if err != nil {
		return
	}

	for _, f := range entries {
		if f.IsDir {
			accumulate(&vfs.FsckLog{
				Type:   vfs.FileMissing,
				IsFile: false,
				DirDoc: f,
			})
		} else {
			accumulate(&vfs.FsckLog{
				Type:    vfs.FileMissing,
				IsFile:  true,
				FileDoc: f,
			})
		}
	}

	return
}

func fileInfosToDirDoc(fullpath string, fileinfo os.FileInfo) *vfs.TreeFile {
	return &vfs.TreeFile{
		DirOrFileDoc: vfs.DirOrFileDoc{
			DirDoc: &vfs.DirDoc{
				Type:      consts.DirType,
				DocName:   fileinfo.Name(),
				DirID:     "",
				CreatedAt: fileinfo.ModTime(),
				UpdatedAt: fileinfo.ModTime(),
				Fullpath:  fullpath,
			},
		},
	}
}

func fileInfosToFileDoc(fullpath string, fileinfo os.FileInfo) *vfs.TreeFile {
	trashed := strings.HasPrefix(fullpath, vfs.TrashDirName)
	contentType, md5sum, _ := extractContentTypeAndMD5(fullpath)
	mime, class := vfs.ExtractMimeAndClass(contentType)
	return &vfs.TreeFile{
		DirOrFileDoc: vfs.DirOrFileDoc{
			DirDoc: &vfs.DirDoc{
				Type:      consts.FileType,
				DocName:   fileinfo.Name(),
				DirID:     "",
				CreatedAt: fileinfo.ModTime(),
				UpdatedAt: fileinfo.ModTime(),
				Fullpath:  fullpath,
			},
			ByteSize:   fileinfo.Size(),
			Mime:       mime,
			Class:      class,
			Executable: int(fileinfo.Mode()|0111) > 0,
			MD5Sum:     md5sum,
			Trashed:    trashed,
		},
	}
}
