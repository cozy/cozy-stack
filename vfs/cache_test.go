package vfs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalCacheCreateAndGetDir(t *testing.T) {
	dir1, err := NewDirDoc("cachedfoo", RootFolderID, nil)
	if !assert.NoError(t, err) {
		return
	}

	err = c.fsCache.CreateDir(c, dir1)
	if !assert.NoError(t, err) {
		return
	}

	dir2, err := c.fsCache.DirByPath(c, "/cachedfoo")
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, dir1, dir2, "should have been retrived from cache")

	dir3, err := c.fsCache.DirByID(c, dir1.ID())
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, dir1, dir3, "should have been retrived from cache")
}

func TestLocalCacheMutations(t *testing.T) {
	tree := H{
		"cached/": H{
			"dirchild1/": H{
				"food/": H{},
				"bard/": H{},
			},
			"dirchild2/": H{
				"foof":   nil,
				"barf":   nil,
				"quzd/":  H{},
				"quzd2/": H{},
				"quzd3/": H{},
				"quzd4/": H{},
			},
			"dirchild3/": H{},
			"filechild1": nil,
		},
	}

	_, err := createTree(tree, RootFolderID)
	if !assert.NoError(t, err) {
		return
	}

	dirchild1, err := c.fsCache.DirByPath(c, "/cached/dirchild1")
	if !assert.NoError(t, err) {
		return
	}

	dirchild2, err := c.fsCache.DirByPath(c, "/cached/dirchild2")
	if !assert.NoError(t, err) {
		return
	}

	quzd, err := c.fsCache.DirByPath(c, "/cached/dirchild2/quzd")
	if !assert.NoError(t, err) {
		return
	}

	foof, err := c.fsCache.FileByPath(c, "/cached/dirchild2/foof")
	if !assert.NoError(t, err) {
		return
	}

	if !assert.Equal(t, foof.Name, "foof") {
		return
	}

	newfolderid := dirchild1.ID()
	_, err = ModifyDirMetadata(c, dirchild2, &DocPatch{FolderID: &newfolderid})
	if !assert.NoError(t, err) {
		return
	}

	dirchild2bis, err := c.fsCache.DirByID(c, dirchild2.ID())
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, dirchild2bis.FolderID, newfolderid) {
		return
	}

	_, err = c.fsCache.DirByPath(c, "/cached/dirchild2/quzd")
	if !assert.Error(t, err) {
		return
	}

	quzdbis, err := c.fsCache.DirByPath(c, "/cached/dirchild1/dirchild2/quzd")
	if !assert.NoError(t, err) {
		return
	}

	if !assert.NotEqual(t, quzd, quzdbis) {
		return
	}

	_, err = c.fsCache.FileByPath(c, "/cached/dirchild2/foof")
	if !assert.Error(t, err) {
		return
	}

	foof2, err := c.fsCache.FileByPath(c, "/cached/dirchild1/dirchild2/foof")
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, foof, foof2)
}
