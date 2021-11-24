package note

import (
	"fmt"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/model/note/custom"
	"github.com/cozy/prosemirror-go/markdown"
	"github.com/cozy/prosemirror-go/model"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"
)

func markdownSerializer(images []*Image) *markdown.Serializer {
	vanilla := markdown.DefaultSerializer
	nodes := map[string]markdown.NodeSerializerFunc{
		"paragraph": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			for _, mark := range node.Marks {
				if mark.Type.Name == "alignment" {
					switch mark.Attrs["align"] {
					case "center":
						state.Write("[^]{.center}")
					case "end":
						state.Write("[^]{.right}")
					}
				}
			}
			state.RenderInline(node)
			state.CloseBlock(node)
		},
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
			state.Write("________________________________________{.table}\n\n")
			state.RenderContent(node)
			state.EnsureNewLine()
			state.Write("________________________________________{.tableEnd}\n")
			state.CloseBlock(node)
		},
		"tableRow": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.Write("________________________________________{.tableRow}\n\n")
			state.RenderContent(node)
			state.EnsureNewLine()
			state.CloseBlock(node)
		},
		"tableHeader": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.Write("____________________{.tableHeader}\n\n")
			state.RenderContent(node)
		},
		"tableCell": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			state.Write("____________________{.tableCell}\n\n")
			state.RenderContent(node)
		},
		"status": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			if txt, ok := node.Attrs["text"].(string); ok {
				state.Write("[")
				state.Text(txt)
				state.Write("]")
				color, _ := node.Attrs["color"].(string)
				id, _ := node.Attrs["localId"].(string)
				state.Text(fmt.Sprintf(`{.status color="%s" localId="%s"}`, color, id))
			}
		},
		"date": func(state *markdown.SerializerState, node, _parent *model.Node, _index int) {
			if ts, ok := node.Attrs["timestamp"].(string); ok {
				if seconds, err := strconv.ParseInt(ts, 10, 64); err == nil {
					txt := time.Unix(seconds/1000, 0).Format("2006-01-02")
					state.Write("[")
					state.Text(txt)
					state.Write("]")
					state.Text(fmt.Sprintf(`{.date ts="%s"}`, ts))
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
			state.Write(fmt.Sprintf("![%s](%s)\n", state.Esc(alt), state.Esc(src)))
		},
	}
	marks := map[string]markdown.MarkSerializerSpec{
		"em":          vanilla.Marks["em"],
		"strong":      vanilla.Marks["strong"],
		"link":        vanilla.Marks["link"],
		"code":        vanilla.Marks["code"],
		"strike":      {Open: "~~", Close: "~~", ExpelEnclosingWhitespace: true},
		"indentation": {Open: "    ", Close: "", ExpelEnclosingWhitespace: true},
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
				return fmt.Sprintf(`]{.color rgb="%s"}`, color)
			},
		},
	}
	return markdown.NewSerializer(nodes, marks)
}

func isTableCell(item *StackItem) bool {
	name := item.Type.Name
	return name == "tableHeader" || name == "tableCell"
}

func markdownNodeMapper() NodeMapper {
	vanilla := DefaultNodeMapper
	return NodeMapper{
		// Blocks
		ast.KindDocument:        vanilla[ast.KindDocument],
		ast.KindParagraph:       vanilla[ast.KindParagraph],
		ast.KindHeading:         vanilla[ast.KindHeading],
		ast.KindList:            vanilla[ast.KindList],
		ast.KindListItem:        vanilla[ast.KindListItem],
		ast.KindTextBlock:       vanilla[ast.KindTextBlock],
		ast.KindBlockquote:      vanilla[ast.KindBlockquote],
		ast.KindCodeBlock:       vanilla[ast.KindCodeBlock],
		ast.KindFencedCodeBlock: vanilla[ast.KindFencedCodeBlock],
		ast.KindThematicBreak:   vanilla[ast.KindThematicBreak],
		custom.KindPanel: func(state *MarkdownParseState, node ast.Node, entering bool) error {
			if entering {
				typ, err := state.Schema.NodeType("panel")
				if err != nil {
					return err
				}
				attrs := map[string]interface{}{
					"panelType": node.(*custom.Panel).PanelType,
				}
				state.OpenNode(typ, attrs)
			} else {
				if _, err := state.CloseNode(); err != nil {
					return err
				}
			}
			return nil
		},
		custom.KindTable: func(state *MarkdownParseState, node ast.Node, entering bool) error {
			tableType, ok := node.AttributeString("class")
			if entering || !ok {
				return nil
			}

			switch tableType {
			case "table":
				// Nothing to do
			case "tableEnd":
				if isTableCell(state.Top()) {
					if _, err := state.CloseNode(); err != nil { // Cell
						return err
					}
					if _, err := state.CloseNode(); err != nil { // Row
						return err
					}
				}
				if _, err := state.CloseNode(); err != nil { // Table
					return err
				}
				return nil
			case "tableRow":
				if isTableCell(state.Top()) {
					if _, err := state.CloseNode(); err != nil { // Cell
						return err
					}
					if _, err := state.CloseNode(); err != nil { // Row
						return err
					}
				}
			case "tableHeader":
				if isTableCell(state.Top()) {
					if _, err := state.CloseNode(); err != nil {
						return err
					}
				}
			case "tableCell":
				if isTableCell(state.Top()) {
					if _, err := state.CloseNode(); err != nil {
						return err
					}
				}
			default:
				return nil
			}
			typ, err := state.Schema.NodeType(tableType.(string))
			if err != nil {
				return err
			}
			state.OpenNode(typ, nil)
			return nil
		},

		// Inlines
		ast.KindText:     vanilla[ast.KindText],
		ast.KindString:   vanilla[ast.KindString],
		ast.KindAutoLink: vanilla[ast.KindAutoLink],
		ast.KindLink:     vanilla[ast.KindLink],
		ast.KindImage: func(state *MarkdownParseState, node ast.Node, entering bool) error {
			if entering {
				return nil
			}
			paragraph := state.Pop()
			var text string
			for _, n := range paragraph.Content {
				if n.Text != nil {
					text += *n.Text
				}
			}
			typeMS, err := state.Schema.NodeType("mediaSingle")
			if err != nil {
				return err
			}
			state.OpenNode(typeMS, map[string]interface{}{"layout": "center"})
			typ, err := state.Schema.NodeType("media")
			if err != nil {
				return err
			}
			n := node.(*ast.Image)
			attrs := map[string]interface{}{
				"url":  string(n.Destination),
				"alt":  text,
				"id":   "",
				"type": "external",
			}
			state.OpenNode(typ, attrs)
			if _, err := state.CloseNode(); err != nil { // media
				return err
			}
			if _, err := state.CloseNode(); err != nil { // mediaSingle
				return err
			}
			state.OpenNode(paragraph.Type, paragraph.Attrs)
			return nil
		},
		ast.KindCodeSpan:               vanilla[ast.KindCodeSpan],
		ast.KindEmphasis:               vanilla[ast.KindEmphasis],
		extensionast.KindStrikethrough: vanilla[extensionast.KindStrikethrough],
		custom.KindSpan: func(state *MarkdownParseState, node ast.Node, entering bool) error {
			text := node.(*custom.Span).Value

			var markType, nodeType string
			var attrs map[string]interface{}
			if class, ok := node.AttributeString("class"); ok {
				switch class {
				case "underlined":
					markType = "underline"
				case "sub":
					markType = "subsup"
					attrs = map[string]interface{}{"type": "sub"}
				case "sup":
					markType = "subsup"
					attrs = map[string]interface{}{"type": "sup"}
				case "color":
					if color, ok := node.AttributeString("rgb"); ok {
						markType = "textColor"
						attrs = map[string]interface{}{"color": color}
					}
				case "status":
					if color, ok := node.AttributeString("color"); ok {
						id, _ := node.AttributeString("localId")
						nodeType = "status"
						attrs = map[string]interface{}{
							"color":   color,
							"localId": id,
							"text":    text,
						}
					}
				case "date":
					nodeType = "date"
					ts, _ := node.AttributeString("ts")
					attrs = map[string]interface{}{"timestamp": ts}
				case "center", "right":
					if !entering {
						return nil
					}
					align := "center"
					if class == "right" {
						align = "end"
					}
					attrs = map[string]interface{}{"align": align}
					typ, err := state.Schema.MarkType("alignment")
					if err != nil {
						return err
					}
					mark := typ.Create(attrs)
					state.AddParentMark(mark)
					return nil
				}
			}

			if markType != "" {
				typ, err := state.Schema.MarkType(markType)
				if err != nil {
					return err
				}
				mark := typ.Create(attrs)
				if entering {
					state.OpenMark(mark)
					state.AddText(text)
				} else {
					state.CloseMark(mark)
				}
			} else if nodeType != "" {
				if entering {
					typ, err := state.Schema.NodeType(nodeType)
					if err != nil {
						return err
					}
					state.OpenNode(typ, attrs)
				} else {
					if _, err := state.CloseNode(); err != nil {
						return err
					}
				}
			} else {
				if entering {
					state.AddText(text)
				}
				return nil
			}
			return nil
		},
	}
}

func markdownParser() parser.Parser {
	return parser.NewParser(
		parser.WithBlockParsers(
			util.Prioritized(custom.NewTableParser(), 50),
			util.Prioritized(parser.NewSetextHeadingParser(), 100),
			util.Prioritized(parser.NewThematicBreakParser(), 200),
			util.Prioritized(parser.NewListParser(), 300),
			util.Prioritized(parser.NewListItemParser(), 400),
			util.Prioritized(parser.NewCodeBlockParser(), 500),
			util.Prioritized(parser.NewATXHeadingParser(), 600),
			util.Prioritized(parser.NewFencedCodeBlockParser(), 700),
			util.Prioritized(parser.NewBlockquoteParser(), 800),
			util.Prioritized(custom.NewPanelParser(), 900),
			util.Prioritized(parser.NewParagraphParser(), 1000),
		),
		parser.WithInlineParsers(
			util.Prioritized(custom.NewSpanParser(), 50),
			util.Prioritized(parser.NewCodeSpanParser(), 100),
			util.Prioritized(parser.NewLinkParser(), 200),
			util.Prioritized(parser.NewAutoLinkParser(), 300),
			util.Prioritized(parser.NewEmphasisParser(), 400),
			util.Prioritized(extension.NewStrikethroughParser(), 500),
		),
		parser.WithParagraphTransformers(
			util.Prioritized(parser.LinkReferenceParagraphTransformer, 100),
		),
	)
}
