package files

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/note"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
)

type apiNoteImage struct {
	inst *instance.Instance
	doc  *note.Image
}

// NewNoteImage creates an object that can be used to serialize an image for a
// note to JSON-API.
func NewNoteImage(inst *instance.Instance, img *note.Image) *apiNoteImage {
	return &apiNoteImage{inst: inst, doc: img}
}

func (img *apiNoteImage) ID() string                             { return img.doc.ID() }
func (img *apiNoteImage) Rev() string                            { return img.doc.Rev() }
func (img *apiNoteImage) SetID(id string)                        { img.doc.SetID(id) }
func (img *apiNoteImage) SetRev(rev string)                      { img.doc.SetRev(rev) }
func (img *apiNoteImage) DocType() string                        { return img.doc.DocType() }
func (img *apiNoteImage) Clone() couchdb.Doc                     { cloned := *img; return &cloned }
func (img *apiNoteImage) MarshalJSON() ([]byte, error)           { return json.Marshal(img.doc) }
func (img *apiNoteImage) Relationships() jsonapi.RelationshipMap { return nil }
func (img *apiNoteImage) Included() []jsonapi.Object             { return nil }
func (img *apiNoteImage) Links() *jsonapi.LinksList {
	links := jsonapi.LinksList{}
	if secret, err := vfs.GetStore().AddThumb(img.inst, img.doc.ID()); err == nil {
		parts := strings.SplitN(img.doc.ID(), "/", 2)
		links.Self = fmt.Sprintf("/notes/%s/images/%s/%s", parts[0], parts[1], secret)
	}
	return &links
}

var _ jsonapi.Object = (*apiNoteImage)(nil)
