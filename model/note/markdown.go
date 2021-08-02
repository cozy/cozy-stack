package note

import (
	"fmt"
	"strconv"
	"time"

	"github.com/cozy/prosemirror-go/markdown"
	"github.com/cozy/prosemirror-go/model"
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
			var alt string
			src, _ := node.Attrs["url"].(string)
			for _, img := range images {
				if img.DocID == src {
					alt = img.Name
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
	}
	return markdown.NewSerializer(nodes, marks)
}
