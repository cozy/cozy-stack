package note

import (
	"errors"
	"fmt"

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
func (state *MarkdownParseState) RemoveMark(mark *model.Mark) {
	state.Marks = mark.RemoveFromSet(state.Marks)
}

// AddNode adds a node at the current position.
func (state *MarkdownParseState) AddNode(typ *model.NodeType, attrs map[string]interface{}, content interface{}) (*model.Node, error) {
	node, err := typ.CreateAndFill(attrs, content, state.Marks)
	if err != nil {
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

// DefaultNodeMapper is a parser parsing unextended
// [CommonMark](http://commonmark.org/), without inline HTML, and producing a
// document in the basic schema.
var DefaultNodeMapper = NodeMapper{
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
	ast.KindText: func(state *MarkdownParseState, node ast.Node, entering bool) error {
		if entering {
			segment := node.(*ast.Text).Segment
			content := segment.Value(state.Source)
			state.AddText(string(content))
		}
		return nil
	},
}
