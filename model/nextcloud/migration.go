package nextcloud

import (
	"errors"
	"time"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
)

const nextcloudAccountType = "nextcloud"

const (
	MigrationStatusPending   = "pending"
	MigrationStatusRunning   = "running"
	MigrationStatusCompleted = "completed"
	MigrationStatusFailed    = "failed"
	MigrationStatusCanceled  = "canceled"
)

const DefaultMigrationTargetDir = "/Nextcloud"

var ErrMigrationConflict = errors.New("a nextcloud migration is already in progress")

// Migration is the io.cozy.nextcloud.migrations tracking document.
//
// The schema (especially the nested Progress object) is the contract with
// twake-migration-nextcloud. Flat counters would crash the service's progress
// reducer because it spreads doc.progress and adds to its fields.
//
// CancelRequested and CanceledAt are written by the migration service;
// the Stack round-trips them without modification.
type Migration struct {
	DocID           string            `json:"_id,omitempty"`
	DocRev          string            `json:"_rev,omitempty"`
	Status          string            `json:"status"`
	TargetDir       string            `json:"target_dir"`
	Progress        MigrationProgress `json:"progress"`
	Errors          []MigrationError  `json:"errors"`
	Skipped         []SkippedFile     `json:"skipped"`
	StartedAt       *time.Time        `json:"started_at"`
	FinishedAt      *time.Time        `json:"finished_at"`
	CancelRequested bool              `json:"cancel_requested,omitempty"`
	CanceledAt      *time.Time        `json:"canceled_at,omitempty"`
}

type MigrationProgress struct {
	FilesImported int64 `json:"files_imported"`
	FilesTotal    int64 `json:"files_total"`
	BytesImported int64 `json:"bytes_imported"`
	BytesTotal    int64 `json:"bytes_total"`
}

type MigrationError struct {
	Path    string    `json:"path"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

type SkippedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Size   int64  `json:"size"`
}

func (m *Migration) ID() string        { return m.DocID }
func (m *Migration) Rev() string       { return m.DocRev }
func (m *Migration) DocType() string   { return consts.NextcloudMigrations }
func (m *Migration) SetID(id string)   { m.DocID = id }
func (m *Migration) SetRev(rev string) { m.DocRev = rev }

func (m *Migration) Clone() couchdb.Doc {
	cloned := *m

	if m.Errors != nil {
		cloned.Errors = make([]MigrationError, len(m.Errors))
		copy(cloned.Errors, m.Errors)
	}
	if m.Skipped != nil {
		cloned.Skipped = make([]SkippedFile, len(m.Skipped))
		copy(cloned.Skipped, m.Skipped)
	}
	if m.StartedAt != nil {
		t := *m.StartedAt
		cloned.StartedAt = &t
	}
	if m.FinishedAt != nil {
		t := *m.FinishedAt
		cloned.FinishedAt = &t
	}
	if m.CanceledAt != nil {
		t := *m.CanceledAt
		cloned.CanceledAt = &t
	}
	return &cloned
}

// IsTerminal reports whether the migration has reached a state that the
// Stack must not try to cancel (completed, failed, or canceled).
func (m *Migration) IsTerminal() bool {
	switch m.Status {
	case MigrationStatusCompleted, MigrationStatusFailed, MigrationStatusCanceled:
		return true
	default:
		return false
	}
}

func (m *Migration) Links() *jsonapi.LinksList              { return nil }
func (m *Migration) Relationships() jsonapi.RelationshipMap { return nil }
func (m *Migration) Included() []jsonapi.Object             { return nil }

var (
	_ couchdb.Doc    = (*Migration)(nil)
	_ jsonapi.Object = (*Migration)(nil)
)

// NewPendingMigration returns a fresh Migration document in the pending state.
// Errors and Skipped are explicit empty slices so the JSON serialization
// produces "[]" rather than "null" — the migration service consumes them as
// arrays and would crash on null.
func NewPendingMigration(targetDir string) *Migration {
	if targetDir == "" {
		targetDir = DefaultMigrationTargetDir
	}
	return &Migration{
		Status:    MigrationStatusPending,
		TargetDir: targetDir,
		Errors:    []MigrationError{},
		Skipped:   []SkippedFile{},
	}
}

func (m *Migration) MarkFailed(inst *instance.Instance, cause error) error {
	now := time.Now().UTC()
	m.Status = MigrationStatusFailed
	if m.FinishedAt == nil {
		m.FinishedAt = &now
	}
	m.Errors = append(m.Errors, MigrationError{
		Message: cause.Error(),
		At:      now,
	})
	return couchdb.UpdateDoc(inst, m)
}

// FindNextcloudAccount returns the unique Nextcloud account for the given
// instance, or (nil, nil) if none exists yet. By design there is at most
// one account with `account_type: "nextcloud"` per instance: the migration
// trigger endpoint overwrites the existing account on every call so a
// retry with a corrected password or a different login does not leave
// orphaned docs the Settings UI cannot surface.
//
// If multiple legacy nextcloud accounts exist (left over from when the
// konnector flow kept one account per (url, login) pair), the function
// returns the first one scanned. Newly-triggered migrations will update
// that single doc; the rest stay in the database untouched until a real
// cleanup is wired up. A linear scan is cheaper than maintaining a Mango
// index because the per-instance account count is tiny.
func FindNextcloudAccount(inst *instance.Instance) (*couchdb.JSONDoc, error) {
	var accounts []*couchdb.JSONDoc
	req := &couchdb.AllDocsRequest{Limit: 1000}
	err := couchdb.GetAllDocs(inst, consts.Accounts, req, &accounts)
	if err != nil {
		if couchdb.IsNoDatabaseError(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, doc := range accounts {
		if doc == nil || doc.M == nil {
			continue
		}
		if accType, _ := doc.M["account_type"].(string); accType == nextcloudAccountType {
			return doc, nil
		}
	}
	return nil, nil
}

// EnsureAccount upserts the single Nextcloud account for the instance and
// returns its id. If an account already exists, its auth block and
// webdav_user_id are rewritten with the given values regardless of what
// they were before — this is the keyed-by-type policy that keeps the
// Settings UI free of orphans at the cost of multi-account support. The
// password is encrypted at rest before persistence.
func EnsureAccount(inst *instance.Instance, ncURL, login, password, userID string) (string, error) {
	authMap := map[string]interface{}{
		"url":      ncURL,
		"login":    login,
		"password": password,
	}

	existing, err := FindNextcloudAccount(inst)
	if err != nil {
		return "", err
	}
	if existing != nil {
		existing.Type = consts.Accounts
		existing.M["webdav_user_id"] = userID
		existing.M["auth"] = authMap
		account.Encrypt(*existing)
		if err := couchdb.UpdateDoc(inst, existing); err != nil {
			return "", err
		}
		return existing.ID(), nil
	}

	doc := &couchdb.JSONDoc{
		Type: consts.Accounts,
		M: map[string]interface{}{
			"account_type":   nextcloudAccountType,
			"webdav_user_id": userID,
			"auth":           authMap,
		},
	}
	account.Encrypt(*doc)
	account.ComputeName(*doc)

	if err := couchdb.CreateDoc(inst, doc); err != nil {
		return "", err
	}
	return doc.ID(), nil
}

// FindActiveMigration returns the first pending or running migration, or
// (nil, nil) if none. A missing doctype database or index is treated as "no
// active migration" so the first call on a fresh instance succeeds.
func FindActiveMigration(inst *instance.Instance) (*Migration, error) {
	var docs []*Migration
	req := &couchdb.FindRequest{
		UseIndex: "by-status",
		Selector: mango.In("status", []interface{}{
			MigrationStatusPending,
			MigrationStatusRunning,
		}),
		Limit: 1,
	}
	err := couchdb.FindDocs(inst, consts.NextcloudMigrations, req, &docs)
	if err != nil {
		if couchdb.IsNoDatabaseError(err) || couchdb.IsNoUsableIndexError(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(docs) == 0 {
		return nil, nil
	}
	return docs[0], nil
}
