package move

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/url"
	"path"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/note"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

// ExportOptions contains the options for launching the export worker.
type ExportOptions struct {
	PartsSize        int64         `json:"parts_size"`
	MaxAge           time.Duration `json:"max_age"`
	WithDoctypes     []string      `json:"with_doctypes,omitempty"`
	ContextualDomain string        `json:"contextual_domain,omitempty"`
}

// minimalPartsSize is the minimal size of a file bucket, to split the index
// into equal-sized parts.
const minimalPartsSize = 1024 * 1024 * 1024 // 1 GB

const (
	// ExportDataDir is the directory for storing the documents from CouchDB in
	// the export archive.
	ExportDataDir = "My Cozy/Data"
	// ExportFilesDir is the directory for storing the content of the files in
	// the export archive.
	ExportFilesDir = "My Cozy/Files"
	// ExportVersionsDir is the directory for storing the content of the old
	// versions of the files in the export archive.
	ExportVersionsDir = "My Cozy/Versions"
)

// ExportCopyData does an HTTP copy of a part of the file indexes.
func ExportCopyData(w io.Writer, inst *instance.Instance, exportDoc *ExportDoc, archiver Archiver, cursor Cursor) error {
	zw := zip.NewWriter(w)
	defer func() {
		_ = zw.Close()
	}()

	if cursor.Number == 0 {
		err := copyJSONData(zw, inst, exportDoc, archiver)
		if err != nil {
			return err
		}
	}

	if !exportDoc.AcceptDoctype(consts.Files) {
		return nil
	}

	return copyFiles(zw, inst, exportDoc, cursor)
}

func copyJSONData(zw *zip.Writer, inst *instance.Instance, exportDoc *ExportDoc, archiver Archiver) error {
	archive, err := archiver.OpenArchive(inst, exportDoc)
	if err != nil {
		return err
	}
	defer func() {
		_ = archive.Close()
	}()

	gr, err := gzip.NewReader(archive)
	if err != nil {
		return err
	}
	now := time.Now()
	tr := tar.NewReader(gr)
	defer func() {
		_ = gr.Close()
	}()

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		zipHeader := &zip.FileHeader{
			Name:     path.Join(ExportDataDir, header.Name),
			Method:   zip.Deflate,
			Modified: now,
		}
		zipHeader.SetMode(0640)
		zipFileWriter, err := zw.CreateHeader(zipHeader)
		if err != nil {
			return err
		}
		_, err = io.Copy(zipFileWriter, tr)
		if err != nil {
			return err
		}
	}

	return nil
}

func copyFiles(zw *zip.Writer, inst *instance.Instance, exportDoc *ExportDoc, cursor Cursor) error {
	files, err := listFilesFromCursor(inst, exportDoc, cursor)
	if err != nil {
		return err
	}

	fs := inst.VFS()
	filepather := vfs.NewFilePatherWithCache(fs)

	for _, file := range files {
		f, err := fs.OpenFile(file)
		if err != nil {
			return err
		}
		fullpath, err := file.Path(filepather)
		if err != nil {
			return err
		}
		header := &zip.FileHeader{
			Name:     path.Join(ExportFilesDir, fullpath),
			Method:   zip.Deflate,
			Modified: file.UpdatedAt,
		}
		if file.Executable {
			header.SetMode(0750)
		} else {
			header.SetMode(0640)
		}
		zipFileWriter, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(zipFileWriter, f)
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateExport is used to create a tarball with the data from an instance.
//
// Note: the tarball is a .tar.gz and not a .zip to allow streaming from Swift
// to the stack, and from the stack to the client, as .tar.gz can be read
// sequentially and reading a .zip need to seek.
func CreateExport(i *instance.Instance, opts ExportOptions, archiver Archiver) (*ExportDoc, error) {
	exportDoc := prepareExportDoc(i, opts)
	if err := exportDoc.CleanPreviousExports(archiver); err != nil {
		return nil, err
	}

	if err := couchdb.CreateDoc(couchdb.GlobalDB, exportDoc); err != nil {
		return nil, err
	}
	realtime.GetHub().Publish(i, realtime.EventCreate, exportDoc.Clone(), nil)

	size, err := writeArchive(i, exportDoc, archiver)
	old := exportDoc.Clone()
	errf := exportDoc.MarksAsFinished(i, size, err)
	realtime.GetHub().Publish(i, realtime.EventUpdate, exportDoc, old)
	if err != nil {
		return nil, err
	}
	return exportDoc, errf
}

func writeArchive(i *instance.Instance, exportDoc *ExportDoc, archiver Archiver) (int64, error) {
	out, err := archiver.CreateArchive(exportDoc)
	if err != nil {
		return 0, err
	}
	size, err := writeArchiveContent(i, exportDoc, out)
	if err != nil {
		return 0, err
	}
	return size, out.Close()
}

func writeArchiveContent(i *instance.Instance, exportDoc *ExportDoc, out io.Writer) (int64, error) {
	gw, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return 0, err
	}
	tw := tar.NewWriter(gw)
	size, err := writeDocuments(i, exportDoc, tw)
	if err != nil {
		return 0, err
	}
	if err := tw.Close(); err != nil {
		return 0, err
	}
	if err := gw.Close(); err != nil {
		return 0, err
	}
	return size, nil
}

func writeDocuments(i *instance.Instance, exportDoc *ExportDoc, tw *tar.Writer) (int64, error) {
	var size int64
	createdAt := exportDoc.CreatedAt

	n, err := writeInstanceDoc(i, "instance", createdAt, tw)
	if err != nil {
		return 0, err
	}
	size += n

	n, err = exportDocuments(i, exportDoc, createdAt, tw)
	if err != nil {
		return 0, err
	}
	size += n

	if exportDoc.AcceptDoctype(consts.Files) {
		n, err := exportFiles(i, exportDoc, tw)
		if err != nil {
			return 0, err
		}
		size += n
	}

	return size, nil
}

func exportFiles(i *instance.Instance, exportDoc *ExportDoc, tw *tar.Writer) (int64, error) {
	_ = note.FlushPendings(i)

	var size int64
	filesizes := make(map[string]int64)
	err := vfs.Walk(i.VFS(), "/", func(fullpath string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
		if err != nil {
			return err
		}
		if dir != nil {
			n, err := writeDoc(consts.Files, dir.DocID, dir, exportDoc.CreatedAt, tw)
			size += n
			return err
		}
		filesizes[file.DocID] = file.ByteSize
		return nil
	})
	if err != nil {
		return 0, err
	}

	exportDoc.PartsCursors = splitFiles(exportDoc.PartsSize, filesizes)
	return size, nil
}

func exportDocuments(in *instance.Instance, doc *ExportDoc, now time.Time, tw *tar.Writer) (int64, error) {
	doctypes, err := couchdb.AllDoctypes(in)
	if err != nil {
		return 0, err
	}

	var size int64
	for _, doctype := range doctypes {
		if !doc.AcceptDoctype(doctype) {
			continue
		}
		switch doctype {
		case consts.Files, consts.FilesVersions:
			// we have code specific to those doctypes
			continue
		}
		dir := url.PathEscape(doctype)
		err := couchdb.ForeachDocs(in, doctype, func(id string, doc json.RawMessage) error {
			n, err := writeMarshaledDoc(dir, id, doc, now, tw)
			if err == nil {
				size += n
			}
			return err
		})
		if err != nil {
			return 0, err
		}
	}
	return size, nil
}

func writeInstanceDoc(in *instance.Instance, name string, now time.Time, tw *tar.Writer) (int64, error) {
	clone := in.Clone().(*instance.Instance)
	clone.PassphraseHash = nil
	clone.PassphraseResetToken = nil
	clone.PassphraseResetTime = nil
	clone.RegisterToken = nil
	clone.SessSecret = nil
	clone.OAuthSecret = nil
	clone.CLISecret = nil
	clone.SwiftLayout = 0
	clone.IndexViewsVersion = 0
	return writeDoc("", name, clone, now, tw)
}

func writeDoc(dir, name string, data interface{}, now time.Time, tw *tar.Writer) (int64, error) {
	doc, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}
	return writeMarshaledDoc(dir, name, doc, now, tw)
}

func writeMarshaledDoc(dir, name string, doc json.RawMessage, now time.Time, tw *tar.Writer) (int64, error) {
	if tw == nil { // For testing purpose
		return 1, nil
	}

	hdr := &tar.Header{
		Name:     path.Join(dir, name+".json"),
		Mode:     0640,
		Size:     int64(len(doc)),
		Typeflag: tar.TypeReg,
		ModTime:  now,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return 0, err
	}
	n, err := tw.Write(doc)
	return int64(n), err
}
