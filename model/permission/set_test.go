package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiffSameSets(t *testing.T) {
	set1 := Set{
		Rule{
			Title:  "rule1",
			Verbs:  Verbs(GET),
			Type:   "io.cozy.files",
			Values: []string{"io.cozy.files.music-dir"},
		},
	}

	set2 := Set{
		Rule{
			Title:  "rule2",
			Verbs:  Verbs(GET),
			Type:   "io.cozy.files",
			Values: []string{"io.cozy.files.music-dir"},
		},
	}

	d, err := Diff(set1, set2)
	assert.NoError(t, err)
	assert.True(t, set1.HasSameRules(d))
}

func TestDiffNotSameSets(t *testing.T) {
	set1 := Set{
		Rule{
			Title:  "rule1",
			Verbs:  Verbs(GET),
			Type:   "io.cozy.files",
			Values: []string{"io.cozy.files.music-dir"},
		},
	}

	set2 := Set{
		Rule{
			Title:  "rule1",
			Verbs:  Verbs(GET, POST),
			Type:   "io.cozy.files",
			Values: []string{"io.cozy.files.music-dir"},
		},
	}

	d, err := Diff(set1, set2)
	assert.NoError(t, err)
	assert.False(t, set1.HasSameRules(d))

	expectedSet := Set{
		Rule{
			Title:  "rule1",
			Verbs:  Verbs(POST), // Only the POST has been added
			Type:   "io.cozy.files",
			Values: []string{}, // No addition has been made
		},
	}

	assert.Equal(t, expectedSet, d)
}

func TestDiffMultipleRules(t *testing.T) {
	set1 := Set{
		Rule{
			Title:  "rule1",
			Verbs:  Verbs(GET),
			Type:   "io.cozy.files",
			Values: []string{"io.cozy.files.music-dir"},
		},
		Rule{
			Title:  "rule2",
			Verbs:  Verbs(PATCH, DELETE),
			Type:   "io.cozy.foobar",
			Values: []string{"io.cozy.files.music-dir"},
		},
	}

	set2 := Set{
		Rule{
			Title:  "rule1",
			Verbs:  Verbs(GET, POST),
			Type:   "io.cozy.files",
			Values: []string{"io.cozy.files.music-dir"},
		},
		Rule{
			Title:  "rule2",
			Verbs:  Verbs(PATCH, DELETE, GET, POST),
			Type:   "io.cozy.foobar",
			Values: []string{"io.cozy.files.music-dir", "io.cozy.files.foobar-dir"},
		},
	}

	d, err := Diff(set1, set2)
	assert.NoError(t, err)
	assert.False(t, set1.HasSameRules(d))

	expectedSet := Set{
		Rule{
			Title:  "rule1",
			Verbs:  Verbs(POST), // Only the POST has been added
			Type:   "io.cozy.files",
			Values: []string{}, // No addition has been made
		},
		Rule{
			Title:  "rule2",
			Verbs:  Verbs(GET, POST), // GET & POST has been added
			Type:   "io.cozy.foobar",
			Values: []string{"io.cozy.files.foobar-dir"}, // A new folder was added
		},
	}
	assert.Equal(t, expectedSet, d)
}

func TestDiffNotSameSetsNewRule(t *testing.T) {
	set1 := Set{
		Rule{
			Title:  "rule1",
			Verbs:  Verbs(GET),
			Type:   "io.cozy.files",
			Values: []string{"io.cozy.files.music-dir"},
		},
	}

	set2 := Set{
		Rule{
			Title:  "rule1",
			Verbs:  Verbs(GET, POST),
			Type:   "io.cozy.files",
			Values: []string{"io.cozy.files.music-dir"},
		},
		Rule{
			Title: "myNewRule",
			Verbs: Verbs(GET, POST),
			Type:  "io.cozy.ducky",
		},
	}

	d, err := Diff(set1, set2)
	assert.NoError(t, err)
	assert.False(t, set1.HasSameRules(d))

	expectedSet := Set{
		Rule{
			Title: "myNewRule",
			Verbs: Verbs(GET, POST),
			Type:  "io.cozy.ducky",
		},
		Rule{
			Title:  "rule1",
			Verbs:  Verbs(POST), // Only the POST has been added
			Type:   "io.cozy.files",
			Values: []string{}, // No addition has been made
		},
	}

	assert.Equal(t, expectedSet, d)
}
