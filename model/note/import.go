package note

import (
	"io"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/note/custom"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/prosemirror-go/model"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"
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
	} else {
		inst.Logger().WithField("nspace", "notes").
			Warnf("Cannot import notes: %s", err)
	}
	if err := file.Close(); err != nil {
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
	buf, err := ioutil.ReadAll(io.LimitReader(r, MaxMarkdownSize))
	if err != nil {
		return nil, err
	}
	parser := parser.NewParser(
		parser.WithBlockParsers(
			util.Prioritized(custom.NewTableParser(), 50),
			util.Prioritized(parser.NewSetextHeadingParser(), 100),
			util.Prioritized(parser.NewThematicBreakParser(), 200),
			util.Prioritized(parser.NewListParser(), 300),
			util.Prioritized(parser.NewListItemParser(), 400),
			util.Prioritized(parser.NewCodeBlockParser(), 500),
			util.Prioritized(parser.NewATXHeadingParser(), 600),
			util.Prioritized(parser.NewFencedCodeBlockParser(), 700),
			util.Prioritized(parser.NewBlockquoteParser(), 800),
			util.Prioritized(custom.NewPanelParser(), 900),
			util.Prioritized(parser.NewParagraphParser(), 1000),
		),
		parser.WithInlineParsers(
			util.Prioritized(custom.NewSpanParser(), 50),
			util.Prioritized(parser.NewCodeSpanParser(), 100),
			util.Prioritized(parser.NewLinkParser(), 200),
			util.Prioritized(parser.NewAutoLinkParser(), 300),
			util.Prioritized(parser.NewEmphasisParser(), 400),
			util.Prioritized(extension.NewStrikethroughParser(), 500),
		),
		parser.WithParagraphTransformers(
			util.Prioritized(parser.LinkReferenceParagraphTransformer, 100),
		),
	)
	funcs := markdownNodeMapper()
	return ParseMarkdown(parser, funcs, buf, schema)
}
