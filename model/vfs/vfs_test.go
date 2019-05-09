package vfs_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/model/vfs/vfsafero"
	"github.com/cozy/cozy-stack/model/vfs/vfsswift"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/ncw/swift/swifttest"
	"github.com/stretchr/testify/assert"
)

var fs vfs.VFS
var diskQuota int64

type diskImpl struct{}

func (d *diskImpl) DiskQuota() int64 {
	return diskQuota
}

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

func createTree(tree H, dirID string) (*vfs.DirDoc, error) {
	if tree == nil {
		return nil, nil
	}

	if dirID == "" {
		dirID = consts.RootDirID
	}

	var err error
	var dirdoc *vfs.DirDoc
	for name, children := range tree {
		if name[len(name)-1] == '/' {
			dirdoc, err = vfs.NewDirDoc(fs, name[:len(name)-1], dirID, nil)
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
			filedoc, err := vfs.NewFileDoc(name, dirID, -1, nil, "", "", time.Now(), false, false, nil)
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

func recFetchTree(parent *vfs.DirDoc, name string) (H, error) {
	h := make(H)
	iter := fs.DirIterator(parent, nil)
	for {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
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
	doc, err := vfs.NewFileDoc("toto", "", -1, nil, "foo/bar", "foo", time.Now(), false, false, []string{})
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
	err := vfs.Remove(fs, "foo/bar")
	assert.Error(t, err)
	assert.Equal(t, vfs.ErrNonAbsolutePath, err)

	err = vfs.Remove(fs, "/foo")
	assert.Error(t, err)
	assert.Equal(t, "file does not exist", err.Error())

	_, err = vfs.Mkdir(fs, "/removeme", nil)
	if !assert.NoError(t, err) {
		err = vfs.Remove(fs, "/removeme")
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
	err = vfs.RemoveAll(fs, "/removemeall")
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
	dir, _ := vfs.NewDirDoc(fs, "container", "", nil)
	err := fs.CreateDir(dir)
	assert.NoError(t, err)

	doc, err := vfs.NewFileDoc("toto", dir.ID(), -1, nil, "foo/bar", "foo", time.Now(), false, false, []string{})
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
	_, err = vfs.ModifyDirMetadata(fs, olddoc, &vfs.DocPatch{
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
	_, err = vfs.ModifyFileMetadata(fs, fileBefore, &vfs.DocPatch{
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
	_, err = vfs.ModifyDirMetadata(fs, doc1, &vfs.DocPatch{
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
	_, err = vfs.ModifyDirMetadata(fs, dirchild3, &vfs.DocPatch{
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
	err = vfs.Walk(fs, "/walk", func(name string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
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
	assert.NoError(t, err)

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

	iter1 := fs.DirIterator(iterDir, &vfs.IteratorOptions{ByFetch: 4})
	iterTree2 := H{}
	var children1 []string
	var nextKey string
	for {
		d, f, err := iter1.Next()
		if err == vfs.ErrIteratorDone {
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

	iter2 := fs.DirIterator(iterDir, &vfs.IteratorOptions{
		ByFetch: 4,
		AfterID: nextKey,
	})
	var children2 []string
	for {
		d, f, err := iter2.Next()
		if err == vfs.ErrIteratorDone {
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

func TestFileCollision(t *testing.T) {
	fileDoc1, err := vfs.NewFileDoc("collision", consts.RootDirID, 10, nil, "text", "text/plain", time.Now(), false, false, nil)
	if !assert.NoError(t, err) {
		return
	}
	file1, err := fs.CreateFile(fileDoc1, nil)
	if !assert.NoError(t, err) {
		return
	}

	_, err = file1.Write(crypto.GenerateRandomBytes(10))
	if !assert.NoError(t, err) {
		return
	}

	fileDoc2, err := vfs.NewFileDoc("collision", consts.RootDirID, 10, nil, "text", "text/plain", time.Now(), false, false, nil)
	if !assert.NoError(t, err) {
		return
	}
	file2, err := fs.CreateFile(fileDoc2, nil)
	assert.Error(t, err)
	assert.True(t, os.IsExist(err))
	assert.Nil(t, file2)

	fileDoc3, err := vfs.NewFileDoc("to-be-collision", consts.RootDirID, 10, nil, "text", "text/plain", time.Now(), false, false, nil)
	if !assert.NoError(t, err) {
		return
	}
	file3, err := fs.CreateFile(fileDoc3, nil)
	if !assert.NoError(t, err) {
		return
	}
	_, err = file3.Write(crypto.GenerateRandomBytes(10))
	if !assert.NoError(t, err) {
		return
	}
	if !assert.NoError(t, file3.Close()) {
		return
	}

	fileDoc4, err := vfs.NewFileDoc("collision", consts.RootDirID, 10, nil, "text", "text/plain", time.Now(), false, false, nil)
	if !assert.NoError(t, err) {
		return
	}

	file4, err := fs.CreateFile(fileDoc4, fileDoc3)
	if !assert.NoError(t, err) {
		return
	}

	_, err = file4.Write(crypto.GenerateRandomBytes(10))
	if !assert.NoError(t, err) {
		return
	}

	err = file1.Close()
	if !assert.NoError(t, err) {
		return
	}

	err = file4.Close()
	if !assert.NoError(t, err) {
		return
	}
}

func TestContentDisposition(t *testing.T) {
	foo := vfs.ContentDisposition("inline", "foo.jpg")
	assert.Equal(t, `inline; filename="foo.jpg"`, foo)
	space := vfs.ContentDisposition("inline", "foo bar.jpg")
	assert.Equal(t, `inline; filename="foobar.jpg"; filename*=UTF-8''foo%20bar.jpg`, space)
	accents := vfs.ContentDisposition("inline", "héçà")
	assert.Equal(t, `inline; filename="h"; filename*=UTF-8''h%C3%A9%C3%A7%C3%A0`, accents)
	tab := vfs.ContentDisposition("inline", "tab\t")
	assert.Equal(t, `inline; filename="tab"; filename*=UTF-8''tab%09`, tab)
	emoji := vfs.ContentDisposition("inline", "🐧")
	assert.Equal(t, `inline; filename="download"; filename*=UTF-8''%F0%9F%90%A7`, emoji)
}

func TestArchive(t *testing.T) {
	tree := H{
		"archive/": H{
			"foo.jpg":    nil,
			"foobar.jpg": nil,
			"hello.jpg":  nil,
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

	foobar, err := fs.FileByPath("/archive/foobar.jpg")
	assert.NoError(t, err)

	a := &vfs.Archive{
		Name: "test",
		IDs: []string{
			foobar.ID(),
		},
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
	assert.Equal(t, `attachment; filename="test.zip"`, disposition)
	assert.Equal(t, "application/zip", res.Header.Get("Content-Type"))

	b, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	assert.NoError(t, err)
	assert.Equal(t, 5, len(z.File))
	zipfiles := H{}
	for _, f := range z.File {
		zipfiles[f.Name] = nil
	}
	assert.EqualValues(t, H{
		"test/foobar.jpg":      nil,
		"test/foo.jpg":         nil,
		"test/bar/baz/one.png": nil,
		"test/bar/baz/two.png": nil,
		"test/bar/z.gif":       nil,
	}, zipfiles)
}

func TestCreateFileTooBig(t *testing.T) {
	diskQuota = 1 << (1 * 10) // 1KB
	defer func() { diskQuota = 0 }()

	diskUsage1, err := fs.DiskUsage()
	if !assert.NoError(t, err) {
		return
	}

	doc1, err := vfs.NewFileDoc(
		"too-big",
		consts.RootDirID,
		diskQuota+1,
		nil,
		"",
		"",
		time.Now(),
		false,
		false,
		nil,
	)
	if !assert.NoError(t, err) {
		return
	}
	_, err = fs.CreateFile(doc1, nil)
	assert.Equal(t, vfs.ErrFileTooBig, err)

	doc2, err := vfs.NewFileDoc(
		"too-big",
		consts.RootDirID,
		diskQuota/2,
		nil,
		"",
		"",
		time.Now(),
		false,
		false,
		nil,
	)
	if !assert.NoError(t, err) {
		return
	}
	f, err := fs.CreateFile(doc2, nil)
	assert.NoError(t, err)
	assert.Error(t, f.Close())

	_, err = fs.FileByPath("/too-big")
	assert.True(t, os.IsNotExist(err))

	doc3, err := vfs.NewFileDoc(
		"too-big",
		consts.RootDirID,
		diskQuota/2,
		nil,
		"",
		"",
		time.Now(),
		false,
		false,
		nil,
	)
	if !assert.NoError(t, err) {
		return
	}
	f, err = fs.CreateFile(doc3, nil)
	assert.NoError(t, err)
	_, err = io.Copy(f, bytes.NewReader(crypto.GenerateRandomBytes(int(doc3.ByteSize))))
	assert.NoError(t, err)
	err = f.Close()
	assert.NoError(t, err)

	diskUsage2, err := fs.DiskUsage()
	assert.NoError(t, err)
	assert.Equal(t, diskUsage1+diskQuota/2, diskUsage2)

	doc4, err := vfs.NewFileDoc(
		"too-big2",
		consts.RootDirID,
		-1,
		nil,
		"",
		"",
		time.Now(),
		false,
		false,
		nil,
	)
	if !assert.NoError(t, err) {
		return
	}
	f, err = fs.CreateFile(doc4, nil)
	assert.NoError(t, err)
	_, err = io.Copy(f, bytes.NewReader(crypto.GenerateRandomBytes(int(diskQuota/2+1))))
	assert.Error(t, err)
	assert.Equal(t, vfs.ErrFileTooBig, err)
	err = f.Close()
	assert.Error(t, err)
	assert.Equal(t, vfs.ErrFileTooBig, err)

	_, err = fs.FileByPath("/too-big2")
	assert.True(t, os.IsNotExist(err))

	root, err := fs.DirByPath("/")
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, fs.DestroyDirContent(root))
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	check, err := checkup.HTTPChecker{URL: config.CouchURL().String()}.Check()
	if err != nil || check.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	var rollback func()
	fs, rollback, err = makeAferoFS()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	res1 := m.Run()
	rollback()

	fs, rollback, err = makeSwiftFS(true)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	res2 := m.Run()
	rollback()

	fs, rollback, err = makeSwiftFS(false)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	res3 := m.Run()
	rollback()

	os.Exit(res1 + res2 + res3)
}

func makeAferoFS() (vfs.VFS, func(), error) {
	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		return nil, nil, errors.New("could not create temporary directory")
	}

	db := prefixer.NewPrefixer("io.cozy.vfs.test", "io.cozy.vfs.test")
	index := vfs.NewCouchdbIndexer(db)
	aferoFs, err := vfsafero.New(db, index, &diskImpl{}, lock.ReadWrite(db, "vfs-afero-test"),
		&url.URL{Scheme: "file", Host: "localhost", Path: tempdir}, "io.cozy.vfs.test")
	if err != nil {
		return nil, nil, err
	}

	err = couchdb.ResetDB(db, consts.Files)
	if err != nil {
		return nil, nil, err
	}

	err = couchdb.DefineIndexes(db, couchdb.IndexesByDoctype(consts.Files))
	if err != nil {
		return nil, nil, err
	}

	if err = couchdb.DefineViews(db, couchdb.ViewsByDoctype(consts.Files)); err != nil {
		return nil, nil, err
	}

	err = aferoFs.InitFs()
	if err != nil {
		return nil, nil, err
	}

	return aferoFs, func() {
		_ = os.RemoveAll(tempdir)
		_ = couchdb.DeleteDB(db, consts.Files)
	}, nil
}

func makeSwiftFS(layoutV2 bool) (vfs.VFS, func(), error) {
	db := prefixer.NewPrefixer("io.cozy.vfs.test", "io.cozy.vfs.test")
	index := vfs.NewCouchdbIndexer(db)
	swiftSrv, err := swifttest.NewSwiftServer("localhost")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create swift server %s", err)
	}

	err = config.InitSwiftConnection(config.Fs{
		URL: &url.URL{
			Scheme:   "swift",
			Host:     "localhost",
			RawQuery: "UserName=swifttest&Password=swifttest&AuthURL=" + url.QueryEscape(swiftSrv.AuthURL),
		},
	})
	if err != nil {
		return nil, nil, err
	}

	var swiftFs vfs.VFS
	if layoutV2 {
		swiftFs, err = vfsswift.NewV2(db,
			index, &diskImpl{}, lock.ReadWrite(db, "vfs-swiftv2-test"))
	} else {
		swiftFs, err = vfsswift.New(db,
			index, &diskImpl{}, lock.ReadWrite(db, "vfs-swift-test"))
	}
	if err != nil {
		return nil, nil, err
	}

	err = couchdb.ResetDB(db, consts.Files)
	if err != nil {
		return nil, nil, err
	}

	err = couchdb.DefineIndexes(db, couchdb.IndexesByDoctype(consts.Files))
	if err != nil {
		return nil, nil, err
	}

	if err = couchdb.DefineViews(db, couchdb.ViewsByDoctype(consts.Files)); err != nil {
		return nil, nil, err
	}

	err = swiftFs.InitFs()
	if err != nil {
		return nil, nil, err
	}

	return swiftFs, func() {
		_ = couchdb.DeleteDB(db, consts.Files)
		if swiftSrv != nil {
			swiftSrv.Close()
		}
	}, nil
}
