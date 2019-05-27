package dispers

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/stretchr/testify/assert"
)

func TestMetadata(t *testing.T) {

	// Let's Create A Metadata
	meta := NewMetadata("test metadata 1", "Cozy-DISPERS", []string{"HTTP", "CI", "Cozy"})
	err := meta.Close("We close this metadata", nil)
	assert.NoError(t, err)
	err = meta.Push("idOfThisTraining")
	assert.NoError(t, err)

	// Let's Create A Metadata Of Another Training
	meta2 := NewMetadata("thisIsNotAMetadata", "just a little message for nothing", []string{"paul", "françois"})
	err2 := meta2.Close("We close this metaata", nil)
	assert.NoError(t, err2)
	err2 = meta2.Push("AnotherTest")
	assert.NoError(t, err2)

	// Let's Create A Second Metadata For The First Training
	meta3 := NewMetadata("Hope It Will Not Crash", "Cozy-DISPERS", []string{"HTTP"})
	err2d2 := meta3.Close("This is another metadata for this training", nil)
	assert.NoError(t, err2d2)
	err2d2 = meta3.Push("idOfThisTraining")
	assert.NoError(t, err2d2)

	// Retrieve the two metadata doc and delete them
	for _, element := range []string{"idOfThisTraining", "AnotherTest"} {
		docs, err3 := RetrieveMetadata(element)
		assert.NoError(t, err3)
		for _, element := range docs {
			err3 = couchdb.DeleteDoc(prefix, &element)
			assert.NoError(t, err3)
		}
	}
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	// First we make sure couchdb is started
	db, err := checkup.HTTPChecker{URL: config.CouchURL().String()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	res := m.Run()
	os.Exit(res)

}
