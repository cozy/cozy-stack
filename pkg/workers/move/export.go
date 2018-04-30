package move

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/url"
	"path"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/utils"
)

func exportDocs(in *instance.Instance, now time.Time, tw *tar.Writer) error {
	doctypes, err := couchdb.AllDoctypes(in)
	if err != nil {
		return err
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
					return writeMarshaledDoc(dir, id, doc, now, tw, nil)
				},
			)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func writeInstanceDoc(in *instance.Instance, name string,
	now time.Time, tw *tar.Writer, records map[string]string) error {
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
	now time.Time, tw *tar.Writer, records map[string]string) error {
	doc, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return writeMarshaledDoc(dir, name, doc, now, tw, records)
}

func writeMarshaledDoc(dir, name string, doc json.RawMessage,
	now time.Time, tw *tar.Writer, records map[string]string) error {
	hdr := &tar.Header{
		Name:       path.Join(dir, name+".json"),
		Mode:       0640,
		Size:       int64(len(doc)),
		Typeflag:   tar.TypeReg,
		ModTime:    now,
		PAXRecords: records,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(doc)
	return err
}

// Export is used to create a tarball with files and photos from an instance
func Export(in *instance.Instance, opts Options) (err error) {
	var w io.WriteCloser
	if opts.NoCompression {
		w = utils.WriteCloser(opts.Output, nil)
	} else {
		w = gzip.NewWriter(opts.Output)
	}

	tw := tar.NewWriter(w)
	now := time.Now()
	if err = writeInstanceDoc(in, "instance", now, tw, nil); err != nil {
		return err
	}

	{
		settings, err := in.SettingsDocument()
		if err != nil {
			return err
		}
		if err = writeDoc("", "settings", settings, now, tw, nil); err != nil {
			return err
		}
	}

	err = exportDocs(in, now, tw)
	if errc := tw.Close(); err == nil {
		err = errc
	}
	if errc := w.Close(); err == nil {
		err = errc
	}
	return
}
