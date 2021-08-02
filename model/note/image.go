package note

import (
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/gofrs/uuid"
)

// MaxWidth is the maximal width of an image for a note. If larger, the image
// will be resized.
const MaxWidth = 768

// MaxImageWeight is the maximal weight (in bytes) for an image.
const MaxImageWeight = 100 * 1024 * 1024

// Image is a file that will be persisted inside the note archive.
type Image struct {
	DocID    string                `json:"_id,omitempty"`
	DocRev   string                `json:"_rev,omitempty"`
	Name     string                `json:"name"`
	Mime     string                `json:"mime"`
	Width    int                   `json:"width,omitempty"`
	Height   int                   `json:"height,omitempty"`
	ToResize bool                  `json:"willBeResized,omitempty"`
	Metadata metadata.CozyMetadata `json:"cozyMetadata,omitempty"`
}

// ID returns the image qualified identifier
func (img *Image) ID() string { return img.DocID }

// Rev returns the image revision
func (img *Image) Rev() string { return img.DocRev }

// DocType returns the image type
func (img *Image) DocType() string { return consts.NotesImages }

// Clone implements couchdb.Doc
func (img *Image) Clone() couchdb.Doc {
	cloned := *img
	return &cloned
}

// SetID changes the image qualified identifier
func (img *Image) SetID(id string) { img.DocID = id }

// SetRev changes the image revision
func (img *Image) SetRev(rev string) { img.DocRev = rev }

// ImageUpload is used while an image is uploaded to the stack.
type ImageUpload struct {
	Image *Image
	note  *vfs.FileDoc
	inst  *instance.Instance
	meta  *vfs.MetaExtractor // extracts metadata from the content
	thumb vfs.ThumbFiler     // the VFs where the file will be uploaded
}

// NewImageUpload can be used to manage uploading a new image for a note.
func NewImageUpload(inst *instance.Instance, note *vfs.FileDoc, name, mime string) (*ImageUpload, error) {
	uuidv4, _ := uuid.NewV4()
	id := note.ID() + "/" + uuidv4.String()
	md := metadata.New()
	md.CreatedByApp = consts.NotesSlug
	img := &Image{DocID: id, Name: name, Mime: mime, Metadata: *md}

	thumb, err := inst.ThumbsFS().CreateNoteThumb(id, mime)
	if err != nil {
		return nil, err
	}

	var meta vfs.MetaExtractor
	switch mime {
	case "image/heic", "image/heif":
		meta = vfs.NewExifExtractor(img.Metadata.CreatedAt, false)
	default:
		meta = vfs.NewImageExtractor(img.Metadata.CreatedAt)
	}

	upload := ImageUpload{inst: inst, note: note, meta: &meta, thumb: thumb, Image: img}
	return &upload, nil
}

// Write implements the io.Writer interface (used by io.Copy).
func (u *ImageUpload) Write(p []byte) (int, error) {
	if u.meta != nil {
		if _, err := (*u.meta).Write(p); err != nil && err != io.ErrClosedPipe {
			(*u.meta).Abort(err)
			u.meta = nil
		}
	}
	return u.thumb.Write(p)
}

// Close is called to finalize an upload.
func (u *ImageUpload) Close() error {
	lock := u.inst.NotesLock()
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()

	if err := u.thumb.Commit(); err != nil {
		if u.meta != nil {
			(*u.meta).Abort(err)
		}
		return err
	}

	if u.meta != nil {
		if errc := (*u.meta).Close(); errc == nil {
			result := (*u.meta).Result()
			if w, ok := result["width"].(int); ok {
				u.Image.Width = w
				if w > MaxWidth {
					u.Image.ToResize = true
				}
			}
			if h, ok := result["height"].(int); ok {
				u.Image.Height = h
			}
		}
	}

	// Check the unicity of the filename
	if images, err := getImages(u.inst, u.note.ID()); err == nil {
		names := make([]string, len(images))
		for i := range images {
			names[i] = images[i].Name
		}
		ext := path.Ext(u.Image.Name)
		basename := strings.TrimSuffix(path.Base(u.Image.Name), ext)
		for i := 2; i < 1000; i++ {
			if !contains(names, u.Image.Name) {
				break
			}
			u.Image.Name = fmt.Sprintf("%s (%d)%s", basename, i, ext)
		}
	}

	// Save in CouchDB
	if err := couchdb.CreateNamedDocWithDB(u.inst, u.Image); err != nil {
		_ = u.inst.ThumbsFS().RemoveNoteThumb(u.Image.ID())
		return err
	}

	// Push a job for the thumbnail worker if the image needs to be resized
	if u.Image.ToResize {
		evt, _ := job.NewEvent(&realtime.Event{Verb: "CREATED"})
		msg, _ := job.NewMessage(struct {
			NoteImage *Image `json:"noteImage"`
		}{
			NoteImage: u.Image,
		})
		_, _ = job.System().PushJob(u.inst, &job.JobRequest{
			WorkerType: "thumbnail",
			Event:      evt,
			Message:    msg,
		})
	}

	return nil
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if needle == v {
			return true
		}
	}
	return false
}

// GetImages returns the images for the given note.
func GetImages(inst *instance.Instance, fileID string) ([]*Image, error) {
	lock := inst.NotesLock()
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	defer lock.Unlock()

	return getImages(inst, fileID)
}

// getImages is the same as GetSteps, but with the notes lock already acquired
func getImages(inst *instance.Instance, fileID string) ([]*Image, error) {
	var images []*Image
	req := couchdb.AllDocsRequest{
		Limit:    1000,
		StartKey: startkey(fileID),
		EndKey:   endkey(fileID),
	}
	if err := couchdb.GetAllDocs(inst, consts.NotesImages, &req, &images); err != nil {
		return nil, err
	}
	return images, nil
}

var _ couchdb.Doc = &Image{}
