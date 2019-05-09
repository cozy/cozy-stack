package move

import (
	"fmt"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
)

var inst *instance.Instance

type Stats struct {
	TotalSize int64
	Dirs      map[string]struct{}
	Files     map[string][]byte
}

func createFile(t *testing.T, fs vfs.VFS, parent *vfs.DirDoc, stats *Stats) int64 {
	size := 1 + rand.Intn(25)
	name := crypto.GenerateRandomString(8)

	doc, err := vfs.NewFileDoc(name, parent.DocID, -1, nil, "application/octet-stream", "application", time.Now(), false, false, nil)
	assert.NoError(t, err)
	file, err := fs.CreateFile(doc, nil)
	assert.NoError(t, err)
	buf := make([]byte, size)
	_, err = file.Write(buf)
	assert.NoError(t, err)
	assert.NoError(t, file.Close())

	fullpath, err := doc.Path(fs)
	assert.NoError(t, err)
	stats.Files[fullpath] = make([]byte, doc.ByteSize)

	return doc.ByteSize
}

func populateTree(t *testing.T, fs vfs.VFS, parent *vfs.DirDoc, nb int, stats *Stats) {
	nbDirs := rand.Intn(5)
	if nbDirs > nb {
		nbDirs = nbDirs % (nb + 1)
	}

	// Create the sub-directories
	for i := 0; i < nbDirs; i++ {
		name := crypto.GenerateRandomString(6)
		fullpath := path.Join(parent.Fullpath, name)
		dir, err := vfs.Mkdir(fs, fullpath, nil)
		assert.NoError(t, err)
		stats.Dirs[fullpath] = struct{}{}
		nbFiles := rand.Intn(nb)
		populateTree(t, fs, dir, nbFiles, stats)
		nb -= nbFiles
	}

	// Create some files
	for j := 0; j < nb; j++ {
		stats.TotalSize += createFile(t, fs, parent, stats)
	}
}

func TestExportFiles(t *testing.T) {
	fs := inst.VFS()

	// The partsSize is voluntary really small to have a lot of parts,
	// which can help to test the edge cases
	var partsSize int64 = 10

	nbFiles := rand.Intn(100)
	root, err := fs.DirByID(consts.RootDirID)
	assert.NoError(t, err)
	stats := &Stats{
		TotalSize: 0,
		Dirs:      make(map[string]struct{}),
		Files:     make(map[string][]byte),
	}
	populateTree(t, fs, root, nbFiles, stats)

	// /* Uncomment this section for debug */
	// vfs.Walk(fs, root.Fullpath, func(fpath string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
	// 	if err != nil {
	// 		return err
	// 	}
	// 	if fpath == root.Fullpath {
	// 		return nil
	// 	}
	// 	level := strings.Count(fpath, "/")
	// 	for i := 0; i < level; i++ {
	// 		if i == level-1 {
	// 			_, err = fmt.Printf("└── ")
	// 		} else {
	// 			_, err = fmt.Printf("|  ")
	// 		}
	// 		if err != nil {
	// 			return err
	// 		}
	// 	}
	// 	if dir != nil {
	// 		_, err = fmt.Println(dir.DocName)
	// 	} else {
	// 		_, err = fmt.Printf("%s (%d)\n", file.DocName, file.ByteSize)
	// 	}
	// 	return err
	// })
	// fmt.Printf("total size = %d // nb dirs = %d // nb files = %d\n", stats.TotalSize, len(stats.Dirs), len(stats.Files))

	// Build the cursors
	tree, err := fs.BuildTree()
	assert.NoError(t, err)
	cursors, _ := splitFilesIndex(tree.Root, nil, nil, partsSize, partsSize)
	assert.Equal(t, (stats.TotalSize-1)/partsSize, int64(len(cursors)))

	cursors = append(cursors, "")
	for _, c := range cursors {
		cursor, err := parseCursor(c)
		assert.NoError(t, err)
		list, _ := listFilesIndex(tree.Root, nil, indexCursor{}, cursor, partsSize, partsSize)
		for _, f := range list {
			dirDoc, fileDoc := f.file.Refine()
			if dirDoc != nil {
				fullpath := dirDoc.Fullpath
				delete(stats.Dirs, fullpath)
				for fullpath != "/" {
					fullpath = path.Dir(fullpath)
					delete(stats.Dirs, fullpath)
				}
			} else {
				fpath, err := fileDoc.Path(fs)
				assert.NoError(t, err)
				for i := f.rangeStart; i < f.rangeEnd; i++ {
					stats.Files[fpath][i] = '1'
				}
				for fpath != "/" {
					fpath = path.Dir(fpath)
					delete(stats.Dirs, fpath)
				}
			}
		}
	}
	assert.Empty(t, stats.Dirs)
	for f, bytes := range stats.Files {
		for _, b := range bytes {
			if assert.NotEqual(t, 0, b, fmt.Sprintf("Failure for file %s", f)) {
				break
			}
		}
	}
}

func TestMain(m *testing.M) {
	seed := time.Now().UTC().Unix()
	fmt.Printf("seed = %d\n", seed)
	rand.Seed(seed)
	config.UseTestFile()
	setup := testutils.NewSetup(m, "export_test")
	inst = setup.GetTestInstance()
	os.Exit(setup.Run())
}
