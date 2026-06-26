package rag

import (
	"errors"
	"os"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

const (
	RAGStatusSuccess      = "success"
	RAGStatusError        = "error"
	RAGStatusNotSupported = "notsupported"
)

func SetRAGStatus(inst *instance.Instance, fileID string, newStatus string, timestamp time.Time) error {
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	fs := inst.VFS()
	file, err := fs.FileByID(fileID)
	if err != nil {
		if couchdb.IsNotFoundError(err) || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	newdoc := file.Clone().(*vfs.FileDoc)
	if newdoc.CozyMetadata == nil {
		newdoc.CozyMetadata = vfs.NewCozyMetadata(inst.Domain)
	}
	if newdoc.CozyMetadata.RAG == nil {
		newdoc.CozyMetadata.RAG = &vfs.RAGMetadata{}
	}
	applyRAGStatus(newdoc.CozyMetadata.RAG, newStatus, timestamp)
	err = couchdb.UpdateDoc(fs, newdoc)
	if err == nil || !couchdb.IsConflictError(err) {
		return err
	}
	// 409: a parallel webhook wrote concurrently. Re-fetch and apply only if
	// our timestamp is still more recent than what was written.
	file, err = fs.FileByID(fileID)
	if err != nil {
		if couchdb.IsNotFoundError(err) || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if file.CozyMetadata != nil && file.CozyMetadata.RAG != nil && !isNewerThan(timestamp, file.CozyMetadata.RAG) {
		return nil
	}
	newdoc = file.Clone().(*vfs.FileDoc)
	if newdoc.CozyMetadata == nil {
		newdoc.CozyMetadata = vfs.NewCozyMetadata(inst.Domain)
	}
	if newdoc.CozyMetadata.RAG == nil {
		newdoc.CozyMetadata.RAG = &vfs.RAGMetadata{}
	}
	applyRAGStatus(newdoc.CozyMetadata.RAG, newStatus, timestamp)
	return couchdb.UpdateDoc(fs, newdoc)
}

func isNewerThan(ts time.Time, rag *vfs.RAGMetadata) bool {
	var latest time.Time
	if rag.LastSuccessDate != nil && rag.LastSuccessDate.After(latest) {
		latest = *rag.LastSuccessDate
	}
	if rag.LastErrorDate != nil && rag.LastErrorDate.After(latest) {
		latest = *rag.LastErrorDate
	}
	return ts.After(latest)
}

func applyRAGStatus(rag *vfs.RAGMetadata, newStatus string, timestamp time.Time) {
	rag.Status = newStatus
	switch newStatus {
	case RAGStatusSuccess:
		rag.Indexed = true
		rag.LastSuccessDate = &timestamp
	case RAGStatusError:
		// Indexed is preserved: stays true if the file was previously indexed.
		rag.LastErrorDate = &timestamp
	}
}
