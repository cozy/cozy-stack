package job

import (
	"errors"

	"github.com/cozy/cozy-stack/pkg/limits"
)

// GetCounterTypeFromWorkerType returns the CounterTypeFromWorkerType
func GetCounterTypeFromWorkerType(workerType string) (limits.CounterType, error) {
	switch workerType {
	case "thumbnail":
		return limits.JobThumbnailType, nil
	case "share-track":
		return limits.JobShareTrackType, nil
	case "share-replicate":
		return limits.JobShareReplicateType, nil
	case "share-upload":
		return limits.JobShareUploadType, nil
	default:
		return -1, errors.New("CounterType was not found")
	}
}
