package workers

import (
	"encoding/json"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/stretchr/testify/assert"
)

func TestKonnectorWorker(t *testing.T) {
	fields, err := json.Marshal(&struct{ Password string }{Password: "mypass"})
	assert.NoError(t, err)

	ctx := jobs.NewWorkerContext("cozy.local")
	msg, err := jobs.NewMessage(jobs.JSONEncoding, &KonnectorOptions{
		Slug:   "slug",
		Fields: fields,
	})
	assert.NoError(t, err)

	config.GetConfig().Konnectors.Cmd = ""
	err = KonnectorWorker(ctx, msg)
	assert.Error(t, err)
	assert.Equal(t, "fork/exec : no such file or directory", err.Error())

	config.GetConfig().Konnectors.Cmd = "echo"
	err = KonnectorWorker(ctx, msg)
	assert.NoError(t, err)
}
