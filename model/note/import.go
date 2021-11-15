package note

import (
	"errors"
	"io"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/prosemirror-go/model"
	"github.com/yuin/goldmark/ast"
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
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	tree := parser.Parse(text.NewReader(buf))

	ctx := &mapperContext{source: buf, schema: schema}
	err = ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if fn, ok := markdownNodeMapper[node.Kind()]; ok {
			if err := fn(ctx, node, entering); err != nil {
				return ast.WalkStop, err
			}
		} else {
			logger.WithNamespace("notes").
				Warnf("Unknown node kind: %s\n", node.Kind())
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	if ctx.root == nil {
		return nil, errors.New("Cannot build prosemirror content")
	}
	return ctx.root, nil
}
