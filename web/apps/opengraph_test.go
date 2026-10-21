package apps

import (
	"encoding/json"
	"testing"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOpenGraphImageName(t *testing.T) {
	assert.Equal(t, "folder", getOpenGraphImageName(&vfs.DirDoc{}, nil))
	assert.Equal(t, "generic", getOpenGraphImageName(nil, nil))
	assert.Equal(t, "note", getOpenGraphImageName(nil, &vfs.FileDoc{Mime: consts.NoteMimeType, Class: "text"}))
	assert.Equal(t, "spreadsheet", getOpenGraphImageName(nil, &vfs.FileDoc{Class: "spreadsheet"}))
	assert.Equal(t, "text", getOpenGraphImageName(nil, &vfs.FileDoc{Class: "text"}))
	assert.Equal(t, "generic", getOpenGraphImageName(nil, &vfs.FileDoc{Class: "binary"}))
	assert.Equal(t, "generic", getOpenGraphImageName(nil, &vfs.FileDoc{Class: "shortcut"}))
}

func TestNewOpenGraph(t *testing.T) {
	config.UseTestFile(t)
	var webapp app.WebappManifest
	require.NoError(t, json.Unmarshal([]byte(`{"slug":"drive","name":"Drive","name_prefix":"Twake"}`), &webapp))
	inst := &instance.Instance{Domain: "alice.example.net"}

	assert.Nil(t, newOpenGraph(inst, &webapp, &permission.Permission{Password: "hash"}))

	og := newOpenGraph(inst, &webapp, &permission.Permission{})
	assert.Equal(t, "Twake Drive", og.Title)
	assert.Contains(t, og.Image, "/assets/images/opengraph/generic.")
	assert.Contains(t, string(og.HTML()), `<meta property="og:title" content="Twake Drive" />`)
	assert.Empty(t, (*openGraph)(nil).HTML())
}
