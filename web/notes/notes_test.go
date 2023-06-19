package notes

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/note"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/files"
	webRealtime "github.com/cozy/cozy-stack/web/realtime"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotes(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var noteID string
	var version int64

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance()
	_, token := setup.GetTestClient(consts.Files)

	ts := setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/files":    files.Routes,
		"/notes":    Routes,
		"/realtime": webRealtime.Routes,
	})
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	t.Run("CreateNote", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/notes").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "data": {
          "type": "io.cozy.notes.documents",
          "attributes": {
            "title": "A super note",
            "schema": {
              "nodes": [
                ["doc", { "content": "block+" }],
                ["paragraph", { "content": "inline*", "group": "block" }],
                ["blockquote", { "content": "block+", "group": "block" }],
                ["horizontal_rule", { "group": "block" }],
                [
                  "heading",
                  {
                    "content": "inline*",
                    "group": "block",
                    "attrs": { "level": { "default": 1 } }
                  }
                ],
                ["code_block", { "content": "text*", "marks": "", "group": "block" }],
                ["text", { "group": "inline" }],
                [
                  "image",
                  {
                    "group": "inline",
                    "inline": true,
                    "attrs": { "alt": {}, "src": {}, "title": {} }
                  }
                ],
                ["hard_break", { "group": "inline", "inline": true }],
                [
                  "ordered_list",
                  {
                    "content": "list_item+",
                    "group": "block",
                    "attrs": { "order": { "default": 1 } }
                  }
                ],
                ["bullet_list", { "content": "list_item+", "group": "block" }],
                ["list_item", { "content": "paragraph block*" }]
              ],
              "marks": [
                ["link", { "attrs": { "href": {}, "title": {} }, "inclusive": false }],
                ["em", {}],
                ["strong", {}],
                ["code", {}]
              ],
              "topNode": "doc"
            }
          }
        }
      }`)).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		assertInitialNote(t, obj)

		noteID = obj.Path("$.data.id").String().NotEmpty().Raw()
	})

	t.Run("GetNote", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/notes/"+noteID).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		assertInitialNote(t, obj)
	})

	t.Run("OpenNote", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/notes/"+noteID+"/open").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", consts.NotesURL)
		data.ValueEqual("id", noteID)

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("note_id", noteID)
		attrs.ValueEqual("subdomain", "nested")
		attrs.ValueEqual("protocol", "https")
		attrs.ValueEqual("instance", inst.Domain)
		attrs.Value("public_name").String().NotEmpty()
	})

	t.Run("ChangeTitleAndSync", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.PUT("/notes/"+noteID+"/title").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "io.cozy.notes.documents",
          "attributes": {
            "sessionID": "543781490137",
            "title": "A new title"
          }
        }
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.files")
		data.ValueEqual("id", noteID)

		attrs := data.Value("attributes").Object()
		meta := attrs.Value("metadata").Object()

		meta.ValueEqual("title", "A new title")
		meta.ValueEqual("version", 0)
		meta.Value("schema").Object().NotEmpty()
		meta.Value("content").Object().NotEmpty()

		// The change was only made in cache, but we have to force persisting the
		// change to the VFS to check that renaming the file works.
		e.POST("/notes/"+noteID+"/sync").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(204)

		obj = e.GET("/notes/"+noteID).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data = obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.files")
		data.ValueEqual("id", noteID)

		attrs = data.Value("attributes").Object()
		attrs.ValueEqual("name", "A new title.cozy-note")

		meta = attrs.Value("metadata").Object()
		meta.ValueEqual("title", "A new title")
		meta.ValueEqual("version", 0)
		meta.Value("schema").Object().NotEmpty()
		meta.Value("content").Object().NotEmpty()
	})

	t.Run("ListNotes", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Change the title
		e.PUT("/notes/"+noteID+"/title").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "io.cozy.notes.documents",
          "attributes": {
            "sessionID": "543781490137",
            "title": "A title in cache"
          }
        }
      }`)).
			Expect().Status(200)

		// The title has been changed in cache, but we don't wait that the file has been renamed
		obj := e.GET("/notes").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Array()
		data.Length().Equal(1)

		doc := data.First().Object()
		doc.ValueEqual("type", "io.cozy.files")
		doc.ValueEqual("id", noteID)

		attrs := doc.Value("attributes").Object()
		attrs.ValueEqual("name", "A new title.cozy-note")
		attrs.Value("path").String().HasSuffix("/A new title.cozy-note")
		attrs.ValueEqual("mime", "text/vnd.cozy.note+markdown")

		meta := attrs.Value("metadata").Object()
		meta.ValueEqual("title", "A title in cache")
		meta.ValueEqual("version", 0)
		meta.Value("schema").Object().NotEmpty()
		meta.Value("content").Object().NotEmpty()
	})

	t.Run("PatchNote", func(t *testing.T) {
		body := []byte(`{
        "data": [{
          "type": "io.cozy.notes.steps",
          "attributes": {
            "sessionID": "543781490137",
            "stepType": "replace",
            "from": 1,
            "to": 1,
            "slice": {
              "content": [{ "type": "text", "text": "H" }]
            }
          }
        }, {
          "type": "io.cozy.notes.steps",
          "attributes": {
            "sessionID": "543781490137",
            "stepType": "replace",
            "from": 2,
            "to": 2,
            "slice": {
              "content": [{ "type": "text", "text": "ello" }]
            }
          }
        }]
      }`)

		t.Run("Success", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			obj := e.PATCH("/notes/"+noteID).
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-Type", "application/vnd.api+json").
				WithHeader("If-Match", "0").
				WithBytes(body).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()

			data := obj.Value("data").Object()
			data.ValueEqual("type", "io.cozy.files")
			data.ValueEqual("id", noteID)

			attrs := data.Value("attributes").Object()
			meta := attrs.Value("metadata").Object()

			version = int64(meta.Value("version").Number().Gt(0).Raw())
			meta.Value("schema").Object().NotEmpty()
			meta.Value("content").Object().NotEmpty()
		})

		t.Run("WithInvalidIfMatchHeader", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.PATCH("/notes/"+noteID).
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-Type", "application/vnd.api+json").
				WithHeader("If-Match", "0").
				WithBytes(body).
				Expect().Status(409)
		})
	})

	t.Run("GetSteps", func(t *testing.T) {
		var lastVersion int

		body := []byte(`{
      "data": [{
        "type": "io.cozy.notes.steps",
        "attributes": {
          "sessionID": "543781490137",
          "stepType": "replace",
          "from": 6,
          "to": 6,
          "slice": {
            "content": [{ "type": "text", "text": " " }]
          }
        }
      }, {
        "type": "io.cozy.notes.steps",
        "attributes": {
          "sessionID": "543781490137",
          "stepType": "replace",
          "from": 7,
          "to": 7,
          "slice": {
            "content": [{ "type": "text", "text": "world" }]
          }
        }
      }]
    }`)

		t.Run("Success", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			obj := e.PATCH("/notes/"+noteID).
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-Type", "application/vnd.api+json").
				WithHeader("If-Match", strconv.Itoa(int(version))).
				WithBytes(body).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()

			lastVersion = int(obj.Path("$.data.attributes.metadata.version").Number().Gt(0).Raw())
		})

		t.Run("GetStepsFromCurrentVersion", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			obj := e.GET("/notes/"+noteID+"/steps").
				WithQuery("Version", int(version)).
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-Type", "application/vnd.api+json").
				WithHeader("If-Match", strconv.Itoa(int(version))).
				WithBytes(body).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()

			obj.Path("$.meta.count").Number().Equal(2)
			obj.Path("$.data").Array().Length().Equal(2)

			first := obj.Path("$.data[0]").Object()
			first.Value("id").String().NotEmpty()

			attrs := first.Value("attributes").Object()
			attrs.ValueEqual("sessionID", "543781490137")
			attrs.ValueEqual("stepType", "replace")
			attrs.ValueEqual("from", 6)
			attrs.ValueEqual("to", 6)
			attrs.Value("version").Number()

			second := obj.Path("$.data[1]").Object()
			second.Value("id").String().NotEmpty()

			attrs = second.Value("attributes").Object()
			attrs.ValueEqual("sessionID", "543781490137")
			attrs.ValueEqual("stepType", "replace")
			attrs.ValueEqual("from", 7)
			attrs.ValueEqual("to", 7)
			attrs.ValueEqual("version", lastVersion)
		})

		t.Run("GetStepsFromLastVersion", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			obj := e.GET("/notes/"+noteID+"/steps").
				WithQuery("Version", lastVersion).
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-Type", "application/vnd.api+json").
				WithHeader("If-Match", strconv.Itoa(int(version))).
				WithBytes(body).
				Expect().Status(200).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()

			obj.Path("$.meta.count").Number().Equal(0)
			obj.Path("$.data").Array().Empty()

			version = int64(lastVersion)
		})
	})

	t.Run("PutSchema", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.PUT("/notes/"+noteID+"/schema").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "data": {
          "type": "io.cozy.notes.documents",
          "attributes": {
            "schema": {
              "nodes": [
                ["doc", { "content": "block+" }],
                [
                  "panel",
                  {
                    "content": "(paragraph | heading | bullet_list | ordered_list)+",
                    "group": "block",
                    "attrs": { "panelType": { "default": "info" } }
                  }
                ],
                ["paragraph", { "content": "inline*", "group": "block" }],
                ["blockquote", { "content": "block+", "group": "block" }],
                ["horizontal_rule", { "group": "block" }],
                [
                  "heading",
                  {
                    "content": "inline*",
                    "group": "block",
                    "attrs": { "level": { "default": 1 } }
                  }
                ],
                ["code_block", { "content": "text*", "marks": "", "group": "block" }],
                ["text", { "group": "inline" }],
                [
                  "image",
                  {
                    "group": "inline",
                    "inline": true,
                    "attrs": { "alt": {}, "src": {}, "title": {} }
                  }
                ],
                ["hard_break", { "group": "inline", "inline": true }],
                [
                  "ordered_list",
                  {
                    "content": "list_item+",
                    "group": "block",
                    "attrs": { "order": { "default": 1 } }
                  }
                ],
                ["bullet_list", { "content": "list_item+", "group": "block" }],
                ["list_item", { "content": "paragraph block*" }]
              ],
              "marks": [
                ["link", { "attrs": { "href": {}, "title": {} }, "inclusive": false }],
                ["em", {}],
                ["strong", {}],
                ["code", {}]
              ],
              "version": 2,
              "topNode": "doc"
            }
          }
        }
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.files")
		data.ValueEqual("id", noteID)

		schema := obj.Path("$.data.attributes.metadata.schema").Object()
		schema.ValueEqual("version", 2)
		schema.Path("$.nodes[1][0]").Equal("panel")

		// TODO: add an explanation why we need this sleep period
		time.Sleep(1 * time.Second)

		e.GET("/notes/"+noteID+"/steps").
			WithQuery("Version", version).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(412)

		version = int64(obj.Path("$.data.attributes.metadata.version").Number().Raw())
	})

	t.Run("PutTelepointer", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			sub := realtime.GetHub().Subscriber(inst)
			sub.Subscribe(consts.NotesEvents)

			// Suscribtion ok, unlock the first wait
			wg.Done()

			e := <-sub.Channel
			assert.Equal(t, "UPDATED", e.Verb)
			assert.Equal(t, noteID, e.Doc.ID())
			doc, ok := e.Doc.(note.Event)
			assert.True(t, ok)
			assert.Equal(t, consts.NotesTelepointers, doc["doctype"])
			assert.Equal(t, "543781490137", doc["sessionID"])
			assert.Equal(t, "textSelection", doc["type"])
			assert.EqualValues(t, 7, doc["anchor"])
			assert.EqualValues(t, 12, doc["head"])

			// Event received and validated, unlock the second wait.
			wg.Done()
		}()

		// Wait that the goroutine has subscribed to the realtime
		wg.Wait()

		wg.Add(1)
		e.PUT("/notes/"+noteID+"/telepointer").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "data": {
          "type": "io.cozy.notes.telepointers",
          "attributes": {
            "sessionID": "543781490137",
            "anchor": 7,
            "head": 12,
            "type": "textSelection"
          }
        }
      }`)).
			Expect().Status(204)

		// Wait that the goroutine has received the telepointer update
		wg.Wait()
	})

	t.Run("NoteMarkdown", func(t *testing.T) {
		// Force the changes to the VFS
		err := note.Update(inst, noteID)
		assert.NoError(t, err)
		doc, err := inst.VFS().FileByID(noteID)
		assert.NoError(t, err)
		file, err := inst.VFS().OpenFile(doc)
		assert.NoError(t, err)
		defer file.Close()
		buf, err := io.ReadAll(file)
		assert.NoError(t, err)
		assert.Equal(t, "Hello world", string(buf))
	})

	t.Run("NoteRealtime", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		ws := e.GET("/realtime/").
			WithWebsocketUpgrade().
			Expect().Status(http.StatusSwitchingProtocols).
			Websocket()
		defer ws.Disconnect()

		ws.WriteText(fmt.Sprintf(`{"method": "AUTH", "payload": "%s"}`, token))

		ws.WriteText(`{"method": "SUBSCRIBE", "payload": { "type": "io.cozy.notes.events", "id": "` + noteID + `" }}`)

		// To check that the realtime has made the subscription, we send a fake
		// message and wait for its response.
		ws.WriteText(`{"method": "PING"}`).
			Expect().TextMessage().
			JSON()

		pointer := note.Event{
			"sessionID": "543781490137",
			"anchor":    7,
			"head":      12,
			"type":      "textSelection",
		}
		pointer.SetID(noteID)
		err := note.PutTelepointer(inst, pointer)
		assert.NoError(t, err)

		obj := ws.Expect().TextMessage().
			JSON().Object()

		obj.ValueEqual("event", "UPDATED")
		payload := obj.Value("payload").Object()
		payload.ValueEqual("id", noteID)
		payload.ValueEqual("type", "io.cozy.notes.events")

		doc := payload.Value("doc").Object()
		doc.ValueEqual("doctype", "io.cozy.notes.telepointers")
		doc.ValueEqual("sessionID", "543781490137")
		doc.ValueEqual("anchor", 7)
		doc.ValueEqual("head", 12)
		doc.ValueEqual("type", "textSelection")

		file, err := inst.VFS().FileByID(noteID)
		require.NoError(t, err)
		file, err = note.UpdateTitle(inst, file, "A very new title", "543781490137")
		require.NoError(t, err)

		obj = ws.Expect().TextMessage().
			JSON().Object()

		obj.ValueEqual("event", "UPDATED")
		payload = obj.Value("payload").Object()
		payload.ValueEqual("id", noteID)
		payload.ValueEqual("type", "io.cozy.notes.events")

		doc = payload.Value("doc").Object()
		doc.ValueEqual("doctype", "io.cozy.notes.documents")
		doc.ValueEqual("title", "A very new title")
		doc.ValueEqual("sessionID", "543781490137")

		slice := map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": "X"},
			},
		}
		steps := []note.Step{
			{"sessionID": "543781490137", "stepType": "replace", "from": 2, "to": 2, "slice": slice},
			{"sessionID": "543781490137", "stepType": "replace", "from": 3, "to": 3, "slice": slice},
		}
		file, err = note.ApplySteps(inst, file, fmt.Sprintf("%d", version), steps)
		require.NoError(t, err)

		obj = ws.Expect().TextMessage().
			JSON().Object()

		obj.ValueEqual("event", "UPDATED")
		payload = obj.Value("payload").Object()
		payload.ValueEqual("id", noteID)
		payload.ValueEqual("type", "io.cozy.notes.events")

		doc4 := payload.Value("doc").Object()

		obj = ws.Expect().TextMessage().
			JSON().Object()

		obj.ValueEqual("event", "UPDATED")
		payload = obj.Value("payload").Object()
		payload.ValueEqual("id", noteID)
		payload.ValueEqual("type", "io.cozy.notes.events")
		doc5 := payload.Value("doc").Object()

		// // In some cases, the steps can be received in the bad order because of the
		// // concurrency between the goroutines in the realtime hub.
		if doc4.Value("version").Number().Raw() > doc5.Value("version").Number().Raw() {
			doc4, doc5 = doc5, doc4
		}

		doc4.ValueEqual("doctype", "io.cozy.notes.steps")
		doc4.ValueEqual("sessionID", "543781490137")
		doc4.ValueEqual("stepType", "replace")
		doc4.ValueEqual("from", 2)
		doc4.ValueEqual("to", 2)
		vers4 := int(doc4.Value("version").Number().Gt(0).Raw())

		doc5.ValueEqual("doctype", "io.cozy.notes.steps")
		doc5.ValueEqual("sessionID", "543781490137")
		doc5.ValueEqual("stepType", "replace")
		doc5.ValueEqual("from", 3)
		doc5.ValueEqual("to", 3)
		vers5 := int(doc5.Value("version").Number().
			NotEqual(0).
			NotEqual(vers4).
			Raw())

		assert.EqualValues(t, file.Metadata["version"], vers5)
	})

	t.Run("UploadImage", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		rawFile, err := os.ReadFile("../../tests/fixtures/wet-cozy_20160910__M4Dz.jpg")
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			obj := e.POST("/notes/"+noteID+"/images").
				WithQuery("Name", "wet.jpg").
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-Type", "image/jpeg").
				WithBytes(rawFile).
				Expect().Status(201).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
				Object()

			data := obj.Value("data").Object()
			data.ValueEqual("type", consts.NotesImages)
			data.Value("id").String().NotEmpty()
			data.Value("meta").Object().NotEmpty()

			attrs := data.Value("attributes").Object()
			if i == 0 {
				attrs.ValueEqual("name", "wet.jpg")
			} else {
				attrs.ValueEqual("name", fmt.Sprintf("wet (%d).jpg", i+1))
			}

			attrs.Value("cozyMetadata").Object().NotEmpty()
			attrs.ValueEqual("mime", "image/jpeg")
			attrs.ValueEqual("width", 440)
			attrs.ValueEqual("height", 294)

			data.Path("$.links.self").String().NotEmpty()
		}
	})

	t.Run("GetImage", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		rawFile, err := os.ReadFile("../../tests/fixtures/wet-cozy_20160910__M4Dz.jpg")
		require.NoError(t, err)

		obj := e.POST("/notes/"+noteID+"/images").
			WithQuery("Name", "wet.jpg").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "image/jpeg").
			WithBytes(rawFile).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", consts.NotesImages)
		data.Value("id").String().NotEmpty()
		data.Value("meta").Object().NotEmpty()

		link := data.Path("$.links.self").String().NotEmpty().Raw()

		e.GET(link).
			Expect().Status(200).
			Body().Equal(string(rawFile))

		obj = e.GET("/files/"+noteID).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		image := obj.Value("included").Array().
			Find(func(_ int, value *httpexpect.Value) bool {
				value.Object().ValueNotEqual("type", consts.FilesVersions)
				return true
			}).
			Object()

		image.ValueEqual("type", consts.NotesImages)
		image.Value("id").String().NotEmpty()
		image.Value("meta").Object().NotEmpty()

		attrs := image.Value("attributes").Object()
		attrs.Value("name").String().NotEmpty()
		attrs.Value("cozyMetadata").Object().NotEmpty()
		attrs.ValueEqual("mime", "image/jpeg")

		data.Path("$.links.self").String().NotEmpty()
	})

	t.Run("ImportNotes", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/files/io.cozy.files.root-dir").
			WithQuery("Type", "file").
			WithQuery("Name", "An imported note.cozy-note").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "text/plain").
			WithBytes([]byte(`
        # Title

        Text with **bold** and [underlined]{.underlined}.
      `)).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.files")
		fileID := data.Value("id").String().NotEmpty().Raw()

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("type", "file")
		attrs.ValueEqual("name", "An imported note.cozy-note")
		attrs.ValueEqual("mime", "text/vnd.cozy.note+markdown")

		meta := attrs.Value("metadata").Object()
		meta.ValueEqual("title", "An imported note")
		meta.Value("schema").Object().NotEmpty()
		meta.Value("content").Object().NotEmpty()

		obj = e.GET("/notes/"+fileID+"/open").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data = obj.Value("data").Object()
		data.ValueEqual("id", fileID)
		data.Path("$.attributes.instance").Equal(inst.Domain)
	})
}

func assertInitialNote(t *testing.T, obj *httpexpect.Object) {
	data := obj.Value("data").Object()

	data.ValueEqual("type", "io.cozy.files")
	data.Value("id").String().NotEmpty()

	attrs := data.Value("attributes").Object()
	attrs.ValueEqual("type", "file")
	attrs.ValueEqual("name", "A super note.cozy-note")
	attrs.ValueEqual("mime", "text/vnd.cozy.note+markdown")

	fcm := attrs.Value("cozyMetadata").Object()
	fcm.Value("createdAt").String().DateTime(time.RFC3339)
	fcm.Value("createdOn").String().NotEmpty()

	meta := attrs.Value("metadata").Object()
	meta.ValueEqual("title", "A super note")
	meta.ValueEqual("version", 0)
	meta.Value("schema").Object().NotEmpty()
	meta.Value("content").Object().NotEmpty()
}
