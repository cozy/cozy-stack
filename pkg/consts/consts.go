package consts

import (
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// Instances doc type for User's instance document
const Instances = "instances"

const (
	// Apps doc type for client-side application manifests
	Apps = "io.cozy.apps"
	// Konnectors doc type for konnector application manifests
	Konnectors = "io.cozy.konnectors"
	// KonnectorResults doc type for konnector last execution result
	KonnectorResults = "io.cozy.konnectors.result"
	// Versions doc type for apps versions from the registries
	Versions = "io.cozy.registry.versions"
	// KonnectorLogs doc type for konnector last execution logs.
	KonnectorLogs = "io.cozy.konnectors.logs"
	// Archives doc type for zip archives with files and directories
	Archives = "io.cozy.files.archives"
	// Doctypes doc type for doctype list
	Doctypes = "io.cozy.doctypes"
	// Files doc type for type for files and directories
	Files = "io.cozy.files"
	// PhotosAlbums doc type for photos albums
	PhotosAlbums = "io.cozy.photos.albums"
	// Intents doc type for intents persisted in couchdb
	Intents = "io.cozy.intents"
	// Jobs doc type for queued jobs
	Jobs = "io.cozy.jobs"
	// JobEvents doc type for realt time events sent by jobs
	JobEvents = "io.cozy.jobs.events"
	// Notifications doc type for notifications
	Notifications = "io.cozy.notifications"
	// OAuthAccessCodes doc type for OAuth2 access codes
	OAuthAccessCodes = "io.cozy.oauth.access_codes"
	// OAuthClients doc type for OAuth2 clients
	OAuthClients = "io.cozy.oauth.clients"
	// Permissions doc type for permissions identifying a connection
	Permissions = "io.cozy.permissions"
	// Contacts doc type for sharing
	Contacts = "io.cozy.contacts"
	// RemoteRequests doc type for logging requests to remote websites
	RemoteRequests = "io.cozy.remote.requests"
	// Sessions doc type for sessions identifying a connection
	Sessions = "io.cozy.sessions"
	// SessionsLogins doc type for sessions identifying a connection
	SessionsLogins = "io.cozy.sessions.logins"
	// Settings doc type for settings to customize an instance
	Settings = "io.cozy.settings"
	// Sharings doc type for document and file sharing
	Sharings = "io.cozy.sharings"
	// Triggers doc type for triggers, jobs launchers
	Triggers = "io.cozy.triggers"
	// Accounts doc type for accounts
	Accounts = "io.cozy.accounts"
	// AccountTypes doc type for account types
	AccountTypes = "io.cozy.account_types"
)

const (
	// DriveSlug is the slug of the default app, files, where the user is
	// redirected after login.
	DriveSlug = "drive"
	// OnboardingSlug is the slug of the onboarding app, where the user is
	// redirected when he has no passphrase.
	OnboardingSlug = "onboarding"
	// StoreSlug is the slug of the only app that can install other apps.
	StoreSlug = "store"
	// CollectSlug is the slug of the only app that can install konnectors.
	CollectSlug = "collect"
	// SettingsSlug is the slog of the settings application.
	SettingsSlug = "settings"
)

const (
	// DirType is the type attribute for directories
	DirType = "directory"
	// FileType is the type attribute for files
	FileType = "file"
)

const (
	// RootDirID is the root directory identifier
	RootDirID = "io.cozy.files.root-dir"
	// TrashDirID is the trash directory identifier
	TrashDirID = "io.cozy.files.trash-dir"
)

const (
	// ContextSettingsID is the id of the settings JSON-API response for the context
	ContextSettingsID = "io.cozy.settings.context"
	// DiskUsageID is the id of the settings JSON-API response for disk-usage
	DiskUsageID = "io.cozy.settings.disk-usage"
	// InstanceSettingsID is the id of settings document for the instance
	InstanceSettingsID = "io.cozy.settings.instance"
	// SharingSettingsID is the id of the settings document for sharings.
	SharingSettingsID = "io.cozy.settings.sharings"
)

const (
	// OneShotSharing is a sharing with no continuous updates
	OneShotSharing = "one-shot"
	// OneWaySharing is a sharing with unilateral continuous updates
	OneWaySharing = "one-way"
	// TwoWaySharing is a sharing with bilateral continuous updates
	TwoWaySharing = "two-way"
)

const (
	// SharingStatusPending is the sharing pending status
	SharingStatusPending = "pending"
	// SharingStatusAccepted is the sharing accepted status
	SharingStatusAccepted = "accepted"
	// SharingStatusError is when the request could not be sent
	SharingStatusError = "error"
	// SharingStatusUnregistered is when the sharer could not register herself
	// as an OAuth client at the recipient's
	SharingStatusUnregistered = "unregistered"
	// SharingStatusMailNotSent is when the mail invitation was not sent
	SharingStatusMailNotSent = "mail-not-sent"
	// SharingStatusRevoked is to tell if a recipient is revoked.
	SharingStatusRevoked = "revoked"
)

const (
	// SelectorReferencedBy is the "referenced_by" selector.
	SelectorReferencedBy = couchdb.SelectorReferencedBy
)

const (
	// QueryParamRev is the key for the revision value in a query string. In
	// web/data the revision is expected as "rev", not "Rev".
	QueryParamRev = "rev"
	// QueryParamDirID is the key for the `DirID` field of a vfs.FileDoc or
	// vfs.DirDoc, in a query string.
	QueryParamDirID = "Dir_id"
	// QueryParamName is the key for the name value in a query string.
	QueryParamName = "Name"
	// QueryParamType is the key for the `type` value (file or directory) in
	// a query string.
	QueryParamType = "Type"
	// QueryParamExecutable is key for the `executable` field of a vfs.FileDoc
	// in a query string.
	QueryParamExecutable = "Executable"
	// QueryParamCreatedAt is the key for the `created_at` value in a query
	// string.
	QueryParamCreatedAt = "Created_at"
	// QueryParamUpdatedAt is the key for the `Updated_at` value in a query
	// string.
	QueryParamUpdatedAt = "Updated_at"
	// QueryParamRecursive is the key for the `recursive` value in a query
	// string.
	QueryParamRecursive = "Recursive"
	// QueryParamTags is the key for the `tags` values in a query string.
	QueryParamTags = "Tags"
	// QueryParamReferencedBy is the key for the `referenced_by` values in a
	// query string.
	QueryParamReferencedBy = "Referenced_by"
	// QueryParamSharer is used to tell if the user that received the query is
	// the sharer or not.
	QueryParamSharer = "Sharer"
	// QueryParamAppSlug is used to transmit the application slug in a query
	// string.
	QueryParamAppSlug = "App_slug"
	// QueryParamSharingID is used to transmit the sharingID in a query string.
	QueryParamSharingID = "Sharing_id"
)

// AppsRegistry is an hard-coded list of known apps, with their source URLs
// TODO remove it when we will have a true registry
var AppsRegistry = map[string]string{
	"onboarding": "git://github.com/cozy/cozy-onboarding-v3.git#build",
	"drive":      "git://github.com/cozy/cozy-drive.git#build-drive",
	"photos":     "git://github.com/cozy/cozy-drive.git#build-photos",
	"settings":   "git://github.com/cozy/cozy-settings.git#build",
	"collect":    "git://github.com/cozy/cozy-collect.git#build",
}
