package consts

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

// ShortcutMimeType is the mime-type for the .url files.
const ShortcutMimeType = "application/internet-shortcut"
