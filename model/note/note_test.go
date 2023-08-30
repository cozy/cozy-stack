package note

import (
	"strings"
	"testing"

	"github.com/cozy/prosemirror-go/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkdown(t *testing.T) {
	initial := `# My title

foobar **bold**

:info: this is a panel

✍ this is a decision
✍ and another decision

- [ ] a todo task

- [X] a done task`

	schemaSpecs := DefaultSchemaSpecs()
	specs := model.SchemaSpecFromJSON(schemaSpecs)
	schema, err := model.NewSchema(&specs)
	require.NoError(t, err)

	node, err := parseFile(strings.NewReader(initial), schema)
	require.NoError(t, err)

	md := markdownSerializer(nil).Serialize(node)
	assert.Equal(t, initial, md)
}

func TestText(t *testing.T) {
	initial := `# My title

hello **world**

- foo
- bar
- baz

✍ This is a decision
✍ and another decision

- [ ] A todo task
- [X] a done task`

	expected := `My title

hello world

- foo

- bar

- baz

This is a decision

and another decision

- A todo task

- a done task`

	schemaSpecs := DefaultSchemaSpecs()
	specs := model.SchemaSpecFromJSON(schemaSpecs)
	schema, err := model.NewSchema(&specs)
	require.NoError(t, err)

	node, err := parseFile(strings.NewReader(initial), schema)
	require.NoError(t, err)

	md := textSerializer().Serialize(node)
	assert.Equal(t, expected, md)
}
