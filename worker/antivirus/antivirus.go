package antivirus

import (
	"os"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/clamav"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "antivirus",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 3,
		Timeout:      10 * time.Minute,
		WorkerFunc:   Worker,
	})
}

// Message is the structure of the antivirus job message
type Message struct {
	FileID string `json:"file_id"`
}

// Worker is the antivirus worker function
func Worker(ctx *job.TaskContext) error {
	var msg Message
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}

	inst := ctx.Instance
	log := inst.Logger().WithNamespace("antivirus")

	log.Debugf("Starting antivirus job for file %s", msg.FileID)

	// Check if antivirus is enabled for this instance
	if !isEnabledForInstance(inst) {
		log.Debugf("Antivirus is disabled for context %s, skipping scan", inst.ContextName)
		return nil
	}

	log.Infof("Scanning file %s", msg.FileID)

	// Get the file document
	fs := inst.VFS()
	file, err := fs.FileByID(msg.FileID)
	if err != nil {
		if os.IsNotExist(err) || couchdb.IsNotFoundError(err) {
			log.Debugf("File %s not found (possibly deleted), skipping scan", msg.FileID)
			return nil // File was deleted, nothing to do
		}
		log.Errorf("Failed to get file %s: %v", msg.FileID, err)
		return err
	}

	log.Debugf("File %s: name=%s, size=%d bytes, mime=%s", msg.FileID, file.DocName, file.ByteSize, file.Mime)

	avConfig := config.GetConfig().Antivirus

	if avConfig.MaxFileSize > 0 && file.ByteSize > avConfig.MaxFileSize {
		log.Infof("File %s is too large (%d bytes > %d max), skipping scan", msg.FileID, file.ByteSize, avConfig.MaxFileSize)
		return updateScanStatus(fs, file, &vfs.AntivirusStatus{
			Status: vfs.AVStatusSkipped,
		})
	}

	// Create ClamAV client
	timeout := avConfig.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	log.Debugf("Creating ClamAV client: address=%s, timeout=%v", avConfig.Address, timeout)
	client := clamav.NewClient(avConfig.Address, timeout, log)

	log.Debugf("Opening file %s for scanning", msg.FileID)
	content, err := fs.OpenFile(file)
	if err != nil {
		log.Errorf("Failed to open file %s: %v", msg.FileID, err)
		return updateScanStatus(fs, file, &vfs.AntivirusStatus{
			Status: vfs.AVStatusError,
			Error:  err.Error(),
		})
	}
	defer content.Close()

	// Scan the file
	log.Debugf("Sending file %s to ClamAV for scanning", msg.FileID)
	result, err := client.Scan(ctx, content)
	if err != nil {
		log.Errorf("Failed to scan the file %s: %v", msg.FileID, err)
		return updateScanStatus(fs, file, &vfs.AntivirusStatus{
			Status: vfs.AVStatusError,
			Error:  err.Error(),
		})
	}

	// Update file with scan result
	now := time.Now()
	st := &vfs.AntivirusStatus{
		ScannedAt: &now,
	}

	switch result.Status {
	case clamav.StatusClean:
		log.Debugf("File %s is clean", msg.FileID)
		st.Status = vfs.AVStatusClean
	case clamav.StatusInfected:
		log.Warnf("File %s is infected with %s", msg.FileID, result.VirusName)
		st.Status = vfs.AVStatusInfected
		st.VirusName = result.VirusName
	default:
		log.Errorf("Scan error for file %s: %s", msg.FileID, result.Error)
		st.Status = vfs.AVStatusError
		st.Error = result.Error
	}

	log.Debugf("Updating scan status for file %s: status=%s", msg.FileID, st.Status)
	return updateScanStatus(fs, file, st)
}

// updateScanStatus updates the file document with the scan status.
// It retries on conflict errors to handle race conditions with the trigger.
func updateScanStatus(fs vfs.VFS, file *vfs.FileDoc, scan *vfs.AntivirusStatus) error {
	const maxRetries = 3
	var err error

	for i := 0; i < maxRetries; i++ {
		var f *vfs.FileDoc
		f, err = fs.FileByID(file.DocID)
		if err != nil {
			return err
		}
		newdoc := f.Clone().(*vfs.FileDoc)
		newdoc.AntivirusStatus = scan
		err = couchdb.UpdateDoc(fs, newdoc)
		if err == nil {
			return nil
		}
		if !couchdb.IsConflictError(err) {
			return err
		}
		// Conflict error, retry with fresh document
	}
	return err
}

// isEnabledForInstance returns true if antivirus is enabled for the given instance
func isEnabledForInstance(inst *instance.Instance) bool {
	cfg := config.GetAntivirusConfig(inst.ContextName)
	return cfg != nil && cfg.Enabled
}
