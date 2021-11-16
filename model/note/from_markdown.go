package note

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cozy/prosemirror-go/model"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func maybeMerge(a, b *model.Node) *model.Node {
	if a.IsText() && b.IsText() && model.SameMarkSet(a.Marks, b.Marks) {
		return a.WithText(*a.Text + *b.Text)
	}
	return nil
}

// MarkdownParseState is an object used to track the context of a running
// parse.
type MarkdownParseState struct {
	Source []byte
	Schema *model.Schema
	Root   *model.Node
	Stack  []*StackItem
	Marks  []*model.Mark
}

type StackItem struct {
	Type    *model.NodeType
	Attrs   map[string]interface{}
	Content []*model.Node
}

type NodeMapper map[ast.NodeKind]NodeMapperFunc

type NodeMapperFunc func(state *MarkdownParseState, node ast.Node, entering bool) error

func (state *MarkdownParseState) Top() *StackItem {
	if len(state.Stack) == 0 {
		panic(errors.New("Empty stack"))
	}
	last := len(state.Stack) - 1
	return state.Stack[last]
}

func (state *MarkdownParseState) Push(node *model.Node) {
	item := state.Top()
	item.Content = append(item.Content, node)
}

func (state *MarkdownParseState) Pop() *StackItem {
	if len(state.Stack) == 0 {
		panic(errors.New("Empty stack"))
	}
	last := len(state.Stack) - 1
	item := state.Stack[last]
	state.Stack = state.Stack[:last]
	return item
}

// AddText adds the given text to the current position in the document, using
// the current marks as styling.
func (state *MarkdownParseState) AddText(text string) {
	item := state.Top()
	node := state.Schema.Text(text, state.Marks)
	if len(item.Content) > 0 {
		last := item.Content[len(item.Content)-1]
		if merged := maybeMerge(last, node); merged != nil {
			item.Content[len(item.Content)-1] = merged
			return
		}
	}
	item.Content = append(item.Content, node)
}

// OpenMark adds the given mark to the set of active marks.
func (state *MarkdownParseState) OpenMark(mark *model.Mark) {
	state.Marks = mark.AddToSet(state.Marks)
}

// CloseMark removes the given mark from the set of active marks.
func (state *MarkdownParseState) CloseMark(mark *model.Mark) {
	state.Marks = mark.RemoveFromSet(state.Marks)
}

// AddNode adds a node at the current position.
func (state *MarkdownParseState) AddNode(typ *model.NodeType, attrs map[string]interface{}, content interface{}) (*model.Node, error) {
	node, err := typ.CreateAndFill(attrs, content, state.Marks)
	if node == nil {
		return nil, err
	}
	state.Push(node)
	return node, nil
}

// OpenNode wraps subsequent content in a node of the given type.
func (state *MarkdownParseState) OpenNode(typ *model.NodeType, attrs map[string]interface{}) {
	item := &StackItem{Type: typ, Attrs: attrs}
	state.Stack = append(state.Stack, item)
}

// CloseNode closes and returns the node that is currently on top of the stack.
func (state *MarkdownParseState) CloseNode() (*model.Node, error) {
	if len(state.Marks) > 0 {
		state.Marks = model.NoMarks
	}
	info := state.Pop()
	return state.AddNode(info.Type, info.Attrs, info.Content)
}

// ParseMarkdown parses a string as [CommonMark](http://commonmark.org/)
// markup, and create a ProseMirror document as prescribed by this parser's
// rules.
func ParseMarkdown(parser parser.Parser, funcs NodeMapper, source []byte, schema *model.Schema) (*model.Node, error) {
	tree := parser.Parse(text.NewReader(source))
	state := &MarkdownParseState{Source: source, Schema: schema}
	err := ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if fn, ok := funcs[node.Kind()]; ok {
			if err := fn(state, node, entering); err != nil {
				return ast.WalkStop, err
			}
		} else {
			return ast.WalkStop, fmt.Errorf("Node kind %s not supported by markdown parser", node.Kind())
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	if state.Root == nil {
		return nil, errors.New("Cannot build prosemirror content")
	}
	return state.Root, nil
}

func GenericBlockHandler(nodeType string) NodeMapperFunc {
	return func(state *MarkdownParseState, node ast.Node, entering bool) error {
		if entering {
			typ, err := state.Schema.NodeType(nodeType)
			if err != nil {
				return err
			}
			state.OpenNode(typ, nil)
		} else {
			if _, err := state.CloseNode(); err != nil {
				return err
			}
		}
		return nil
	}
}

func GenericMarkHandler(markType string) NodeMapperFunc {
	return func(state *MarkdownParseState, node ast.Node, entering bool) error {
		typ, err := state.Schema.MarkType(markType)
		if err != nil {
			return err
		}
		var attrs map[string]interface{}
		mark := typ.Create(attrs)
		if entering {
			state.OpenMark(mark)
		} else {
			state.CloseMark(mark)
		}
		return nil
	}
}

func WithoutTrailingNewline(node ast.Node, source []byte) string {
	var lines []string
	segments := node.Lines()
	for i := 0; i < segments.Len(); i++ {
		segment := segments.At(i)
		line := segment.Value(source)
		lines = append(lines, string(line))
	}
	str := strings.Join(lines, "")
	return strings.TrimSuffix(str, "\n")
}

// DefaultNodeMapper is a parser parsing unextended
// [CommonMark](http://commonmark.org/), without inline HTML, and producing a
// document in the basic schema.
var DefaultNodeMapper = NodeMapper{
	// Blocks
	ast.KindDocument: func(state *MarkdownParseState, node ast.Node, entering bool) error {
		if entering {
			typ, err := state.Schema.NodeType(state.Schema.Spec.TopNode)
			if err != nil {
				return err
			}
			state.OpenNode(typ, nil)
		} else {
			info := state.Pop()
			node, err := info.Type.CreateAndFill(info.Attrs, info.Content, state.Marks)
			if err != nil {
				return err
			}
			state.Root = node
		}
		return nil
	},
	ast.KindParagraph: GenericBlockHandler("paragraph"),
	ast.KindHeading: func(state *MarkdownParseState, node ast.Node, entering bool) error {
		if entering {
			typ, err := state.Schema.NodeType("heading")
			if err != nil {
				return err
			}
			level := node.(*ast.Heading).Level
			attrs := map[string]interface{}{"level": level}
			state.OpenNode(typ, attrs)
		} else {
			if _, err := state.CloseNode(); err != nil {
				return err
			}
		}
		return nil
	},
	ast.KindList: func(state *MarkdownParseState, node ast.Node, entering bool) error {
		nodeType := "bulletList"
		if node.(*ast.List).IsOrdered() {
			nodeType = "orderedList"
		}
		if entering {
			typ, err := state.Schema.NodeType(nodeType)
			if err != nil {
				return err
			}
			state.OpenNode(typ, nil)
		} else {
			if _, err := state.CloseNode(); err != nil {
				return err
			}
		}
		return nil
	},
	ast.KindListItem:   GenericBlockHandler("listItem"),
	ast.KindTextBlock:  GenericBlockHandler("paragraph"),
	ast.KindBlockquote: GenericBlockHandler("blockquote"),
	ast.KindCodeBlock: func(state *MarkdownParseState, node ast.Node, entering bool) error {
		if entering {
			node := node.(*ast.CodeBlock)
			typ, err := state.Schema.NodeType("codeBlock")
			if err != nil {
				return err
			}
			state.OpenNode(typ, nil)
			state.AddText(WithoutTrailingNewline(node, state.Source))
		} else {
			if _, err := state.CloseNode(); err != nil {
				return err
			}
		}
		return nil
	},
	ast.KindFencedCodeBlock: func(state *MarkdownParseState, node ast.Node, entering bool) error {
		if entering {
			node := node.(*ast.FencedCodeBlock)
			typ, err := state.Schema.NodeType("codeBlock")
			if err != nil {
				return err
			}
			lang := node.Language(state.Source)
			attrs := map[string]interface{}{
				"language": string(lang),
			}
			state.OpenNode(typ, attrs)
			state.AddText(WithoutTrailingNewline(node, state.Source))
		} else {
			if _, err := state.CloseNode(); err != nil {
				return err
			}
		}
		return nil
	},

	// Inlines
	ast.KindText: func(state *MarkdownParseState, node ast.Node, entering bool) error {
		if entering {
			segment := node.(*ast.Text).Segment
			content := segment.Value(state.Source)
			state.AddText(string(content))
		}
		return nil
	},
	ast.KindString: func(state *MarkdownParseState, node ast.Node, entering bool) error {
		if entering {
			content := node.(*ast.String).Value
			state.AddText(string(content))
		}
		return nil
	},
	ast.KindLink: func(state *MarkdownParseState, node ast.Node, entering bool) error {
		typ, err := state.Schema.MarkType("link")
		if err != nil {
			return err
		}
		n := node.(*ast.Link)
		attrs := map[string]interface{}{
			"href":  string(n.Destination),
			"title": string(n.Title),
		}
		mark := typ.Create(attrs)
		if entering {
			state.OpenMark(mark)
		} else {
			state.CloseMark(mark)
		}
		return nil
	},
	ast.KindCodeSpan: GenericMarkHandler("code"),
	ast.KindEmphasis: func(state *MarkdownParseState, node ast.Node, entering bool) error {
		var typ *model.MarkType
		var err error
		if node.(*ast.Emphasis).Level == 2 {
			typ, err = state.Schema.MarkType("strong")
		} else {
			typ, err = state.Schema.MarkType("em")
		}
		if err != nil {
			return err
		}
		var attrs map[string]interface{}
		mark := typ.Create(attrs)
		if entering {
			state.OpenMark(mark)
		} else {
			state.CloseMark(mark)
		}
		return nil
	},
}
