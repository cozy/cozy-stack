package intent

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/stretchr/testify/assert"
)

var ins *instance.Instance

func TestGenerateHref(t *testing.T) {
	intent := &Intent{IID: "6fba9dd6-1487-11e7-b90d-130a5dedd6d6"}

	href := intent.GenerateHref(ins, "files", "/pick")
	assert.Equal(t, "https://files.cozy.example.net/pick?intent=6fba9dd6-1487-11e7-b90d-130a5dedd6d6", href)

	href = intent.GenerateHref(ins, "files", "/view")
	assert.Equal(t, "https://files.cozy.example.net/view?intent=6fba9dd6-1487-11e7-b90d-130a5dedd6d6", href)
}

func TestFillServices(t *testing.T) {
	files := &app.WebappManifest{
		DocID:   consts.Apps + "/files",
		DocSlug: "files",
		Intents: []app.Intent{
			{
				Action: "PICK",
				Types:  []string{"io.cozy.files", "image/gif"},
				Href:   "/pick",
			},
		},
	}
	err := couchdb.CreateNamedDoc(ins, files)
	assert.NoError(t, err)
	photos := &app.WebappManifest{
		DocID:   consts.Apps + "/photos",
		DocSlug: "photos",
		Intents: []app.Intent{
			{
				Action: "PICK",
				Types:  []string{"image/*"},
				Href:   "/picker",
			},
			{
				Action: "VIEW",
				Types:  []string{"io.cozy.files"},
				Href:   "/viewer",
			},
		},
	}
	err = couchdb.CreateNamedDoc(ins, photos)
	assert.NoError(t, err)

	intent := &Intent{
		IID:    "6b44d8d0-148b-11e7-a1cf-a38d75a77df6",
		Action: "PICK",
		Type:   "io.cozy.files",
	}
	err = intent.FillServices(ins)
	assert.NoError(t, err)
	assert.Len(t, intent.Services, 1)
	service := intent.Services[0]
	assert.Equal(t, "files", service.Slug)
	assert.Equal(t, "https://files.cozy.example.net/pick?intent=6b44d8d0-148b-11e7-a1cf-a38d75a77df6", service.Href)

	intent = &Intent{
		IID:    "6b44d8d0-148b-11e7-a1cf-a38d75a77df6",
		Action: "view",
		Type:   "io.cozy.files",
	}
	err = intent.FillServices(ins)
	assert.NoError(t, err)
	assert.Len(t, intent.Services, 1)
	service = intent.Services[0]
	assert.Equal(t, "photos", service.Slug)
	assert.Equal(t, "https://photos.cozy.example.net/viewer?intent=6b44d8d0-148b-11e7-a1cf-a38d75a77df6", service.Href)

	intent = &Intent{
		IID:    "6b44d8d0-148b-11e7-a1cf-a38d75a77df6",
		Action: "PICK",
		Type:   "image/gif",
	}
	err = intent.FillServices(ins)
	assert.NoError(t, err)
	assert.Len(t, intent.Services, 2)
	service = intent.Services[0]
	assert.Equal(t, "files", service.Slug)
	assert.Equal(t, "https://files.cozy.example.net/pick?intent=6b44d8d0-148b-11e7-a1cf-a38d75a77df6", service.Href)
	service = intent.Services[1]
	assert.Equal(t, "photos", service.Slug)
	assert.Equal(t, "https://photos.cozy.example.net/picker?intent=6b44d8d0-148b-11e7-a1cf-a38d75a77df6", service.Href)

	intent = &Intent{
		IID:    "6b44d8d0-148b-11e7-a1cf-a38d75a77df6",
		Action: "VIEW",
		Type:   "image/gif",
	}
	err = intent.FillServices(ins)
	assert.NoError(t, err)
	assert.Len(t, intent.Services, 0)

}

func TestMain(m *testing.M) {
	config.UseTestFile()

	ins = &instance.Instance{Domain: "cozy.example.net"}

	if err := couchdb.ResetDB(ins, consts.Apps); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if err := couchdb.DefineIndexes(ins, couchdb.IndexesByDoctype(consts.Apps)); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	res := m.Run()

	_ = couchdb.DeleteDB(ins, consts.Apps)

	os.Exit(res)
}
