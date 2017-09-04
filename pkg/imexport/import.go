package imexport

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

func album(fs vfs.VFS, hdr *tar.Header, tr *tar.Reader, dstDoc *vfs.DirDoc, db couchdb.Database) error {
	m := make(map[string]*couchdb.DocReference)

	bs := bufio.NewScanner(tr)

	for bs.Scan() {
		jsondoc := &couchdb.JSONDoc{}
		err := jsondoc.UnmarshalJSON(bs.Bytes())
		if err != nil {
			return err
		}
		doctype, ok := jsondoc.M["type"].(string)
		if ok {
			jsondoc.Type = doctype
		}
		delete(jsondoc.M, "type")

		id := jsondoc.ID()
		jsondoc.SetID("")
		jsondoc.SetRev("")

		err = couchdb.CreateDoc(db, jsondoc)
		if err != nil {
			return err
		}

		m[id] = &couchdb.DocReference{
			ID:   jsondoc.ID(),
			Type: jsondoc.DocType(),
		}

	}

	_, err := tr.Next()
	if err != nil {
		return err
	}

	bs = bufio.NewScanner(tr)
	for bs.Scan() {
		ref := &References{}
		err := json.Unmarshal(bs.Bytes(), &ref)
		if err != nil {
			return err
		}

		file, err := fs.FileByPath(dstDoc.Fullpath + ref.Filepath)
		if err != nil {
			return err
		}

		if m[ref.Albumid] != nil {
			file.AddReferencedBy(*m[ref.Albumid])
			if err = couchdb.UpdateDoc(db, file); err != nil {
				return err
			}
		}

	}

	return nil

}

// Untardir untar doc directory
func Untardir(r io.Reader, dst string, instance *instance.Instance) error {
	fs := instance.VFS()
	domain := instance.Domain
	db := couchdb.SimpleDatabasePrefix(domain)

	dstDoc, err := fs.DirByID(dst)
	if err != nil {
		return err
	}

	//gzip reader
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gr.Close()

	//tar reader
	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		doc := path.Join(dstDoc.Fullpath, hdr.Name)

		switch hdr.Typeflag {

		case tar.TypeDir:

			if strings.TrimPrefix(hdr.Name, "/") == "metadata/album/" {
				for {
					hdr, err = tr.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						return err
					}

					if path.Base(hdr.Name) == "album.json" {
						err = album(fs, hdr, tr, dstDoc, db)
						if err != nil {
							return err
						}
					}
				}
			} else {
				if _, err := vfs.MkdirAll(fs, doc, nil); err != nil {
					return err
				}
			}

		case tar.TypeReg:

			name := path.Base(hdr.Name)
			mime, class := vfs.ExtractMimeAndClassFromFilename(hdr.Name)
			now := time.Now()
			executable := hdr.FileInfo().Mode()&0100 != 0

			dirDoc, err := fs.DirByPath(path.Join(dstDoc.Fullpath, path.Dir(hdr.Name)))
			if err != nil {
				return err
			}

			fileDoc, err := vfs.NewFileDoc(name, dirDoc.ID(), hdr.Size, nil, mime, class, now, executable, false, nil)
			if err != nil {
				return err
			}

			file, err := fs.CreateFile(fileDoc, nil)
			if err != nil {
				extension := path.Ext(fileDoc.DocName)
				fileName := fileDoc.DocName[0 : len(fileDoc.DocName)-len(extension)]
				fileDoc.DocName = fmt.Sprintf("%s-conflict-%d%s", fileName, time.Now().Unix(), extension)
				file, err = fs.CreateFile(fileDoc, nil)
				if err != nil {
					return err
				}
			}

			_, err = io.Copy(file, tr)
			cerr := file.Close()
			if err != nil {
				return err
			}
			if cerr != nil {
				return cerr
			}

		default:
			fmt.Println("Unknown typeflag", hdr.Typeflag)
			return errors.New("Unknown typeflag")

		}

	}

	return nil

}
