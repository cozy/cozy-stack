package instance

import "github.com/cozy/cozy-stack/pkg/jobs"

// Triggers is the list of the triggers to add when an instance is created
var Triggers = []jobs.TriggerInfos{
	// Create/update/remove thumbnails when an image is created/updated/removed
	jobs.TriggerInfos{
		Type:       "@event",
		WorkerType: "thumbnail",
		Arguments:  "io.cozy.files:CREATED,UPDATED,DELETED:image:class",
	},
}
