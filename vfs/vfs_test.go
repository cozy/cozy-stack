package vfs

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/sourcegraph/checkup"
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

func createTree(tree H, folderID string) (*DirDoc, error) {
	if tree == nil {
		return nil, nil
	}

	if folderID == "" {
		folderID = RootFolderID
	}

	var err error
	var dirdoc *DirDoc
	for name, children := range tree {
		if name[len(name)-1] == '/' {
			dirdoc, err = NewDirDoc(name[:len(name)-1], folderID, nil, nil)
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
			filedoc, err := NewFileDoc(name, folderID, -1, nil, "", "", false, nil)
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
	parent, err := GetDirDocFromPath(vfsC, root, false)
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
	err := parent.FetchFiles(vfsC)
	if err != nil {
		return nil, err
	}
	for _, d := range parent.dirs {
		if path.Join(name, d.Name) != d.Fullpath {
			return nil, fmt.Errorf("Bad fullpath: %s instead of %s", d.Fullpath, path.Join(name, d.Name))
		}
		children, err := recFetchTree(d, d.Fullpath)
		if err != nil {
			return nil, err
		}
		h[d.Name+"/"] = children
	}
	for _, f := range parent.files {
		h[f.Name] = nil
	}
	return h, nil
}

func TestGetFileDocFromPathAtRoot(t *testing.T) {
	doc, err := NewFileDoc("toto", "", -1, nil, "foo/bar", "foo", false, []string{})
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

func TestGetFileDocFromPath(t *testing.T) {
	dir, _ := NewDirDoc("container", "", nil, nil)
	err := CreateDir(vfsC, dir)
	assert.NoError(t, err)

	doc, err := NewFileDoc("toto", dir.ID(), -1, nil, "foo/bar", "foo", false, []string{})
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

func TestCreateAndGetFile(t *testing.T) {
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

	olddoc, err := createTree(origtree, RootFolderID)

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

	doc1, err := createTree(origtree, RootFolderID)
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

	dirchild2, err := GetDirDocFromPath(vfsC, "/update2/dirchild2", false)
	if !assert.NoError(t, err) {
		return
	}

	dirchild3, err := GetDirDocFromPath(vfsC, "/update2/dirchild3", false)
	if !assert.NoError(t, err) {
		return
	}

	newfolid := dirchild2.ID()
	_, err = ModifyDirMetadata(vfsC, dirchild3, &DocPatch{
		FolderID: &newfolid,
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

	_, err := createTree(walktree, RootFolderID)
	if !assert.NoError(t, err) {
		return
	}

	walked := ""
	Walk(vfsC, "/walk", func(name string, typ string, dir *DirDoc, file *FileDoc, err error) error {
		if !assert.NoError(t, err) {
			return err
		}

		if typ == DirType && !assert.Equal(t, dir.Fullpath, name) {
			return fmt.Errorf("Bad fullpath")
		}

		if typ == FileType && !assert.True(t, strings.HasSuffix(name, file.Name)) {
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

	err = couchdb.ResetDB(vfsC, FsDocType)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	for _, index := range Indexes {
		err = couchdb.DefineIndex(vfsC, FsDocType, index)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	CreateRootDirDoc(vfsC)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	res := m.Run()

	os.RemoveAll(tempdir)
	couchdb.DeleteDB(vfsC, FsDocType)

	os.Exit(res)
}
