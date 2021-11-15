package note

import (
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

func markdownNodeMapper() NodeMapper {
	vanilla := DefaultNodeMapper
	return NodeMapper{
		ast.KindDocument:  vanilla[ast.KindDocument],
		ast.KindParagraph: vanilla[ast.KindParagraph],
		ast.KindHeading:   vanilla[ast.KindHeading],
		ast.KindText:      vanilla[ast.KindText],
		// ast.KindEmphasis: func(state *markdownParseState, node ast.Node, entering bool) error {
		// 	if entering {
		// 		node := node.(*ast.Text)
		// 		content := node.Segment.Value(state.source)
		// 		mark := model.Mark{}
		// 		emphasis := state.schema.Text(string(content)).Mark([]*model.Mark{mark})
		// 		state.Push(emphasis)
		// 	}
		// 	return nil
		// },
	}
}
