package vfs

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/permissions"
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

	A, err := GetDirDocFromPath(vfsC, "/O/A", false)
	if !assert.NoError(t, err) {
		return
	}

	B, err := GetDirDocFromPath(vfsC, "/O/B", false)
	if !assert.NoError(t, err) {
		return
	}
	ModifyDirMetadata(vfsC, B, &DocPatch{
		Tags: &[]string{"testtagparent"},
	})

	B2, err := GetDirDocFromPath(vfsC, "/O/B2", false)
	if !assert.NoError(t, err) {
		return
	}

	f, err := GetFileDocFromPath(vfsC, "/O/B/b1.txt")
	if !assert.NoError(t, err) {
		return
	}
	ModifyFileMetadata(vfsC, f, &DocPatch{
		Tags: &[]string{"testtag"},
	})
	// reload
	f, err = GetFileDocFromPath(vfsC, "/O/B/b1.txt")
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
	assert.NoError(t, Allows(vfsC, psetWholeType, permissions.GET, f))

	psetSelfID := permissions.Set{
		permissions.Rule{
			Type:   consts.Files,
			Verbs:  permissions.ALL,
			Values: []string{f.ID()},
		},
	}
	assert.NoError(t, Allows(vfsC, psetSelfID, permissions.GET, f))

	psetSelfAttributes := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "class",
			Values:   []string{"superfile"},
		},
	}
	assert.NoError(t, Allows(vfsC, psetSelfAttributes, permissions.GET, f))

	psetOnlyFiles := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "type",
			Values:   []string{"file"},
		},
	}
	assert.NoError(t, Allows(vfsC, psetOnlyFiles, permissions.GET, f))

	psetOnlyDirs := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "type",
			Values:   []string{"directory"},
		},
	}
	assert.NoError(t, Allows(vfsC, psetOnlyDirs, permissions.GET, B))

	psetMime := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "mime",
			Values:   []string{"text/plain"},
		},
	}
	f.Mime = "text/plain"
	assert.NoError(t, Allows(vfsC, psetMime, permissions.GET, f))

	psetName := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "name",
			Values:   []string{"b1.txt"},
		},
	}
	assert.NoError(t, Allows(vfsC, psetName, permissions.GET, f))

	psetSelfTag := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "tags",
			Values:   []string{"testtag"},
		},
	}
	assert.NoError(t, Allows(vfsC, psetSelfTag, permissions.GET, f))

	psetParentID := permissions.Set{
		permissions.Rule{
			Type:   consts.Files,
			Verbs:  permissions.ALL,
			Values: []string{O.ID()},
		},
	}
	assert.NoError(t, Allows(vfsC, psetParentID, permissions.GET, f))

	psetSelfParentTag := permissions.Set{
		permissions.Rule{
			Type:     consts.Files,
			Verbs:    permissions.ALL,
			Selector: "tags",
			Values:   []string{"testtagparent"},
		},
	}
	assert.NoError(t, Allows(vfsC, psetSelfParentTag, permissions.GET, f))

	psetWrongType := permissions.Set{
		permissions.Rule{
			Type:   "io.cozy.not-files",
			Verbs:  permissions.ALL,
			Values: []string{A.ID()},
		},
	}
	assert.Error(t, Allows(vfsC, psetWrongType, permissions.GET, f))

	psetWrongVerb := permissions.Set{
		permissions.Rule{
			Type:   consts.Files,
			Verbs:  permissions.Verbs(permissions.POST),
			Values: []string{A.ID()},
		},
	}
	assert.Error(t, Allows(vfsC, psetWrongVerb, permissions.GET, f))

	psetUncleID := permissions.Set{
		permissions.Rule{
			Type:   consts.Files,
			Verbs:  permissions.ALL,
			Values: []string{B.ID()},
		},
	}
	assert.Error(t, Allows(vfsC, psetUncleID, permissions.GET, B2))

	psetUnclePrefixID := permissions.Set{
		permissions.Rule{
			Type:   consts.Files,
			Verbs:  permissions.ALL,
			Values: []string{A.ID()},
		},
	}
	assert.Error(t, Allows(vfsC, psetUnclePrefixID, permissions.GET, f))

}
