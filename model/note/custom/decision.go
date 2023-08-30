package custom

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// A DecisionList struct represents a decisionList in atlaskit.
type DecisionList struct {
	ast.BaseBlock
}

// Dump implements Node.Dump.
func (n *DecisionList) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// KindDecisionList is a NodeKind of the DecisionList node.
var KindDecisionList = ast.NewNodeKind("DecisionList")

// Kind implements Node.Kind.
func (n *DecisionList) Kind() ast.NodeKind {
	return KindDecisionList
}

// NewDecisionList returns a new DecisionList node.
func NewDecisionList() *DecisionList {
	return &DecisionList{
		BaseBlock: ast.BaseBlock{},
	}
}

// A DecisionItem struct represents a decisionItem in atlaskit.
type DecisionItem struct {
	ast.BaseBlock
}

// Dump implements Node.Dump.
func (n *DecisionItem) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// KindDecisionItem is a NodeKind of the DecisionItem node.
var KindDecisionItem = ast.NewNodeKind("DecisionItem")

// Kind implements Node.Kind.
func (n *DecisionItem) Kind() ast.NodeKind {
	return KindDecisionItem
}

// NewDecisionItem returns a new DecisionItem node.
func NewDecisionItem() *DecisionItem {
	return &DecisionItem{
		BaseBlock: ast.BaseBlock{},
	}
}

type decisionListParser struct{}

var defaultDecisionListParser = &decisionListParser{}

// NewDecisionListParser returns a new BlockParser that
// parses decisionLists.
func NewDecisionListParser() parser.BlockParser {
	return defaultDecisionListParser
}

func (b *decisionListParser) Trigger() []byte {
	return []byte{0xe2}
}

func (b *decisionListParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	if _, ok := parent.(*DecisionList); ok {
		return nil, parser.NoChildren
	}
	line, _ := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	if w > 3 || pos+4 > len(line) || line[pos+1] != 0x9c || line[pos+2] != 0x8d || line[pos+3] != ' ' {
		return nil, parser.NoChildren
	}
	return NewDecisionList(), parser.HasChildren
}

func (b *decisionListParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, _ := reader.PeekLine()
	if util.IsBlank(line) {
		return parser.Close
	}
	return parser.Continue | parser.HasChildren
}

func (b *decisionListParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	// nothing to do
}

func (b *decisionListParser) CanInterruptParagraph() bool {
	return true
}

func (b *decisionListParser) CanAcceptIndentedLine() bool {
	return false
}

type decisionItemParser struct{}

var defaultDecisionItemParser = &decisionItemParser{}

// NewDecisionItemParser returns a new BlockParser that
// parses decisionItems.
func NewDecisionItemParser() parser.BlockParser {
	return defaultDecisionItemParser
}

func (b *decisionItemParser) Trigger() []byte {
	return []byte{0xe2} // 0xe2 is the first byte of ✍
}

func (b *decisionItemParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	if _, ok := parent.(*DecisionList); !ok {
		return nil, parser.NoChildren
	}
	line, _ := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	// 0x9c and 0x8d are the second and third bytes of ✍
	if w > 3 || pos+4 > len(line) || line[pos+1] != 0x9c || line[pos+2] != 0x8d || line[pos+3] != ' ' {
		return nil, parser.NoChildren
	}
	reader.Advance(pos + 4)
	return NewDecisionItem(), parser.HasChildren
}

func (b *decisionItemParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	return parser.Close
}

func (b *decisionItemParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	// nothing to do
}

func (b *decisionItemParser) CanInterruptParagraph() bool {
	return true
}

func (b *decisionItemParser) CanAcceptIndentedLine() bool {
	return false
}
