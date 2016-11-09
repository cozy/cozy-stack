package instance

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/couchdb/mango"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/sourcegraph/checkup"
	"github.com/stretchr/testify/assert"
)

func TestGetInstanceNoDB(t *testing.T) {
	instance, err := Get("no.instance.cozycloud.cc")
	if assert.Error(t, err, "An error is expected") {
		assert.Nil(t, instance)
		assert.Contains(t, err.Error(), "Instance not found", "the error is not explicit")
	}
}

func TestCreateInstance(t *testing.T) {
	instance, err := Create("test.cozycloud.cc", "en", nil)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, instance.ID())
		assert.Equal(t, instance.Domain, "test.cozycloud.cc")
	}
}

func TestCreateInstanceBadDomain(t *testing.T) {
	_, err := Create("..", "en", nil)
	assert.Error(t, err, "An error is expected")

	_, err = Create(".", "en", nil)
	assert.Error(t, err, "An error is expected")

	_, err = Create("foo/bar", "en", nil)
	assert.Error(t, err, "An error is expected")
}

func TestGetWrongInstance(t *testing.T) {
	instance, err := Get("no.instance.cozycloud.cc")
	if assert.Error(t, err, "An error is expected") {
		assert.Nil(t, instance)
		assert.Contains(t, err.Error(), "Instance not found", "the error is not explicit")
	}
}

func TestGetCorrectInstance(t *testing.T) {
	instance, err := Get("test.cozycloud.cc")
	if assert.NoError(t, err, "An error is expected") {
		assert.NotNil(t, instance)
		assert.Equal(t, instance.Domain, "test.cozycloud.cc")
	}
}

func TestInstanceHasRootFolder(t *testing.T) {
	var root vfs.DirDoc
	prefix := getDBPrefix(t, "test.cozycloud.cc")
	err := couchdb.GetDoc(prefix, vfs.FsDocType, vfs.RootFolderID, &root)
	if assert.NoError(t, err) {
		assert.Equal(t, root.Fullpath, "/")
	}
}

func TestInstanceHasIndexes(t *testing.T) {
	var results []*vfs.DirDoc
	prefix := getDBPrefix(t, "test.cozycloud.cc")
	req := &couchdb.FindRequest{Selector: mango.Equal("path", "/")}
	err := couchdb.FindDocs(prefix, vfs.FsDocType, req, &results)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestInstanceNoDuplicate(t *testing.T) {
	_, err := Create("test.cozycloud.cc.duplicate", "en", nil)
	if !assert.NoError(t, err) {
		return
	}
	i, err := Create("test.cozycloud.cc.duplicate", "en", nil)
	if assert.Error(t, err, "Should not be possible to create duplicate") {
		assert.Nil(t, i)
		assert.Contains(t, err.Error(), "Instance already exists", "the error is not explicit")
	}
}

func TestMain(m *testing.M) {
	const CouchDBURL = "http://localhost:5984/"
	const TestPrefix = "dev/"

	config.UseTestFile("..")

	db, err := checkup.HTTPChecker{URL: CouchDBURL}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}
	couchdb.DeleteDB(globalDBPrefix, instanceType)
	couchdb.DeleteDB("test.cozycloud.cc/", vfs.FsDocType)
	os.RemoveAll("/usr/local/var/cozy2/")

	os.Exit(m.Run())
}

func getDBPrefix(t *testing.T, domain string) string {
	instance, err := Get(domain)
	if !assert.NoError(t, err, "Should get instance %v", domain) {
		t.FailNow()
	}
	return instance.GetDatabasePrefix()
}
