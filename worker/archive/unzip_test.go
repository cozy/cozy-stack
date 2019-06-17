package archive

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

func TestZip(t *testing.T) {
	fs := inst.VFS()
	src, err := vfs.Mkdir(fs, "/src", nil)
	assert.NoError(t, err)
	dst, err := vfs.Mkdir(fs, "/dst", nil)
	assert.NoError(t, err)

	fd, err := os.Open("../../tests/fixtures/wet-cozy_20160910__M4Dz.jpg")
	assert.NoError(t, err)
	defer fd.Close()
	one, err := vfs.NewFileDoc("wet-cozy.jpg", src.ID(), -1, nil, "image/jpeg", "image", time.Now(), false, false, nil)
	assert.NoError(t, err)
	file, err := fs.CreateFile(one, nil)
	assert.NoError(t, err)
	_, err = io.Copy(file, fd)
	assert.NoError(t, err)
	assert.NoError(t, file.Close())

	at := time.Date(2019, 6, 15, 1, 2, 3, 4, time.UTC)
	two, err := vfs.NewFileDoc("hello.txt", src.ID(), -1, nil, "text/plain", "text", at, false, false, nil)
	assert.NoError(t, err)
	file, err = fs.CreateFile(two, nil)
	assert.NoError(t, err)
	_, err = file.Write([]byte("world"))
	assert.NoError(t, err)
	assert.NoError(t, file.Close())

	files := map[string]string{
		"wet-cozy.jpg": one.ID(),
		"hello.txt":    two.ID(),
	}

	err = createZip(fs, files, src.ID(), "archive.zip")
	assert.NoError(t, err)

	zipDoc, err := fs.FileByPath("/src/archive.zip")
	assert.NoError(t, err)

	err = unzip(fs, zipDoc.ID(), dst.ID())
	assert.NoError(t, err)

	f, err := fs.FileByPath("/dst/wet-cozy.jpg")
	assert.NoError(t, err)
	assert.Equal(t, one.ByteSize, f.ByteSize)
	assert.Equal(t, one.MD5Sum, f.MD5Sum)
	// The zip archive has only a precision of a second for the modified field
	assert.Equal(t, one.UpdatedAt.Unix(), f.UpdatedAt.Unix())

	f, err = fs.FileByPath("/dst/hello.txt")
	assert.NoError(t, err)
	assert.Equal(t, two.ByteSize, f.ByteSize)
	assert.Equal(t, two.MD5Sum, f.MD5Sum)
	// The zip archive has only a precision of a second for the modified field
	assert.Equal(t, two.UpdatedAt.Unix(), f.UpdatedAt.Unix())
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	setup := testutils.NewSetup(m, "unzip_test")
	inst = setup.GetTestInstance()
	os.Exit(setup.Run())
}
