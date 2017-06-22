package imexport

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/cozy/cozy-stack/pkg/vfs"
)

// Untardir untar doc directory
func Untardir(fs vfs.VFS, r io.Reader, dst string) error {

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
			if _, err := vfs.MkdirAll(fs, doc, nil); err != nil {
				return err
			}

		case tar.TypeReg:
			name := path.Base(hdr.Name)
			mime, class := vfs.ExtractMimeAndClassFromFilename(hdr.Name)
			now := time.Now()
			executable := true
			if hdr.FileInfo().Mode() == 644 {
				executable = false
			}

			dirDoc, err := fs.DirByPath(fmt.Sprintf("%s%s", dstDoc.Fullpath, path.Dir(hdr.Name)))
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

			if _, err := io.Copy(file, tr); err != nil {
				return err
			}
		}

	}

	return nil

}
