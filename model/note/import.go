package note

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/filetype"
	"github.com/cozy/prosemirror-go/markdown"
	"github.com/cozy/prosemirror-go/model"
	"github.com/gofrs/uuid"
)

// MaxMarkdownSize is the maximal size of a markdown that can be parsed.
const MaxMarkdownSize = 2 * 1024 * 1024

func ImportFile(inst *instance.Instance, newdoc, olddoc *vfs.FileDoc, body io.ReadCloser) error {
	schemaSpecs := DefaultSchemaSpecs()
	specs := model.SchemaSpecFromJSON(schemaSpecs)
	schema, err := model.NewSchema(&specs)
	if err != nil {
		return err
	}

	// We need a fileID for saving images
	if newdoc.ID() == "" {
		uuidv4, _ := uuid.NewV4()
		newdoc.SetID(uuidv4.String())
	}
	images, _ := getImages(inst, newdoc.ID())

	fs := inst.VFS()
	file, err := fs.CreateFile(newdoc, olddoc)
	if err != nil {
		return err
	}

	reader := io.TeeReader(body, file)
	content, err := importReader(inst, newdoc, reader, schema)

	if content != nil {
		fillMetadata(newdoc, olddoc, schemaSpecs, content)
	} else {
		_, _ = io.Copy(io.Discard, reader)
		plog.WithDomain(inst.Domain).Warnf("Cannot import notes: %s", err)
	}
	if err := file.Close(); err != nil {
		return err
	}

	if olddoc != nil {
		purgeAllSteps(inst, olddoc.DocID)
	}
	for _, img := range images {
		img.seen = false
		img.ToRemove = true
	}
	cleanImages(inst, images)
	return nil
}

func importReader(inst *instance.Instance, doc *vfs.FileDoc, reader io.Reader, schema *model.Schema) (*model.Node, error) {
	buf := &bytes.Buffer{}
	var hasImages bool
	if _, err := io.CopyN(buf, reader, 512); err != nil {
		if !errors.Is(err, io.EOF) {
			return nil, err
		}
		hasImages = false
	} else {
		hasImages = isTar(buf.Bytes())
	}

	if !hasImages {
		if _, err := buf.ReadFrom(reader); err != nil {
			return nil, err
		}
		return parseFile(buf, schema)
	}

	var content *model.Node
	var err error
	var images []*Image
	defer func() {
		if err == nil && images != nil {
			fixURLForProsemirrorImages(content, images)
		}
	}()

	tr := tar.NewReader(io.MultiReader(buf, reader))
	for {
		header, errh := tr.Next()
		if errh != nil {
			return content, err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if header.Name == "index.md" {
			content, err = parseFile(tr, schema)
			if err != nil {
				return nil, err
			}
		} else {
			ext := path.Ext(header.Name)
			contentType := filetype.ByExtension(ext)
			upload, erru := NewImageUpload(inst, doc, header.Name, contentType)
			if erru != nil {
				err = erru
			} else {
				_, errc := io.Copy(upload, tr)
				if cerr := upload.Close(); cerr != nil && (errc == nil || errc == io.ErrUnexpectedEOF) {
					errc = cerr
				}
				if errc != nil {
					err = errc
				} else {
					images = append(images, upload.Image)
				}
			}
		}
	}
}

func fixURLForProsemirrorImages(node *model.Node, images []*Image) {
	if node.Type.Name == "media" {
		name, _ := node.Attrs["alt"].(string)
		for _, img := range images {
			if img.originalName == name {
				node.Attrs["url"] = img.DocID
			}
		}
	}

	node.ForEach(func(child *model.Node, _ int, _ int) {
		fixURLForProsemirrorImages(child, images)
	})
}

func fillMetadata(newdoc, olddoc *vfs.FileDoc, schemaSpecs map[string]interface{}, content *model.Node) {
	version := 1
	if olddoc != nil {
		rev := strings.Split(olddoc.DocRev, "-")[0]
		n, _ := strconv.Atoi(rev)
		version = n * 1000
	}

	newdoc.Mime = consts.NoteMimeType
	newdoc.Class = "text"
	newdoc.Metadata = vfs.Metadata{
		"title":   strings.TrimSuffix(newdoc.DocName, ".cozy-note"),
		"content": content.ToJSON(),
		"version": version,
		"schema":  schemaSpecs,
	}
}

func parseFile(r io.Reader, schema *model.Schema) (*model.Node, error) {
	buf, err := io.ReadAll(io.LimitReader(r, MaxMarkdownSize))
	if err != nil {
		return nil, err
	}
	parser := markdownParser()
	funcs := markdownNodeMapper()
	return markdown.ParseMarkdown(parser, funcs, buf, schema)
}

func isTar(buf []byte) bool {
	if len(buf) < 263 {
		return false
	}
	// https://en.wikipedia.org/wiki/Tar_(computing)#UStar_format
	return buf[257] == 'u' && buf[258] == 's' && buf[259] == 't' &&
		buf[260] == 'a' && buf[261] == 'r' && buf[262] == 0
}
