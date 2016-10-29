package vfs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalCacheCreateAndGetDir(t *testing.T) {
	cache := NewLocalCache(5)

	dir1, err := NewDirDoc("foo", RootFolderID, nil)
	if !assert.NoError(t, err) {
		return
	}

	err = cache.CreateDir(vfsC, dir1)
	if !assert.NoError(t, err) {
		return
	}

	dir2, err := cache.DirByPath(vfsC, "/foo")
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, dir1, dir2, "should have been retrived from cache")

	dir3, err := cache.DirByID(vfsC, dir1.ID())
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, dir1, dir3, "should have been retrived from cache")
}
