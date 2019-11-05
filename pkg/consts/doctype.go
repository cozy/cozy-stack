package consts

// Instances doc type for User's instance document
const Instances = "instances"

// Configs doc type assets documents configuration
const Configs = "configs"

const (
	// Apps doc type for client-side application manifests
	Apps = "io.cozy.apps"
	// AppsSuggestion doc type for suggesting apps to the user
	AppsSuggestion = "io.cozy.apps.suggestions"
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
	// FilesMetadata doc type for metadata of files
	FilesMetadata = "io.cozy.files.metadata"
	// FilesVersions doc type for versioning file contents
	FilesVersions = "io.cozy.files.versions"
	// Thumbnails is a synthetic doctype for thumbnails, used for realtime
	// events
	Thumbnails = "io.cozy.files.thumbnails"
	// PhotosAlbums doc type for photos albums
	PhotosAlbums = "io.cozy.photos.albums"
	// Intents doc type for intents persisted in couchdb
	Intents = "io.cozy.intents"
	// Jobs doc type for queued jobs
	Jobs = "io.cozy.jobs"
	// JobEvents doc type for real time events sent by jobs
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
	// BitwardenProfiles doc type for Bitwarden profile
	BitwardenProfiles = "com.bitwarden.profiles"
	// BitwardenCiphers doc type for Bitwarden ciphers
	BitwardenCiphers = "com.bitwarden.ciphers"
	// BitwardenFolders doc type for Bitwarden folders
	BitwardenFolders = "com.bitwarden.folders"
	// BitwardenOrganizations doc type for Bitwarden organizations (and
	// collections inside them)
	BitwardenOrganizations = "com.bitwarden.organizations"
	// NotesDocuments doc type is used for manipulating the documents that
	// represents a note before they are persisted to a file.
	NotesDocuments = "io.cozy.notes.documents"
)
