package intent

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
)

var ins *instance.Instance

func TestIntents(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()

	ins = &instance.Instance{Domain: "cozy.example.net"}

	if err := couchdb.ResetDB(ins, consts.Apps); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	g, _ := errgroup.WithContext(context.Background())
	couchdb.DefineIndexes(g, ins, couchdb.IndexesByDoctype(consts.Apps))
	if err := g.Wait(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	t.Cleanup(func() {
		_ = couchdb.DeleteDB(ins, consts.Apps)
	})

	t.Run("GenerateHref", func(t *testing.T) {
		intent := &Intent{IID: "6fba9dd6-1487-11e7-b90d-130a5dedd6d6"}

		href := intent.GenerateHref(ins, "files", "/pick")
		assert.Equal(t, "https://files.cozy.example.net/pick?intent=6fba9dd6-1487-11e7-b90d-130a5dedd6d6", href)

		href = intent.GenerateHref(ins, "files", "/view")
		assert.Equal(t, "https://files.cozy.example.net/view?intent=6fba9dd6-1487-11e7-b90d-130a5dedd6d6", href)
	})

	t.Run("FillServices", func(t *testing.T) {
		files := &couchdb.JSONDoc{
			Type: consts.Apps,
			M: map[string]interface{}{
				"_id":  consts.Apps + "/files",
				"slug": "files",
				"intents": []app.Intent{
					{
						Action: "PICK",
						Types:  []string{"io.cozy.files", "image/gif"},
						Href:   "/pick",
					},
				},
			},
		}
		err := couchdb.CreateNamedDoc(ins, files)
		assert.NoError(t, err)
		photos := &couchdb.JSONDoc{
			Type: consts.Apps,
			M: map[string]interface{}{
				"_id":  consts.Apps + "/photos",
				"slug": "photos",
				"intents": []app.Intent{
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
	})

	t.Run("FillAvailableWebapps", func(t *testing.T) {
		intent := &Intent{
			IID:    "6b44d8d0-148b-11e7-a1cf-a38d75a77df6",
			Action: "REDIRECT",
			Type:   "io.cozy.accounts",
		}
		err := intent.FillAvailableWebapps(ins)
		assert.NoError(t, err)

		// Should have Home and Collect
		assert.Equal(t, 2, len(intent.AvailableApps))

		res := map[string]interface{}{}
		for _, v := range intent.AvailableApps {
			res[v.Slug] = struct{}{}
		}

		assert.Contains(t, res, "home")
		assert.Contains(t, res, "collect")
	})

}
