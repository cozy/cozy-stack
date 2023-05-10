package utils

import (
	"io/fs"
	"math/rand"
	"os"
	"sync"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomString(t *testing.T) {
	rand.Seed(42)
	s1 := RandomString(10)
	s2 := RandomString(20)

	rand.Seed(42)
	s3 := RandomString(10)
	s4 := RandomString(20)

	assert.Len(t, s1, 10)
	assert.Len(t, s2, 20)
	assert.Len(t, s3, 10)
	assert.Len(t, s4, 20)

	assert.NotEqual(t, s1, s2)
	assert.Equal(t, s1, s3)
	assert.Equal(t, s2, s4)
}

func TestRandomStringConcurrentAccess(t *testing.T) {
	n := 10000
	var wg sync.WaitGroup
	wg.Add(n)

	ms := make(map[string]struct{})
	var mu sync.Mutex

	gotDup := false

	for i := 0; i < n; i++ {
		go func() {
			s := RandomString(10)
			defer wg.Done()
			mu.Lock()
			defer mu.Unlock()
			if _, ok := ms[s]; ok {
				gotDup = true
			}
			var q struct{}
			ms[s] = q
		}()
	}
	wg.Wait()

	if gotDup {
		t.Fatal("should be unique strings")
	}
}

func TestStripPort(t *testing.T) {
	d1 := StripPort("localhost")
	assert.Equal(t, "localhost", d1)
	d2 := StripPort("localhost:8080")
	assert.Equal(t, "localhost", d2)
	d3 := StripPort("localhost:8080:8081")
	assert.Equal(t, "localhost:8080:8081", d3)
}

func TestSplitTrimString(t *testing.T) {
	parts1 := SplitTrimString("", ",")
	assert.EqualValues(t, []string{}, parts1)
	parts2 := SplitTrimString("foo,bar,baz,", ",")
	assert.EqualValues(t, []string{"foo", "bar", "baz"}, parts2)
	parts3 := SplitTrimString(",,,,", ",")
	assert.EqualValues(t, []string{}, parts3)
	parts4 := SplitTrimString("foo  ,, bar,  baz  ,", ",")
	assert.EqualValues(t, []string{"foo", "bar", "baz"}, parts4)
	parts5 := SplitTrimString("    ", ",")
	assert.EqualValues(t, []string{}, parts5)
}

func TestFileExistsFs(t *testing.T) {
	t.Run("not exists", func(t *testing.T) {
		afs := afero.NewMemMapFs()

		err := FileExists(afs, "/no/such/file")
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("exists", func(t *testing.T) {
		afs := afero.NewMemMapFs()
		_, err := afs.Create("/some/file")
		require.NoError(t, err)

		err = FileExists(afs, "/some/file")
		assert.NoError(t, err)
	})

	t.Run("is not file", func(t *testing.T) {
		afs := afero.NewMemMapFs()
		afs.MkdirAll("/some/dir", 0o755)

		err := FileExists(afs, "/some/dir")
		assert.ErrorIs(t, err, ErrIsNotFile)
	})
}

func TestDirExistsFs(t *testing.T) {
	t.Run("not exists", func(t *testing.T) {
		afs := afero.NewMemMapFs()

		err := DirExists(afs, "/no/such/dir")
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("exists", func(t *testing.T) {
		afs := afero.NewMemMapFs()
		afs.MkdirAll("/some/dir", 0o755)

		err := DirExists(afs, "/some/dir")
		assert.NoError(t, err)
	})

	t.Run("not a dir", func(t *testing.T) {
		afs := afero.NewMemMapFs()
		afs.Create("/some/file")

		err := DirExists(afs, "/some/file")
		assert.ErrorIs(t, err, ErrIsNotDir)
	})
}

func TestAbsPath(t *testing.T) {
	home, err := os.UserHomeDir()
	assert.NoError(t, err)
	assert.NotEmpty(t, home)
	assert.Equal(t, home, AbsPath("~"))
	foo := AbsPath("foo")
	wd, _ := os.Getwd()
	assert.Equal(t, wd+"/foo", foo)
	bar := AbsPath("~/bar")
	assert.Equal(t, home+"/bar", bar)
	baz := AbsPath("$HOME/baz")
	assert.Equal(t, home+"/baz", baz)
	qux := AbsPath("/qux")
	assert.Equal(t, "/qux", qux)
	quux := AbsPath("////qux//quux/../quux")
	assert.Equal(t, "/qux/quux", quux)
}
