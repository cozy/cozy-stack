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
	default:
		return -1, errors.New("CounterType was not found")
	}
}
