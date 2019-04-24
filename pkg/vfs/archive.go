package vfs

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/hashicorp/go-multierror"
)

// ZipMime is the content-type for zip archives
const ZipMime = "application/zip"

// Archive is the data to create a zip archive
type Archive struct {
	Name   string   `json:"name"`
	Secret string   `json:"-"`
	IDs    []string `json:"ids"`
	Files  []string `json:"files"`

	// archiveEntries cache
	entries []ArchiveEntry
}

// ArchiveEntry is an utility struct to store a file or doc to be placed
// in the archive.
type ArchiveEntry struct {
	root string
	Dir  *DirDoc
	File *FileDoc
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
		n := len(a.IDs)
		entries := make([]ArchiveEntry, n+len(a.Files))
		for i, id := range a.IDs {
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
			entries[i] = ArchiveEntry{
				root: root,
				Dir:  d,
				File: f,
			}
		}
		for i, root := range a.Files {
			d, f, err := fs.DirOrFileByPath(root)
			if err != nil {
				return nil, err
			}
			entries[n+i] = ArchiveEntry{
				root: root,
				Dir:  d,
				File: f,
			}
		}

		a.entries = entries
	}

	return a.entries, nil
}

// Serve creates on the fly the zip archive and streams in a http response
func (a *Archive) Serve(fs VFS, w http.ResponseWriter) error {
	header := w.Header()
	header.Set("Content-Type", ZipMime)
	header.Set("Content-Disposition", ContentDisposition("attachment", a.Name+".zip"))

	zw := zip.NewWriter(w)
	defer zw.Close()

	entries, err := a.GetEntries(fs)
	if err != nil {
		return err
	}

	var errm error
	for _, entry := range entries {
		base := filepath.Dir(entry.root)
		err = walk(fs, entry.root, entry.Dir, entry.File, func(name string, dir *DirDoc, file *FileDoc, err error) error {
			if err != nil {
				return err
			}
			if dir != nil {
				return nil
			}
			name, err = filepath.Rel(base, name)
			if err != nil {
				return fmt.Errorf("Invalid filepath <%s>: %s", name, err)
			}
			header := &zip.FileHeader{
				Name:   a.Name + "/" + name,
				Method: zip.Deflate,
				Flags:  0x800, // bit 11 set to force utf-8
			}
			header.SetModTime(file.UpdatedAt) // nolint: megacheck
			ze, err := zw.CreateHeader(header)
			if err != nil {
				return fmt.Errorf("Can't create zip entry <%s>: %s", name, err)
			}
			f, err := fs.OpenFile(file)
			if err != nil {
				return fmt.Errorf("Can't open file <%s>: %s", name, err)
			}
			defer f.Close()
			_, err = io.Copy(ze, f)
			return err
		}, 0)
		if err != nil {
			errm = multierror.Append(errm, err)
		}
	}

	return errm
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
