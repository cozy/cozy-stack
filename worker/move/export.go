package move

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/echo"
)

const (
	// ExportFilesDir is the directory for storing the files in the export
	// archive.
	ExportFilesDir = "My Cozy/Files"
	// ExportMetasDir is the directory for storing the metadata in the export
	// archive.
	ExportMetasDir = "My Cozy/Metadata"
)

// ExportDoc is a documents storing the metadata of an export.
type ExportDoc struct {
	DocID     string `json:"_id,omitempty"`
	DocRev    string `json:"_rev,omitempty"`
	Domain    string `json:"domain"`
	PartsSize int64  `json:"parts_size,omitempty"`

	PartsCursors     []string      `json:"parts_cursors,omitempty"`
	WithDoctypes     []string      `json:"with_doctypes,omitempty"`
	WithoutFiles     bool          `json:"without_files,omitempty"`
	State            string        `json:"state"`
	CreatedAt        time.Time     `json:"created_at"`
	ExpiresAt        time.Time     `json:"expires_at"`
	TotalSize        int64         `json:"total_size,omitempty"`
	CreationDuration time.Duration `json:"creation_duration,omitempty"`
	Error            string        `json:"error,omitempty"`
}

// PartsSize is the default size of a file bucket, to split the index into
// equal-sized parts.
const PartsSize = 100 * 1024 * 1024 // 100 MB

var (
	// ErrExportNotFound is used when a export document could not be found
	ErrExportNotFound = echo.NewHTTPError(http.StatusNotFound, "exports: not found")
	// ErrExportExpired is used when the export document has expired along with
	// its associated data.
	ErrExportExpired = echo.NewHTTPError(http.StatusNotFound, "exports: has expired")
	// ErrMACInvalid is used when the given MAC is not valid.
	ErrMACInvalid = echo.NewHTTPError(http.StatusUnauthorized, "exports: invalid mac")
	// ErrExportConflict is used when an export is already being perfomed.
	ErrExportConflict = echo.NewHTTPError(http.StatusConflict, "export: an archive is already being created")
	// ErrExportDoesNotContainIndex is used when we could not find the index data
	// in the archive.
	ErrExportDoesNotContainIndex = echo.NewHTTPError(http.StatusBadRequest, "export: archive does not contain index data")
	// ErrExportInvalidCursor is used when the given index cursor is invalid
	ErrExportInvalidCursor = echo.NewHTTPError(http.StatusBadRequest, "export: cursor is invalid")
)

const (
	// ExportStateExporting is the state used when the export document is being
	// created.
	ExportStateExporting = "exporting"
	// ExportStateDone is used when the export document is finished, without
	// error.
	ExportStateDone = "done"
	// ExportStateError is used when the export document is finshed with error.
	ExportStateError = "error"
)

// DocType implements the couchdb.Doc interface
func (e *ExportDoc) DocType() string { return consts.Exports }

// ID implements the couchdb.Doc interface
func (e *ExportDoc) ID() string { return e.DocID }

// Rev implements the couchdb.Doc interface
func (e *ExportDoc) Rev() string { return e.DocRev }

// SetID implements the couchdb.Doc interface
func (e *ExportDoc) SetID(id string) { e.DocID = id }

// SetRev implements the couchdb.Doc interface
func (e *ExportDoc) SetRev(rev string) { e.DocRev = rev }

// Clone implements the couchdb.Doc interface
func (e *ExportDoc) Clone() couchdb.Doc {
	clone := *e

	clone.PartsCursors = make([]string, len(e.PartsCursors))
	copy(clone.PartsCursors, e.PartsCursors)

	clone.WithDoctypes = make([]string, len(e.WithDoctypes))
	copy(clone.WithDoctypes, e.WithDoctypes)

	return &clone
}

// Links implements the jsonapi.Object interface
func (e *ExportDoc) Links() *jsonapi.LinksList { return nil }

// Relationships implements the jsonapi.Object interface
func (e *ExportDoc) Relationships() jsonapi.RelationshipMap { return nil }

// Included implements the jsonapi.Object interface
func (e *ExportDoc) Included() []jsonapi.Object { return nil }

// HasExpired returns whether or not the export document has expired.
func (e *ExportDoc) HasExpired() bool {
	return time.Until(e.ExpiresAt) <= 0
}

var _ jsonapi.Object = &ExportDoc{}

// GenerateAuthMessage generates a MAC authentificating the access to the
// export data.
func (e *ExportDoc) GenerateAuthMessage(i *instance.Instance) []byte {
	mac, err := crypto.EncodeAuthMessage(archiveMACConfig, i.SessionSecret, []byte(e.ID()), nil)
	if err != nil {
		panic(fmt.Errorf("could not generate archive auth message: %s", err))
	}
	return mac
}

// verifyAuthMessage verifies the given MAC to authenticate and grant the
// access to the export data.
func verifyAuthMessage(i *instance.Instance, mac []byte) (string, bool) {
	exportID, err := crypto.DecodeAuthMessage(archiveMACConfig, i.SessionSecret, mac, nil)
	return string(exportID), err == nil
}

// GetExport returns an Export document associated with the given instance and
// with the given MAC message.
func GetExport(inst *instance.Instance, mac []byte) (*ExportDoc, error) {
	exportID, ok := verifyAuthMessage(inst, mac)
	if !ok {
		return nil, ErrMACInvalid
	}
	var exportDoc ExportDoc
	if err := couchdb.GetDoc(couchdb.GlobalDB, consts.Exports, exportID, &exportDoc); err != nil {
		if couchdb.IsNotFoundError(err) || couchdb.IsNoDatabaseError(err) {
			return nil, ErrExportNotFound
		}
		return nil, err
	}
	return &exportDoc, nil
}

// GetExports returns the list of exported documents.
func GetExports(domain string) ([]*ExportDoc, error) {
	var docs []*ExportDoc
	req := &couchdb.FindRequest{
		UseIndex: "by-domain",
		Selector: mango.Equal("domain", domain),
		Sort: mango.SortBy{
			{Field: "domain", Direction: mango.Desc},
			{Field: "created_at", Direction: mango.Desc},
		},
		Limit: 256,
	}
	err := couchdb.FindDocs(couchdb.GlobalDB, consts.Exports, req, &docs)
	if err != nil && !couchdb.IsNoDatabaseError(err) {
		return nil, err
	}
	return docs, nil
}

// ExportData returns a io.ReadCloser of the metadata archive.
func ExportData(inst *instance.Instance, archiver Archiver, mac []byte) (io.ReadCloser, int64, error) {
	exportDoc, err := GetExport(inst, mac)
	if err != nil {
		return nil, 0, err
	}
	if exportDoc.HasExpired() {
		return nil, 0, ErrExportExpired
	}
	return archiver.OpenArchive(inst, exportDoc)
}

// ExportCopyData does an HTTP copy of a part of the file indexes.
func ExportCopyData(w http.ResponseWriter, inst *instance.Instance, archiver Archiver, mac []byte, cursorStr string) (err error) {
	exportDoc, err := GetExport(inst, mac)
	if err != nil {
		return err
	}
	if exportDoc.HasExpired() {
		return ErrExportExpired
	}

	partNumber := 0
	// check that the given cursor is part of our pre-defined list of cursors.
	if cursorStr != "" {
		for i, c := range exportDoc.PartsCursors {
			if c == cursorStr {
				partNumber = i + 1
				break
			}
		}
		if partNumber == 0 {
			return ErrExportInvalidCursor
		}
	} else if exportDoc.WithoutFiles {
		return ErrExportDoesNotContainIndex
	}

	exportMetadata := partNumber == 0
	cursor, err := parseCursor(cursorStr)
	if err != nil {
		return ErrExportInvalidCursor
	}

	archive, _, err := archiver.OpenArchive(inst, exportDoc)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=cozy-export.part%03d.zip", partNumber))
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	defer func() {
		if errc := zw.Close(); err == nil {
			err = errc
		}
	}()

	var root *vfs.TreeFile
	gr, err := gzip.NewReader(archive)
	if err != nil {
		return err
	}

	now := time.Now()
	tr := tar.NewReader(gr)
	for {
		var hdr *tar.Header
		hdr, err = tr.Next()
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			return
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeDir {
			continue
		}

		var zipFileWriter io.Writer
		zipFileHdr := &zip.FileHeader{
			Name:   path.Join(ExportMetasDir, hdr.Name),
			Method: zip.Deflate,
			Flags:  0x800, // bit 11 set to force utf-8
		}
		zipFileHdr.SetModTime(now) // nolint: megacheck
		zipFileHdr.SetMode(0750)

		isIndexFile := hdr.Typeflag == tar.TypeReg && hdr.Name == "files-index.json"

		if isIndexFile && !exportDoc.WithoutFiles {
			var jsonData []byte
			jsonData, err = ioutil.ReadAll(tr)
			if err != nil {
				return
			}
			if err = json.NewDecoder(bytes.NewReader(jsonData)).Decode(&root); err != nil {
				return
			}
			if exportMetadata {
				zipFileWriter, err = zw.CreateHeader(zipFileHdr)
				if err != nil {
					return
				}
				_, err = io.Copy(zipFileWriter, bytes.NewReader(jsonData))
				if err != nil {
					return
				}
			}
		} else if exportMetadata {
			zipFileWriter, err = zw.CreateHeader(zipFileHdr)
			if err != nil {
				return
			}
			_, err = io.Copy(zipFileWriter, tr)
			if err != nil {
				return
			}
		}

		if isIndexFile && !exportMetadata {
			break
		}
	}

	if errc := gr.Close(); err == nil {
		err = errc
	}
	if errc := archive.Close(); err == nil {
		err = errc
	}
	if err != nil || exportDoc.WithoutFiles {
		return
	}

	if root == nil {
		return ErrExportDoesNotContainIndex
	}

	fs := inst.VFS()
	list, _ := listFilesIndex(root, nil, indexCursor{}, cursor,
		exportDoc.PartsSize, exportDoc.PartsSize)
	for _, file := range list {
		dirDoc, fileDoc := file.file.Refine()
		if fileDoc != nil {
			var f vfs.File
			f, err = fs.OpenFile(fileDoc)
			if err != nil {
				return
			}
			size := file.rangeEnd - file.rangeStart
			hdr := &zip.FileHeader{
				Name:   path.Join(ExportFilesDir, file.file.Fullpath),
				Method: zip.Deflate,
				Flags:  0x800, // bit 11 set to force utf-8
			}
			hdr.SetModTime(fileDoc.UpdatedAt) // nolint: megacheck
			if fileDoc.Executable {
				hdr.SetMode(0750)
			} else {
				hdr.SetMode(0640)
			}
			if size < file.file.ByteSize {
				hdr.Name += fmt.Sprintf(".range%d-%d", file.rangeStart, file.rangeEnd)
			}
			var zipFileWriter io.Writer
			zipFileWriter, err = zw.CreateHeader(hdr)
			if err != nil {
				return
			}
			if file.rangeStart > 0 {
				_, err = f.Seek(file.rangeStart, 0)
				if err != nil {
					return
				}
			}
			_, err = io.CopyN(zipFileWriter, f, size)
			if err != nil {
				return
			}
		} else {
			hdr := &zip.FileHeader{
				Name:   path.Join(ExportFilesDir, dirDoc.Fullpath) + "/",
				Method: zip.Deflate,
				Flags:  0x800, // bit 11 set to force utf-8
			}
			hdr.SetMode(0750)
			hdr.SetModTime(dirDoc.UpdatedAt) // nolint: megacheck
			_, err = zw.CreateHeader(hdr)
			if err != nil {
				return
			}
		}
	}

	return
}

// Export is used to create a tarball with files and photos from an instance
func Export(i *instance.Instance, opts ExportOptions, archiver Archiver) (exportDoc *ExportDoc, err error) {
	createdAt := time.Now()

	bucketSize := opts.PartsSize
	if bucketSize == 0 || bucketSize > PartsSize {
		bucketSize = PartsSize
	}

	maxAge := opts.MaxAge
	if maxAge == 0 || maxAge > archiveMaxAge {
		maxAge = archiveMaxAge
	}

	exportDoc = &ExportDoc{
		Domain:       i.Domain,
		State:        ExportStateExporting,
		CreatedAt:    createdAt,
		ExpiresAt:    createdAt.Add(maxAge),
		WithDoctypes: opts.WithDoctypes,
		WithoutFiles: opts.WithoutFiles,
		TotalSize:    -1,
		PartsSize:    bucketSize,
	}

	// Cleanup previously archived exports.
	{
		var exportedDocs []*ExportDoc
		exportedDocs, err = GetExports(i.Domain)
		if err != nil {
			return
		}
		notRemovedDocs := exportedDocs[:0]
		for _, e := range exportedDocs {
			if e.State == ExportStateExporting && time.Since(e.CreatedAt) < 24*time.Hour {
				return nil, ErrExportConflict
			}
			notRemovedDocs = append(notRemovedDocs, e)
		}
		if len(notRemovedDocs) > 0 {
			_ = archiver.RemoveArchives(notRemovedDocs)
		}
	}

	var size, n int64
	if err = couchdb.CreateDoc(couchdb.GlobalDB, exportDoc); err != nil {
		return
	}
	realtime.GetHub().Publish(i, realtime.EventCreate, exportDoc.Clone(), nil)
	defer func() {
		newExportDoc := exportDoc.Clone().(*ExportDoc)
		newExportDoc.CreationDuration = time.Since(createdAt)
		if err == nil {
			newExportDoc.State = ExportStateDone
			newExportDoc.TotalSize = size
		} else {
			newExportDoc.State = ExportStateError
			newExportDoc.Error = err.Error()
		}
		if erru := couchdb.UpdateDoc(couchdb.GlobalDB, newExportDoc); err == nil {
			err = erru
		}
		realtime.GetHub().Publish(i, realtime.EventUpdate,
			newExportDoc.Clone(), exportDoc.Clone())
	}()

	out, err := archiver.CreateArchive(exportDoc)
	if err != nil {
		return
	}
	defer func() {
		if errc := out.Close(); err == nil {
			err = errc
		}
	}()

	gw, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return
	}
	tw := tar.NewWriter(gw)
	defer func() {
		if errc := tw.Close(); err == nil {
			err = errc
		}
		if errc := gw.Close(); err == nil {
			err = errc
		}
	}()

	if n, err = writeInstanceDoc(i, "instance", createdAt, tw); err != nil {
		return
	}
	size += n

	settings, err := i.SettingsDocument()
	if err != nil {
		return
	}
	if n, err = writeDoc("", "settings", settings, createdAt, tw); err != nil {
		return
	}
	size += n

	if !opts.WithoutFiles {
		var tree *vfs.Tree
		tree, err = i.VFS().BuildTree()
		if err != nil {
			return
		}
		n, err = writeDoc("", "files-index", tree.Root, createdAt, tw)
		if err != nil {
			return
		}
		size += n

		exportDoc.PartsCursors, _ = splitFilesIndex(tree.Root, nil, nil, exportDoc.PartsSize, exportDoc.PartsSize)
	}

	n, err = exportDocs(i, opts.WithDoctypes, createdAt, tw)
	if err == nil {
		size += n
	}
	return
}

// splitFilesIndex devides the index into equal size bucket of maximum size
// `bucketSize`. Files can be splitted into multiple parts to accommodate the
// bucket size, using a range. It is used to be able to download the files into
// separate chunks.
//
// The method returns a list of cursor into the index tree for each beginning
// of a new bucket. A cursor has the following format:
//
//    ${dirname}/../${filename}-${byterange-start}
func splitFilesIndex(root *vfs.TreeFile, cursor []string, cursors []string, bucketSize, sizeLeft int64) ([]string, int64) {
	for childIndex, child := range root.FilesChildren {
		size := child.ByteSize
		if size <= sizeLeft {
			sizeLeft -= size
			continue
		}
		size -= sizeLeft
		for size > 0 {
			rangeStart := (child.ByteSize - size)
			cursorStr := strings.Join(append(cursor, strconv.Itoa(childIndex)), "/")
			cursorStr += ":" + strconv.FormatInt(rangeStart, 10)
			cursorStr = "/" + cursorStr
			cursors = append(cursors, cursorStr)
			size -= bucketSize
		}
		sizeLeft = -size
	}
	for dirIndex, dir := range root.DirsChildren {
		cursors, sizeLeft = splitFilesIndex(dir, append(cursor, strconv.Itoa(dirIndex)),
			cursors, bucketSize, sizeLeft)
	}
	return cursors, sizeLeft
}

type fileRanged struct {
	file       *vfs.TreeFile
	rangeStart int64
	rangeEnd   int64
}

// listFilesIndex browse the index with the given cursor and returns the
// flatting list of file entering the bucket.
func listFilesIndex(root *vfs.TreeFile, list []fileRanged, currentCursor, cursor indexCursor, bucketSize, sizeLeft int64) ([]fileRanged, int64) {
	if sizeLeft < 0 {
		return list, sizeLeft
	}

	cursorDiff := cursor.diff(currentCursor)
	cursorEqual := cursorDiff == 0 && currentCursor.equal(cursor)

	if cursorDiff >= 0 {
		for childIndex, child := range root.FilesChildren {
			var fileRangeStart, fileRangeEnd int64
			if cursorEqual {
				if childIndex < cursor.fileCursor {
					continue
				} else if childIndex == cursor.fileCursor {
					fileRangeStart = cursor.fileRangeStart
				}
			}
			if sizeLeft <= 0 {
				return list, sizeLeft
			}
			size := child.ByteSize - fileRangeStart
			if sizeLeft-size < 0 {
				fileRangeEnd = fileRangeStart + sizeLeft
			} else {
				fileRangeEnd = child.ByteSize
			}
			list = append(list, fileRanged{child, fileRangeStart, fileRangeEnd})
			sizeLeft -= size
			if sizeLeft < 0 {
				return list, sizeLeft
			}
		}

		// append empty directory so that we explicitly create them in the tarball
		if len(root.DirsChildren) == 0 && len(root.FilesChildren) == 0 {
			list = append(list, fileRanged{root, 0, 0})
		}
	}

	for dirIndex, dir := range root.DirsChildren {
		list, sizeLeft = listFilesIndex(dir, list, currentCursor.next(dirIndex),
			cursor, bucketSize, sizeLeft)
	}

	return list, sizeLeft
}

type indexCursor struct {
	dirCursor      []int
	fileCursor     int
	fileRangeStart int64
}

func (c indexCursor) diff(d indexCursor) int {
	l := len(d.dirCursor)
	if len(c.dirCursor) < l {
		l = len(c.dirCursor)
	}
	for i := 0; i < l; i++ {
		if diff := d.dirCursor[i] - c.dirCursor[i]; diff != 0 {
			return diff
		}
	}
	if len(d.dirCursor) > len(c.dirCursor) {
		return 1
	} else if len(d.dirCursor) < len(c.dirCursor) {
		return -1
	}
	return 0
}

func (c indexCursor) equal(d indexCursor) bool {
	l := len(d.dirCursor)
	if l != len(c.dirCursor) {
		return false
	}
	for i := 0; i < l; i++ {
		if d.dirCursor[i] != c.dirCursor[i] {
			return false
		}
	}
	return true
}

func (c indexCursor) next(dirIndex int) (next indexCursor) {
	next.dirCursor = append(c.dirCursor, dirIndex)
	next.fileCursor = 0
	next.fileRangeStart = 0
	return
}

func parseCursor(cursor string) (c indexCursor, err error) {
	if cursor == "" {
		return
	}
	ss := strings.Split(cursor, "/")
	if len(ss) < 2 {
		err = ErrExportInvalidCursor
		return
	}
	if ss[0] != "" {
		err = ErrExportInvalidCursor
		return
	}
	ss = ss[1:]
	c.dirCursor = make([]int, len(ss)-1)
	for i, s := range ss {
		if i == len(ss)-1 {
			rangeSplit := strings.SplitN(s, ":", 2)
			if len(rangeSplit) != 2 {
				err = ErrExportInvalidCursor
				return
			}
			c.fileCursor, err = strconv.Atoi(rangeSplit[0])
			if err != nil {
				return
			}
			c.fileRangeStart, err = strconv.ParseInt(rangeSplit[1], 10, 64)
			if err != nil {
				return
			}
		} else {
			c.dirCursor[i], err = strconv.Atoi(s)
			if err != nil {
				return
			}
		}
	}
	return
}

func exportDocs(in *instance.Instance, withDoctypes []string, now time.Time, tw *tar.Writer) (size int64, err error) {
	doctypes, err := couchdb.AllDoctypes(in)
	if err != nil {
		return
	}
	for _, doctype := range doctypes {
		if len(withDoctypes) > 0 && !utils.IsInArray(doctype, withDoctypes) {
			continue
		}
		switch doctype {
		case consts.KonnectorLogs, consts.Archives,
			consts.Sessions, consts.OAuthClients, consts.OAuthAccessCodes:
			// ignore these doctypes
		case consts.Sharings, consts.SharingsAnswer, consts.Shared:
			// ignore sharings ? TBD
		case consts.Files, consts.Settings:
			// already written out in a special file
		default:
			dir := url.PathEscape(doctype)
			err = couchdb.ForeachDocs(in, doctype,
				func(id string, doc json.RawMessage) error {
					n, errw := writeMarshaledDoc(dir, id, doc, now, tw)
					if errw == nil {
						size += n
					}
					return errw
				})
			if err != nil {
				return
			}
		}
	}
	return
}

func writeInstanceDoc(in *instance.Instance, name string,
	now time.Time, tw *tar.Writer) (int64, error) {
	clone := in.Clone().(*instance.Instance)
	clone.PassphraseHash = nil
	clone.PassphraseResetToken = nil
	clone.PassphraseResetTime = nil
	clone.RegisterToken = nil
	clone.SessionSecret = nil
	clone.OAuthSecret = nil
	clone.CLISecret = nil
	clone.SwiftCluster = 0
	return writeDoc("", name, clone, now, tw)
}

func writeDoc(dir, name string, data interface{},
	now time.Time, tw *tar.Writer) (int64, error) {
	doc, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}
	return writeMarshaledDoc(dir, name, doc, now, tw)
}

func writeMarshaledDoc(dir, name string, doc json.RawMessage,
	now time.Time, tw *tar.Writer) (int64, error) {
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
