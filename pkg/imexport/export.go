package imexport

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

func writeFile(fs vfs.VFS, name string, tw *tar.Writer, doc *vfs.FileDoc) error {
	file, err := fs.OpenFile(doc)
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name:       name,
		Mode:       0644,
		Size:       doc.Size(),
		ModTime:    doc.ModTime(),
		AccessTime: doc.CreatedAt,
		ChangeTime: doc.UpdatedAt,
		Typeflag:   tar.TypeReg,
	}
	if doc.Executable {
		hdr.Mode = 0755
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.Copy(tw, file); err != nil {
		return err
	}
	return nil
}

func createDir(name string, tw *tar.Writer, dir *vfs.DirDoc) error {

	hdr := &tar.Header{
		Name:     name,
		Mode:     0755,
		Size:     dir.Size(),
		ModTime:  dir.ModTime(),
		Typeflag: tar.TypeDir,
	}
	err := tw.WriteHeader(hdr)

	return err
}

func metadata(tw *tar.Writer, fs vfs.VFS, domain string) error {
	db := couchdb.SimpleDatabasePrefix(domain)
	doctype := "io.cozy.files"
	req := &couchdb.AllDocsRequest{}
	var results []map[string]interface{}
	if err := couchdb.GetAllDocs(db, doctype, req, &results); err != nil {
		return err
	}

	metaDir := "Metadata/"
	hdrDir := &tar.Header{
		Name:     metaDir,
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tw.WriteHeader(hdrDir); err != nil {
		return err
	}

	hdr := &tar.Header{
		Name:       metaDir + "album.json",
		Size:       int64(0),
		Mode:       0644,
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
		Typeflag:   tar.TypeReg,
	}

	var content [][]byte
	var size int64

	for _, val := range results {
		if val["referenced_by"] != nil {

			id := val["_id"].(string)
			doc, err := fs.FileByID(id)
			if err != nil {
				return err
			}

			path, err := fs.FilePath(doc)
			if err != nil {
				return err
			}
			val["album_filepath"] = path

			ref := doc.ReferencedBy
			for _, v := range ref {
				out := &couchdb.JSONDoc{}
				if err = couchdb.GetDoc(db, v.Type, v.ID, out); err != nil {
					return err
				}
				m := out.ToMapWithType()

				albumName := m["name"]
				val["album_name"] = albumName
			}

			jsonString, err := json.Marshal(val)
			if err != nil {
				return err
			}

			size += int64(len(jsonString) + 1) // len([]byte("\n"))

			content = append(content, jsonString)
			content = append(content, []byte("\n"))

		}

	}

	hdr.Size = size
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	for _, val := range content {
		if _, err := io.Copy(tw, bytes.NewReader(val)); err != nil {
			return err
		}
	}
	return nil
}

func export(tw *tar.Writer, fs vfs.VFS, domain string) error {
	root := "/"

	err := vfs.Walk(fs, root, func(name string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
		if err != nil {
			return err
		}

		if dir != nil {
			if err := createDir(name, tw, dir); err != nil {
				return err
			}
		}

		if file != nil {
			if err := writeFile(fs, name, tw, file); err != nil {
				return err
			}

		}

		return nil
	})
	if err != nil {
		return err
	}
	err = metadata(tw, fs, domain)
	return err
}

// Tardir tar doc directory
func Tardir(w io.Writer, fs vfs.VFS, domain string) error {
	//gzip writer
	gw := gzip.NewWriter(w)
	defer gw.Close()

	//tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	err := export(tw, fs, domain)

	return err

}
