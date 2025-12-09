package antivirus_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/clamav"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/worker/antivirus"
	"github.com/stretchr/testify/require"
)

func TestAntivirus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping ClamAV tests in short mode")
	}

	fixture := testutils.StartClamAV(t)
	clamavAddress := fixture.Address

	// ClamAV client tests can run in parallel
	t.Run("ClamAVClient", func(t *testing.T) {
		t.Run("CleanFile", func(t *testing.T) {
			t.Parallel()

			client := clamav.NewClient(clamavAddress, 30*time.Second)

			// Test PING
			err := client.Ping(context.Background())
			require.NoError(t, err)

			// Test scanning a clean file
			cleanContent := []byte("This is a clean test file content")
			result, err := client.Scan(context.Background(), bytes.NewReader(cleanContent))
			require.NoError(t, err)
			require.Equal(t, clamav.StatusClean, result.Status)
			require.Empty(t, result.VirusName)
			require.Empty(t, result.Error)
		})

		t.Run("InfectedFile", func(t *testing.T) {
			t.Parallel()

			client := clamav.NewClient(clamavAddress, 30*time.Second)

			// Test scanning an infected file (EICAR test signature)
			infectedContent := testutils.EICARTestSignature()
			result, err := client.Scan(context.Background(), bytes.NewReader(infectedContent))
			require.NoError(t, err)
			require.Equal(t, clamav.StatusInfected, result.Status)
			require.Contains(t, result.VirusName, "EICAR")
			require.Empty(t, result.Error)
		})
	})

	// Worker tests share a single instance setup
	t.Run("Worker", func(t *testing.T) {
		config.UseTestFile(t)
		setup := testutils.NewSetup(t, "antivirus_worker_test")
		inst := setup.GetTestInstance(&lifecycle.Options{})

		// Configure antivirus for all worker tests
		conf := config.GetConfig()
		conf.Antivirus.Enabled = true
		conf.Antivirus.Address = clamavAddress
		conf.Antivirus.Timeout = 30 * time.Second
		t.Cleanup(func() {
			conf.Antivirus.Enabled = false
			conf.Antivirus.MaxFileSize = 0
		})

		t.Run("CleanFile", func(t *testing.T) {
			testWorkerCleanFile(t, inst)
		})

		t.Run("InfectedFile", func(t *testing.T) {
			testWorkerInfectedFile(t, inst)
		})

		t.Run("SkipsLargeFiles", func(t *testing.T) {
			testWorkerSkipsLargeFiles(t, inst)
		})

		t.Run("DeletedFile", func(t *testing.T) {
			testWorkerDeletedFile(t, inst)
		})

		t.Run("Disabled", func(t *testing.T) {
			testWorkerDisabled(t, inst)
		})
	})
}

func testWorkerCleanFile(t *testing.T, inst *instance.Instance) {
	// Create a test file
	fs := inst.VFS()
	doc := createTestFile(t, fs, "clean-test.txt", []byte("This is a clean file"))
	defer deleteTestFile(t, fs, doc)

	doc.AntivirusStatus = &vfs.AntivirusStatus{
		Status: antivirus.ScanStatusPending,
	}

	// Update the document
	err := couchdb.UpdateDoc(fs, doc)
	require.NoError(t, err)

	// Create and run the job
	msg, err := job.NewMessage(&antivirus.Message{FileID: doc.DocID})
	require.NoError(t, err)

	j := &job.Job{
		Domain:     inst.Domain,
		WorkerType: "antivirus",
		Message:    msg,
	}

	ctx, cancel := job.NewTaskContext("test", j, inst)
	defer cancel()
	err = antivirus.Worker(ctx)
	require.NoError(t, err)

	// Verify the file was updated
	updatedDoc, err := fs.FileByID(doc.DocID)
	require.NoError(t, err)
	require.NotNil(t, updatedDoc.AntivirusStatus)
	require.Equal(t, antivirus.ScanStatusClean, updatedDoc.AntivirusStatus.Status)
	require.NotNil(t, updatedDoc.AntivirusStatus.ScannedAt)
	require.Empty(t, updatedDoc.AntivirusStatus.VirusName)
}

func testWorkerInfectedFile(t *testing.T, inst *instance.Instance) {
	// Create a test file with EICAR signature
	fs := inst.VFS()
	doc := createTestFile(t, fs, "infected-test.txt", testutils.EICARTestSignature())
	defer deleteTestFile(t, fs, doc)

	doc.AntivirusStatus = &vfs.AntivirusStatus{
		Status: antivirus.ScanStatusPending,
	}
	err := couchdb.UpdateDoc(fs, doc)
	require.NoError(t, err)

	// Create and run the job
	msg, err := job.NewMessage(&antivirus.Message{FileID: doc.DocID})
	require.NoError(t, err)

	j := &job.Job{
		Domain:     inst.Domain,
		WorkerType: "antivirus",
		Message:    msg,
	}

	ctx, cancel := job.NewTaskContext("test", j, inst)
	defer cancel()
	err = antivirus.Worker(ctx)
	require.NoError(t, err)

	// Verify the file was updated
	updatedDoc, err := fs.FileByID(doc.DocID)
	require.NoError(t, err)
	require.NotNil(t, updatedDoc.AntivirusStatus)
	require.Equal(t, antivirus.ScanStatusInfected, updatedDoc.AntivirusStatus.Status)
	require.NotNil(t, updatedDoc.AntivirusStatus.ScannedAt)
	require.Contains(t, updatedDoc.AntivirusStatus.VirusName, "EICAR")
}

func testWorkerSkipsLargeFiles(t *testing.T, inst *instance.Instance) {
	// Configure antivirus with a small max file size
	conf := config.GetConfig()
	originalMaxFileSize := conf.Antivirus.MaxFileSize
	conf.Antivirus.MaxFileSize = 10 // 10 bytes
	defer func() {
		conf.Antivirus.MaxFileSize = originalMaxFileSize
	}()

	// Create a test file larger than max size
	fs := inst.VFS()
	largeContent := []byte("This is a file larger than 10 bytes")
	doc := createTestFile(t, fs, "large-test.txt", largeContent)
	defer deleteTestFile(t, fs, doc)

	// Create and run the job
	msg, err := job.NewMessage(&antivirus.Message{FileID: doc.DocID})
	require.NoError(t, err)

	j := &job.Job{
		Domain:     inst.Domain,
		WorkerType: "antivirus",
		Message:    msg,
	}

	ctx, cancel := job.NewTaskContext("test", j, inst)
	defer cancel()
	err = antivirus.Worker(ctx)
	require.NoError(t, err)

	// Verify the file was marked as skipped
	updatedDoc, err := fs.FileByID(doc.DocID)
	require.NoError(t, err)
	require.NotNil(t, updatedDoc.AntivirusStatus)
	require.Equal(t, antivirus.ScanStatusSkipped, updatedDoc.AntivirusStatus.Status)
}

func testWorkerDeletedFile(t *testing.T, inst *instance.Instance) {
	// Create and run the job for a non-existent file
	msg, err := job.NewMessage(&antivirus.Message{FileID: "non-existent-file-id"})
	require.NoError(t, err)

	j := &job.Job{
		Domain:     inst.Domain,
		WorkerType: "antivirus",
		Message:    msg,
	}

	ctx, cancel := job.NewTaskContext("test", j, inst)
	defer cancel()
	err = antivirus.Worker(ctx)
	// Should not error - just skip
	require.NoError(t, err)
}

func testWorkerDisabled(t *testing.T, inst *instance.Instance) {
	// Temporarily disable antivirus
	conf := config.GetConfig()
	originalEnabled := conf.Antivirus.Enabled
	conf.Antivirus.Enabled = false
	defer func() {
		conf.Antivirus.Enabled = originalEnabled
	}()

	// Create a test file
	fs := inst.VFS()
	doc := createTestFile(t, fs, "disabled-test.txt", []byte("test content"))
	defer deleteTestFile(t, fs, doc)

	// Create and run the job
	msg, err := job.NewMessage(&antivirus.Message{FileID: doc.DocID})
	require.NoError(t, err)

	j := &job.Job{
		Domain:     inst.Domain,
		WorkerType: "antivirus",
		Message:    msg,
	}

	ctx, cancel := job.NewTaskContext("test", j, inst)
	defer cancel()
	err = antivirus.Worker(ctx)
	require.NoError(t, err)

	// Verify the file was not updated (no scan status)
	updatedDoc, err := fs.FileByID(doc.DocID)
	require.NoError(t, err)
	require.Nil(t, updatedDoc.AntivirusStatus)
}

func createTestFile(t *testing.T, fs vfs.VFS, name string, content []byte) *vfs.FileDoc {
	t.Helper()
	parent, err := fs.DirByPath("/")
	require.NoError(t, err)
	doc, err := vfs.NewFileDoc(name, parent.DocID, int64(len(content)), nil, "text/plain", "text", time.Now(), false, false, false, nil)
	require.NoError(t, err)
	file, err := fs.CreateFile(doc, nil)
	require.NoError(t, err)
	_, err = file.Write(content)
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)
	updatedDoc, err := fs.FileByID(doc.DocID)
	require.NoError(t, err)
	return updatedDoc
}

func deleteTestFile(t *testing.T, fs vfs.VFS, doc *vfs.FileDoc) {
	t.Helper()
	_ = fs.DestroyFile(doc)
}
