package note

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/cozy/prosemirror-go/markdown"
	"github.com/cozy/prosemirror-go/model"
	"github.com/yuin/goldmark/ast"
)

func markdownSerializer(images []*Image) *markdown.Serializer {
	vanilla := markdown.DefaultSerializer
	nodes := map[string]markdown.NodeSerializerFunc{
		"paragraph":   vanilla.Nodes["paragraph"],
		"text":        vanilla.Nodes["text"],
		"bulletList":  vanilla.Nodes["bullet_list"],
		"orderedList": vanilla.Nodes["ordered_list"],
		"listItem":    vanilla.Nodes["list_item"],
		"heading":     vanilla.Nodes["heading"],
		"blockquote":  vanilla.Nodes["blockquote"],
		"rule":        vanilla.Nodes["horizontal_rule"],
		"hardBreak":   vanilla.Nodes["hard_break"],
		"image":       vanilla.Nodes["image"],
		"codeBlock": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			lang, _ := node.Attrs["language"].(string)
			state.Write("```" + lang + "\n")
			state.Text(node.TextContent(), false)
			state.EnsureNewLine()
			state.Write("```")
			state.CloseBlock(node)
		},
		"panel": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			if typ, ok := node.Attrs["panelType"].(string); ok {
				state.Write(":" + typ + ": ")
			}
			state.RenderContent(node)
		},
		"table": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.Write("`````table\n")
			state.RenderContent(node)
			state.EnsureNewLine()
			state.Write("`````")
			state.CloseBlock(node)
		},
		"tableRow": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.Write("|=======================================================================\n")
			state.RenderContent(node)
			state.EnsureNewLine()
			state.CloseBlock(node)
		},
		"tableHeader": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.Write("|")
			state.RenderContent(node)
		},
		"tableCell": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.Write("|")
			state.RenderContent(node)
		},
		"status": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			if txt, ok := node.Attrs["text"].(string); ok {
				state.Write("[")
				state.Text(txt)
				state.Write("]")
				color, _ := node.Attrs["color"].(string)
				id, _ := node.Attrs["localId"].(string)
				state.Text(fmt.Sprintf("{.status color=%s localId=%s}", color, id))
			}
		},
		"date": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			if ts, ok := node.Attrs["timestamp"].(string); ok {
				if seconds, err := strconv.ParseInt(ts, 10, 64); err == nil {
					txt := time.Unix(seconds/1000, 0).Format("2006-01-02")
					state.Write("[")
					state.Text(txt)
					state.Write("]")
					state.Text(fmt.Sprintf("{.date ts=%s}", ts))
				}
			}
		},
		"mediaSingle": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderContent(node)
		},
		"media": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			var alt string
			src, _ := node.Attrs["url"].(string)
			for _, img := range images {
				if img.DocID == src {
					alt = img.Name
					img.seen = true
				}
			}
			state.Write(fmt.Sprintf("![%s](%s)", state.Esc(alt), state.Esc(src)))
		},
	}
	marks := map[string]markdown.MarkSerializerSpec{
		"em":          vanilla.Marks["em"],
		"strong":      vanilla.Marks["strong"],
		"link":        vanilla.Marks["link"],
		"code":        vanilla.Marks["code"],
		"strike":      {Open: "~~", Close: "~~", ExpelEnclosingWhitespace: true},
		"indentation": {Open: "    ", Close: "", ExpelEnclosingWhitespace: true},
		"alignement":  {Open: "", Close: "", ExpelEnclosingWhitespace: true},
		"breakout":    {Open: "", Close: "", ExpelEnclosingWhitespace: true},
		"underline":   {Open: "[", Close: "]{.underlined}", ExpelEnclosingWhitespace: true},
		"subsup": {
			Open: "[",
			Close: func(state *markdown.SerializerState, mark *model.Mark, parent *model.Node, index int) string {
				typ, _ := mark.Attrs["type"].(string)
				return fmt.Sprintf("]{.%s}", typ)
			},
		},
		"textColor": {
			Open: "[",
			Close: func(state *markdown.SerializerState, mark *model.Mark, parent *model.Node, index int) string {
				color, _ := mark.Attrs["color"].(string)
				return fmt.Sprintf("]{color=%s}", color)
			},
		},
	}
	return markdown.NewSerializer(nodes, marks)
}

type mapperContext struct {
	source []byte
	schema *model.Schema
	root   *model.Node
	stack  [][]*model.Node
}

func (ctx *mapperContext) Extend() {
	ctx.stack = append(ctx.stack, []*model.Node{})
}

func (ctx *mapperContext) Push(node *model.Node) {
	if len(ctx.stack) == 0 {
		panic(errors.New("Empty stack for push"))
	}
	last := len(ctx.stack) - 1
	ctx.stack[last] = append(ctx.stack[last], node)
}

func (ctx *mapperContext) Pop() []*model.Node {
	if len(ctx.stack) == 0 {
		return nil
	}
	last := ctx.stack[len(ctx.stack)-1]
	ctx.stack = ctx.stack[:len(ctx.stack)-1]
	return last
}

type mapperFn func(ctx *mapperContext, node ast.Node, entering bool) error

var markdownNodeMapper = map[ast.NodeKind]mapperFn{
	ast.KindDocument: func(ctx *mapperContext, node ast.Node, entering bool) error {
		if entering {
			ctx.stack = [][]*model.Node{
				{},
			}
		} else {
			root, err := ctx.schema.Node(ctx.schema.Spec.TopNode, nil, ctx.Pop())
			if err != nil {
				return err
			}
			ctx.root = root
			ctx.stack = nil
		}
		return nil
	},
	ast.KindHeading: func(ctx *mapperContext, node ast.Node, entering bool) error {
		if entering {
			ctx.Extend()
		} else {
			level := node.(*ast.Heading).Level
			attrs := map[string]interface{}{"level": level}
			heading, err := ctx.schema.Node("heading", attrs, ctx.Pop())
			if err != nil {
				return err
			}
			ctx.Push(heading)
		}
		return nil
	},
	ast.KindParagraph: func(ctx *mapperContext, node ast.Node, entering bool) error {
		if entering {
			ctx.Extend()
		} else {
			paragraph, err := ctx.schema.Node("paragraph", nil, ctx.Pop())
			if err != nil {
				return err
			}
			ctx.Push(paragraph)
		}
		return nil
	},
	ast.KindText: func(ctx *mapperContext, node ast.Node, entering bool) error {
		if entering {
			segment := node.(*ast.Text).Segment
			content := segment.Value(ctx.source)
			text := ctx.schema.Text(string(content))
			ctx.Push(text)
		}
		return nil
	},
}
