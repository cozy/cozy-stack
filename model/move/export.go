package move

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
)

const (
	albumsFile     = "albums.json"
	referencesFile = "references.json"
)

// Reference between albumid and filepath
type Reference struct {
	Albumid  string `json:"albumid"`
	Filepath string `json:"filepath"`
}

func writeFile(tw *tar.Writer, doc *vfs.FileDoc, name string, fs vfs.VFS) error {
	file, err := fs.OpenFile(doc)
	if err != nil {
		return err
	}
	defer file.Close()
	hdr := &tar.Header{
		Name:       "files/" + name,
		Mode:       0640,
		Size:       doc.Size(),
		ModTime:    doc.ModTime(),
		AccessTime: doc.CreatedAt,
		ChangeTime: doc.UpdatedAt,
		Typeflag:   tar.TypeReg,
	}
	if doc.Executable {
		hdr.Mode = 0750
	}
	if err = tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, file)
	return err
}

func createDir(tw *tar.Writer, dir *vfs.DirDoc, name string) error {
	hdr := &tar.Header{
		Name:     "files/" + name,
		Mode:     0755,
		Size:     dir.Size(),
		ModTime:  dir.ModTime(),
		Typeflag: tar.TypeDir,
	}
	return tw.WriteHeader(hdr)
}

func albums(tw *tar.Writer, instance *instance.Instance) error {
	doctype := consts.PhotosAlbums
	allReq := &couchdb.AllDocsRequest{}
	var results []map[string]interface{}
	if err := couchdb.GetAllDocs(instance, doctype, allReq, &results); err != nil {
		if couchdb.IsNoDatabaseError(err) {
			return nil
		}
		return err
	}

	hdrDir := &tar.Header{
		Name:     "albums",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tw.WriteHeader(hdrDir); err != nil {
		return err
	}

	var content bytes.Buffer
	size := 0
	for _, val := range results {
		b, err := json.Marshal(val)
		if err != nil {
			return err
		}
		b = append(b, '\n')
		size += len(b)
		if _, err = content.Write(b); err != nil {
			return err
		}
	}

	hdrAlbum := &tar.Header{
		Name:       "albums/" + albumsFile,
		Mode:       0644,
		Size:       int64(size),
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
		Typeflag:   tar.TypeReg,
	}
	if err := tw.WriteHeader(hdrAlbum); err != nil {
		return err
	}
	if _, err := content.WriteTo(tw); err != nil {
		return err
	}

	req := &couchdb.ViewRequest{
		StartKey:    []string{consts.PhotosAlbums},
		EndKey:      []string{consts.PhotosAlbums, couchdb.MaxString},
		IncludeDocs: true,
		Reduce:      false,
	}
	res := &couchdb.ViewResponse{}
	if err := couchdb.ExecView(instance, couchdb.FilesReferencedByView, req, res); err != nil {
		return err
	}

	var buf bytes.Buffer
	fs := instance.VFS()
	size = 0
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
		ref := Reference{
			Albumid:  id,
			Filepath: path,
		}
		b, err := json.Marshal(ref)
		if err != nil {
			return err
		}
		b = append(b, '\n')
		size += len(b)
		if _, err = buf.Write(b); err != nil {
			return err
		}
	}

	hdrRef := &tar.Header{
		Name:       "albums/" + referencesFile,
		Mode:       0644,
		Size:       int64(size),
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
		Typeflag:   tar.TypeReg,
	}
	if err := tw.WriteHeader(hdrRef); err != nil {
		return err
	}
	_, err := buf.WriteTo(tw)
	return err
}

func export(tw *tar.Writer, instance *instance.Instance) error {
	fs := instance.VFS()
	err := vfs.Walk(fs, "/", func(name string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
		if err != nil {
			return err
		}

		if dir != nil {
			if err := createDir(tw, dir, name); err != nil {
				return err
			}
		}

		if file != nil {
			if err := writeFile(tw, file, name, fs); err != nil {
				return err
			}

		}

		return nil
	})
	if err != nil {
		return err
	}
	return albums(tw, instance)
}

// Export is used to create a tarball with files and photos from an instance
func Export(instance *instance.Instance) (filename string, err error) {
	domain := instance.Domain
	tab := crypto.GenerateRandomBytes(20)
	id := base32.StdEncoding.EncodeToString(tab)
	filename = fmt.Sprintf("%s-%s.tar.gz", domain, id)

	w, err := os.Create(filename)
	if err != nil {
		return
	}
	defer w.Close()

	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)
	err = export(tw, instance)
	defer func() {
		if errc := tw.Close(); err == nil && errc != nil {
			err = errc
		}
		if errc := gw.Close(); err == nil && errc != nil {
			err = errc
		}
	}()
	return
}
