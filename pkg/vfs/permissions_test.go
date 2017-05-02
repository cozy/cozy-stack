package vfs_test

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/stretchr/testify/assert"
)

func TestPermissions(t *testing.T) {
	origtree := H{
		"O/": H{
			"A/": H{
				"a1/": H{},
				"a2/": H{},
			},
			"B/": H{
				"b1.txt": nil,
				"c1.txt": nil,
			},
			"B2/": H{
				"b1.txt": nil,
				"c1.txt": nil,
			},
			"C/":    H{},
			"d.txt": nil,
		},
	}
	O, err := createTree(origtree, consts.RootDirID)
	if !assert.NoError(t, err) {
		return
	}

	A, err := fs.DirByPath("/O/A")
	if !assert.NoError(t, err) {
		return
	}

	B, err := fs.DirByPath("/O/B")
	if !assert.NoError(t, err) {
		return
	}
	vfs.ModifyDirMetadata(fs, B, &vfs.DocPatch{
		Tags: &[]string{"testtagparent"},
	})

	B2, err := fs.DirByPath("/O/B2")
	if !assert.NoError(t, err) {
		return
	}

	f, err := fs.FileByPath("/O/B/b1.txt")
	if !assert.NoError(t, err) {
		return
	}
	vfs.ModifyFileMetadata(fs, f, &vfs.DocPatch{
		Tags: &[]string{"testtag"},
	})
	// reload
	f, err = fs.FileByPath("/O/B/b1.txt")
	if !assert.NoError(t, err) {
		return
	}
	// hack to have a Class attribute
	f.Class = "superfile"

	psetWholeType := permissions.Set{
		permissions.Rule{
			Type:  consts.Files,
			Verbs: permissions.ALL,
		},
	}
	assert.NoError(t, vfs.Allows(fs, psetWholeType, permissions.GET, f))

	psetSelfID := permissions.Set{
		permissions.Rule{
			Type:   consts.Files,
			Verbs:  permissions.ALL,
			Values: []string{f.ID()},
		},
	}
	assert.NoError(t, vfs.Allows(fs, psetSelfID, permissions.GET, f))

	psetSelfAttributes := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "class",
			Values:   []string{"superfile"},
		},
	}
	assert.NoError(t, vfs.Allows(fs, psetSelfAttributes, permissions.GET, f))

	psetOnlyFiles := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "type",
			Values:   []string{"file"},
		},
	}
	assert.NoError(t, vfs.Allows(fs, psetOnlyFiles, permissions.GET, f))

	psetOnlyDirs := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "type",
			Values:   []string{"directory"},
		},
	}
	assert.NoError(t, vfs.Allows(fs, psetOnlyDirs, permissions.GET, B))

	psetMime := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "mime",
			Values:   []string{"text/plain"},
		},
	}
	f.Mime = "text/plain"
	assert.NoError(t, vfs.Allows(fs, psetMime, permissions.GET, f))

	psetReferences := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "referenced_by",
			Values:   []string{"somealbumid"},
		},
	}
	f.ReferencedBy = []couchdb.DocReference{{Type: "io.cozy.albums", ID: "somealbumid"}}
	assert.NoError(t, vfs.Allows(fs, psetReferences, permissions.GET, f))

	psetBadReferences := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "referenced_by",
			Values:   []string{"anotheralbumid"},
		},
	}
	assert.Error(t, vfs.Allows(fs, psetBadReferences, permissions.GET, f))

	psetName := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "name",
			Values:   []string{"b1.txt"},
		},
	}
	assert.NoError(t, vfs.Allows(fs, psetName, permissions.GET, f))

	psetSelfTag := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "tags",
			Values:   []string{"testtag"},
		},
	}
	assert.NoError(t, vfs.Allows(fs, psetSelfTag, permissions.GET, f))

	psetParentID := permissions.Set{
		permissions.Rule{
			Type:   consts.Files,
			Verbs:  permissions.ALL,
			Values: []string{O.ID()},
		},
	}
	assert.NoError(t, vfs.Allows(fs, psetParentID, permissions.GET, f))

	psetSelfParentTag := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "tags",
			Values:   []string{"testtagparent"},
		},
	}
	assert.NoError(t, vfs.Allows(fs, psetSelfParentTag, permissions.GET, f))

	psetWrongType := permissions.Set{
		permissions.Rule{
			Type:   "io.cozy.not-files",
			Verbs:  permissions.ALL,
			Values: []string{A.ID()},
		},
	}
	assert.Error(t, vfs.Allows(fs, psetWrongType, permissions.GET, f))

	psetWrongVerb := permissions.Set{
		permissions.Rule{
			Type:   consts.Files,
			Verbs:  permissions.Verbs(permissions.POST),
			Values: []string{A.ID()},
		},
	}
	assert.Error(t, vfs.Allows(fs, psetWrongVerb, permissions.GET, f))

	psetUncleID := permissions.Set{
		permissions.Rule{
			Type:   consts.Files,
			Verbs:  permissions.ALL,
			Values: []string{B.ID()},
		},
	}
	assert.Error(t, vfs.Allows(fs, psetUncleID, permissions.GET, B2))

	psetUnclePrefixID := permissions.Set{
		permissions.Rule{
			Type:   consts.Files,
			Verbs:  permissions.ALL,
			Values: []string{A.ID()},
		},
	}
	assert.Error(t, vfs.Allows(fs, psetUnclePrefixID, permissions.GET, f))

}
