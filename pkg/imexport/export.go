package imexport

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

const (
	metaAlbumDir   = "metadata/album/"
	albumFile      = "album.json"
	referencesFile = "references.json"
)

// References between albumid and filepath
type References struct {
	Albumid  string `json:"albumid"`
	Filepath string `json:"filepath"`
}

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

func metadata(tw *tar.Writer, instance *instance.Instance) error {
	fs := instance.VFS()
	domain := instance.Domain
	db := couchdb.SimpleDatabasePrefix(domain)
	doctype := consts.PhotosAlbums
	var results []map[string]interface{}
	if err := couchdb.GetAllDocs(db, doctype, &couchdb.AllDocsRequest{}, &results); err != nil {
		if couchdb.IsNoDatabaseError(err) {
			return nil
		}
		return err
	}

	hdrDir := &tar.Header{
		Name:     metaAlbumDir,
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tw.WriteHeader(hdrDir); err != nil {
		return err
	}

	hdrAlbum := &tar.Header{
		Name:       metaAlbumDir + albumFile,
		Mode:       0644,
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
		Typeflag:   tar.TypeReg,
	}

	var content bytes.Buffer
	var size int64

	for _, val := range results {
		jsonString, err := json.Marshal(val)
		if err != nil {
			return err
		}

		size += int64(len(jsonString) + 1) // len([]byte("\n"))

		if _, err = content.Write(jsonString); err != nil {
			return err
		}
		if _, err = content.Write([]byte("\n")); err != nil {
			return err
		}

	}

	hdrAlbum.Size = size
	if err := tw.WriteHeader(hdrAlbum); err != nil {
		return err
	}

	if _, err := content.WriteTo(tw); err != nil {
		return err
	}

	var buf bytes.Buffer
	size = 0

	hdrRef := &tar.Header{
		Name:       metaAlbumDir + referencesFile,
		Mode:       0644,
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
		Typeflag:   tar.TypeReg,
	}

	req := &couchdb.ViewRequest{
		StartKey:    []string{consts.PhotosAlbums},
		EndKey:      []string{consts.PhotosAlbums, couchdb.MaxString},
		IncludeDocs: true,
		Reduce:      false,
	}
	res := &couchdb.ViewResponse{}
	if err := couchdb.ExecView(db, consts.FilesReferencedByView, req, res); err != nil {
		return err
	}

	for _, v := range res.Rows {
		key := v.Key.([]interface{})
		id := key[1].(string)

		doc, err := fs.FileByID(v.ID)
		if err != nil {
			return err
		}

		path, err := fs.FilePath(doc)
		if err != nil {
			return err
		}

		ref := References{
			Albumid:  id,
			Filepath: path,
		}
		b, err := json.Marshal(ref)
		if err != nil {
			return err
		}

		size += int64(len(b) + 1) // len([]byte("\n"))
		if _, err = buf.Write(b); err != nil {
			return err
		}
		if _, err = buf.Write([]byte("\n")); err != nil {
			return err
		}
	}

	hdrRef.Size = size
	if err := tw.WriteHeader(hdrRef); err != nil {
		return err
	}

	if _, err := buf.WriteTo(tw); err != nil {
		return err
	}

	return nil
}

func export(tw *tar.Writer, instance *instance.Instance) error {
	fs := instance.VFS()
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
	return metadata(tw, instance)
}

// Tardir tar doc directory
func Tardir(w io.Writer, instance *instance.Instance) error {
	//gzip writer
	gw := gzip.NewWriter(w)
	defer gw.Close()

	//tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	err := export(tw, instance)

	return err

}
