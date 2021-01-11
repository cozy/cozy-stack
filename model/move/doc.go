package move

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/mail"
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

// ExportDoc is a documents storing the metadata of an export.
type ExportDoc struct {
	DocID     string `json:"_id,omitempty"`
	DocRev    string `json:"_rev,omitempty"`
	Domain    string `json:"domain"`
	PartsSize int64  `json:"parts_size,omitempty"`

	PartsCursors     []string      `json:"parts_cursors"`
	WithDoctypes     []string      `json:"with_doctypes,omitempty"`
	State            string        `json:"state"`
	CreatedAt        time.Time     `json:"created_at"`
	ExpiresAt        time.Time     `json:"expires_at"`
	TotalSize        int64         `json:"total_size,omitempty"`
	CreationDuration time.Duration `json:"creation_duration,omitempty"`
	Error            string        `json:"error,omitempty"`
}

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

// AcceptDoctype returns true if the documents of the given doctype must be
// exported.
func (e *ExportDoc) AcceptDoctype(doctype string) bool {
	if len(e.WithDoctypes) == 0 {
		return true
	}
	for _, typ := range e.WithDoctypes {
		if typ == doctype {
			return true
		}
	}
	return false
}

// MarksAsFinished saves the document when the export is done.
func (e *ExportDoc) MarksAsFinished(i *instance.Instance, size int64, err error) error {
	e.CreationDuration = time.Since(e.CreatedAt)
	if err == nil {
		e.State = ExportStateDone
		e.TotalSize = size
	} else {
		e.State = ExportStateError
		e.Error = err.Error()
	}
	return couchdb.UpdateDoc(couchdb.GlobalDB, e)
}

// SendExportMail sends a mail to the user with a link where they can download
// the export tarballs.
func (e *ExportDoc) SendExportMail(inst *instance.Instance) error {
	link := e.GenerateLink(inst)
	publicName, _ := inst.PublicName()
	mail := mail.Options{
		Mode:         mail.ModeFromStack,
		TemplateName: "archiver",
		TemplateValues: map[string]interface{}{
			"ArchiveLink": link,
			"PublicName":  publicName,
		},
	}

	msg, err := job.NewMessage(&mail)
	if err != nil {
		return err
	}

	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}

// NotifyTarget sends an HTTP request to the target so that it can start
// importing the tarballs.
func (e *ExportDoc) NotifyTarget(inst *instance.Instance, to *MoveToOptions, token string) error {
	link := e.GenerateLink(inst)
	u := to.ImportsURL()
	payload, err := json.Marshal(map[string]interface{}{
		"data": map[string]interface{}{
			"attributes": map[string]interface{}{
				"url":   link,
				"vault": settings.HasVault(inst),
				"move_from": map[string]interface{}{
					"url":   inst.PageURL("/", nil),
					"token": token,
				},
			},
		},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", u, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+to.Token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("Cannot notify target: %d", res.StatusCode)
	}
	return nil
}

// GenerateLink generates a link to download the export with a MAC.
func (e *ExportDoc) GenerateLink(i *instance.Instance) string {
	mac, err := crypto.EncodeAuthMessage(archiveMACConfig, i.SessionSecret(), []byte(e.ID()), nil)
	if err != nil {
		panic(fmt.Errorf("could not generate archive auth message: %s", err))
	}
	encoded := base64.URLEncoding.EncodeToString(mac)
	link := i.SubDomain(consts.SettingsSlug)
	link.Fragment = fmt.Sprintf("/exports/%s", encoded)
	return link.String()
}

// CleanPreviousExports ensures that we have no old exports (or clean them).
func (e *ExportDoc) CleanPreviousExports(archiver Archiver) error {
	exportedDocs, err := GetExports(e.Domain)
	if err != nil {
		return err
	}
	notRemovedDocs := exportedDocs[:0]
	for _, e := range exportedDocs {
		if e.State == ExportStateExporting && time.Since(e.CreatedAt) < 24*time.Hour {
			return ErrExportConflict
		}
		notRemovedDocs = append(notRemovedDocs, e)
	}
	if len(notRemovedDocs) > 0 {
		_ = archiver.RemoveArchives(notRemovedDocs)
	}
	return nil
}

func prepareExportDoc(i *instance.Instance, opts ExportOptions) *ExportDoc {
	createdAt := time.Now()

	// The size of the buckets can be specified by the options. If it is not
	// the case, it is computed from the disk usage. An instance with 4x more
	// bytes than another instance will have 2x more buckets and the buckets
	// will be 2x larger.
	bucketSize := opts.PartsSize
	if bucketSize < minimalPartsSize {
		bucketSize = minimalPartsSize
		if usage, err := i.VFS().DiskUsage(); err == nil && usage > bucketSize {
			factor := math.Sqrt(float64(usage) / float64(minimalPartsSize))
			bucketSize = int64(factor * float64(bucketSize))
		}
	}

	maxAge := opts.MaxAge
	if maxAge == 0 || maxAge > archiveMaxAge {
		maxAge = archiveMaxAge
	}

	return &ExportDoc{
		Domain:       i.Domain,
		State:        ExportStateExporting,
		CreatedAt:    createdAt,
		ExpiresAt:    createdAt.Add(maxAge),
		WithDoctypes: opts.WithDoctypes,
		TotalSize:    -1,
		PartsSize:    bucketSize,
	}
}

// verifyAuthMessage verifies the given MAC to authenticate and grant the
// access to the export data.
func verifyAuthMessage(i *instance.Instance, mac []byte) (string, bool) {
	exportID, err := crypto.DecodeAuthMessage(archiveMACConfig, i.SessionSecret(), mac, nil)
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
	if exportDoc.HasExpired() {
		return nil, ErrExportExpired
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
