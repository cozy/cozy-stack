package vfs

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
)

// ZipMime is the content-type for zip archives
const ZipMime = "application/zip"

// Archive is the data to create a zip archive
type Archive struct {
	Name  string   `json:"name"`
	Files []string `json:"files"`
}

// Serve creates on the fly the zip archive and streams in a http response
func (a *Archive) Serve(c Context, w http.ResponseWriter) error {
	header := w.Header()
	header.Set("Content-Type", ZipMime)
	header.Set("Content-Disposition", "attachment; filename="+a.Name+".zip")

	fs := c.FS()
	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, root := range a.Files {
		base := filepath.Dir(root)
		Walk(c, root, func(name string, dir *DirDoc, file *FileDoc, err error) error {
			if err != nil {
				return err
			}
			if dir != nil {
				return nil
			}
			name, err = filepath.Rel(base, name)
			if err != nil {
				return fmt.Errorf("Invalid filepath <%s>: %s\n", name, err)
			}
			ze, err := zw.Create(a.Name + "/" + name)
			if err != nil {
				return fmt.Errorf("Can't create zip entry <%s>: %s\n", name, err)
			}
			path, err := file.Path(c)
			if err != nil {
				return fmt.Errorf("Can't find file <%s>: %s\n", name, err)
			}
			f, err := fs.Open(path)
			if err != nil {
				return fmt.Errorf("Can't open file <%s>: %s\n", name, err)
			}
			defer f.Close()
			_, err = io.Copy(ze, f)
			return err
		})
	}

	return nil
}
