package note

import (
	"io"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/prosemirror-go/model"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// MaxMarkdownSize is the maximal size of a markdown that can be parsed.
const MaxMarkdownSize = 10 * 1024 * 1024

func ImportFile(inst *instance.Instance, newdoc, olddoc *vfs.FileDoc, body io.ReadCloser) error {
	fs := inst.VFS()
	file, err := fs.CreateFile(newdoc, olddoc)
	if err != nil {
		return err
	}

	schemaSpecs := DefaultSchemaSpecs()
	specs := model.SchemaSpecFromJSON(schemaSpecs)
	schema, err := model.NewSchema(&specs)
	if err != nil {
		return err
	}

	reader := io.TeeReader(body, file)
	if content, err := parseFile(reader, schema); err == nil {
		fillMetadata(newdoc, olddoc, schemaSpecs, content)
	}
	if cerr := file.Close(); cerr != nil && (err == nil || err == io.ErrUnexpectedEOF) {
		err = cerr
	}
	if err != nil {
		return err
	}

	if olddoc != nil {
		purgeAllSteps(inst, olddoc.DocID)
	}
	return nil
}

func fillMetadata(newdoc, olddoc *vfs.FileDoc, schemaSpecs map[string]interface{}, content *model.Node) {
	version := 1
	if olddoc != nil {
		rev := strings.Split(olddoc.DocRev, "-")[0]
		n, _ := strconv.Atoi(rev)
		version = n * 1000
	}

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

	txt := text.NewReader(buf)
	parser := parser.NewParser()
	tree := parser.Parse(txt)
	return fromMarkdownAST(tree)
}
