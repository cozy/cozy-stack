package consts

// Instances doc type for User's instance document
const Instances = "instances"

const (
	// Apps doc type for client-side application manifests
	Apps = "io.cozy.apps"
	// Konnectors doc type for konnector application manifests
	Konnectors = "io.cozy.konnectors"
	// Archives doc type for zip archives with files and directories
	Archives = "io.cozy.files.archives"
	// Doctypes doc type for doctype list
	Doctypes = "io.cozy.doctypes"
	// Files doc type for type for files and directories
	Files = "io.cozy.files"
	// Intents doc type for intents persisted in couchdb
	Intents = "io.cozy.intents"
	// Jobs doc type for queued jobs
	Jobs = "io.cozy.jobs"
	// OAuthAccessCodes doc type for OAuth2 access codes
	OAuthAccessCodes = "io.cozy.oauth.access_codes"
	// OAuthClients doc type for OAuth2 clients
	OAuthClients = "io.cozy.oauth.clients"
	// Permissions doc type for permissions identifying a connection
	Permissions = "io.cozy.permissions"
	// Queues doc type for jobs queues
	Queues = "io.cozy.queues"
	// Recipients doc type for sharing recipients
	Recipients = "io.cozy.recipients"
	// Sessions doc type for sessions identifying a connection
	Sessions = "io.cozy.sessions"
	// Settings doc type for settings to customize an instance
	Settings = "io.cozy.settings"
	// Sharings doc type for document and file sharing
	Sharings = "io.cozy.sharings"
	// Triggers doc type for triggers, jobs launchers
	Triggers = "io.cozy.triggers"
)

const (
	// FilesSlug is the slug of the default app, files, where the user is redirected after login
	FilesSlug = "files"
	// OnboardingSlug is the slug of the onboarding app, where the user is redirected when he has no passphrase
	OnboardingSlug = "onboarding"
	// StoreSlug is the slug of the only app that can install other apps
	// FIXME update me when the store app will be available
	StoreSlug = "settings"
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
	// DiskUsageID is the id of the settings JSON-API response for disk-usage
	DiskUsageID = "io.cozy.settings.disk-usage"
	// InstanceSettingsID is the id of settings document for the instance
	InstanceSettingsID = "io.cozy.settings.instance"
)

const (
	// OneShotSharing is a sharing with no continuous updates
	OneShotSharing = "one-shot"
	// MasterSlaveSharing is a sharing with unilateral continuous updates
	MasterSlaveSharing = "master-slave"
	// MasterMasterSharing is a sharing with bilateral continuous updates
	MasterMasterSharing = "master-master"
)

const (
	// PendingSharingStatus is the sharing pending status
	PendingSharingStatus = "pending"
	// RefusedSharingStatus is the sharing refused status
	RefusedSharingStatus = "refused"
	// AcceptedSharingStatus is the sharing accepted status
	AcceptedSharingStatus = "accepted"
	// ErrorSharingStatus is when the request could not be sent
	ErrorSharingStatus = "error"
	// UnregisteredSharingStatus is when the sharer could not register herself
	// as an OAuth client at the recipient's
	UnregisteredSharingStatus = "unregistered"
	// MailNotSentSharingStatus is when the mail invitation was not sent
	MailNotSentSharingStatus = "mail-not-sent"
)

// AppsRegistry is an hard-coded list of known apps, with their source URLs
// TODO remove it when we will have a true registry
var AppsRegistry = map[string]string{
	"onboarding": "git://github.com/cozy/cozy-onboarding-v3.git#build",
	"files":      "git://github.com/cozy/cozy-files-v3.git#build",
	"photos":     "git://github.com/cozy/cozy-photos-v3.git#build",
	"settings":   "git://github.com/cozy/cozy-settings.git#build",
}
