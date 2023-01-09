package vfs_test

import (
	"testing"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermissions(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()
	testutils.NeedCouchdb(t)

	aferoFS := makeAferoFS(t)
	swiftFS := makeSwiftFS(t, 2)

	var tests = []struct {
		name string
		fs   vfs.VFS
	}{
		{"afero", aferoFS},
		{"swift", swiftFS},
	}

	for _, tt := range tests {
		fs := tt.fs
		t.Run("Permissions", func(t *testing.T) {
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
			O, err := createTree(fs, origtree, consts.RootDirID)
			require.NoError(t, err)

			A, err := fs.DirByPath("/O/A")
			require.NoError(t, err)

			B, err := fs.DirByPath("/O/B")
			require.NoError(t, err)

			_, err = vfs.ModifyDirMetadata(fs, B, &vfs.DocPatch{
				Tags: &[]string{"testtagparent"},
			})
			assert.NoError(t, err)

			B2, err := fs.DirByPath("/O/B2")
			require.NoError(t, err)

			f, err := fs.FileByPath("/O/B/b1.txt")
			require.NoError(t, err)

			_, err = vfs.ModifyFileMetadata(fs, f, &vfs.DocPatch{
				Tags: &[]string{"testtag"},
			})
			assert.NoError(t, err)
			// reload
			f, err = fs.FileByPath("/O/B/b1.txt")
			require.NoError(t, err)

			// hack to have a Class attribute
			f.Class = "superfile"

			psetWholeType := permission.Set{
				permission.Rule{
					Type:  consts.Files,
					Verbs: permission.ALL,
				},
			}
			assert.NoError(t, vfs.Allows(fs, psetWholeType, permission.GET, f))

			psetSelfID := permission.Set{
				permission.Rule{
					Type:   consts.Files,
					Verbs:  permission.ALL,
					Values: []string{f.ID()},
				},
			}
			assert.NoError(t, vfs.Allows(fs, psetSelfID, permission.GET, f))

			psetSelfAttributes := permission.Set{
				permission.Rule{
					Type:     consts.Files,
					Verbs:    permission.ALL,
					Selector: "class",
					Values:   []string{"superfile"},
				},
			}
			assert.NoError(t, vfs.Allows(fs, psetSelfAttributes, permission.GET, f))

			psetOnlyFiles := permission.Set{
				permission.Rule{
					Type:     consts.Files,
					Verbs:    permission.ALL,
					Selector: "type",
					Values:   []string{"file"},
				},
			}
			assert.NoError(t, vfs.Allows(fs, psetOnlyFiles, permission.GET, f))

			psetOnlyDirs := permission.Set{
				permission.Rule{
					Type:     consts.Files,
					Verbs:    permission.ALL,
					Selector: "type",
					Values:   []string{"directory"},
				},
			}
			assert.NoError(t, vfs.Allows(fs, psetOnlyDirs, permission.GET, B))

			psetMime := permission.Set{
				permission.Rule{
					Type:     consts.Files,
					Verbs:    permission.ALL,
					Selector: "mime",
					Values:   []string{"text/plain"},
				},
			}
			f.Mime = "text/plain"
			assert.NoError(t, vfs.Allows(fs, psetMime, permission.GET, f))

			psetReferences := permission.Set{
				permission.Rule{
					Type:     consts.Files,
					Verbs:    permission.ALL,
					Selector: "referenced_by",
					Values:   []string{"somealbumid"},
				},
			}
			f.ReferencedBy = []couchdb.DocReference{{Type: "io.cozy.albums", ID: "somealbumid"}}
			assert.NoError(t, vfs.Allows(fs, psetReferences, permission.GET, f))

			psetBadReferences := permission.Set{
				permission.Rule{
					Type:     consts.Files,
					Verbs:    permission.ALL,
					Selector: "referenced_by",
					Values:   []string{"anotheralbumid"},
				},
			}
			assert.Error(t, vfs.Allows(fs, psetBadReferences, permission.GET, f))

			psetName := permission.Set{
				permission.Rule{
					Type:     consts.Files,
					Verbs:    permission.ALL,
					Selector: "name",
					Values:   []string{"b1.txt"},
				},
			}
			assert.NoError(t, vfs.Allows(fs, psetName, permission.GET, f))

			psetSelfTag := permission.Set{
				permission.Rule{
					Type:     consts.Files,
					Verbs:    permission.ALL,
					Selector: "tags",
					Values:   []string{"testtag"},
				},
			}
			assert.NoError(t, vfs.Allows(fs, psetSelfTag, permission.GET, f))

			psetParentID := permission.Set{
				permission.Rule{
					Type:   consts.Files,
					Verbs:  permission.ALL,
					Values: []string{O.ID()},
				},
			}
			assert.NoError(t, vfs.Allows(fs, psetParentID, permission.GET, f))

			psetSelfParentTag := permission.Set{
				permission.Rule{
					Type:     consts.Files,
					Verbs:    permission.ALL,
					Selector: "tags",
					Values:   []string{"testtagparent"},
				},
			}
			assert.NoError(t, vfs.Allows(fs, psetSelfParentTag, permission.GET, f))

			psetWrongType := permission.Set{
				permission.Rule{
					Type:   "io.cozy.not.files",
					Verbs:  permission.ALL,
					Values: []string{A.ID()},
				},
			}
			assert.Error(t, vfs.Allows(fs, psetWrongType, permission.GET, f))

			psetWrongVerb := permission.Set{
				permission.Rule{
					Type:   consts.Files,
					Verbs:  permission.Verbs(permission.POST),
					Values: []string{A.ID()},
				},
			}
			assert.Error(t, vfs.Allows(fs, psetWrongVerb, permission.GET, f))

			psetUncleID := permission.Set{
				permission.Rule{
					Type:   consts.Files,
					Verbs:  permission.ALL,
					Values: []string{B.ID()},
				},
			}
			assert.Error(t, vfs.Allows(fs, psetUncleID, permission.GET, B2))

			psetUnclePrefixID := permission.Set{
				permission.Rule{
					Type:   consts.Files,
					Verbs:  permission.ALL,
					Values: []string{A.ID()},
				},
			}
			assert.Error(t, vfs.Allows(fs, psetUnclePrefixID, permission.GET, f))
		})
	}
}
