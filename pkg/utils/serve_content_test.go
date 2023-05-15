package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_checkIfNoneMatch(t *testing.T) {
	var tests = []struct {
		Name        string
		IfNoneMatch string
		Etag        string
		Match       bool
	}{
		{
			Name:        "strong inm with string etag",
			IfNoneMatch: `"some-etag"`,
			Etag:        `"some-etag"`,
			Match:       true,
		},
		{
			Name:        "weak inm with strong etag",
			IfNoneMatch: `W/"some-etag"`,
			Etag:        `"some-etag"`,
			Match:       true,
		},
		{
			Name:        "strong inm with weak etag",
			IfNoneMatch: `"some-etag"`,
			Etag:        `W/"some-etag"`,
			Match:       true,
		},
		{
			Name:        "multiple inm values match etag",
			IfNoneMatch: `"first-etag","second-etag"`,
			Etag:        `"second-etag"`,
			Match:       true,
		},
		{
			Name:        "multiple inm values are trimmed",
			IfNoneMatch: `"first-etag" , "second-etag"`,
			Etag:        `"second-etag"`,
			Match:       true,
		},
		{
			Name:        "inm is trimmed",
			IfNoneMatch: "  \"second-etag\"\t",
			Etag:        `"second-etag"`,
			Match:       true,
		},
		{
			Name:        "multiple inm values not matching etag",
			IfNoneMatch: `"first-etag","second-etag`,
			Etag:        `"third-etag"`,
			Match:       false,
		},
		{
			Name:        "inm not matching with etag",
			IfNoneMatch: `"some-etag"`,
			Etag:        `"some-other-etag"`,
			Match:       false,
		},
		{
			Name:        "* match every etag",
			IfNoneMatch: `*`,
			Etag:        `"some--etag"`,
			Match:       true,
		},
		{
			Name:        "Invalid etag quote",
			IfNoneMatch: `"some-etag`,
			Etag:        `W/"some-etag"`,
			Match:       false,
		},
		{
			Name:        "Invalid etag quote",
			IfNoneMatch: `some-etag`,
			Etag:        `W/"some-etag"`,
			Match:       false,
		},
		{
			Name:        "Invalid etag quote",
			IfNoneMatch: `"some-etag"`,
			Etag:        `some-etag`,
			Match:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			assert.Equal(t, test.Match, checkIfNoneMatch(test.IfNoneMatch, test.Etag))
		})
	}
}
