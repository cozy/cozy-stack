package imexport

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
)

var inst *instance.Instance

func TestTardir(t *testing.T) {
	fs := inst.VFS()
	domain := inst.Domain

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

	w, err := os.Create("cozy_test.tar.gz")
	assert.NoError(t, err)

	err = Tardir(w, fs, domain)
	assert.NoError(t, err)

	r, err := os.Open("cozy_test.tar.gz")
	assert.NoError(t, err)

	gr, err := gzip.NewReader(r)
	assert.NoError(t, err)
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		assert.NoError(t, err)
		if hdr.Name == "logos.zip" {
			assert.Equal(t, int64(2814), hdr.Size)
		}
	}

}

func TestUntardir(t *testing.T) {
	fs := inst.VFS()
	domain := inst.Domain

	r, err := os.Open("cozy_test.tar.gz")
	assert.NoError(t, err)

	dst, err := vfs.Mkdir(fs, "/destination", nil)
	assert.NoError(t, err)

	err = Untardir(fs, r, dst.ID(), domain)
	assert.NoError(t, err)

	logo, err := fs.FileByPath("/destination/logos.zip")
	assert.NoError(t, err)
	assert.Equal(t, int64(2814), logo.Size())

	err = os.Remove("cozy_test.tar.gz")
	assert.NoError(t, err)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()

	setup := testutils.NewSetup(m, "export_test")
	inst = setup.GetTestInstance()
	os.Exit(setup.Run())
}
