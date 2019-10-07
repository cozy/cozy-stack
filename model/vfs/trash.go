package vfs

// TrashJournal is a list of files thathave been deleted of CouchDB when the
// trash was cleared, but removing them from Swift is slow and should be done
// later via the trash-files worker.
type TrashJournal struct {
	FileIDs     []string `json:"ids"`
	ObjectNames []string `json:"objects"`
}
