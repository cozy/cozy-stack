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

const (
	ScanStatusPending  = "pending"
	ScanStatusClean    = "clean"
	ScanStatusInfected = "infected"
	ScanStatusError    = "error"
	ScanStatusSkipped  = "skipped"
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

	// Check if antivirus is enabled for this instance
	if !isEnabledForInstance(inst) {
		log.Infof("Antivirus is disabled for this context, skipping scan")
		return nil
	}

	log.Infof("Scanning file %s", msg.FileID)

	// Get the file document
	fs := inst.VFS()
	file, err := fs.FileByID(msg.FileID)
	if err != nil {
		if os.IsNotExist(err) || couchdb.IsNotFoundError(err) {
			log.Infof("File %s not found, skipping scan", msg.FileID)
			return nil // File was deleted, nothing to do
		}
		return err
	}

	avConfig := config.GetConfig().Antivirus

	if avConfig.MaxFileSize > 0 && file.ByteSize > avConfig.MaxFileSize {
		log.Infof("File %s is too large (%d bytes), skipping scan", msg.FileID, file.ByteSize)
		return updateScanStatus(fs, file, &vfs.AntivirusStatus{
			Status: ScanStatusSkipped,
		})
	}

	// Create ClamAV client
	timeout := avConfig.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	client := clamav.NewClient(avConfig.Address, timeout)

	content, err := fs.OpenFile(file)
	if err != nil {
		log.Errorf("Failed to open file %s: %v", msg.FileID, err)
		return updateScanStatus(fs, file, &vfs.AntivirusStatus{
			Status: ScanStatusError,
			Error:  err.Error(),
		})
	}
	defer content.Close()

	// Scan the file
	result, err := client.Scan(ctx, content)
	if err != nil {
		log.Errorf("Failed to scan file %s: %v", msg.FileID, err)
		return updateScanStatus(fs, file, &vfs.AntivirusStatus{
			Status: ScanStatusError,
			Error:  err.Error(),
		})
	}

	// Update file with scan result
	now := time.Now()
	scan := &vfs.AntivirusStatus{
		ScannedAt: &now,
	}

	switch result.Status {
	case clamav.StatusClean:
		scan.Status = ScanStatusClean
	case clamav.StatusInfected:
		log.Warnf("File %s is infected with %s", msg.FileID, result.VirusName)
		scan.Status = ScanStatusInfected
		scan.VirusName = result.VirusName
	default:
		log.Errorf("Scan error for file %s: %s", msg.FileID, result.Error)
		scan.Status = ScanStatusError
		scan.Error = result.Error
	}

	return updateScanStatus(fs, file, scan)
}

// updateScanStatus updates the file document with the scan status
func updateScanStatus(fs vfs.VFS, file *vfs.FileDoc, scan *vfs.AntivirusStatus) error {
	file, err := fs.FileByID(file.DocID)
	if err != nil {
		return err
	}
	newdoc := file.Clone().(*vfs.FileDoc)
	newdoc.AntivirusStatus = scan
	return couchdb.UpdateDoc(fs, newdoc)
}

// PushScanJob pushes an antivirus scan job for a file
func PushScanJob(inst *instance.Instance, fileID string) (*job.Job, error) {
	if !isEnabledForInstance(inst) {
		return nil, nil // Antivirus disabled for this context
	}

	msg, err := job.NewMessage(&Message{FileID: fileID})
	if err != nil {
		return nil, err
	}

	return job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "antivirus",
		Message:    msg,
	})
}

// isEnabledForInstance returns true if antivirus is enabled for the given instance
func isEnabledForInstance(inst *instance.Instance) bool {
	cfg := config.GetAntivirusConfig(inst.ContextName)
	return cfg != nil && cfg.Enabled
}
