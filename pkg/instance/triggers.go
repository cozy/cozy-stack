package instance

import (
	"github.com/cozy/cozy-stack/pkg/jobs"
)

// Triggers returns the list of the triggers to add when an instance is created
func Triggers(domain string) []jobs.TriggerInfos {
	// Create/update/remove thumbnails when an image is created/updated/removed
	return []jobs.TriggerInfos{
		{
			Domain:     domain,
			Type:       "@event",
			WorkerType: "thumbnail",
			Arguments:  "io.cozy.files:CREATED,UPDATED,DELETED:image:class",
		},
	}
}
