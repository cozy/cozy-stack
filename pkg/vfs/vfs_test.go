package vfs

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

type TestContext struct {
	prefix string
	fs     afero.Fs
}

func (c TestContext) Prefix() string { return c.prefix }
func (c TestContext) FS() afero.Fs   { return c.fs }

var vfsC TestContext

type H map[string]H

func (h H) String() string {
	return printH(h, "", 0)
}

func printH(h H, str string, count int) string {
	for name, hh := range h {
		for i := 0; i < count; i++ {
			str += "\t"
		}
		str += fmt.Sprintf("%s:\n", name)
		str += printH(hh, "", count+1)
	}
	return str
}

func createTree(tree H, dirID string) (*DirDoc, error) {
	if tree == nil {
		return nil, nil
	}

	if dirID == "" {
		dirID = consts.RootDirID
	}

	var err error
	var dirdoc *DirDoc
	for name, children := range tree {
		if name[len(name)-1] == '/' {
			dirdoc, err = NewDirDoc(name[:len(name)-1], dirID, nil)
			if err != nil {
				return nil, err
			}
			if err = CreateDir(vfsC, dirdoc); err != nil {
				return nil, err
			}
			if _, err = createTree(children, dirdoc.ID()); err != nil {
				return nil, err
			}
		} else {
			filedoc, err := NewFileDoc(name, dirID, -1, nil, "", "", time.Now(), false, nil)
			if err != nil {
				return nil, err
			}
			f, err := CreateFile(vfsC, filedoc, nil)
			if err != nil {
				return nil, err
			}
			if err = f.Close(); err != nil {
				return nil, err
			}
		}
	}
	return dirdoc, nil
}

func fetchTree(root string) (H, error) {
	parent, err := GetDirDocFromPath(vfsC, root)
	if err != nil {
		return nil, err
	}
	h, err := recFetchTree(parent, path.Clean(root))
	if err != nil {
		return nil, err
	}
	hh := make(H)
	hh[parent.Name+"/"] = h
	return hh, nil
}

func recFetchTree(parent *DirDoc, name string) (H, error) {
	h := make(H)
	iter := parent.ChildrenIterator(vfsC, nil)
	for {
		d, f, err := iter.Next()
		if err == ErrIteratorDone {
			break
		}
		if err != nil {
			return nil, err
		}
		if d != nil {
			if path.Join(name, d.Name) != d.Fullpath {
				return nil, fmt.Errorf("Bad fullpath: %s instead of %s", d.Fullpath, path.Join(name, d.Name))
			}
			children, err := recFetchTree(d, d.Fullpath)
			if err != nil {
				return nil, err
			}
			h[d.Name+"/"] = children
		} else {
			h[f.Name] = nil
		}
	}
	return h, nil
}

func TestDiskUsageIsInitiallyZero(t *testing.T) {
	used, err := DiskUsage(vfsC)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), used)
}

func TestGetFileDocFromPathAtRoot(t *testing.T) {
	doc, err := NewFileDoc("toto", "", -1, nil, "foo/bar", "foo", time.Now(), false, []string{})
	assert.NoError(t, err)

	body := bytes.NewReader([]byte("hello !"))

	file, err := CreateFile(vfsC, doc, nil)
	assert.NoError(t, err)

	n, err := io.Copy(file, body)
	assert.NoError(t, err)
	assert.Equal(t, len("hello !"), int(n))

	err = file.Close()
	assert.NoError(t, err)

	_, err = GetFileDocFromPath(vfsC, "/toto")
	assert.NoError(t, err)

	_, err = GetFileDocFromPath(vfsC, "/noooo")
	assert.Error(t, err)
}

func TestRemove(t *testing.T) {
	err := Remove(vfsC, "foo/bar")
	assert.Error(t, err)
	assert.Equal(t, ErrNonAbsolutePath, err)

	err = Remove(vfsC, "/foo")
	assert.Error(t, err)
	assert.Equal(t, "file does not exist", err.Error())

	_, err = Mkdir(vfsC, "/removeme", nil)
	if !assert.NoError(t, err) {
		err = Remove(vfsC, "/removeme")
		assert.NoError(t, err)
	}
}

func TestRemoveAll(t *testing.T) {
	origtree := H{
		"removemeall/": H{
			"dirchild1/": H{
				"food/": H{},
				"bard/": H{},
			},
			"dirchild2/": H{
				"foof": nil,
				"barf": nil,
			},
			"dirchild3/": H{},
			"filechild1": nil,
		},
	}
	_, err := createTree(origtree, consts.RootDirID)
	if !assert.NoError(t, err) {
		return
	}
	err = RemoveAll(vfsC, "/removemeall")
	if !assert.NoError(t, err) {
		return
	}
	_, err = Stat(vfsC, "/removemeall/dirchild1")
	assert.Error(t, err)
	_, err = Stat(vfsC, "/removemeall")
	assert.Error(t, err)
}

func TestDiskUsage(t *testing.T) {
	used, err := DiskUsage(vfsC)
	assert.NoError(t, err)
	assert.Equal(t, len("hello !"), int(used))
}

func TestGetFileDocFromPath(t *testing.T) {
	dir, _ := NewDirDoc("container", "", nil)
	err := CreateDir(vfsC, dir)
	assert.NoError(t, err)

	doc, err := NewFileDoc("toto", dir.ID(), -1, nil, "foo/bar", "foo", time.Now(), false, []string{})
	assert.NoError(t, err)

	body := bytes.NewReader([]byte("hello !"))

	file, err := CreateFile(vfsC, doc, nil)
	assert.NoError(t, err)

	n, err := io.Copy(file, body)
	assert.NoError(t, err)
	assert.Equal(t, len("hello !"), int(n))

	err = file.Close()
	assert.NoError(t, err)

	_, err = GetFileDocFromPath(vfsC, "/container/toto")
	assert.NoError(t, err)

	_, err = GetFileDocFromPath(vfsC, "/container/noooo")
	assert.Error(t, err)
}

func TestCreateGetAndModifyFile(t *testing.T) {
	origtree := H{
		"createandget1/": H{
			"dirchild1/": H{
				"food/": H{},
				"bard/": H{},
			},
			"dirchild2/": H{
				"foof": nil,
				"barf": nil,
			},
			"dirchild3/": H{},
			"filechild1": nil,
		},
	}

	olddoc, err := createTree(origtree, consts.RootDirID)

	if !assert.NoError(t, err) {
		return
	}

	newname := "createandget2"
	_, err = ModifyDirMetadata(vfsC, olddoc, &DocPatch{
		Name: &newname,
	})
	if !assert.NoError(t, err) {
		return
	}

	tree, err := fetchTree("/createandget2")
	if !assert.NoError(t, err) {
		return
	}

	assert.EqualValues(t, origtree["createandget1/"], tree["createandget2/"], "should have same tree")

	fileBefore, err := GetFileDocFromPath(vfsC, "/createandget2/dirchild2/foof")
	if !assert.NoError(t, err) {
		return
	}
	newfilename := "foof.jpg"
	_, err = ModifyFileMetadata(vfsC, fileBefore, &DocPatch{
		Name: &newfilename,
	})
	if !assert.NoError(t, err) {
		return
	}
	fileAfter, err := GetFileDocFromPath(vfsC, "/createandget2/dirchild2/foof.jpg")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "", fileBefore.Class)
	assert.Equal(t, "", fileBefore.Mime)
	assert.Equal(t, "image", fileAfter.Class)
	assert.Equal(t, "image/jpeg", fileAfter.Mime)
}

func TestUpdateDir(t *testing.T) {
	origtree := H{
		"update1/": H{
			"dirchild1/": H{
				"food/": H{},
				"bard/": H{},
			},
			"dirchild2/": H{
				"foof": nil,
				"barf": nil,
			},
			"dirchild3/": H{},
			"filechild1": nil,
		},
	}

	doc1, err := createTree(origtree, consts.RootDirID)
	if !assert.NoError(t, err) {
		return
	}

	newname := "update2"
	_, err = ModifyDirMetadata(vfsC, doc1, &DocPatch{
		Name: &newname,
	})
	if !assert.NoError(t, err) {
		return
	}

	tree, err := fetchTree("/update2")
	if !assert.NoError(t, err) {
		return
	}

	if !assert.EqualValues(t, origtree["update1/"], tree["update2/"], "should have same tree") {
		return
	}

	dirchild2, err := GetDirDocFromPath(vfsC, "/update2/dirchild2")
	if !assert.NoError(t, err) {
		return
	}

	dirchild3, err := GetDirDocFromPath(vfsC, "/update2/dirchild3")
	if !assert.NoError(t, err) {
		return
	}

	newfolid := dirchild2.ID()
	_, err = ModifyDirMetadata(vfsC, dirchild3, &DocPatch{
		DirID: &newfolid,
	})
	if !assert.NoError(t, err) {
		return
	}

	tree, err = fetchTree("/update2")
	if !assert.NoError(t, err) {
		return
	}

	assert.EqualValues(t, H{
		"update2/": H{
			"dirchild1/": H{
				"bard/": H{},
				"food/": H{},
			},
			"filechild1": nil,
			"dirchild2/": H{
				"barf":       nil,
				"foof":       nil,
				"dirchild3/": H{},
			},
		},
	}, tree)
}

func TestWalk(t *testing.T) {
	walktree := H{
		"walk/": H{
			"dirchild1/": H{
				"food/": H{},
				"bard/": H{},
			},
			"dirchild2/": H{
				"foof": nil,
				"barf": nil,
			},
			"dirchild3/": H{},
			"filechild1": nil,
		},
	}

	_, err := createTree(walktree, consts.RootDirID)
	if !assert.NoError(t, err) {
		return
	}

	walked := ""
	Walk(vfsC, "/walk", func(name string, dir *DirDoc, file *FileDoc, err error) error {
		if !assert.NoError(t, err) {
			return err
		}

		if dir != nil && !assert.Equal(t, dir.Fullpath, name) {
			return fmt.Errorf("Bad fullpath")
		}

		if file != nil && !assert.True(t, strings.HasSuffix(name, file.Name)) {
			return fmt.Errorf("Bad fullpath")
		}

		walked += name + "\n"

		return nil
	})

	expectedWalk := `/walk
/walk/dirchild1
/walk/dirchild1/bard
/walk/dirchild1/food
/walk/dirchild2
/walk/dirchild2/barf
/walk/dirchild2/foof
/walk/dirchild3
/walk/filechild1
`

	assert.Equal(t, expectedWalk, walked)
}

func TestContentDisposition(t *testing.T) {
	foo := ContentDisposition("inline", "foo.jpg")
	assert.Equal(t, `inline; filename=foo.jpg`, foo)
	space := ContentDisposition("inline", "foo bar.jpg")
	assert.Equal(t, `inline; filename="foobar.jpg"; filename*=UTF-8''foo%20bar.jpg`, space)
	accents := ContentDisposition("inline", "héçà")
	assert.Equal(t, `inline; filename="h"; filename*=UTF-8''h%C3%A9%C3%A7%C3%A0`, accents)
	tab := ContentDisposition("inline", "tab\t")
	assert.Equal(t, `inline; filename="tab"; filename*=UTF-8''tab%09`, tab)
	emoji := ContentDisposition("inline", "🐧")
	assert.Equal(t, `inline; filename="download"; filename*=UTF-8''%F0%9F%90%A7`, emoji)
}

func TestArchive(t *testing.T) {
	tree := H{
		"archive/": H{
			"foo.jpg":   nil,
			"hello.jpg": nil,
			"bar/": H{
				"baz/": H{
					"one.png": nil,
					"two.png": nil,
				},
				"z.gif": nil,
			},
			"qux/": H{
				"quux":   nil,
				"courge": nil,
			},
		},
	}
	_, err := createTree(tree, consts.RootDirID)
	assert.NoError(t, err)

	a := &Archive{
		Name: "test",
		Files: []string{
			"/archive/foo.jpg",
			"/archive/bar",
		},
	}
	w := httptest.NewRecorder()
	err = a.Serve(vfsC, w)
	assert.NoError(t, err)

	res := w.Result()
	disposition := res.Header.Get("Content-Disposition")
	assert.Equal(t, `attachment; filename=test.zip`, disposition)
	assert.Equal(t, "application/zip", res.Header.Get("Content-Type"))

	b, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	assert.NoError(t, err)
	assert.Equal(t, 4, len(z.File))
	assert.Equal(t, "test/foo.jpg", z.File[0].Name)
	assert.Equal(t, "test/bar/baz/one.png", z.File[1].Name)
	assert.Equal(t, "test/bar/baz/two.png", z.File[2].Name)
	assert.Equal(t, "test/bar/z.gif", z.File[3].Name)
}

func TestDonwloadStore(t *testing.T) {
	domainA := "alice.cozycloud.local"
	domainB := "bob.cozycloud.local"
	storeA := GetStore(domainA)
	storeB := GetStore(domainB)

	path := "/test/random/path.txt"
	key1, err := storeA.AddFile(path)
	assert.NoError(t, err)

	path2, err := storeB.GetFile(key1)
	assert.NoError(t, err)
	assert.Zero(t, path2, "Inter-instances store leaking")

	path3, err := storeA.GetFile(key1)
	assert.NoError(t, err)
	assert.Equal(t, path, path3)

	storeStore[domainA].Files[key1].ExpiresAt = time.Now().Add(-2 * downloadStoreTTL)

	path4, err := storeA.GetFile(key1)
	assert.NoError(t, err)
	assert.Zero(t, path4, "no expiration")

	a := &Archive{
		Name: "test",
		Files: []string{
			"/archive/foo.jpg",
			"/archive/bar",
		},
	}
	key2, err := storeA.AddArchive(a)
	assert.NoError(t, err)

	a2, err := storeA.GetArchive(key2)
	assert.NoError(t, err)
	assert.Equal(t, a, a2)

	storeStore[domainA].Archives[key2].ExpiresAt = time.Now().Add(-2 * downloadStoreTTL)

	a3, err := storeA.GetArchive(key2)
	assert.NoError(t, err)
	assert.Nil(t, a3, "no expiration")
}

func TestFileSerialization(t *testing.T) {
	var f = &FileDoc{
		Type: consts.FileType,
		Name: "test/foo/bar.jpg",
		ReferencedBy: []jsonapi.ResourceIdentifier{
			{Type: "io.cozy.photo.album", ID: "foorefid"},
		},
	}

	f2 := f.HideFields()

	b, err := json.Marshal(f)
	assert.NoError(t, err)
	assert.Contains(t, string(b), "referenced_by")

	b2, err := json.Marshal(f2)
	assert.NoError(t, err)
	assert.NotContains(t, string(b2), "referenced_by")

	b3, err := jsonapi.MarshalObject(f2)
	assert.NoError(t, err)
	assert.Contains(t, string(b3), "foorefid")
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	db, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		fmt.Println("Could not create temporary directory.")
		os.Exit(1)
	}

	vfsC.prefix = "dev/"
	vfsC.fs = afero.NewBasePathFs(afero.NewOsFs(), tempdir)

	err = couchdb.ResetDB(vfsC, consts.Files)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.DefineIndexes(vfsC, consts.IndexesByDoctype(consts.Files))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err = couchdb.DefineViews(vfsC, consts.ViewsByDoctype(consts.Files)); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	CreateRootDirDoc(vfsC)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	res := m.Run()

	os.RemoveAll(tempdir)
	couchdb.DeleteDB(vfsC, consts.Files)

	os.Exit(res)
}
