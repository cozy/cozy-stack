package move

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/url"
	"path"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
)

type ExportDoc struct {
	DocID            string        `json:"_id"`
	DocRev           string        `json:"_rev"`
	Domain           string        `json:"domain"`
	IsRemoved        bool          `json:"is_removed"`
	Secret           []byte        `json:"secret"`
	CreatedAt        time.Time     `json:"created_at"`
	ExpiresAt        time.Time     `json:"expires_at"`
	TotalSize        int64         `json:"total_size"`
	CreationDuration time.Duration `json:"creation_duration"`
	Error            string        `json:"error"`
}

var (
	ErrArchiveNotFound = errors.New("export: archive not found")
)

type Archiver interface {
	OpenArchive(exportDoc *ExportDoc) (io.ReadCloser, error)
	CreateArchive(exportDoc *ExportDoc) (io.WriteCloser, error)
	RemoveArchives(exportDocs []*ExportDoc) error
}

func (e *ExportDoc) DocType() string { return consts.Exports }
func (e *ExportDoc) ID() string      { return e.DocID }
func (e *ExportDoc) Rev() string     { return e.DocRev }

func (e *ExportDoc) SetID(id string)   { e.DocID = id }
func (e *ExportDoc) SetRev(rev string) { e.DocRev = rev }

func (e *ExportDoc) Clone() couchdb.Doc {
	clone := *e
	return &clone
}

func getExports(domain string) ([]*ExportDoc, error) {
	var docs []*ExportDoc
	req := &couchdb.FindRequest{
		UseIndex: "by-domain",
		Selector: mango.Equal("domain", domain),
		Limit:    256,
	}
	err := couchdb.FindDocs(couchdb.GlobalDB, consts.Exports, req, &docs)
	if err != nil && !couchdb.IsNoDatabaseError(err) {
		return nil, err
	}
	return docs, nil
}

// Export is used to create a tarball with files and photos from an instance
func Export(i *instance.Instance, archiver Archiver) (exportDoc *ExportDoc, err error) {
	secret := crypto.GenerateRandomBytes(16)
	createdAt := time.Now()

	exportDoc = &ExportDoc{
		Domain:    i.Domain,
		CreatedAt: createdAt,
		Secret:    secret,
		TotalSize: -1,
	}

	// Cleanup previously archived exports.
	{
		var exportedDocs []*ExportDoc
		exportedDocs, err = getExports(i.Domain)
		if err != nil {
			return
		}
		notRemovedDocs := exportedDocs[:0]
		for _, e := range exportedDocs {
			if !e.IsRemoved {
				notRemovedDocs = append(notRemovedDocs, e)
			}
		}
		if len(notRemovedDocs) > 0 {
			archiver.RemoveArchives(notRemovedDocs)
		}
	}

	var size, n int64
	if err = couchdb.CreateDoc(couchdb.GlobalDB, exportDoc); err != nil {
		return
	}
	defer func() {
		exportDoc.CreationDuration = time.Until(createdAt)
		if err == nil {
			exportDoc.TotalSize = size
		} else {
			exportDoc.Error = err.Error()
		}
		if erru := couchdb.UpdateDoc(couchdb.GlobalDB, exportDoc); err == nil {
			err = erru
		}
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

	if n, err = writeInstanceDoc(i, "instance", createdAt, tw, nil); err != nil {
		return
	}
	size += n

	settings, err := i.SettingsDocument()
	if err != nil {
		return
	}
	if n, err = writeDoc("", "settings", settings, createdAt, tw, nil); err != nil {
		return
	}
	size += n

	n, err = exportDocs(i, createdAt, tw)
	if errc := tw.Close(); err == nil {
		err = errc
	}
	if errc := gw.Close(); err == nil {
		err = errc
	}
	size += n

	return
}

func exportDocs(in *instance.Instance, now time.Time, tw *tar.Writer) (size int64, err error) {
	doctypes, err := couchdb.AllDoctypes(in)
	if err != nil {
		return
	}
	for _, doctype := range doctypes {
		switch doctype {
		case consts.Jobs, consts.KonnectorLogs,
			consts.Archives,
			consts.OAuthClients, consts.OAuthAccessCodes:
			// ignore these doctypes
		case consts.Sharings, consts.SharingsAnswer:
			// ignore sharings ? TBD
		case consts.Settings:
			// already written out in a special file
		default:
			dir := url.PathEscape(doctype)
			err = couchdb.ForeachDocs(in, doctype,
				func(id string, doc json.RawMessage) error {
					n, errw := writeMarshaledDoc(dir, id, doc, now, tw, nil)
					if errw == nil {
						size += n
					}
					return errw
				})
		}
		if err != nil {
			return
		}
	}
	return
}

func writeInstanceDoc(in *instance.Instance, name string,
	now time.Time, tw *tar.Writer, records map[string]string) (int64, error) {
	clone := in.Clone().(*instance.Instance)
	clone.PassphraseHash = nil
	clone.PassphraseResetToken = nil
	clone.PassphraseResetTime = nil
	clone.RegisterToken = nil
	clone.SessionSecret = nil
	clone.OAuthSecret = nil
	clone.CLISecret = nil
	clone.SwiftCluster = 0
	return writeDoc("", name, clone, now, tw, records)
}

func writeDoc(dir, name string, data interface{},
	now time.Time, tw *tar.Writer, records map[string]string) (int64, error) {
	doc, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}
	return writeMarshaledDoc(dir, name, doc, now, tw, records)
}

func writeMarshaledDoc(dir, name string, doc json.RawMessage,
	now time.Time, tw *tar.Writer, records map[string]string) (int64, error) {
	hdr := &tar.Header{
		Name:       path.Join(dir, name+".json"),
		Mode:       0640,
		Size:       int64(len(doc)),
		Typeflag:   tar.TypeReg,
		ModTime:    now,
		PAXRecords: records,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return 0, err
	}
	n, err := tw.Write(doc)
	return int64(n), err
}

type nopArchiver struct{}

func (a nopArchiver) OpenArchive(exportDoc *ExportDoc) (io.ReadCloser, error) {
	return nil, nil
}

func (a nopArchiver) CreateArchive(exportDoc *ExportDoc) (io.WriteCloser, error) {
	return ioutil.TempFile("", "cozy-archive-test")
}

func (a nopArchiver) RemoveArchives(exportDocs []*ExportDoc) error {
	return nil
}
