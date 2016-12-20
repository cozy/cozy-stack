package consts

const (
	// Instances doc type for User's instance document
	Instances = "instances"

	// Files doc type for type for files and directories
	Files = "io.cozy.files"
	// Manifests doc type for application manifests
	Manifests = "io.cozy.manifests"
	// Settings doc type for settings to customize an instance
	Settings = "io.cozy.settings"
	// Sessions doc type for sessions identifying a connection
	Sessions = "io.cozy.sessions"

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
