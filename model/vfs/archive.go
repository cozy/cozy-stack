package vfs

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/labstack/echo/v4"
)

// ZipMime is the content-type for zip archives
const ZipMime = "application/zip"

// Archive is the data to create a zip archive
type Archive struct {
	Name   string   `json:"name"`
	Secret string   `json:"-"`
	IDs    []string `json:"ids"`
	Files  []string `json:"files"`
	Pages  []Page   `json:"pages"`

	// archiveEntries cache
	entries []ArchiveEntry
}

type Page struct {
	ID   string `json:"id"`
	Page int    `json:"page"`
}

// ArchiveEntry is an utility struct to store a file or doc to be placed
// in the archive.
type ArchiveEntry struct {
	root string
	Dir  *DirDoc
	File *FileDoc
	Page int
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
		return fmt.Sprintf(`%s; filename="%s"`, disposition, escaped)
	}
	// RFC5987 §3.2 - syntax of ext value
	encoded := url.QueryEscape(filename)
	encoded = plusEscaper.Replace(encoded)
	// RFC5987 §3.2.1 - syntax of regular and extended header value encoding
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disposition, escaped, encoded)
}

// GetEntries returns all files and folders in the archive as ArchiveEntry.
func (a *Archive) GetEntries(fs VFS) ([]ArchiveEntry, error) {
	if a.entries == nil {
		entries := make([]ArchiveEntry, 0, len(a.IDs)+len(a.Files)+len(a.Pages))
		for _, id := range a.IDs {
			d, f, err := fs.DirOrFileByID(id)
			if err != nil {
				return nil, err
			}
			var root string
			if d != nil {
				root = d.Fullpath
			} else {
				root, err = f.Path(fs)
				if err != nil {
					return nil, err
				}
			}
			entries = append(entries, ArchiveEntry{
				root: root,
				Dir:  d,
				File: f,
			})
		}
		for _, root := range a.Files {
			d, f, err := fs.DirOrFileByPath(root)
			if err != nil {
				return nil, err
			}
			entries = append(entries, ArchiveEntry{
				root: root,
				Dir:  d,
				File: f,
			})
		}
		for _, page := range a.Pages {
			f, err := fs.FileByID(page.ID)
			if err != nil {
				return nil, err
			}
			root, err := f.Path(fs)
			if err != nil {
				return nil, err
			}
			entries = append(entries, ArchiveEntry{
				root: root,
				File: f,
				Page: page.Page,
			})
		}

		a.entries = entries
	}

	return a.entries, nil
}

// Serve creates on the fly the zip archive and streams in a http response
func (a *Archive) Serve(fs VFS, w http.ResponseWriter) error {
	header := w.Header()
	header.Set(echo.HeaderContentType, ZipMime)
	header.Set(echo.HeaderContentDisposition,
		ContentDisposition("attachment", a.Name+".zip"))

	zw := zip.NewWriter(w)
	defer zw.Close()

	entries, err := a.GetEntries(fs)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		base := filepath.Dir(entry.root)
		err = walk(fs, entry.root, entry.Dir, entry.File, func(name string, dir *DirDoc, file *FileDoc, err error) error {
			if err != nil {
				return err
			}
			name, err = filepath.Rel(base, name)
			if err != nil {
				return fmt.Errorf("Invalid filepath <%s>: %s", name, err)
			}
			if dir != nil {
				_, err = zw.Create(a.Name + "/" + name + "/")
				return err
			}
			header := &zip.FileHeader{
				Name:     a.Name + "/" + name,
				Method:   zip.Deflate,
				Modified: file.UpdatedAt,
			}
			if entry.Page > 0 {
				header.Name = addPageToName(header.Name, entry.Page)
			}
			ze, err := zw.CreateHeader(header)
			if err != nil {
				return fmt.Errorf("Can't create zip entry <%s>: %s", name, err)
			}
			f, err := fs.OpenFile(file)
			if err != nil {
				return fmt.Errorf("Can't open file <%s>: %s", name, err)
			}
			defer f.Close()

			if entry.Page <= 0 {
				_, err = io.Copy(ze, f)
				return err
			}
			extracted, err := config.PDF().ExtractPage(f, entry.Page)
			if err != nil {
				return err
			}
			_, err = io.Copy(ze, extracted)
			return err
		}, 0)
		if err != nil {
			return err
		}
	}

	return nil
}

// ID makes Archive a jsonapi.Object
func (a *Archive) ID() string { return a.Secret }

// Rev makes Archive a jsonapi.Object
func (a *Archive) Rev() string { return "" }

// DocType makes Archive a jsonapi.Object
func (a *Archive) DocType() string { return consts.Archives }

// Clone implements couchdb.Doc
func (a *Archive) Clone() couchdb.Doc {
	cloned := *a

	cloned.IDs = make([]string, len(a.IDs))
	copy(cloned.IDs, a.IDs)

	cloned.Files = make([]string, len(a.Files))
	copy(cloned.Files, a.Files)

	cloned.entries = make([]ArchiveEntry, len(a.entries))
	copy(cloned.entries, a.entries)
	return &cloned
}

// SetID makes Archive a jsonapi.Object
func (a *Archive) SetID(_ string) {}

// SetRev makes Archive a jsonapi.Object
func (a *Archive) SetRev(_ string) {}

func addPageToName(name string, page int) string {
	ext := filepath.Ext(name)
	basename := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s (%d)%s", basename, page, ext)
}
