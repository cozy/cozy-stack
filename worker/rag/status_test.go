package rag

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/require"
)

func TestWorkerIndexStatus(t *testing.T) {
	config.UseTestFile(t)
	setup := testutils.NewSetup(t, "rag_status_test")
	inst := setup.GetTestInstance(&lifecycle.Options{})

	t.Run("a success status sets Indexed and LastSuccessDate", func(t *testing.T) {
		fs := inst.VFS()
		doc := createStatusTestFile(t, fs, "rag-true.txt")
		defer destroyStatusTestFile(t, fs, doc)

		now := time.Now()
		err := updateRAGStatus(inst, doc, statusMessage{FileID: doc.DocID, Indexed: true, LastSuccessDate: &now})
		require.NoError(t, err)

		updated, err := fs.FileByID(doc.DocID)
		require.NoError(t, err)
		require.NotNil(t, updated.CozyMetadata)
		require.NotNil(t, updated.CozyMetadata.RAG)
		require.True(t, updated.CozyMetadata.RAG.Indexed)
		require.NotNil(t, updated.CozyMetadata.RAG.LastSuccessDate)
		require.Nil(t, updated.CozyMetadata.RAG.LastErrorDate)
	})

	t.Run("an error status sets Indexed=false and LastErrorDate", func(t *testing.T) {
		fs := inst.VFS()
		doc := createStatusTestFile(t, fs, "rag-false.txt")
		defer destroyStatusTestFile(t, fs, doc)

		now := time.Now()
		err := updateRAGStatus(inst, doc, statusMessage{FileID: doc.DocID, Indexed: false, LastErrorDate: &now})
		require.NoError(t, err)

		updated, err := fs.FileByID(doc.DocID)
		require.NoError(t, err)
		require.NotNil(t, updated.CozyMetadata)
		require.NotNil(t, updated.CozyMetadata.RAG)
		require.False(t, updated.CozyMetadata.RAG.Indexed)
		require.Nil(t, updated.CozyMetadata.RAG.LastSuccessDate)
		require.NotNil(t, updated.CozyMetadata.RAG.LastErrorDate)
	})

	t.Run("non-existent file_id returns nil error", func(t *testing.T) {
		payload := map[string]interface{}{
			"partition": inst.Domain,
			"file_id":   "non-existent-file-id",
			"indexed":   true,
		}
		data, err := json.Marshal(payload)
		require.NoError(t, err)

		j := &job.Job{
			Domain:     inst.Domain,
			WorkerType: "rag-index-status",
			Payload:    job.Payload(data),
		}
		ctx, cancel := job.NewTaskContext("test", j, inst)
		defer cancel()

		err = WorkerIndexStatus(ctx)
		require.NoError(t, err)
	})
}

func createStatusTestFile(t *testing.T, fs vfs.VFS, name string) *vfs.FileDoc {
	t.Helper()
	parent, err := fs.DirByPath("/")
	require.NoError(t, err)
	doc, err := vfs.NewFileDoc(name, parent.DocID, 4, nil, "text/plain", "text", time.Now(), false, false, false, nil)
	require.NoError(t, err)
	f, err := fs.CreateFile(doc, nil)
	require.NoError(t, err)
	_, err = f.Write([]byte("test"))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	updated, err := fs.FileByID(doc.DocID)
	require.NoError(t, err)
	return updated
}

func destroyStatusTestFile(t *testing.T, fs vfs.VFS, doc *vfs.FileDoc) {
	t.Helper()
	_ = fs.DestroyFile(doc)
}
