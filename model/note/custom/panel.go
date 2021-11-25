package custom

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// A Panel struct represents a panel in atlaskit.
type Panel struct {
	ast.BaseBlock
	PanelType string
}

// Dump implements Node.Dump.
func (n *Panel) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// KindPanel is a NodeKind of the Panel node.
var KindPanel = ast.NewNodeKind("Panel")

// Kind implements Node.Kind.
func (n *Panel) Kind() ast.NodeKind {
	return KindPanel
}

// NewPanel returns a new Panel node.
func NewPanel(panelType string) *Panel {
	return &Panel{
		BaseBlock: ast.BaseBlock{},
		PanelType: panelType,
	}
}

type panelParser struct{}

var defaultPanelParser = &panelParser{}

// NewPanelParser returns a new BlockParser that
// parses panels.
func NewPanelParser() parser.BlockParser {
	return defaultPanelParser
}

func (b *panelParser) Trigger() []byte {
	return []byte{':'}
}

func (b *panelParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	if w > 3 || pos >= len(line) || line[pos] != ':' {
		return nil, parser.NoChildren
	}
	pos++
	for start := pos; pos-start < 10; pos++ {
		if pos >= len(line) || line[pos] == '\n' {
			break
		}
		if line[pos] == ':' {
			panelType := string(line[start:pos])
			pos++
			if pos >= len(line) || line[pos] != ' ' {
				break
			}
			switch panelType {
			case "info", "note", "success", "warning", "error":
				reader.Advance(pos)
				return NewPanel(panelType), parser.HasChildren
			}
			break
		}
	}
	return nil, parser.NoChildren
}

func (b *panelParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, segment := reader.PeekLine()
	if util.IsBlank(line) {
		return parser.Close
	}
	reader.Advance(segment.Len() - 1)
	return parser.Continue | parser.HasChildren
}

func (b *panelParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	// nothing to do
}

func (b *panelParser) CanInterruptParagraph() bool {
	return false
}

func (b *panelParser) CanAcceptIndentedLine() bool {
	return false
}
