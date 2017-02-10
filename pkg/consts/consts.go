package consts

const (
	// Instances doc type for User's instance document
	Instances = "instances"

	// Files doc type for type for files and directories
	Files = "io.cozy.files"
	// Archives doc type for zip archives with files and directories
	Archives = "io.cozy.files.archives"
	// Manifests doc type for application manifests
	Manifests = "io.cozy.manifests"
	// Jobs doc type for queued jobs
	Jobs = "io.cozy.jobs"
	// Queues doc type for jobs queues
	Queues = "io.cozy.queues"
	// Settings doc type for settings to customize an instance
	Settings = "io.cozy.settings"
	// Sessions doc type for sessions identifying a connection
	Sessions = "io.cozy.sessions"
	// Triggers doc type for triggers, jobs launchers
	Triggers = "io.cozy.triggers"

	// Permissions doc type for permissions identifying a connection
	Permissions = "io.cozy.permissions"

	// Doctypes doc type for doctype list
	Doctypes = "io.cozy.doctypes"

	// Sharings doc type for sharing descriptors
	Sharings = "io.cozy.sharings"
	// Recipients doc type for sharing recipients
	Recipients = "io.cozy.recipients"

	// OAuthClients doc type for OAuth2 clients
	OAuthClients = "io.cozy.oauth.clients"
	// OAuthAccessCodes doc type for OAuth2 access codes
	OAuthAccessCodes = "io.cozy.oauth.access_codes"
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
	MasterSlaveSharing = ""
	// MasterMasterSharing is a sharing with bilateral continuous updates
	MasterMasterSharing = ""
)
