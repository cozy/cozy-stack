package note

import (
	"github.com/cozy/prosemirror-go/markdown"
	"github.com/cozy/prosemirror-go/model"
)

func markdownSerializer() *markdown.Serializer {
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
		"codeBlock":   vanilla.Nodes["code_block"],
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
	}
	marks := map[string]markdown.MarkSerializerSpec{
		"em":     vanilla.Marks["em"],
		"strong": vanilla.Marks["strong"],
		"link":   vanilla.Marks["link"],
		"code":   vanilla.Marks["code"],
		"strike": {Open: "~~", Close: "~~", ExpelEnclosingWhitespace: true},
	}
	return markdown.NewSerializer(nodes, marks)
}
