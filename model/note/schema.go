package note

import (
	"encoding/json"
	"sync"
)

// DefaultSchemaSpecs returns the prosemirror schema used when importing files.
func DefaultSchemaSpecs() map[string]interface{} {
	loadSchemaOnce.Do(func() {
		raw := []byte(defaultSchemaString)
		if err := json.Unmarshal(raw, &defaultSchemaSpecs); err != nil {
			panic(err)
		}
	})
	return defaultSchemaSpecs
}

var defaultSchemaSpecs map[string]interface{}
var loadSchemaOnce sync.Once

const defaultSchemaString = `
{
  "marks": [
    [
      "link",
      {
        "attrs": {
          "__confluenceMetadata": {
            "default": null
          },
          "href": {}
        },
        "excludes": "link color",
        "group": "link",
        "inclusive": false,
        "parseDOM": [
          {
            "tag": "[data-block-link]"
          },
          {
            "context": "paragraph/|heading/|mediaSingle/",
            "tag": "a[href]"
          },
          {
            "tag": "a[href]"
          }
        ]
      }
    ],
    [
      "em",
      {
        "group": "fontStyle",
        "inclusive": true,
        "parseDOM": [
          {
            "tag": "i"
          },
          {
            "tag": "em"
          },
          {
            "style": "font-style=italic"
          }
        ]
      }
    ],
    [
      "strong",
      {
        "group": "fontStyle",
        "inclusive": true,
        "parseDOM": [
          {
            "tag": "strong"
          },
          {
            "tag": "b"
          },
          {
            "tag": "span"
          }
        ]
      }
    ],
    [
      "textColor",
      {
        "attrs": {
          "color": {}
        },
        "group": "color",
        "inclusive": true,
        "parseDOM": [
          {
            "style": "color"
          }
        ]
      }
    ],
    [
      "strike",
      {
        "group": "fontStyle",
        "inclusive": true,
        "parseDOM": [
          {
            "tag": "strike"
          },
          {
            "tag": "s"
          },
          {
            "tag": "del"
          },
          {
            "style": "text-decoration"
          }
        ]
      }
    ],
    [
      "subsup",
      {
        "attrs": {
          "type": {
            "default": "sub"
          }
        },
        "group": "fontStyle",
        "inclusive": true,
        "parseDOM": [
          {
            "attrs": {
              "type": "sub"
            },
            "tag": "sub"
          },
          {
            "attrs": {
              "type": "sup"
            },
            "tag": "sup"
          },
          {
            "style": "vertical-align=super",
            "tag": "span"
          },
          {
            "style": "vertical-align=sub",
            "tag": "span"
          }
        ]
      }
    ],
    [
      "underline",
      {
        "group": "fontStyle",
        "inclusive": true,
        "parseDOM": [
          {
            "tag": "u"
          },
          {
            "style": "text-decoration"
          }
        ]
      }
    ],
    [
      "code",
      {
        "excludes": "fontStyle link searchQuery color",
        "inclusive": true,
        "parseDOM": [
          {
            "preserveWhitespace": true,
            "tag": "span.code"
          },
          {
            "preserveWhitespace": true,
            "tag": "code"
          },
          {
            "preserveWhitespace": true,
            "tag": "tt"
          },
          {
            "preserveWhitespace": true,
            "tag": "span"
          }
        ]
      }
    ],
    [
      "typeAheadQuery",
      {
        "attrs": {
          "trigger": {
            "default": ""
          }
        },
        "group": "searchQuery",
        "inclusive": true,
        "parseDOM": [
          {
            "tag": "span[data-type-ahead-query]"
          }
        ]
      }
    ],
    [
      "alignment",
      {
        "attrs": {
          "align": {}
        },
        "excludes": "alignment indentation",
        "group": "alignment",
        "parseDOM": [
          {
            "tag": "div.fabric-editor-block-mark"
          }
        ]
      }
    ],
    [
      "breakout",
      {
        "attrs": {
          "mode": {
            "default": "wide"
          }
        },
        "inclusive": false,
        "parseDOM": [
          {
            "tag": "div.fabric-editor-breakout-mark"
          }
        ],
        "spanning": false
      }
    ],
    [
      "indentation",
      {
        "attrs": {
          "level": {}
        },
        "excludes": "indentation alignment",
        "group": "indentation",
        "parseDOM": [
          {
            "tag": "div.fabric-editor-indentation-mark"
          }
        ]
      }
    ],
    [
      "unsupportedMark",
      {
        "attrs": {
          "originalValue": {}
        }
      }
    ],
    [
      "unsupportedNodeAttribute",
      {
        "attrs": {
          "type": {},
          "unsupported": {}
        }
      }
    ]
  ],
  "nodes": [
    [
      "date",
      {
        "attrs": {
          "timestamp": {
            "default": ""
          }
        },
        "group": "inline",
        "inline": true,
        "parseDOM": [
          {
            "tag": "span[data-node-type=\"date\"]"
          }
        ],
        "selectable": true
      }
    ],
    [
      "status",
      {
        "attrs": {
          "color": {
            "default": ""
          },
          "localId": {
            "default": "adf1f61a-da02-4a41-a6c3-183fdcac102d"
          },
          "style": {
            "default": ""
          },
          "text": {
            "default": ""
          }
        },
        "group": "inline",
        "inline": true,
        "parseDOM": [
          {
            "tag": "span[data-node-type=\"status\"]"
          }
        ],
        "selectable": true
      }
    ],
    [
      "doc",
      {
        "content": "(block)+",
        "marks": "alignment breakout indentation link unsupportedMark unsupportedNodeAttribute"
      }
    ],
    [
      "paragraph",
      {
        "content": "inline*",
        "group": "block",
        "marks": "strong code em link strike subsup textColor typeAheadQuery underline unsupportedMark unsupportedNodeAttribute",
        "parseDOM": [
          {
            "tag": "p"
          }
        ],
        "selectable": false
      }
    ],
    [
      "text",
      {
        "group": "inline"
      }
    ],
    [
      "bulletList",
      {
        "content": "listItem+",
        "group": "block",
        "marks": "unsupportedMark unsupportedNodeAttribute",
        "parseDOM": [
          {
            "tag": "ul"
          }
        ],
        "selectable": false
      }
    ],
    [
      "orderedList",
      {
        "content": "listItem+",
        "group": "block",
        "marks": "unsupportedMark unsupportedNodeAttribute",
        "parseDOM": [
          {
            "tag": "ol"
          }
        ],
        "selectable": false
      }
    ],
    [
      "listItem",
      {
        "content": "(paragraph | mediaSingle | codeBlock) (paragraph | bulletList | orderedList | mediaSingle | codeBlock)*",
        "defining": true,
        "marks": "link unsupportedMark unsupportedNodeAttribute",
        "parseDOM": [
          {
            "tag": "li"
          }
        ],
        "selectable": false
      }
    ],
    [
      "heading",
      {
        "attrs": {
          "level": {
            "default": 1
          }
        },
        "content": "inline*",
        "defining": true,
        "group": "block",
        "parseDOM": [
          {
            "attrs": {
              "level": 1
            },
            "tag": "h1"
          },
          {
            "attrs": {
              "level": 2
            },
            "tag": "h2"
          },
          {
            "attrs": {
              "level": 3
            },
            "tag": "h3"
          },
          {
            "attrs": {
              "level": 4
            },
            "tag": "h4"
          },
          {
            "attrs": {
              "level": 5
            },
            "tag": "h5"
          },
          {
            "attrs": {
              "level": 6
            },
            "tag": "h6"
          }
        ],
        "selectable": false
      }
    ],
    [
      "blockquote",
      {
        "content": "paragraph+",
        "defining": true,
        "group": "block",
        "parseDOM": [
          {
            "tag": "blockquote"
          }
        ],
        "selectable": false
      }
    ],
    [
      "codeBlock",
      {
        "attrs": {
          "language": {
            "default": null
          },
          "uniqueId": {
            "default": null
          }
        },
        "code": true,
        "content": "text*",
        "defining": true,
        "group": "block",
        "marks": "unsupportedMark unsupportedNodeAttribute",
        "parseDOM": [
          {
            "preserveWhitespace": "full",
            "tag": "pre"
          },
          {
            "preserveWhitespace": "full",
            "tag": "div[style]"
          },
          {
            "preserveWhitespace": "full",
            "tag": "table[style]"
          },
          {
            "preserveWhitespace": "full",
            "tag": "div.code-block"
          }
        ]
      }
    ],
    [
      "rule",
      {
        "group": "block",
        "parseDOM": [
          {
            "tag": "hr"
          }
        ]
      }
    ],
    [
      "panel",
      {
        "attrs": {
          "panelType": {
            "default": "info"
          }
        },
        "content": "(paragraph | heading | bulletList | orderedList )+",
        "group": "block",
        "marks": "unsupportedMark unsupportedNodeAttribute",
        "parseDOM": [
          {
            "tag": "div[data-panel-type]"
          }
        ]
      }
    ],
    [
      "confluenceUnsupportedBlock",
      {
        "attrs": {
          "cxhtml": {
            "default": null
          }
        },
        "group": "block",
        "parseDOM": [
          {
            "tag": "div[data-node-type=\"confluenceUnsupportedBlock\"]"
          }
        ]
      }
    ],
    [
      "confluenceUnsupportedInline",
      {
        "atom": true,
        "attrs": {
          "cxhtml": {
            "default": null
          }
        },
        "group": "inline",
        "inline": true,
        "parseDOM": [
          {
            "tag": "div[data-node-type=\"confluenceUnsupportedInline\"]"
          }
        ]
      }
    ],
    [
      "unsupportedBlock",
      {
        "atom": true,
        "attrs": {
          "originalValue": {
            "default": {}
          }
        },
        "group": "block",
        "inline": false,
        "parseDOM": [
          {
            "tag": "[data-node-type=\"unsupportedBlock\"]"
          }
        ],
        "selectable": true
      }
    ],
    [
      "unsupportedInline",
      {
        "attrs": {
          "originalValue": {
            "default": {}
          }
        },
        "group": "inline",
        "inline": true,
        "parseDOM": [
          {
            "tag": "[data-node-type=\"unsupportedInline\"]"
          }
        ],
        "selectable": true
      }
    ],
    [
      "hardBreak",
      {
        "group": "inline",
        "inline": true,
        "parseDOM": [
          {
            "tag": "br"
          }
        ],
        "selectable": false
      }
    ],
    [
      "mediaSingle",
      {
        "atom": true,
        "attrs": {
          "layout": {
            "default": "center"
          },
          "width": {
            "default": null
          }
        },
        "content": "media | media ( unsupportedBlock)",
        "group": "block",
        "inline": false,
        "marks": "unsupportedMark unsupportedNodeAttribute link",
        "parseDOM": [
          {
            "tag": "div[data-node-type=\"mediaSingle\"]"
          }
        ],
        "selectable": true
      }
    ],
    [
      "table",
      {
        "attrs": {
          "__autoSize": {
            "default": false
          },
          "isNumberColumnEnabled": {
            "default": false
          },
          "layout": {
            "default": "default"
          }
        },
        "content": "tableRow+",
        "group": "block",
        "isolating": true,
        "marks": "unsupportedMark unsupportedNodeAttribute",
        "parseDOM": [
          {
            "tag": "table"
          }
        ],
        "selectable": false,
        "tableRole": "table"
      }
    ],
    [
      "media",
      {
        "attrs": {
          "__contextId": {
            "default": null
          },
          "__displayType": {
            "default": null
          },
          "__external": {
            "default": false
          },
          "__fileMimeType": {
            "default": null
          },
          "__fileName": {
            "default": null
          },
          "__fileSize": {
            "default": null
          },
          "alt": {
            "default": null
          },
          "collection": {
            "default": ""
          },
          "height": {
            "default": null
          },
          "id": {
            "default": ""
          },
          "occurrenceKey": {
            "default": null
          },
          "type": {
            "default": "file"
          },
          "url": {
            "default": null
          },
          "width": {
            "default": null
          }
        },
        "parseDOM": [
          {
            "tag": "div[data-node-type=\"media\"]"
          },
          {
            "ignore": true,
            "tag": "img[src^=\"data:image\"]"
          },
          {
            "tag": "img:not(.smart-link-icon)"
          }
        ],
        "selectable": true
      }
    ],
    [
      "tableHeader",
      {
        "attrs": {
          "background": {
            "default": null
          },
          "colspan": {
            "default": 1
          },
          "colwidth": {
            "default": null
          },
          "rowspan": {
            "default": 1
          }
        },
        "content": "(paragraph | panel | blockquote | orderedList | bulletList | rule | heading | codeBlock | mediaSingle )+",
        "isolating": true,
        "marks": "link alignment unsupportedMark unsupportedNodeAttribute",
        "parseDOM": [
          {
            "tag": "th"
          }
        ],
        "selectable": false,
        "tableRole": "header_cell"
      }
    ],
    [
      "tableRow",
      {
        "content": "(tableCell | tableHeader)+",
        "marks": "unsupportedMark unsupportedNodeAttribute",
        "parseDOM": [
          {
            "tag": "tr"
          }
        ],
        "selectable": false,
        "tableRole": "row"
      }
    ],
    [
      "tableCell",
      {
        "attrs": {
          "background": {
            "default": null
          },
          "colspan": {
            "default": 1
          },
          "colwidth": {
            "default": null
          },
          "rowspan": {
            "default": 1
          }
        },
        "content": "(paragraph | panel | blockquote | orderedList | bulletList | rule | heading | codeBlock | mediaSingle | unsupportedBlock)+",
        "isolating": true,
        "marks": "link alignment unsupportedMark unsupportedNodeAttribute",
        "parseDOM": [
          {
            "ignore": true,
            "tag": ".ak-renderer-table-number-column"
          },
          {
            "tag": "td"
          }
        ],
        "selectable": false,
        "tableRole": "cell"
      }
    ]
  ],
  "version": 3
}
`
