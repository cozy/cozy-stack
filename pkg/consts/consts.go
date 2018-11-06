package consts

// Instances doc type for User's instance document
const Instances = "instances"

// Configs doc type assets documents configuration
const Configs = "configs"

const (
	// Apps doc type for client-side application manifests
	Apps = "io.cozy.apps"
	// Konnectors doc type for konnector application manifests
	Konnectors = "io.cozy.konnectors"
	// Versions doc type for apps versions from the registries
	Versions = "io.cozy.registry.versions"
	// KonnectorLogs doc type for konnector last execution logs.
	KonnectorLogs = "io.cozy.konnectors.logs"
	// Archives doc type for zip archives with files and directories
	Archives = "io.cozy.files.archives"
	// Exports doc type for global exports archives
	Exports = "io.cozy.exports"
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
	// Shared doc type for keepking track of documents in sharings
	Shared = "io.cozy.shared"
	// Sharings doc type for document and file sharing
	Sharings = "io.cozy.sharings"
	// SharingsAnswer doc type for credentials exchange for sharings
	SharingsAnswer = "io.cozy.sharings.answer"
	// SharingsInitialSync doc type for real-time events for initial sync of a
	// sharing
	SharingsInitialSync = "io.cozy.sharings.initial-sync"
	// Triggers doc type for triggers, jobs launchers
	Triggers = "io.cozy.triggers"
	// TriggersState doc type for triggers current state, jobs launchers
	TriggersState = "io.cozy.triggers.state"
	// Accounts doc type for accounts
	Accounts = "io.cozy.accounts"
	// AccountTypes doc type for account types
	AccountTypes = "io.cozy.account_types"
)

const (
	// OnboardingSlug is the slug of the onboarding app, where the user is
	// redirected when he has no passphrase.
	OnboardingSlug = "onboarding"
	// StoreSlug is the slug of the store application: it can install
	// konnectors and applications.
	StoreSlug = "store"
	// HomeSlug is the slug of the default app, where the user is redirected
	// after login.
	HomeSlug = "home"
	// SettingsSlug is the slog of the settings application.
	SettingsSlug = "settings"
	// DriveSlug is the slug of the default app, files, where the user is
	// redirected after login.
	DriveSlug = "drive"
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
	// SharedWithMeDirID is the identifier of the directory where the sharings
	// will put their folders for the shared files
	SharedWithMeDirID = "io.cozy.files.shared-with-me-dir"
	// NoLongerSharedDirID is the identifier of the directory where the files &
	// folders removed from a sharing but still used via a reference are put
	NoLongerSharedDirID = "io.cozy.files.no-longer-shared-dir"
)

const (
	// ContextSettingsID is the id of the settings JSON-API response for the context
	ContextSettingsID = "io.cozy.settings.context"
	// DiskUsageID is the id of the settings JSON-API response for disk-usage
	DiskUsageID = "io.cozy.settings.disk-usage"
	// InstanceSettingsID is the id of settings document for the instance
	InstanceSettingsID = "io.cozy.settings.instance"
)

// KnownFlatDomains is a list of top-domains that can hosts cozy instances with
// flat sub-domains.
var KnownFlatDomains = []string{
	"cozy.rocks",
	"mycozy.cloud",
}
