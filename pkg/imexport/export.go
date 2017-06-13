package imexport

import (
	"archive/tar"
	"compress/gzip"
	"io"

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

func export(tw *tar.Writer, fs vfs.VFS) error {
	root := "/Documents"

	err := vfs.Walk(fs, root, func(name string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
		if err != nil {
			return err
		}

		if file != nil {
			if err := writeFile(fs, name, tw, file); err != nil {
				return err
			}

		}

		return nil
	})
	return err
}

// Tardir tar doc directory
func Tardir(w io.Writer, fs vfs.VFS) error {
	//gzip writer
	gw := gzip.NewWriter(w)
	defer gw.Close()

	//tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	err := export(tw, fs)

	return err

}
