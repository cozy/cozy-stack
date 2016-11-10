package vfs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/sourcegraph/checkup"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

const TestPrefix = "dev/"

const CouchDBURL = "http://localhost:5984/"

type TestContext struct {
	prefix string
	fs     afero.Fs
}

func (c TestContext) Prefix() string { return c.prefix }
func (c TestContext) FS() afero.Fs   { return c.fs }

var vfsC TestContext

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
	err := CreateDirectory(vfsC, dir)
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

func TestMain(m *testing.M) {
	config.UseTestFile()

	db, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	vfsC.prefix = "dev/"
	vfsC.fs = afero.NewMemMapFs()

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

	os.Exit(m.Run())
}
