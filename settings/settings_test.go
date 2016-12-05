package settings

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/stretchr/testify/assert"
)

var TestPrefix = couchdb.SimpleDatabasePrefix("couchdb-tests")

func TestTheme(t *testing.T) {
	err := CreateDefaultTheme(TestPrefix)
	assert.NoError(t, err)
	theme, err := DefaultTheme(TestPrefix)
	assert.NoError(t, err)
	assert.Equal(t, "/assets/images/cozy-dev.svg", theme.Logo)
	assert.Equal(t, "#EAEEF2", theme.Base00)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	err := couchdb.ResetDB(TestPrefix, SettingsDocType)
	if err != nil {
		fmt.Printf("Cant reset db (%s, %s) %s\n", TestPrefix, SettingsDocType, err.Error())
		os.Exit(1)
	}
	res := m.Run()
	couchdb.DeleteDB(TestPrefix, SettingsDocType)
	os.Exit(res)
}
