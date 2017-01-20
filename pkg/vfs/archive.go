package vfs

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

// ZipMime is the content-type for zip archives
const ZipMime = "application/zip"

// Archive is the data to create a zip archive
type Archive struct {
	Name  string   `json:"name"`
	Files []string `json:"files"`
}

var plusEscaper = strings.NewReplacer("+", "%20")

// ContentDisposition creates an HTTP header value for Content-Disposition
func ContentDisposition(disposition, filename string) string {
	// RFC2616 §2.2 - syntax of quoted strings
	escaped := strings.Map(func(r rune) rune {
		if r == 34 || r == 47 || r == 92 { // double quote, slash, and anti-slash
			return -1
		}
		if r > 32 && r < 127 {
			return r
		}
		return -1
	}, filename)
	if escaped == "" {
		escaped = "download"
	}
	if filename == escaped {
		return fmt.Sprintf("%s; filename=%s", disposition, escaped)
	}
	// RFC5987 §3.2 - syntax of ext value
	encoded := url.QueryEscape(filename)
	encoded = plusEscaper.Replace(encoded)
	// RFC5987 §3.2.1 - syntax of regular and extended header value encoding
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disposition, escaped, encoded)
}

// Serve creates on the fly the zip archive and streams in a http response
func (a *Archive) Serve(c Context, w http.ResponseWriter) error {
	header := w.Header()
	header.Set("Content-Type", ZipMime)
	header.Set("Content-Disposition", ContentDisposition("attachment", a.Name+".zip"))

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
