package note

import (
	"fmt"
	"strconv"
	"time"

	"github.com/cozy/prosemirror-go/markdown"
	"github.com/cozy/prosemirror-go/model"
)

func textSerializer() *markdown.Serializer {
	nodes := map[string]markdown.NodeSerializerFunc{
		"paragraph": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderInline(node)
			state.CloseBlock(node)
		},
		"text": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.Text(*node.Text)
		},
		"bulletList": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderList(node, "", func(_ int) string { return "- " })
		},
		"orderedList": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderList(node, "", func(i int) string { return fmt.Sprintf("%d.", i+1) })
		},
		"listItem": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderContent(node)
		},
		"heading": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderInline(node)
			state.CloseBlock(node)
		},
		"blockquote": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderContent(node)
		},
		"rule": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.Write("---")
			state.CloseBlock(node)
		},
		"hardBreak": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.Write("\n")
		},
		"image": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			// Nothing
		},
		"codeBlock": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.Text(node.TextContent(), false)
			state.EnsureNewLine()
			state.CloseBlock(node)
		},
		"panel": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderContent(node)
		},
		"table": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderContent(node)
			state.EnsureNewLine()
			state.CloseBlock(node)
		},
		"tableRow": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderContent(node)
			state.EnsureNewLine()
			state.CloseBlock(node)
		},
		"tableHeader": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderContent(node)
		},
		"tableCell": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderContent(node)
		},
		"status": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			if txt, ok := node.Attrs["text"].(string); ok {
				state.Text(txt)
			}
		},
		"date": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			if ts, ok := node.Attrs["timestamp"].(string); ok {
				if seconds, err := strconv.ParseInt(ts, 10, 64); err == nil {
					txt := time.Unix(seconds/1000, 0).Format("2006-01-02")
					state.Text(txt)
				}
			}
		},
		"mediaSingle": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.RenderContent(node)
		},
		"media": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			// Nothing
		},
	}
	marks := map[string]markdown.MarkSerializerSpec{
		"em":          {},
		"strong":      {},
		"link":        {},
		"code":        {},
		"strike":      {},
		"indentation": {},
		"breakout":    {},
		"underline":   {},
		"subsup":      {},
		"textColor":   {},
	}
	return markdown.NewSerializer(nodes, marks)
}
