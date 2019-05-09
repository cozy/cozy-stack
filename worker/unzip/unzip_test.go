package unzip

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
)

var inst *instance.Instance

func TestUnzip(t *testing.T) {
	fs := inst.VFS()
	dst, err := vfs.Mkdir(fs, "/destination", nil)
	assert.NoError(t, err)
	_, err = vfs.Mkdir(fs, "/destination/foo", nil)
	assert.NoError(t, err)

	fd, err := os.Open("../../tests/fixtures/logos.zip")
	assert.NoError(t, err)
	defer fd.Close()
	zip, err := vfs.NewFileDoc("logos.zip", consts.RootDirID, -1, nil, "application/zip", "application", time.Now(), false, false, nil)
	assert.NoError(t, err)
	file, err := fs.CreateFile(zip, nil)
	assert.NoError(t, err)
	_, err = io.Copy(file, fd)
	assert.NoError(t, err)
	assert.NoError(t, file.Close())

	_, err = fs.OpenFile(zip)
	assert.NoError(t, err)

	err = unzip(fs, zip.ID(), dst.ID())
	assert.NoError(t, err)

	blue, err := fs.FileByPath("/destination/blue.svg")
	assert.NoError(t, err)
	assert.Equal(t, int64(2029), blue.ByteSize)

	white, err := fs.FileByPath("/destination/white.svg")
	assert.NoError(t, err)
	assert.Equal(t, int64(2030), white.ByteSize)

	baz, err := fs.FileByPath("/destination/foo/bar/baz")
	assert.NoError(t, err)
	assert.Equal(t, int64(4), baz.ByteSize)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	setup := testutils.NewSetup(m, "unzip_test")
	inst = setup.GetTestInstance()
	os.Exit(setup.Run())
}
