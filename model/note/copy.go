package note

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/prosemirror-go/model"
	"github.com/gofrs/uuid/v5"
)

// CopyFile is an overloaded version of Fs.CopyFile that take care of also
// copying the images in the note.
func CopyFile(inst *instance.Instance, olddoc, newdoc *vfs.FileDoc) error {
	// Check available disk space
	_, _, _, err := vfs.CheckAvailableDiskSpace(inst.VFS(), newdoc)
	if err != nil {
		return err
	}

	// Load data from the source note
	noteDoc, err := get(inst, olddoc)
	if err != nil {
		return err
	}
	content, err := noteDoc.Content()
	if err != nil {
		return err
	}
	srcImages, err := getImages(inst, olddoc.ID())
	if err != nil {
		return err
	}

	// We need a fileID for saving images
	uuidv7, _ := uuid.NewV7()
	newdoc.SetID(uuidv7.String())

	// id of the image in the source doc -> image in the destination doc
	mapping := make(map[string]*Image)
	var dstImages []*Image
	for _, img := range srcImages {
		if img.ToRemove {
			continue
		}
		copied, err := CopyImageToAnotherNote(inst, img.ID(), newdoc)
		if err != nil {
			return err
		}
		mapping[img.ID()] = copied
		dstImages = append(dstImages, copied)
	}

	updateProsemirrorImageURLs(content, mapping)

	md := markdownSerializer(dstImages).Serialize(content)
	if err != nil {
		return err
	}
	body := []byte(md)
	if hasImages(dstImages) {
		body, _ = buildArchive(inst, []byte(md), dstImages)
	}
	newdoc.ByteSize = int64(len(body))
	newdoc.MD5Sum = nil
	newdoc.Metadata["content"] = content.ToJSON()

	file, err := inst.VFS().CreateFile(newdoc, nil)
	if err != nil {
		return err
	}
	_, err = file.Write(body)
	if cerr := file.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return SetupTrigger(inst, newdoc.ID())
}

func updateProsemirrorImageURLs(node *model.Node, mapping map[string]*Image) {
	if node.Type.Name == "media" {
		nodeURL, _ := node.Attrs["url"].(string)
		for id, img := range mapping {
			if nodeURL == id {
				node.Attrs["url"] = img.ID()
			}
		}
	}

	node.ForEach(func(child *model.Node, _ int, _ int) {
		updateProsemirrorImageURLs(child, mapping)
	})
}
