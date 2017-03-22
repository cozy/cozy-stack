package vfs

import (
	"archive/zip"
	"bytes"
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
	"github.com/stretchr/testify/assert"
)

var fs VFS

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
			if err = fs.CreateDir(dirdoc); err != nil {
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
			f, err := fs.CreateFile(filedoc, nil)
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
	parent, err := fs.DirByPath(root)
	if err != nil {
		return nil, err
	}
	h, err := recFetchTree(parent, path.Clean(root))
	if err != nil {
		return nil, err
	}
	hh := make(H)
	hh[parent.DocName+"/"] = h
	return hh, nil
}

func recFetchTree(parent *DirDoc, name string) (H, error) {
	h := make(H)
	iter := fs.DirIterator(parent, nil)
	for {
		d, f, err := iter.Next()
		if err == ErrIteratorDone {
			break
		}
		if err != nil {
			return nil, err
		}
		if d != nil {
			if path.Join(name, d.DocName) != d.Fullpath {
				return nil, fmt.Errorf("Bad fullpath: %s instead of %s", d.Fullpath, path.Join(name, d.DocName))
			}
			children, err := recFetchTree(d, d.Fullpath)
			if err != nil {
				return nil, err
			}
			h[d.DocName+"/"] = children
		} else {
			h[f.DocName] = nil
		}
	}
	return h, nil
}

func TestDiskUsageIsInitiallyZero(t *testing.T) {
	used, err := fs.DiskUsage()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), used)
}

func TestGetFileDocFromPathAtRoot(t *testing.T) {
	doc, err := NewFileDoc("toto", "", -1, nil, "foo/bar", "foo", time.Now(), false, []string{})
	assert.NoError(t, err)

	body := bytes.NewReader([]byte("hello !"))

	file, err := fs.CreateFile(doc, nil)
	assert.NoError(t, err)

	n, err := io.Copy(file, body)
	assert.NoError(t, err)
	assert.Equal(t, len("hello !"), int(n))

	err = file.Close()
	assert.NoError(t, err)

	_, err = fs.FileByPath("/toto")
	assert.NoError(t, err)

	_, err = fs.FileByPath("/noooo")
	assert.Error(t, err)
}

func TestRemove(t *testing.T) {
	err := Remove(fs, "foo/bar")
	assert.Error(t, err)
	assert.Equal(t, ErrNonAbsolutePath, err)

	err = Remove(fs, "/foo")
	assert.Error(t, err)
	assert.Equal(t, "file does not exist", err.Error())

	_, err = Mkdir(fs, "/removeme", nil)
	if !assert.NoError(t, err) {
		err = Remove(fs, "/removeme")
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
	err = RemoveAll(fs, "/removemeall")
	if !assert.NoError(t, err) {
		return
	}
	_, err = fs.DirByPath("/removemeall/dirchild1")
	assert.Error(t, err)
	_, err = fs.DirByPath("/removemeall")
	assert.Error(t, err)
}

func TestDiskUsage(t *testing.T) {
	used, err := fs.DiskUsage()
	assert.NoError(t, err)
	assert.Equal(t, len("hello !"), int(used))
}

func TestGetFileDocFromPath(t *testing.T) {
	dir, _ := NewDirDoc("container", "", nil)
	err := fs.CreateDir(dir)
	assert.NoError(t, err)

	doc, err := NewFileDoc("toto", dir.ID(), -1, nil, "foo/bar", "foo", time.Now(), false, []string{})
	assert.NoError(t, err)

	body := bytes.NewReader([]byte("hello !"))

	file, err := fs.CreateFile(doc, nil)
	assert.NoError(t, err)

	n, err := io.Copy(file, body)
	assert.NoError(t, err)
	assert.Equal(t, len("hello !"), int(n))

	err = file.Close()
	assert.NoError(t, err)

	_, err = fs.FileByPath("/container/toto")
	assert.NoError(t, err)

	_, err = fs.FileByPath("/container/noooo")
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
	_, err = ModifyDirMetadata(fs, olddoc, &DocPatch{
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

	fileBefore, err := fs.FileByPath("/createandget2/dirchild2/foof")
	if !assert.NoError(t, err) {
		return
	}
	newfilename := "foof.jpg"
	_, err = ModifyFileMetadata(fs, fileBefore, &DocPatch{
		Name: &newfilename,
	})
	if !assert.NoError(t, err) {
		return
	}
	fileAfter, err := fs.FileByPath("/createandget2/dirchild2/foof.jpg")
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
	_, err = ModifyDirMetadata(fs, doc1, &DocPatch{
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

	dirchild2, err := fs.DirByPath("/update2/dirchild2")
	if !assert.NoError(t, err) {
		return
	}

	dirchild3, err := fs.DirByPath("/update2/dirchild3")
	if !assert.NoError(t, err) {
		return
	}

	newfolid := dirchild2.ID()
	_, err = ModifyDirMetadata(fs, dirchild3, &DocPatch{
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

	walked := H{}
	Walk(fs, "/walk", func(name string, dir *DirDoc, file *FileDoc, err error) error {
		if !assert.NoError(t, err) {
			return err
		}

		if dir != nil && !assert.Equal(t, dir.Fullpath, name) {
			return fmt.Errorf("Bad fullpath")
		}

		if file != nil && !assert.True(t, strings.HasSuffix(name, file.DocName)) {
			return fmt.Errorf("Bad fullpath")
		}

		walked[name] = nil
		return nil
	})

	expectedWalk := H{
		"/walk":                nil,
		"/walk/dirchild1":      nil,
		"/walk/dirchild1/food": nil,
		"/walk/dirchild1/bard": nil,
		"/walk/dirchild2":      nil,
		"/walk/dirchild2/foof": nil,
		"/walk/dirchild2/barf": nil,
		"/walk/dirchild3":      nil,
		"/walk/filechild1":     nil,
	}

	assert.Equal(t, expectedWalk, walked)
}

func TestIterator(t *testing.T) {
	iterTree := H{
		"iter/": H{
			"dirchild1/":  H{},
			"dirchild2/":  H{},
			"dirchild3/":  H{},
			"filechild1":  nil,
			"filechild2":  nil,
			"filechild3":  nil,
			"filechild4":  nil,
			"filechild5":  nil,
			"filechild6":  nil,
			"filechild7":  nil,
			"filechild8":  nil,
			"filechild9":  nil,
			"filechild10": nil,
			"filechild11": nil,
			"filechild12": nil,
			"filechild13": nil,
			"filechild14": nil,
			"filechild15": nil,
		},
	}

	iterDir, err := createTree(iterTree, consts.RootDirID)
	if !assert.NoError(t, err) {
		return
	}

	iter1 := fs.DirIterator(iterDir, &IteratorOptions{ByFetch: 4})
	iterTree2 := H{}
	var children1 []string
	var nextKey string
	for {
		d, f, err := iter1.Next()
		if err == ErrIteratorDone {
			break
		}
		if !assert.NoError(t, err) {
			return
		}
		if nextKey != "" {
			if d != nil {
				children1 = append(children1, d.DocName)
			} else {
				children1 = append(children1, f.DocName)
			}
		}
		if d != nil {
			iterTree2[d.DocName+"/"] = H{}
		} else {
			iterTree2[f.DocName] = nil
			if f.DocName == "filechild4" {
				nextKey = f.ID()
			}
		}
	}
	assert.EqualValues(t, iterTree["iter/"], iterTree2)

	iter2 := fs.DirIterator(iterDir, &IteratorOptions{
		ByFetch: 4,
		AfterID: nextKey,
	})
	var children2 []string
	for {
		d, f, err := iter2.Next()
		if err == ErrIteratorDone {
			break
		}
		if !assert.NoError(t, err) {
			return
		}
		if d != nil {
			children2 = append(children2, d.DocName)
		} else {
			children2 = append(children2, f.DocName)
		}
	}

	assert.EqualValues(t, children1, children2)
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
	err = a.Serve(fs, w)
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
	zipfiles := H{}
	for _, f := range z.File {
		zipfiles[f.Name] = nil
	}
	assert.EqualValues(t, H{
		"test/foo.jpg":         nil,
		"test/bar/baz/one.png": nil,
		"test/bar/baz/two.png": nil,
		"test/bar/z.gif":       nil,
	}, zipfiles)
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

func TestMain(m *testing.M) {
	config.UseTestFile()

	check, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || check.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		fmt.Println("Could not create temporary directory.")
		os.Exit(1)
	}

	db := couchdb.SimpleDatabasePrefix("io.cozy.vfs.test")
	fs, err = NewAferoVFS(db, "file://localhost"+tempdir)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.ResetDB(db, consts.Files)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.DefineIndexes(db, consts.IndexesByDoctype(consts.Files))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err = couchdb.DefineViews(db, consts.ViewsByDoctype(consts.Files)); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = fs.Init()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	res := m.Run()

	os.RemoveAll(tempdir)
	couchdb.DeleteDB(db, consts.Files)

	os.Exit(res)
}
