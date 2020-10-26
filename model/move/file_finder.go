package move

import (
	"strings"

	"github.com/cozy/cozy-stack/model/vfs"
)

type fileFinderWithCache struct {
	vfs   vfs.VFS
	cache map[string]*vfs.FileDoc // fileID -> file
}

func newFileFinderWithCache(fs vfs.VFS) *fileFinderWithCache {
	return &fileFinderWithCache{
		vfs:   fs,
		cache: make(map[string]*vfs.FileDoc),
	}
}

func (ff *fileFinderWithCache) Find(versionID string) (*vfs.FileDoc, error) {
	fileID := strings.SplitN(versionID, "/", 2)[0]
	if file, ok := ff.cache[fileID]; ok {
		return file, nil
	}
	file, err := ff.vfs.FileByID(fileID)
	if err != nil {
		return nil, err
	}
	ff.cache[fileID] = file
	return file, nil
}
