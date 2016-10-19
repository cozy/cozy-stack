package vfs

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/couchdb/mango"
	"github.com/sourcegraph/checkup"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

const CouchDBURL = "http://localhost:5984/"

const TestPrefix = "dev/"

var vfsC *Context

func TestGetFileDocFromPath(t *testing.T) {
	doc, err := NewFileDoc("toto", "", -1, nil, "foo/bar", "foo", false, []string{})
	assert.NoError(t, err)

	body := bytes.NewReader([]byte("hello !"))

	err = CreateFileAndUpload(vfsC, doc, body)
	assert.NoError(t, err)

	_, err = GetFileDocFromPath(vfsC, "/toto")
	assert.NoError(t, err)

	_, err = GetFileDocFromPath(vfsC, "/noooo")
	assert.Error(t, err)
}

func TestMain(m *testing.M) {
	db, err := checkup.HTTPChecker{URL: CouchDBURL}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}
	err = couchdb.ResetDB(TestPrefix, string(FileDocType))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.DefineIndex(TestPrefix, string(FileDocType), mango.IndexOnFields("path"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fs := afero.NewMemMapFs()

	vfsC = NewContext(fs, TestPrefix)

	os.Exit(m.Run())
}
